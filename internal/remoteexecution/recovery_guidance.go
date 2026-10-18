package remoteexecution

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func (s Service) recoveryRequired(ctx context.Context, p toolruntime.Principal, session db.RemoteExecutionSession) error {
	details := map[string]any{"requires_recovery": true, "requires_confirmation": true, "session_id": session.ID.String(), "generation": session.Generation.String()}
	previous, err := s.Store.Queries.GetLastRemoteGenerationRequest(ctx, db.GetLastRemoteGenerationRequestParams{SessionID: session.ID, Generation: session.Generation, EnterpriseID: p.EnterpriseID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err == nil {
		if previous.OwnerUserID != p.UserID || previous.ConversationID != p.ConversationID {
			return Error("REMOTE_EXECUTION_FORBIDDEN")
		}
		details["recovery_of_id"] = previous.ID.String()
		details["previous_status"] = previous.Status
	}
	return toolruntime.Error{Kind: "REMOTE_CONTEXT_LOST", Message: "The shared interpreter is unavailable. Inspect durable succeeded, unknown and not-sent facts. Prepare a new request with recovery_of_id from these details and wait for a new confirmation. Do not replay completed or unknown writes. Keep ordinary batches in the shared shell; do not append exit only to report status.", Details: details}
}
