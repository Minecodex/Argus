package conversation

import (
	"context"
	"encoding/json"
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
	c, err := q.GetConversation(ctx, db.GetConversationParams{ID: conversation, EnterpriseID: enterprise, OwnerUserID: owner})
	if err != nil {
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
	facts := map[string]any{"actions": actions, "workspace": workspace, "access_mode": c.AccessMode,
		"action_result_semantics": "resource_id/resource_type describe the original target and may be null for creation. result_resource_id/result_resource_type identify the produced object. result_resource_version is its object lifecycle version, not a dashboard revision number or conversation context version."}
	run, e := q.GetActiveRunForConversation(ctx, db.GetActiveRunForConversationParams{ConversationID: conversation, EnterpriseID: enterprise})
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return nil, workspace, e
	}
	if e == nil && run.ActorUserID == owner {
		if run.ApprovedPlanID.Valid {
			plan, e := q.GetConversationInteraction(ctx, db.GetConversationInteractionParams{ID: run.ApprovedPlanID.UUID, EnterpriseID: enterprise, ConversationID: conversation, OwnerUserID: owner})
			if e != nil {
				return nil, workspace, e
			}
			facts["approved_plan"] = map[string]any{"id": plan.ID, "revision": plan.Revision, "content": publicPlanContext(plan.Content), "semantics": "Continue the ordinary Harness conversation. Commands with approved_step_id are already authorized once for this implementation Run, in listed order: invoke connect with only that ID, never retype or modify the script. New or changed commands use normal connect arguments and current approval policy. Unmarked examples and legacy guidance have no command grant. Never replay failed or unknown effects."}
			grants, e := q.ListPlanCommandAuthorizations(ctx, db.ListPlanCommandAuthorizationsParams{PlanID: plan.ID, EnterpriseID: enterprise})
			if e != nil {
				return nil, workspace, e
			}
			states := []map[string]any{}
			for _, g := range grants {
				state := map[string]any{"approved_step_id": g.ID, "block_id": g.BlockID, "status": "unused"}
				if g.UsedRequestID.Valid {
					execution, e := q.GetRemoteRequest(ctx, db.GetRemoteRequestParams{ID: g.UsedRequestID.UUID, EnterpriseID: enterprise})
					if e != nil {
						return nil, workspace, e
					}
					state["status"] = execution.Status
					state["execution_id"] = execution.ID
				}
				states = append(states, state)
			}
			facts["plan_command_states"] = states
		} else if plan, e := q.LatestRunPlan(ctx, db.LatestRunPlanParams{RunID: run.ID, EnterpriseID: enterprise}); e == nil {
			facts["latest_plan"] = map[string]any{"id": plan.ID, "revision": plan.Revision, "status": plan.Status, "content": publicPlanContext(plan.Content)}
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return nil, workspace, e
		}
	}
	return facts, workspace, nil
}

func publicPlanContext(raw []byte) map[string]any {
	var content map[string]any
	_ = json.Unmarshal(raw, &content)
	delete(content, "execution")
	delete(content, "request_hash")
	if rendered, ok := content["document_markdown"]; ok {
		content["markdown"] = rendered
		delete(content, "document_markdown")
	}
	return content
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
