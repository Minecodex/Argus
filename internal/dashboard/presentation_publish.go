package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// PublishPresentations seals UI facts and its outbox event in one transaction.
// INSERT ON CONFLICT gives one notification for each Presentation/Attempt,
// including when completion raced initial ToolCall persistence or worker restart.
func (jobs QueryJobs) PublishPresentations(ctx context.Context, enterprise, id uuid.UUID) error {
	job, err := jobs.Runtime.Store.Queries.GetDashboardQueryJob(ctx, db.GetDashboardQueryJobParams{EnterpriseID: enterprise, ID: id})
	if err != nil {
		return err
	}
	// Seal after the complete background query (including delivery) ends. A
	// manifest can exist before delivery fails; persisting that earlier status
	// would hide the final error forever in an immutable presentation.
	if !terminalQueryJob(job.Status) {
		return nil
	}
	rows, err := jobs.Runtime.Store.Queries.ListQueryPresentations(ctx, db.ListQueryPresentationsParams{EnterpriseID: enterprise, JobID: id.String()})
	if err != nil || len(rows) == 0 {
		return err
	}
	user, err := jobs.Runtime.Store.Queries.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{EnterpriseID: enterprise, ID: job.OwnerUserID})
	if err != nil {
		return err
	}
	actor := Actor{EnterpriseID: enterprise, SubjectID: job.OwnerUserID, SubjectType: "user", AuthorizationVersion: user.AuthorizationVersion}
	projected, err := jobs.PresentQuery(ctx, actor, job.ConversationID, id)
	if err != nil {
		return err
	}
	full, _ := projected.Binding["data"].(map[string]any)
	return jobs.Runtime.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		locked, e := q.LockDashboardQueryJob(ctx, db.LockDashboardQueryJobParams{ID: id, EnterpriseID: enterprise})
		if e != nil {
			return e
		}
		if locked.Version != job.Version || locked.AttemptID != job.AttemptID {
			return ErrConflict
		}
		for _, row := range rows {
			binding := map[string]any{}
			if json.Unmarshal(row.ResultBinding, &binding) != nil {
				return ErrInvalid
			}
			original := map[string]any{}
			if json.Unmarshal(row.DetailData, &original) != nil {
				return ErrInvalid
			}
			data := presentation.PublicSnapshot(full)
			if value, ok := original["resources"]; ok {
				data["resources"] = value
			}
			attempt, _ := binding["attempt_id"].(string)
			if attempt != "" && (!job.AttemptID.Valid || attempt != job.AttemptID.UUID.String()) {
				data = original
				data["status"], data["error_code"] = "failed", "DASHBOARD_ATTEMPT_REPLACED"
			}
			raw, e := json.Marshal(data)
			if e != nil || len(raw) > 16<<20 {
				return ErrInvalid
			}
			inserted, e := q.CreatePresentationSnapshot(ctx, db.CreatePresentationSnapshotParams{PresentationID: row.ID, EnterpriseID: enterprise, SourceVersion: presentation.SnapshotVersion(raw), Data: raw})
			if e != nil {
				return e
			}
			if inserted == 0 {
				continue
			}
			_, e = conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: enterprise, ConversationID: row.ConversationID, RunID: job.RunID, Type: "tool_presentation", ActorType: "worker", Payload: map[string]any{"tool_call_id": row.ToolCallID.String(), "presentation_id": row.ID.String(), "status": "data_ready", "attempt_id": job.AttemptID.UUID.String(), "result_key": "dashboard:" + job.ID.String() + ":" + job.AttemptID.UUID.String(), "source_version": presentation.SnapshotVersion(raw)}, Classification: "internal"})
			if e != nil {
				return e
			}
		}
		return nil
	})
}

func (jobs QueryJobs) RunPresentationPublisher(ctx context.Context, logger *slog.Logger) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		rows, err := jobs.Runtime.Store.Queries.ListUnsealedPresentationJobs(ctx)
		if err == nil {
			for _, row := range rows {
				if e := jobs.PublishPresentations(ctx, row.EnterpriseID, row.ID); e != nil && !errors.Is(e, ErrDenied) && !errors.Is(e, ErrArchived) {
					logger.Warn("presentation sealing will retry", "job_id", row.ID, "error", e)
				}
			}
		} else if ctx.Err() == nil {
			logger.Warn("presentation publisher will retry", "error", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
