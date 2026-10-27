package action

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/resource"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type pendingActionPreparer interface {
	PreparePendingAction(context.Context, db.PendingAction) (context.Context, error)
}

func (service Service) prepareConfirmation(ctx context.Context, actor, enterprise uuid.UUID, actionRef string, version int64) (context.Context, error) {
	preparer, ok := service.Resources.(pendingActionPreparer)
	if !ok {
		return ctx, nil
	}
	action, err := service.Store.Queries.GetPendingAction(ctx, db.GetPendingActionParams{EnterpriseID: enterprise, ActionRef: actionRef})
	if errors.Is(err, pgx.ErrNoRows) {
		return ctx, nil
	}
	if err != nil {
		return ctx, err
	}
	if action.Status != "awaiting_confirmation" {
		return ctx, nil // Idempotent replays remain handled by the normal store.
	}
	if action.CreatorSubjectType != "user" || action.CreatorSubjectID != actor || action.AuthorizationVersion != version || !authorizationCurrent(ctx, service.Store.Queries, action) {
		return ctx, ErrInvalidated
	}
	prepared, err := preparer.PreparePendingAction(ctx, action)
	if errors.Is(err, resource.ErrActionInvalidated) || errors.Is(err, resource.ErrActionUnavailable) {
		return ctx, errors.Join(ErrInvalidated, err)
	}
	return prepared, err
}

func (executor Executor) prepareExecution(ctx context.Context, payload ExecutionTask) (context.Context, error) {
	preparer, ok := executor.Resources.(pendingActionPreparer)
	if !ok {
		return ctx, nil
	}
	execution, err := executor.Store.Queries.GetExecution(ctx, db.GetExecutionParams{ID: payload.ExecutionID, EnterpriseID: payload.EnterpriseID})
	if err != nil {
		return ctx, err
	}
	if execution.Status != "pending" && execution.Status != "running" {
		return ctx, nil
	}
	action, err := executor.Store.Queries.GetPendingActionByID(ctx, db.GetPendingActionByIDParams{ID: execution.PendingActionID, EnterpriseID: payload.EnterpriseID})
	if err != nil {
		return ctx, err
	}
	if action.Status != "ready" || !authorizationCurrent(ctx, executor.Store.Queries, action) || !approvalCurrent(ctx, executor.Store.Queries, action) {
		return ctx, nil // The locked commit path records the invalidated outcome.
	}
	return preparer.PreparePendingAction(ctx, action)
}
