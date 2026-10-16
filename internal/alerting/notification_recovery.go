package alerting

import (
	"context"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"time"
)

func (w NotificationWorker) beginSend(ctx context.Context, task runtime.Task, batch deliveryBatch) error {
	return alertTransaction(ctx, w.Service.Store, func(q *db.Queries) error {
		owned, err := q.AlertRuntimeTaskOwned(ctx, db.AlertRuntimeTaskOwnedParams{ID: task.ID, LeaseOwner: task.LeaseOwner, FenceToken: task.FenceToken})
		if err != nil || !owned {
			return ErrConflict
		}
		current, err := q.GetAlertDeliveryBatch(ctx, db.GetAlertDeliveryBatchParams{EnterpriseID: batch.Persistent.EnterpriseID, ID: batch.Persistent.ID})
		if err != nil {
			return err
		}
		if current.OwnerTaskID != task.ID || current.Fence != task.FenceToken || current.Status != "prepared" {
			return ErrConflict
		}
		return q.SetAlertBatchStatus(ctx, db.SetAlertBatchStatusParams{EnterpriseID: current.EnterpriseID, ID: current.ID, Status: "sending"})
	})
}

// Each delivery retains a task, but only the batch owner may send. A member's
// task observes an active owner and finishes without altering the batch.
func (w NotificationWorker) recoverBatch(ctx context.Context, q *db.Queries, task runtime.Task, d db.AlertDelivery) error {
	if !d.BatchID.Valid {
		return w.markUnknown(ctx, q, d)
	}
	batch, err := q.GetAlertDeliveryBatch(ctx, db.GetAlertDeliveryBatchParams{EnterpriseID: d.EnterpriseID, ID: d.BatchID.UUID})
	if err != nil {
		return err
	}
	if batch.Status != "prepared" && batch.Status != "sending" {
		return nil
	}
	if batch.OwnerTaskID == task.ID && batch.Fence == task.FenceToken {
		return nil
	}
	if batch.OwnerTaskID != task.ID {
		live, err := q.RuntimeTaskLeaseLive(ctx, batch.OwnerTaskID)
		if err != nil {
			return err
		}
		if live {
			return nil
		}
	}
	return w.reconcileBatch(ctx, q, batch)
}

func (w NotificationWorker) reconcileBatch(ctx context.Context, q *db.Queries, batch db.AlertDeliveryBatch) error {
	members, err := q.ListAlertBatchDeliveries(ctx, db.ListAlertBatchDeliveriesParams{EnterpriseID: batch.EnterpriseID, BatchID: nullID(batch.ID)})
	if err != nil {
		return err
	}
	unknown := batch.SendStartedAt.Valid
	status, code := "pending", "NOTIFICATION_WORKER_LOST_BEFORE_SEND"
	batchStatus := "failed"
	if unknown {
		status, code, batchStatus = "unknown", "NOTIFICATION_RESULT_UNKNOWN", "unknown"
	}
	for _, d := range members {
		if d.Status != "sending" {
			continue
		}
		if !unknown && time.Now().After(d.RetryUntil.Time) {
			status = "failed"
		}
		next := time.Now().UTC()
		updated, err := q.UpdateAlertDelivery(ctx, db.UpdateAlertDeliveryParams{EnterpriseID: d.EnterpriseID, ID: d.ID, Status: status, Attempt: d.Attempt, LastError: code, LastAttemptAt: d.LastAttemptAt, AvailableAt: timestamp(next), Version: d.Version})
		if err != nil {
			return err
		}
		attemptStatus := "failed"
		if unknown {
			attemptStatus = "unknown"
		}
		if err = q.AddAlertDeliveryAttempt(ctx, db.AddAlertDeliveryAttemptParams{ID: uuid.New(), EnterpriseID: d.EnterpriseID, DeliveryID: d.ID, Attempt: d.Attempt, Status: attemptStatus, ErrorCode: code, StartedAt: d.LastAttemptAt, FinishedAt: timestamp(next)}); err != nil {
			return err
		}
		if status == "pending" {
			if err = enqueueDelivery(ctx, q, updated, next); err != nil {
				return err
			}
		}
	}
	return q.SetAlertBatchStatus(ctx, db.SetAlertBatchStatusParams{EnterpriseID: batch.EnterpriseID, ID: batch.ID, Status: batchStatus})
}

// The runtime invokes exhaustion even when the last attempt lost its lease.
// No sending delivery remains stranded after its task becomes terminal.
func (w NotificationWorker) HandleExhausted(ctx context.Context, task runtime.Task, _ error) error {
	return alertTransaction(ctx, w.Service.Store, func(q *db.Queries) error {
		owned, err := q.AlertRuntimeTaskOwned(ctx, db.AlertRuntimeTaskOwnedParams{ID: task.ID, LeaseOwner: task.LeaseOwner, FenceToken: task.FenceToken})
		if err != nil || !owned {
			return ErrConflict
		}
		batches, err := q.ListTaskAlertBatches(ctx, db.ListTaskAlertBatchesParams{EnterpriseID: task.EnterpriseID.UUID, OwnerTaskID: task.ID})
		if err != nil {
			return err
		}
		for _, batch := range batches {
			if err = w.reconcileBatch(ctx, q, batch); err != nil {
				return err
			}
		}
		return nil
	})
}
