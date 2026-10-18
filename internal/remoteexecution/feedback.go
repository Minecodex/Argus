package remoteexecution

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func (s Service) Feedback(ctx context.Context, p toolruntime.Principal, id uuid.UUID, text, key string) (View, error) {
	if strings.TrimSpace(text) == "" || len(text) > 16384 {
		return View{}, Error("REMOTE_INPUT_INVALID")
	}
	if _, err := s.Get(ctx, p, id); err != nil {
		return View{}, err
	}
	request := map[string]any{"execution_id": id, "feedback": text}
	_, err := postgres.ExecuteIdempotent(ctx, s.Store, s.Actions.Idempotency, "enterprise", p.UserID.String(), "remote_execution.feedback", key, request, 202, func(q *db.Queries) (bool, error) {
		preview, err := q.GetRemoteRequest(ctx, db.GetRemoteRequestParams{ID: id, EnterpriseID: p.EnterpriseID})
		if err != nil {
			return false, err
		}
		if _, err = q.GetPendingActionForUpdate(ctx, db.GetPendingActionForUpdateParams{ActionRef: preview.ActionRef, EnterpriseID: p.EnterpriseID}); err != nil {
			return false, err
		}
		run, err := q.GetRunForUpdate(ctx, db.GetRunForUpdateParams{ID: preview.RunID, EnterpriseID: p.EnterpriseID})
		if err != nil {
			return false, err
		}
		old, err := q.LockRemoteRequest(ctx, db.LockRemoteRequestParams{ID: id, EnterpriseID: p.EnterpriseID})
		if err != nil || old.OwnerUserID != p.UserID || old.ConversationID != p.ConversationID || old.Status != "awaiting_confirmation" {
			return false, Error("REMOTE_PLAN_NOT_REVISABLE")
		}
		if err = s.supersede(ctx, q, p, id); err != nil {
			return false, err
		}
		if run.Status != "waiting_input" && run.Status != "waiting_approval" && run.Status != "waiting_system" {
			return false, Error("REMOTE_PLAN_NOT_REVISABLE")
		}
		content := text
		if _, err = q.LockConversation(ctx, db.LockConversationParams{ID: p.ConversationID, EnterpriseID: p.EnterpriseID, OwnerUserID: p.UserID}); err != nil {
			return false, err
		}
		if _, err = conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, Type: "user_message", ActorType: "user", ActorID: p.UserID.String(), Payload: map[string]any{"content": content, "revision_of": id}, Classification: "internal"}); err != nil {
			return false, err
		}
		if _, err = q.UpdateRunStatus(ctx, db.UpdateRunStatusParams{ID: run.ID, EnterpriseID: p.EnterpriseID, Status: "pending", Version: run.Version}); err != nil {
			return false, err
		}
		payload, _ := json.Marshal(conversation.AgentTask{RunID: run.ID, EnterpriseID: p.EnterpriseID, Reason: "remote_plan_feedback", ActionRef: old.ActionRef})
		_, err = q.CreateRuntimeTask(ctx, db.CreateRuntimeTaskParams{ID: uuid.New(), EnterpriseID: uuid.NullUUID{UUID: p.EnterpriseID, Valid: true}, Queue: "agent", RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, Payload: payload, MaxAttempts: 3, AvailableAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}})
		return true, err
	})
	if err != nil {
		return View{}, err
	}
	return s.Get(ctx, p, id)
}
