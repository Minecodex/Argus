package remoteexecution

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func (s Service) view(ctx context.Context, r db.RemoteExecutionRequest) (View, error) {
	var spec Spec
	if json.Unmarshal(r.Spec, &spec) != nil {
		return View{}, Error("REMOTE_PLAN_INVALID")
	}
	steps, err := s.Store.Queries.ListRemoteSteps(ctx, db.ListRemoteStepsParams{RequestID: r.ID, EnterpriseID: r.EnterpriseID})
	if err != nil {
		return View{}, err
	}
	v := View{StopStatus: r.StopStatus, ID: r.ID, ActionRef: r.ActionRef, ContentHash: r.ContentHash, Status: r.Status, StopRequested: r.StopRequested, Revision: int(r.Revision), Spec: spec, Results: []StepResult{}, ErrorCode: r.ErrorCode, UpdatedAt: r.UpdatedAt.Time}
	if r.Status == "awaiting_confirmation" {
		a, e := s.Store.Queries.GetPendingAction(ctx, db.GetPendingActionParams{ActionRef: r.ActionRef, EnterpriseID: r.EnterpriseID})
		if e != nil {
			return v, e
		}
		v.ConfirmationRequired = a.Status == "awaiting_confirmation" && (a.Risk == "critical" || (!a.ApprovedPlanCommandID.Valid && a.ConfirmationMode != "full_access"))
		if time.Now().After(a.ExpiresAt.Time) {
			v.Status = "expired"
		}
		switch a.Status {
		case "cancelled", "expired":
			v.Status = a.Status
		case "invalidated", "rejected":
			v.Status = "failed"
		}
		if v.Status == "awaiting_confirmation" {
			for _, step := range spec.Steps {
				current, err := s.Store.Queries.GetRemoteSession(ctx, db.GetRemoteSessionParams{ID: step.SessionID, EnterpriseID: r.EnterpriseID})
				if err != nil {
					return View{}, err
				}
				if current.Generation != step.Generation || current.ContextRevision != step.ContextRevision || current.ActiveRequestID.Valid || !observedContextMatches(current, step) || (current.Status != "new" && current.Status != "active") {
					// A stale preview remains historical evidence, but cannot offer
					// approval against a context that has already changed.
					v.Status, v.ErrorCode = "expired", "REMOTE_CONTEXT_CHANGED"
					break
				}
			}
		}
	}
	for _, step := range steps {
		result := StepResult{OutputFiles: []File{}}
		_ = json.Unmarshal(step.Result, &result)
		result.Sequence = int(step.Sequence)
		result.Status = step.Status
		v.Results = append(v.Results, result)
	}
	return v, nil
}

type inputReadScopeKey struct{}
type inputReadScope struct{ visited map[uuid.UUID]bool }

