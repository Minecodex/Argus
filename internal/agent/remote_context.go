package agent

import (
	"context"
	"encoding/json"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Store the sanitized assembled block; the dispatch transaction binds its hash
// to the actual provider request. This private record has no chat/API projection.
func recordRemoteModelContext(ctx context.Context, q *db.Queries, call db.ModelCall, run db.Run, projection ContextProjection) error {
	for _, part := range projection.Parts {
		if part.Kind != "typed_checkpoint" {
			continue
		}
		checkpoint, ok := part.Content.(map[string]any)
		if !ok {
			continue
		}
		remote, ok := checkpoint["remote_execution"].(map[string]any)
		if !ok {
			continue
		}
		activity, ok := remote["concurrent_activity"]
		if !ok {
			continue
		}
		raw, err := json.Marshal(activity)
		if err != nil {
			return err
		}
		var scope struct {
			AuthorizationVersion int64 `json:"authorization_version"`
		}
		if err = json.Unmarshal(raw, &scope); err != nil {
			return err
		}
		return q.RecordRemoteModelContext(ctx, db.RecordRemoteModelContextParams{ModelCallID: call.ID, EnterpriseID: run.EnterpriseID, RunID: run.ID, AuthorizationVersion: scope.AuthorizationVersion, ProjectionHash: call.ProjectionHash, Activity: raw})
	}
	return nil
}
