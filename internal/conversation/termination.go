package conversation

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TerminalRun(status string) bool {
	return status == "succeeded" || status == "failed" || status == "cancelled" || status == "timed_out"
}

// FinishRunRecord requires the caller's transaction and locked Run row. Every
// terminal transition publishes exactly one event with the committed state.
func FinishRunRecord(ctx context.Context, q *db.Queries, run db.Run, status, reason, code, actorType, actorID string) (db.Run, error) {
	if !TerminalRun(status) {
		return db.Run{}, fmt.Errorf("nonterminal Run completion: %s", status)
	}
	if TerminalRun(run.Status) {
		return run, nil
	}
	updated, err := q.UpdateRunStatus(ctx, db.UpdateRunStatusParams{ID: run.ID, EnterpriseID: run.EnterpriseID, Status: status,
		StopReason: pgtype.Text{String: reason, Valid: reason != ""}, ErrorCode: pgtype.Text{String: code, Valid: code != ""}, Version: run.Version})
	if err != nil {
		return db.Run{}, err
	}
	payload := map[string]any{"status": status, "stop_reason": reason}
	if code != "" {
		payload["error_code"] = code
	}
	_, err = AppendEvent(ctx, q, EventInput{EnterpriseID: run.EnterpriseID, ConversationID: run.ConversationID,
		RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, Type: "run_state_changed", ActorType: actorType, ActorID: actorID, Payload: payload, Classification: "internal"})
	return updated, err
}