func (s Service) Get(ctx context.Context, p toolruntime.Principal, id uuid.UUID) (View, error) {
	// Workspace source sets intentionally retain remote outputs. Validate each
	// request's input dependencies once per synchronous read traversal, so that
	// Workspace -> remote result -> Workspace cannot recurse indefinitely.
	scope, _ := ctx.Value(inputReadScopeKey{}).(*inputReadScope)
	if scope == nil {
		scope = &inputReadScope{visited: map[uuid.UUID]bool{}}
		ctx = context.WithValue(ctx, inputReadScopeKey{}, scope)
	}
	current, err := s.principal(ctx, p.EnterpriseID, p.UserID, p.ConversationID, "remote_execution.read")
	if err != nil {
		return View{}, err
	}
	r, err := s.Store.Queries.GetRemoteRequest(ctx, db.GetRemoteRequestParams{ID: id, EnterpriseID: p.EnterpriseID})
	if err != nil || r.OwnerUserID != p.UserID || r.ConversationID != p.ConversationID {
		return View{}, Error("REMOTE_EXECUTION_FORBIDDEN")
	}
	var spec Spec
	if json.Unmarshal(r.Spec, &spec) != nil {
		return View{}, Error("REMOTE_PLAN_INVALID")
	}
	for _, step := range spec.Steps {
		if err = authorizeTarget(ctx, s.Store.Queries, current, step.Target); err != nil {
			return View{}, err
		}
	}
	if len(spec.Inputs) > 0 && !scope.visited[r.ID] {
		scope.visited[r.ID] = true
		if s.Files == nil {
			return View{}, Error("REMOTE_TRANSFER_UNAVAILABLE")
		}
		if err = s.Files.ValidateRemoteInputs(ctx, current, spec.Inputs); err != nil {
			return View{}, err
		}
	}
	return s.view(ctx, r)
}
func (s Service) List(ctx context.Context, p toolruntime.Principal) ([]View, error) {
	if _, err := s.principal(ctx, p.EnterpriseID, p.UserID, p.ConversationID, "remote_execution.read"); err != nil {
		return nil, err
	}
	rows, err := s.Store.Queries.ListConversationRemoteRequests(ctx, db.ListConversationRemoteRequestsParams{EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, OwnerUserID: p.UserID})
	if err != nil {
		return nil, err
	}
	views := []View{}
	for _, r := range rows {
		v, e := s.Get(ctx, p, r.ID)
		if e != nil {
			return nil, e
		}
		views = append(views, v)
	}
	return views, nil
}
func (s Service) Stop(ctx context.Context, p toolruntime.Principal, id uuid.UUID) (View, error) {
	current, err := s.principal(ctx, p.EnterpriseID, p.UserID, p.ConversationID, "remote_execution.read")
	if err != nil {
		return View{}, err
	}
	if !current.Allows("remote_execution.stop") {
		return View{}, Error("REMOTE_EXECUTION_FORBIDDEN")
	}
	if _, err = s.Get(ctx, p, id); err != nil {
		return View{}, err
	}
	r, err := s.Store.Queries.GetRemoteRequest(ctx, db.GetRemoteRequestParams{ID: id, EnterpriseID: p.EnterpriseID})
	if err != nil {
		return View{}, err
	}
	err = retryStateTx(ctx, s.Store, func(q *db.Queries) error {
		action, e := q.GetPendingActionForUpdate(ctx, db.GetPendingActionForUpdateParams{ActionRef: r.ActionRef, EnterpriseID: r.EnterpriseID})
		if e != nil {
			return e
		}
		run, e := q.GetRunForUpdate(ctx, db.GetRunForUpdateParams{ID: r.RunID, EnterpriseID: r.EnterpriseID})
		if e != nil {
			return e
		}
		r, e = q.LockRemoteRequest(ctx, db.LockRemoteRequestParams{ID: id, EnterpriseID: p.EnterpriseID})
		if e != nil {
			return e
		}
		if r.Status != "awaiting_confirmation" && r.Status != "ready" && r.Status != "running" && r.Status != "stopping" && r.Status != "result_unknown" && r.Status != "paused_for_recovery" {
			return nil
		}
		terminalRecovery := r.Status == "result_unknown" || r.Status == "paused_for_recovery"
		r, e = q.StopRemoteRequest(ctx, db.StopRemoteRequestParams{ID: id, EnterpriseID: p.EnterpriseID, OwnerUserID: p.UserID})
		if e != nil {
			return e
		}
		if terminalRecovery {
			payload, _ := json.Marshal(Task{Operation: "stop", RequestID: r.ID, EnterpriseID: r.EnterpriseID})
			if _, e = q.CreateRuntimeTask(ctx, db.CreateRuntimeTaskParams{ID: uuid.New(), EnterpriseID: uuid.NullUUID{UUID: r.EnterpriseID, Valid: true}, Queue: ControlQueue, Payload: payload, MaxAttempts: 3, AvailableAt: timeValue(time.Now())}); e != nil {
				return e
			}
		}
		if action.Status == "awaiting_confirmation" || action.Status == "awaiting_approval" || action.Status == "ready" {
			if _, e = q.CancelPendingAction(ctx, db.CancelPendingActionParams{ActionRef: r.ActionRef, EnterpriseID: p.EnterpriseID, CreatorSubjectID: p.UserID}); e != nil {
				return e
			}
		}
		if _, e = q.LockConversation(ctx, db.LockConversationParams{ID: r.ConversationID, EnterpriseID: r.EnterpriseID, OwnerUserID: r.OwnerUserID}); e != nil {
			return e
		}
		_, e = conversation.CancelRunRecord(ctx, q, run, "remote_execution_stopped", "user", p.UserID.String())
		return e
	})
	if err != nil {
		return View{}, err
	}
	return s.view(ctx, r)
}
func (s Service) supersede(ctx context.Context, q *db.Queries, p toolruntime.Principal, id uuid.UUID) error {
	preview, e := q.GetRemoteRequest(ctx, db.GetRemoteRequestParams{ID: id, EnterpriseID: p.EnterpriseID})
	if e != nil {
		return e
	}
	if _, e = q.GetPendingActionForUpdate(ctx, db.GetPendingActionForUpdateParams{ActionRef: preview.ActionRef, EnterpriseID: p.EnterpriseID}); e != nil {
		return e
	}
	old, err := q.LockRemoteRequest(ctx, db.LockRemoteRequestParams{ID: id, EnterpriseID: p.EnterpriseID})
	if err != nil || old.OwnerUserID != p.UserID || old.ConversationID != p.ConversationID {
		return Error("REMOTE_EXECUTION_FORBIDDEN")
	}
	n, err := q.SupersedeRemoteRequest(ctx, db.SupersedeRemoteRequestParams{ID: id, EnterpriseID: p.EnterpriseID, OwnerUserID: p.UserID})
	if err != nil {
		return err
	}
	if n != 1 {
		return Error("REMOTE_SESSION_BUSY")
	}
	if old.Status == "awaiting_confirmation" {
		a, e := q.GetPendingActionForUpdate(ctx, db.GetPendingActionForUpdateParams{ActionRef: old.ActionRef, EnterpriseID: p.EnterpriseID})
		if e != nil {
			return e
		}
		_, err = q.InvalidatePendingActionM4(ctx, db.InvalidatePendingActionM4Params{ID: a.ID, EnterpriseID: p.EnterpriseID, ErrorCode: pgtype.Text{String: "ACTION_SUPERSEDED", Valid: true}})
	}
	return err
}

