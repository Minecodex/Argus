package workspace

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// ContextFacts performs no compute allocation, filesystem scan or model call.
// The shared Workspace source authorization protects every registered file.
func (service Service) ContextFacts(ctx context.Context, request toolruntime.ContextRequest) (map[string]any, error) {
	if !service.Config.Enabled || !request.Principal.Allows("workspace.use") {
		return map[string]any{}, nil
	}
	value, err := service.Get(ctx, request.Principal)
	if errors.Is(err, pgx.ErrNoRows) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	if value.Status != "ready" {
		return map[string]any{"workspace_references": map[string]any{"available": false, "status": value.Status}}, nil
	}
	if err := service.authorizeWorkspace(ctx, request.Principal, value); err != nil {
		return map[string]any{"workspace_references": map[string]any{"available": false, "reason": "WORKSPACE_SOURCE_UNAVAILABLE", "items": []any{}}}, nil
	}
	rows, err := service.Store.Queries.ListWorkspaceContextFiles(ctx, db.ListWorkspaceContextFilesParams{
		WorkspaceID: value.ID, EnterpriseID: request.Principal.EnterpriseID, ThroughSequence: request.ThroughSequence})
	if err != nil {
		return nil, err
	}
	items := []map[string]any{}
	for _, row := range rows[:min(len(rows), 64)] {
		items = append(items, map[string]any{"kind": "workspace_file", "file_id": row.ID.String(), "workspace_id": value.ID.String(),
			"path": "/workspace/" + row.Path, "name": row.Name, "registered_sha256": row.ContentHash, "registered_bytes": row.ByteSize,
			"media_type": row.MediaType, "source_result_refs": row.SourceResultRefs, "source_sequence": row.SourceSequence,
			"registered_at": row.CreatedAt.Time.UTC().Format(time.RFC3339Nano), "content_state": "read_required"})
	}
	index := toolruntime.BoundReferenceIndex(items, len(rows) <= 64, 64, 24<<10)
	index["workspace_id"], index["through_sequence"], index["available"] = value.ID.String(), request.ThroughSequence, true
	current, err := service.Get(ctx, request.Principal)
	if err != nil || current.ID != value.ID || service.authorizeWorkspace(ctx, request.Principal, current) != nil {
		return map[string]any{"workspace_references": map[string]any{"available": false, "reason": "WORKSPACE_SOURCE_UNAVAILABLE", "items": []any{}}}, nil
	}
	return map[string]any{"workspace_references": index}, nil
}
