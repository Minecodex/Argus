package interaction

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"reflect"
	"strings"
	"time"
)

func (s Service) Respond(ctx context.Context, p toolruntime.Principal, id uuid.UUID, input Response, key, locale string) (View, error) {
	if _, err := s.Get(ctx, p, id); err != nil {
		return View{}, err
	}
	if input.Version < 1 || input.ContentHash == "" || len(input.Feedback) > 16384 {
		return View{}, invalid()
	}
	request := struct {
		ID       uuid.UUID `json:"id"`
		Response Response  `json:"response"`
	}{id, input}
	raw, _ := json.Marshal(request)
	prior, err := s.Idempotency.Lookup(ctx, s.Store.Queries, "enterprise", p.UserID.String(), "interaction.respond", key, raw)
	if err != nil {
		return View{}, err
	}
	if prior.Replay {
		var v View
		err = json.Unmarshal(prior.Body, &v)
		return v, err
	}
	var prepared conversation.Preflight
	if input.Action == "confirm" {
		prepared, err = s.Conversations.Preflight(ctx, p.EnterpriseID, p.UserID, p.ConversationID, implementationMessage(locale), nil)
		if err != nil {
			return View{}, err
		}
		if !prepared.Ready {
			return View{}, toolruntime.Error{Kind: prepared.ErrorCode}
		}
	}
	return postgres.ExecuteIdempotent(ctx, s.Store, s.Idempotency, "enterprise", p.UserID.String(), "interaction.respond", key, request, 200, func(q *db.Queries) (View, error) {
		initial, e := q.GetConversationInteraction(ctx, db.GetConversationInteractionParams{ID: id, EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, OwnerUserID: p.UserID})
		if errors.Is(e, pgx.ErrNoRows) {
			return View{}, forbidden()
		}
		// Preserve serialization/deadlock errors so ExecuteIdempotent can retry
		// this database-only transaction. They are not authorization failures.
		if e != nil {
			return View{}, e
		}
		run, e := q.GetOwnedRunForUpdate(ctx, db.GetOwnedRunForUpdateParams{ID: initial.RunID, EnterpriseID: p.EnterpriseID, ActorUserID: p.UserID})
		if e != nil {
			return View{}, e
		}
		r, e := q.LockConversationInteraction(ctx, db.LockConversationInteractionParams{ID: id, EnterpriseID: p.EnterpriseID, OwnerUserID: p.UserID})
		if e != nil {
			return View{}, e
		}
		if r.AuthorizationVersion != p.AuthorizationVersion {
			return View{}, forbidden()
		}
		responseBytes, _ := json.Marshal(input)
		if r.Status != "pending" {
			var previous Response
			if r.ContentHash == input.ContentHash && json.Unmarshal(r.Response, &previous) == nil && reflect.DeepEqual(previous, input) {
				return view(r), nil
			}
			return View{}, conflict()
		}
		if r.Version != input.Version || r.ContentHash != input.ContentHash || run.Status != "waiting_input" || run.StopReason.String != "interaction:"+id.String() {
			return View{}, conflict()
		}
		status := "answered"
		if r.Kind == "user_input" {
			var questions Questions
			if json.Unmarshal(r.Content, &questions) != nil {
				return View{}, invalid()
			}
			if e = validateAnswers(questions, input); e != nil {
				return View{}, e
			}
			if input.Action == "skip" {
				status = "skipped"
			}
		}
		if r.Kind == "plan" {
			unfinished, e := q.ListUnfinishedRunToolCalls(ctx, db.ListUnfinishedRunToolCallsParams{RunID: run.ID, EnterpriseID: p.EnterpriseID})
			if e != nil {
				return View{}, e
			}
			if len(unfinished) > 0 {
				return View{}, conflict()
			}
			switch input.Action {
			case "confirm":
				var plan Plan
				if json.Unmarshal(r.Content, &plan) != nil {
					return View{}, invalid()
				}
				if plan.Execution != nil && len(plan.Execution.Spec.Steps) > 0 {
					if s.Remote == nil {
						return View{}, invalid()
					}
					if e = s.Remote.ValidatePlanCommands(ctx, q, p, r.AuthorizationVersion, *plan.Execution); e != nil {
						return View{}, e
					}
				}
				status = "confirmed"
			case "revise":
				if strings.TrimSpace(input.Feedback) == "" {
					return View{}, invalid()
				}
				status = "superseded"
			default:
				return View{}, invalid()
			}
		}
		r, e = q.ResolveConversationInteraction(ctx, db.ResolveConversationInteractionParams{ID: r.ID, EnterpriseID: p.EnterpriseID, Version: r.Version, Status: status, Response: responseBytes})
		if e != nil {
			return View{}, e
		}
		if status == "confirmed" {
			c, e := q.LockConversation(ctx, db.LockConversationParams{ID: p.ConversationID, EnterpriseID: p.EnterpriseID, OwnerUserID: p.UserID})
			if e != nil {
				return View{}, e
			}
			if c.Version != prepared.ConversationVersion {
				return View{}, toolruntime.Error{Kind: "TOOL_CONFIGURATION_CHANGED"}
			}
			if _, e = conversation.FinishRunRecord(ctx, q, run, "succeeded", "plan_confirmed", "", "user", p.UserID.String()); e != nil {
				return View{}, e
			}
			c, e = q.UpdateConversation(ctx, db.UpdateConversationParams{ID: c.ID, EnterpriseID: c.EnterpriseID, OwnerUserID: c.OwnerUserID, ExpectedVersion: c.Version, Mode: pgtype.Text{String: "execute", Valid: true}})
			if e != nil {
				return View{}, e
			}
			// Transaction retries must start from the original preflight version.
			implementationPreflight := prepared
			implementationPreflight.ConversationVersion = c.Version
			accepted, e := s.Conversations.BeginPreparedRun(ctx, q, p.UserID.String(), p.EnterpriseID, p.UserID, p.ConversationID, p.AuthorizationVersion, locale, implementationMessage(locale), nil, implementationPreflight)
			if e != nil {
				return View{}, e
			}
			if _, e = q.LinkRunApprovedPlan(ctx, db.LinkRunApprovedPlanParams{ID: accepted.Run.ID, EnterpriseID: p.EnterpriseID, ApprovedPlanID: uuid.NullUUID{UUID: r.ID, Valid: true}}); e != nil {
				return View{}, e
			}
			if e = q.ActivatePlanCommandAuthorizations(ctx, db.ActivatePlanCommandAuthorizationsParams{PlanID: r.ID, EnterpriseID: p.EnterpriseID, ImplementationRunID: uuid.NullUUID{UUID: accepted.Run.ID, Valid: true}}); e != nil {
				return View{}, e
			}
			r, e = q.AttachInteractionImplementation(ctx, db.AttachInteractionImplementationParams{ID: r.ID, EnterpriseID: p.EnterpriseID, ImplementationRunID: uuid.NullUUID{UUID: accepted.Run.ID, Valid: true}})
			if e != nil {
				return View{}, e
			}
		} else {
			if status == "superseded" {
				if _, e = conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, Type: "user_message", ActorType: "user", ActorID: p.UserID.String(), Payload: map[string]any{"content": input.Feedback, "revision_of": r.ID}, Classification: "internal"}); e != nil {
					return View{}, e
				}
			}
			if _, e = q.UpdateRunStatus(ctx, db.UpdateRunStatusParams{ID: run.ID, EnterpriseID: p.EnterpriseID, Version: run.Version, Status: "pending"}); e != nil {
				return View{}, e
			}
			task, _ := json.Marshal(conversation.AgentTask{RunID: run.ID, EnterpriseID: p.EnterpriseID, Reason: "interaction_resolved"})
			if _, e = q.CreateRuntimeTask(ctx, db.CreateRuntimeTaskParams{ID: uuid.New(), EnterpriseID: uuid.NullUUID{UUID: p.EnterpriseID, Valid: true}, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, Queue: "agent", Payload: task, MaxAttempts: 5, AvailableAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}}); e != nil {
				return View{}, e
			}
		}
		if _, e = conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, Type: "interaction_resolved", ActorType: "user", ActorID: p.UserID.String(), Payload: map[string]any{"interaction_id": r.ID, "kind": r.Kind, "status": status}, Classification: "internal"}); e != nil {
			return View{}, e
		}
		if _, e = audit.Append(ctx, q, audit.Entry{Domain: "enterprise", EnterpriseID: uuid.NullUUID{UUID: p.EnterpriseID, Valid: true}, ActorType: "enterprise_user", ActorID: p.UserID.String(), Action: "conversation.interaction.respond", ResourceType: "conversation", ResourceID: p.ConversationID.String(), Result: "success", Details: map[string]any{"interaction_id": r.ID, "kind": r.Kind, "status": status, "content_hash": r.ContentHash}}); e != nil {
			return View{}, e
		}
		return view(r), nil
	})
}
func implementationMessage(locale string) string {
	if strings.HasPrefix(locale, "en") {
		return "Yes, implement this plan."
	}
	return "是的，执行此计划"
}
