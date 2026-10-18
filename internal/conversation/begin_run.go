package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/dashboardcontext"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"time"
)

// BeginPreparedRun is shared by normal messages and atomic plan implementation.
// The caller owns the transaction and has completed Preflight with current identity.
func (service Service) BeginPreparedRun(ctx context.Context, q *db.Queries, actorID string, enterpriseID, ownerID, conversationID uuid.UUID, authorizationVersion int64, locale, content string, fileIDs []uuid.UUID, preflight Preflight) (MessageAccepted, error) {
	conversation, err := q.LockConversation(ctx, db.LockConversationParams{ID: conversationID, EnterpriseID: enterpriseID, OwnerUserID: ownerID})
	if err != nil {
		return MessageAccepted{}, err
	}
	if conversation.Status != "active" {
		return MessageAccepted{}, ErrConversationClosed
	}
	if conversation.Version != preflight.ConversationVersion {
		return MessageAccepted{}, toolruntime.Error{Kind: "TOOL_CONFIGURATION_CHANGED"}
	}
	if err := validateFiles(ctx, q, enterpriseID, conversationID, fileIDs); err != nil {
		return MessageAccepted{}, err
	}
	if err := validateSelectedConnections(ctx, q, enterpriseID, ownerID, conversationID, preflight.Snapshot); err != nil {
		return MessageAccepted{}, err
	}
	if _, err := q.GetActiveRunForConversation(ctx, db.GetActiveRunForConversationParams{ConversationID: conversationID, EnterpriseID: enterpriseID}); err == nil {
		return MessageAccepted{}, ErrRunAlreadyActive
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return MessageAccepted{}, err
	}
	model, err := q.GetAIModel(ctx, db.GetAIModelParams{ID: conversation.SelectedModelID, EnterpriseID: enterpriseID})
	if err != nil || model.Status != "enabled" || model.HealthStatus != "healthy" {
		return MessageAccepted{}, ErrModelUnavailable
	}
	if int32(model.Revision) != preflight.ModelRevision {
		return MessageAccepted{}, toolruntime.Error{Kind: "TOOL_CONFIGURATION_CHANGED"}
	}
	checkpoint, _ := json.Marshal(map[string]any{"schema_version": "argus.run_checkpoint/v1", "goal": content,
		"authorization_version": authorizationVersion, "completed_step_ids": []string{}, "tool_call_refs": []string{},
		"active_public_pending_action_refs": []string{}, "execution_refs": []string{}})
	run, err := q.CreateRun(ctx, db.CreateRunParams{ID: newID(), ConversationID: conversationID, EnterpriseID: enterpriseID,
		ActorUserID: ownerID, ModelID: model.ID, ModelRevision: int32(model.Revision), Locale: locale,
		AuthorizationVersion: authorizationVersion, Checkpoint: checkpoint})
	if err != nil {
		return MessageAccepted{}, err
	}
	if err := dashboardcontext.Commit(ctx, q, preflight.Principal, run.ID, preflight.Dashboard); err != nil {
		return MessageAccepted{}, err
	}
	snapshotBytes, _ := json.Marshal(preflight.Snapshot)
	run, err = q.SetRunToolSnapshot(ctx, db.SetRunToolSnapshotParams{ID: run.ID, EnterpriseID: enterpriseID, ToolSnapshot: snapshotBytes, ToolSnapshotHash: preflight.SnapshotHash})
	if err != nil {
		return MessageAccepted{}, err
	}
	messagePayload := map[string]any{"content": content, "dashboard_context": preflight.Dashboard.Snapshot}
	files := make([]map[string]any, 0, len(fileIDs))
	for _, id := range fileIDs {
		file, e := q.GetWorkspaceFile(ctx, db.GetWorkspaceFileParams{ID: id, EnterpriseID: enterpriseID})
		if e != nil {
			return MessageAccepted{}, e
		}
		files = append(files, FileReference(file))
	}
	messagePayload["files"] = files
	event, err := AppendEvent(ctx, q, EventInput{EnterpriseID: enterpriseID, ConversationID: conversationID,
		RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, Type: "user_message", ActorType: "user", ActorID: actorID,
		Payload: messagePayload, Classification: "internal"})
	if err != nil {
		return MessageAccepted{}, err
	}
	payload, _ := json.Marshal(AgentTask{RunID: run.ID, EnterpriseID: enterpriseID, Reason: "user_message"})
	if _, err := q.CreateRuntimeTask(ctx, db.CreateRuntimeTaskParams{ID: newID(), EnterpriseID: uuid.NullUUID{UUID: enterpriseID, Valid: true},
		Queue: "agent", RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, Payload: payload, MaxAttempts: 5,
		AvailableAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}}); err != nil {
		return MessageAccepted{}, err
	}
	return MessageAccepted{Event: event, Run: run}, nil
}
