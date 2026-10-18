package remoteexecution

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type approvedCommand struct {
	Grant db.PlanCommandAuthorization
	Spec  Spec
}

func (s Service) ValidatePlanCommands(ctx context.Context, q *db.Queries, p toolruntime.Principal, authorization int64, m CommandManifest) error {
	if p.AuthorizationVersion != authorization {
		return Error("REMOTE_EXECUTION_FORBIDDEN")
	}
	return s.validateRequest(ctx, q, db.RemoteExecutionRequest{EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, OwnerUserID: p.UserID, AuthorizationVersion: authorization}, m.Spec, true)
}

func (s Service) PrepareApproved(ctx context.Context, call toolruntime.Invocation, id uuid.UUID) (View, error) {
	p, err := s.principal(ctx, call.Principal.EnterpriseID, call.Principal.UserID, call.Principal.ConversationID)
	if err != nil {
		return View{}, err
	}
	g, err := s.Store.Queries.GetPlanCommandAuthorization(ctx, db.GetPlanCommandAuthorizationParams{ID: id, EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, OwnerUserID: p.UserID})
	if err != nil {
		return View{}, Error("REMOTE_EXECUTION_FORBIDDEN")
	}
	if err = s.checkPlanCommandOwner(ctx, s.Store.Queries, call, g); err != nil {
		return View{}, err
	}
	if g.UsedRequestID.Valid {
		r, e := s.Store.Queries.GetRemoteRequest(ctx, db.GetRemoteRequestParams{ID: g.UsedRequestID.UUID, EnterpriseID: p.EnterpriseID})
		if e != nil {
			return View{}, e
		}
		return s.Get(ctx, p, r.ID)
	}
	spec, err := s.planCommandSpec(ctx, s.Store.Queries, call, g)
	if err != nil {
		return View{}, err
	}
	input := Input{Title: spec.Title, Description: spec.Description, Steps: spec.Steps}
	return s.prepare(ctx, call, "connect", input, &approvedCommand{Grant: g, Spec: spec})
}

func (s Service) checkPlanCommandOwner(ctx context.Context, q *db.Queries, call toolruntime.Invocation, g db.PlanCommandAuthorization) error {
	p := call.Principal
	if g.EnterpriseID != p.EnterpriseID || g.ConversationID != p.ConversationID || g.OwnerUserID != p.UserID || !g.ImplementationRunID.Valid || g.ImplementationRunID.UUID != call.RunID {
		return Error("REMOTE_EXECUTION_FORBIDDEN")
	}
	plan, err := q.GetConversationInteraction(ctx, db.GetConversationInteractionParams{ID: g.PlanID, EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, OwnerUserID: p.UserID})
	if err != nil || plan.Status != "confirmed" || plan.ImplementationRunID != g.ImplementationRunID || plan.AuthorizationVersion != p.AuthorizationVersion {
		return Error("REMOTE_COMMAND_CONFLICT")
	}
	run, err := q.GetRun(ctx, db.GetRunParams{ID: call.RunID, EnterpriseID: p.EnterpriseID})
	if err != nil || run.ApprovedPlanID.UUID != g.PlanID || run.ActorUserID != p.UserID || run.AuthorizationVersion != p.AuthorizationVersion || conversation.TerminalRun(run.Status) {
		return Error("REMOTE_EXECUTION_FORBIDDEN")
	}
	return nil
}

