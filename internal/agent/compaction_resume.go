package agent

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// A soft compactor may have already shortened history while a hard request was
// queued. Reassemble its actual provider input before deciding to fail or pay
// for another summary; never acknowledge a hard request and strand the Run.
func (compactor Compactor) currentContextFits(ctx context.Context, run db.Run) (bool, error) {
	var snapshot toolruntime.Snapshot
	if err := json.Unmarshal(run.ToolSnapshot, &snapshot); err != nil {
		return false, err
	}
	var tools []modelprovider.Tool
	for _, definition := range snapshot.Definitions {
		tools = append(tools, definition.Model)
	}
	revision, err := compactor.Store.Queries.GetEnabledAIModelRevision(ctx, db.GetEnabledAIModelRevisionParams{ModelID: run.ModelID, EnterpriseID: run.EnterpriseID, Revision: run.ModelRevision})
	if err != nil {
		return false, err
	}
	loop := Loop{Store: compactor.Store}
	messages, current, _, source, err := loop.messages(ctx, run)
	if err != nil {
		return false, err
	}
	projection, err := loop.context(ctx, run, revision, messages, current, tools, source)
	if err != nil {
		return false, nil
	}
	size, err := (modelprovider.Provider{Protocol: modelprovider.Protocol(revision.ApiProtocol), BaseURL: revision.BaseUrl}).RequestBytes(modelprovider.Request{Model: revision.ProviderModelID, Messages: projection.Messages, Tools: tools, MaxTokens: int(revision.MaxOutputTokens)})
	return size < projection.HardLimit, err
}

func resumeCompactedRun(ctx context.Context, q *db.Queries, run db.Run) error {
	current, err := q.GetRun(ctx, db.GetRunParams{ID: run.ID, EnterpriseID: run.EnterpriseID})
	if err != nil {
		return err
	}
	if current.Status != "waiting_system" {
		return nil
	}
	updated, err := q.UpdateRunStatus(ctx, db.UpdateRunStatusParams{ID: run.ID, EnterpriseID: run.EnterpriseID, Status: "pending", Version: current.Version})
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(conversation.AgentTask{RunID: run.ID, EnterpriseID: run.EnterpriseID, Reason: "compaction_completed"})
	_, err = q.CreateRuntimeTask(ctx, db.CreateRuntimeTaskParams{ID: newAgentID(), EnterpriseID: uuid.NullUUID{UUID: run.EnterpriseID, Valid: true}, Queue: "agent", RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, Payload: payload, MaxAttempts: 5, AvailableAt: pgtype.Timestamptz{Time: updated.UpdatedAt.Time, Valid: true}})
	return err
}
