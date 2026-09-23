package agent

import (
	"context"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type taskLeaseKey struct{}
type taskLeaseGuard struct {
	store  *postgres.Store
	task   runtime.Task
	cancel context.CancelFunc
}

// The heartbeat cancels promptly; this guard also fences an old process that
// resumes after another Worker has taken over its task.
func withTaskLease(ctx context.Context, store *postgres.Store, task runtime.Task) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	return context.WithValue(ctx, taskLeaseKey{}, taskLeaseGuard{store, task, cancel}), cancel
}
func assertTaskLease(ctx context.Context) error {
	guard, ok := ctx.Value(taskLeaseKey{}).(taskLeaseGuard)
	if !ok {
		return ctx.Err()
	}
	owned, err := guard.store.Queries.IsRuntimeTaskLeaseCurrent(ctx, db.IsRuntimeTaskLeaseCurrentParams{ID: guard.task.ID, LeaseOwner: guard.task.LeaseOwner, FenceToken: guard.task.FenceToken})
	if err != nil || !owned {
		guard.cancel()
		if err != nil {
			return err
		}
		return context.Canceled
	}
	return nil
}

// Hold the task row through each event transaction. A takeover either sees the
// complete committed event group, or wins first and prevents the old writer.
func fencedTransaction(ctx context.Context, store *postgres.Store, fn func(*db.Queries) error) error {
	return store.InTx(ctx, func(q *db.Queries) error {
		if err := lockTaskLease(ctx, q); err != nil {
			return err
		}
		return fn(q)
	})
}

// Terminal transitions lock their Run before reading state. Read committed
// lets concurrent cancellation/completion observe the winning terminal state
// instead of rejecting an otherwise idempotent retry with a stale snapshot.
func fencedRunTransition(ctx context.Context, store *postgres.Store, fn func(*db.Queries) error) error {
	return store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		if err := lockTaskLease(ctx, q); err != nil {
			return err
		}
		return fn(q)
	})
}

func lockTaskLease(ctx context.Context, q *db.Queries) error {
	if guard, ok := ctx.Value(taskLeaseKey{}).(taskLeaseGuard); ok {
		if _, err := q.LockRuntimeTaskLease(ctx, db.LockRuntimeTaskLeaseParams{ID: guard.task.ID, LeaseOwner: guard.task.LeaseOwner, FenceToken: guard.task.FenceToken}); err != nil {
			guard.cancel()
			return err
		}
	}
	return nil
}
