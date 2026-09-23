package conversation

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Only public workspace identity and lifecycle belong in model context. PVC,
// namespace, runtime UID, credentials and compute leases remain private.
type WorkspaceContext struct {
	ID     string `json:"id,omitempty"`
	Status string `json:"status"`
}

func CurrentWorkspaceContext(ctx context.Context, q *db.Queries, enterprise, conversation uuid.UUID) (WorkspaceContext, error) {
	value, err := q.GetConversationWorkspaceContext(ctx, db.GetConversationWorkspaceContextParams{ConversationID: conversation, EnterpriseID: enterprise})
	if errors.Is(err, pgx.ErrNoRows) {
		return WorkspaceContext{Status: "not_created"}, nil
	}
	if err != nil {
		return WorkspaceContext{}, err
	}
	return WorkspaceContext{ID: value.ID.String(), Status: value.Status}, nil
}

// Admission and inference consume exactly the same public fact projection.
func ContextFacts(ctx context.Context, q *db.Queries, enterprise, owner, conversation uuid.UUID) (map[string]any, WorkspaceContext, error) {
	if _, err := q.GetConversation(ctx, db.GetConversationParams{ID: conversation, EnterpriseID: enterprise, OwnerUserID: owner}); err != nil {
		return nil, WorkspaceContext{}, err
	}
	workspace, err := CurrentWorkspaceContext(ctx, q, enterprise, conversation)
	if err != nil {
		return nil, workspace, err
	}
	actions, err := q.ListConversationActionFacts(ctx, db.ListConversationActionFactsParams{ConversationID: conversation, EnterpriseID: enterprise, CreatorSubjectID: owner})
	if err != nil {
		return nil, workspace, err
	}
	return map[string]any{"actions": actions, "workspace": workspace}, workspace, nil
}

func FileReference(file db.WorkspaceFile) map[string]any {
	return map[string]any{"id": file.ID.String(), "workspace_id": file.WorkspaceID.String(), "name": file.Name, "path": "/workspace/" + file.Path, "byte_size": file.ByteSize}
}

func ProjectFileReference(file db.WorkspaceFile, current WorkspaceContext) map[string]any {
	if current.Status != "ready" || file.WorkspaceID.String() != current.ID || file.DeletedAt.Valid {
		return map[string]any{"id": file.ID.String(), "workspace_id": file.WorkspaceID.String(), "available": false, "reason": "WORKSPACE_FILE_UNAVAILABLE"}
	}
	value := FileReference(file)
	value["available"], value["content_state"] = true, "read_required"
	return value
}

// Original events remain intact. Revalidate their references when projecting
// history; a reused pathname in a new workspace never rebinds an old file ID.
func ProjectHistoricalFiles(ctx context.Context, q *db.Queries, enterprise, conversation uuid.UUID, current WorkspaceContext, files []map[string]any) ([]map[string]any, error) {
	result := make([]map[string]any, 0, len(files))
	for _, ref := range files {
		text, _ := ref["id"].(string)
		id, parseErr := uuid.Parse(text)
		if parseErr == nil {
			file, err := q.GetWorkspaceFile(ctx, db.GetWorkspaceFileParams{ID: id, EnterpriseID: enterprise})
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
			if err == nil && file.ConversationID == conversation {
				result = append(result, ProjectFileReference(file, current))
				continue
			}
		}
		result = append(result, map[string]any{"id": text, "available": false, "reason": "WORKSPACE_FILE_UNAVAILABLE"})
	}
	return result, nil
}
