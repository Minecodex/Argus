package action

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Expiry never touches an already dispatched execution. Approval -> action ->
// Run is the same lock order used by decisions and their continuation.
func (reconciler Reconciler) ReconcileExpired(ctx context.Context) error {
	store := reconciler.Executor.Store
	items, err := store.Queries.ListExpiredPendingActions(ctx, 100)
	if err != nil {
		return err
	}
	for _, item := range items {
		err := store.InTx(ctx, func(q *db.Queries) error {
			approval, err := q.GetApprovalRequestByAction(ctx, db.GetApprovalRequestByActionParams{ActionRef: item.ActionRef, EnterpriseID: item.EnterpriseID})
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			hasApproval := err == nil
			if hasApproval {
				approval, err = q.GetApprovalRequestForUpdate(ctx, db.GetApprovalRequestForUpdateParams{ID: approval.ID, EnterpriseID: item.EnterpriseID})
				if err != nil {
					return err
				}
			}
			if _, err := q.GetPendingActionByIDForUpdate(ctx, db.GetPendingActionByIDForUpdateParams{ID: item.ID, EnterpriseID: item.EnterpriseID}); err != nil {
				return err
			}
			expired, err := q.ExpirePendingAction(ctx, db.ExpirePendingActionParams{ID: item.ID, EnterpriseID: item.EnterpriseID})
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
			if hasApproval && approval.Status == "pending" {
				if _, err := q.UpdateApprovalRequestStatus(ctx, db.UpdateApprovalRequestStatusParams{ID: approval.ID, EnterpriseID: item.EnterpriseID, Status: "expired"}); err != nil {
					return err
				}
			}
			return conversation.ScheduleActionRun(ctx, q, expired, false)
		})
		if err != nil {
			return err
		}
	}
	return nil
}
