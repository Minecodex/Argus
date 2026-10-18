package interaction

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/remoteexecution"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type Service struct {
	Remote        *remoteexecution.Service
	Store         *postgres.Store
	Conversations conversation.Service
	Idempotency   postgres.Idempotency
}

func (s Service) authorize(ctx context.Context, p toolruntime.Principal) error {
	c, err := s.Conversations.Get(ctx, p.EnterpriseID, p.UserID, p.ConversationID)
	if err != nil || c.Status != "active" {
		return forbidden()
	}
	current, err := s.Conversations.ToolPrincipal(ctx, p.EnterpriseID, p.UserID, p.ConversationID)
	if err != nil {
		return err
	}
	if current.AuthorizationVersion != p.AuthorizationVersion || !current.Allows("conversation.use") {
		return forbidden()
	}
	return nil
}
func (s Service) List(ctx context.Context, p toolruntime.Principal) ([]View, error) {
	if err := s.authorize(ctx, p); err != nil {
		return nil, err
	}
	rows, err := s.Store.Queries.ListConversationInteractions(ctx, db.ListConversationInteractionsParams{EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, OwnerUserID: p.UserID})
	if err != nil {
		return nil, err
	}
	out := make([]View, 0, len(rows))
	for _, r := range rows {
		if r.AuthorizationVersion != p.AuthorizationVersion {
			continue
		}
		v := view(r)
		if r.ImplementationRunID.Valid {
			run, e := s.Store.Queries.GetRun(ctx, db.GetRunParams{ID: r.ImplementationRunID.UUID, EnterpriseID: p.EnterpriseID})
			if e != nil {
				return nil, e
			}
			v.ImplementationStatus = run.Status
		}
		out = append(out, v)
	}
	return out, nil
}
func (s Service) Get(ctx context.Context, p toolruntime.Principal, id uuid.UUID) (View, error) {
	if err := s.authorize(ctx, p); err != nil {
		return View{}, err
	}
	r, err := s.Store.Queries.GetConversationInteraction(ctx, db.GetConversationInteractionParams{ID: id, EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, OwnerUserID: p.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, forbidden()
	}
	if err == nil && r.AuthorizationVersion != p.AuthorizationVersion {
		return View{}, forbidden()
	}
	return view(r), err
}

func (s Service) prepare(ctx context.Context, call toolruntime.Invocation, kind string, content any) (db.ConversationInteraction, error) {
	if err := s.authorize(ctx, call.Principal); err != nil {
		return db.ConversationInteraction{}, err
	}
	raw, err := json.Marshal(content)
	if err != nil {
		return db.ConversationInteraction{}, err
	}
	if len(raw) > 1<<20 {
		return db.ConversationInteraction{}, invalid()
	}
	if prior, e := s.Store.Queries.GetToolInteraction(ctx, db.GetToolInteractionParams{ToolCallID: call.ID, EnterpriseID: call.Principal.EnterpriseID}); e == nil {
		if prior.OwnerUserID != call.Principal.UserID || prior.ConversationID != call.Principal.ConversationID || prior.RunID != call.RunID || prior.ContentHash != digest(raw) || prior.Kind != kind || prior.AuthorizationVersion != call.Principal.AuthorizationVersion {
			return db.ConversationInteraction{}, conflict()
		}
		return prior, nil
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return db.ConversationInteraction{}, e
	}
	var result db.ConversationInteraction
	prepare := func(q *db.Queries) error {
		run, e := q.GetRunForUpdate(ctx, db.GetRunForUpdateParams{ID: call.RunID, EnterpriseID: call.Principal.EnterpriseID})
		if e != nil {
			return e
		}
		if run.ActorUserID != call.Principal.UserID || run.ConversationID != call.Principal.ConversationID || run.AuthorizationVersion != call.Principal.AuthorizationVersion || conversation.TerminalRun(run.Status) {
			return conflict()
		}
		id := uuid.NewSHA1(call.ID, []byte("argus.interaction/v1"))
		family, revision := id, int32(1)
		if kind == "plan" {
			plan := content.(Plan)
			var old db.ConversationInteraction
			if plan.SupersedesID != nil {
				old, e = q.GetConversationInteraction(ctx, db.GetConversationInteractionParams{ID: *plan.SupersedesID, EnterpriseID: run.EnterpriseID, ConversationID: run.ConversationID, OwnerUserID: run.ActorUserID})
			} else {
				old, e = q.LatestRunPlan(ctx, db.LatestRunPlanParams{RunID: run.ID, EnterpriseID: run.EnterpriseID})
			}
			if e != nil && !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
			if e == nil {
				if old.Kind != "plan" || old.RunID != run.ID || (old.Status != "pending" && old.Status != "superseded") {
					return conflict()
				}
				family, revision = old.FamilyID, old.Revision+1
				if old.Status == "pending" {
					if _, e = q.ResolveConversationInteraction(ctx, db.ResolveConversationInteractionParams{ID: old.ID, EnterpriseID: run.EnterpriseID, Version: old.Version, Status: "superseded", Response: []byte(`{}`)}); e != nil {
						return e
					}
				}
			}
		}
		result, e = q.CreateConversationInteraction(ctx, db.CreateConversationInteractionParams{ID: id, EnterpriseID: run.EnterpriseID, ConversationID: run.ConversationID, OwnerUserID: run.ActorUserID, RunID: run.ID, ToolCallID: call.ID, Kind: kind, FamilyID: family, Revision: revision, Content: raw, ContentHash: digest(raw), AuthorizationVersion: call.Principal.AuthorizationVersion})
		if e != nil {
			return e
		}
		if kind == "plan" {
			plan := content.(Plan)
			if plan.Execution != nil {
				for index, step := range plan.Execution.Spec.Steps {
					spec := plan.Execution.Spec
					spec.Steps = []remoteexecution.Step{step}
					encoded, _ := json.Marshal(spec)
					if _, e = q.CreatePlanCommandAuthorization(ctx, db.CreatePlanCommandAuthorizationParams{ID: step.ID, EnterpriseID: run.EnterpriseID, ConversationID: run.ConversationID, OwnerUserID: run.ActorUserID, PlanID: id, Sequence: int32(index + 1), BlockID: plan.Execution.Blocks[index], Spec: encoded, SpecHash: remoteexecution.Digest(spec)}); e != nil {
						return e
					}
				}
			}
		}
		if _, e = Pause(ctx, q, run, id); e != nil {
			return e
		}
		_, e = conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: run.EnterpriseID, ConversationID: run.ConversationID, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, Type: "interaction_requested", ActorType: "service", Payload: map[string]any{"interaction_id": id, "kind": kind, "family_id": family, "revision": revision}, Classification: "internal"})
		return e
	}
	result, err = postgres.ExecuteIdempotent(ctx, s.Store, s.Idempotency, "enterprise", call.Principal.UserID.String(), "interaction.prepare", call.ID.String(), map[string]any{"kind": kind, "content": content, "run_id": call.RunID}, 201, func(q *db.Queries) (db.ConversationInteraction, error) { e := prepare(q); return result, e })
	return result, err
}

// Pause is a durable barrier; no worker or HTTP request waits for human input.
func Pause(ctx context.Context, q *db.Queries, run db.Run, id uuid.UUID) (db.Run, error) {
	current, err := q.GetRunForUpdate(ctx, db.GetRunForUpdateParams{ID: run.ID, EnterpriseID: run.EnterpriseID})
	if err != nil {
		return db.Run{}, err
	}
	if conversation.TerminalRun(current.Status) {
		return current, conflict()
	}
	updated, err := q.UpdateRunStatus(ctx, db.UpdateRunStatusParams{ID: run.ID, EnterpriseID: run.EnterpriseID, Version: current.Version, Status: "waiting_input", StopReason: pgtype.Text{String: "interaction:" + id.String(), Valid: true}})
	if err != nil {
		return db.Run{}, err
	}
	_, err = conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: run.EnterpriseID, ConversationID: run.ConversationID, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, Type: "run_state_changed", ActorType: "service", Payload: map[string]any{"status": "waiting_input", "stop_reason": "interaction", "interaction_id": id}, Classification: "internal"})
	return updated, err
}