// Rebind only the context produced by earlier approved commands. Scripts,
// identities, resources and sealed input bytes remain the approved snapshot.
func (s Service) planCommandSpec(ctx context.Context, q *db.Queries, call toolruntime.Invocation, g db.PlanCommandAuthorization) (Spec, error) {
	var spec Spec
	if json.Unmarshal(g.Spec, &spec) != nil || Digest(spec) != g.SpecHash || len(spec.Steps) != 1 {
		return Spec{}, Error("REMOTE_COMMAND_CONFLICT")
	}
	step := &spec.Steps[0]
	grants, err := q.ListPlanCommandAuthorizations(ctx, db.ListPlanCommandAuthorizationsParams{PlanID: g.PlanID, EnterpriseID: g.EnterpriseID})
	if err != nil {
		return Spec{}, err
	}
	for _, prior := range grants {
		if prior.Sequence >= g.Sequence {
			break
		}
		if !prior.UsedRequestID.Valid {
			return Spec{}, Error("REMOTE_COMMAND_CONFLICT")
		}
		r, e := q.GetRemoteRequest(ctx, db.GetRemoteRequestParams{ID: prior.UsedRequestID.UUID, EnterpriseID: g.EnterpriseID})
		if e != nil || r.Status != "succeeded" {
			return Spec{}, Error("REMOTE_COMMAND_CONFLICT")
		}
		var completed Spec
		if json.Unmarshal(r.Spec, &completed) != nil || len(completed.Steps) != 1 {
			return Spec{}, Error("REMOTE_COMMAND_CONFLICT")
		}
		previous := completed.Steps[0]
		if previous.SessionID == step.SessionID {
			if previous.Generation != step.Generation {
				return Spec{}, Error("REMOTE_CONTEXT_CHANGED")
			}
			step.ContextRevision = previous.ContextRevision + 1
			step.PreviousRequestID = &r.ID
			step.PreviousSequence = 1
			current, e := q.GetRemoteSession(ctx, db.GetRemoteSessionParams{ID: step.SessionID, EnterpriseID: g.EnterpriseID})
			if e != nil {
				return Spec{}, e
			}
			step.KnownDirectory = current.KnownDirectory
		}
	}
	previous, err := q.GetRemoteContextPredecessor(ctx, db.GetRemoteContextPredecessorParams{SessionID: step.SessionID, Generation: step.Generation, EnterpriseID: g.EnterpriseID})
	if step.PreviousRequestID == nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return Spec{}, Error("REMOTE_CONTEXT_CHANGED")
		}
	} else if err != nil || previous.RequestID != *step.PreviousRequestID || int(previous.Sequence) != step.PreviousSequence {
		return Spec{}, Error("REMOTE_CONTEXT_CHANGED")
	}
	p := call.Principal
	if err = s.validateRequest(ctx, q, db.RemoteExecutionRequest{EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, OwnerUserID: p.UserID, AuthorizationVersion: p.AuthorizationVersion}, spec, true); err != nil {
		return Spec{}, err
	}
	spec.ID = uuid.NewSHA1(call.ID, []byte("argus.remote_execution/v1"))
	spec.RunID = call.RunID
	spec.Kind = "connect"
	spec.Description = step.Description
	step.ID = uuid.NewSHA1(spec.ID, []byte("1ns"))
	for i := range spec.Inputs {
		spec.Inputs[i].ID = uuid.NewSHA1(spec.ID, []byte(spec.Inputs[i].Path))
	}
	return spec, nil
}

func (s Service) lockPlanCommand(ctx context.Context, q *db.Queries, call toolruntime.Invocation, a *approvedCommand) (db.PlanCommandAuthorization, error) {
	// Match the lock order used by Run cancellation and confirmation.
	if _, err := q.GetRunForUpdate(ctx, db.GetRunForUpdateParams{ID: call.RunID, EnterpriseID: call.Principal.EnterpriseID}); err != nil {
		return db.PlanCommandAuthorization{}, err
	}
	if _, err := q.LockConversationInteraction(ctx, db.LockConversationInteractionParams{ID: a.Grant.PlanID, EnterpriseID: call.Principal.EnterpriseID, OwnerUserID: call.Principal.UserID}); err != nil {
		return db.PlanCommandAuthorization{}, err
	}
	g, err := q.LockPlanCommandAuthorization(ctx, db.LockPlanCommandAuthorizationParams{ID: a.Grant.ID, EnterpriseID: call.Principal.EnterpriseID})
	if err != nil {
		return g, err
	}
	if err = s.checkPlanCommandOwner(ctx, q, call, g); err != nil {
		return g, err
	}
	if !g.UsedRequestID.Valid {
		current, e := s.planCommandSpec(ctx, q, call, g)
		if e != nil {
			return g, e
		}
		if Digest(current) != Digest(a.Spec) {
			return g, Error("REMOTE_CONTEXT_CHANGED")
		}
	}
	return g, nil
}
