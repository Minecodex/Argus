package resource

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"time"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// ActionExecutionPreparer performs external read-only checks before the commit
// transaction. Its result is request-local, never a substitute for the token,
// authorization, plan hash or version checks inside ExecuteReady.
type ActionExecutionPreparer interface {
	PrepareActionExecution(context.Context, db.PendingAction, json.RawMessage) (context.Context, error)
}

// VerifiedPlan reads the immutable database plan without consuming or exposing
// the private token. ExecuteReady repeats this check under the action lock.
func (service PendingActionService) VerifiedPlan(ctx context.Context, q *db.Queries, action db.PendingAction) (json.RawMessage, error) {
	if action.Status != "ready" && action.Status != "awaiting_confirmation" || time.Now().UTC().After(action.ExpiresAt.Time) {
		return nil, ErrActionUnavailable
	}
	plan, err := q.GetPendingActionPlan(ctx, db.GetPendingActionPlanParams{ActionRef: action.ActionRef, EnterpriseID: action.EnterpriseID})
	if err != nil {
		return nil, pendingActionStorageError("load immutable plan", err, ErrActionInvalidated)
	}
	if plan.AuthorizationVersion != action.AuthorizationVersion {
		return nil, ErrActionInvalidated
	}
	canonical, err := CanonicalJSON(plan.ImmutablePlan)
	if err != nil {
		return nil, ErrActionInvalidated
	}
	digest := sha256.Sum256(canonical)
	if !subtleEqual(digest[:], plan.PlanHash) {
		return nil, ErrActionInvalidated
	}
	return canonical, nil
}

func (service Service) PreparePendingAction(ctx context.Context, action db.PendingAction) (context.Context, error) {
	preparer, ok := service.Extension.(ActionExecutionPreparer)
	if !ok {
		return ctx, nil
	}
	raw, err := service.Actions.VerifiedPlan(ctx, service.Store.Queries, action)
	if err != nil {
		return ctx, err
	}
	return preparer.PrepareActionExecution(ctx, action, raw)
}