// ActivityFacts is model-only; no HTTP handler or conversation event serializes it.
// One bounded query applies current resource grants before selecting any activity.
// Command labels are deliberately lossy, not a parser or an authorization decision.
func (s Service) ActivityFacts(ctx context.Context, run db.Run) (map[string]any, error) {
	p, err := s.principal(ctx, run.EnterpriseID, run.ActorUserID, run.ConversationID, "remote_execution.read")
	if err != nil {
		return map[string]any{"status": "unavailable", "observed_at": time.Now().UTC()}, nil
	}
	rows, err := s.Store.Queries.ListRemoteResourceActivity(ctx, db.ListRemoteResourceActivityParams{EnterpriseID: p.EnterpriseID, UserID: p.UserID, ConversationID: p.ConversationID, CanReadOthers: p.Allows("execution.read")})
	if err != nil {
		return nil, err
	}
	facts := []map[string]any{}
	for _, row := range rows {
		facts = append(facts, map[string]any{"execution_id": row.ID, "resource_type": row.ResourceType, "resource_id": row.ResourceID, "command_summary": activityCommandLabel(row.Script), "status": row.StepStatus, "observed_at": row.UpdatedAt.Time})
	}
	return map[string]any{"schema_version": "argus.remote_activity/v1", "status": "observed", "authorization_version": p.AuthorizationVersion, "scope": "Argus controlled executions permitted for the current actor; not all operating-system processes. Arguments, scripts, output and chat content are withheld.", "untrusted_data": true, "observed_at": time.Now().UTC(), "bounded": len(facts) >= 16, "activities": facts}, nil
}
