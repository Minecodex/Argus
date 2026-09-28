package agent

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		entered := false
		err = store.InTx(ctx, func(q *db.Queries) error {
			if err := lockTaskLease(ctx, q); err != nil {
				return err
			}
			entered = true
			return fn(q)
		})
		// A heartbeat can update the lease row after a SERIALIZABLE snapshot
		// starts. Retry only the acquisition: the callback never ran, so no
		// application side effects can be repeated. Recheck owner/fence/expiry.
		var conflict *pgconn.PgError
		if entered || !errors.As(err, &conflict) || (conflict.Code != "40001" && conflict.Code != "40P01") || ctx.Err() != nil {
			return err
		}
	}
	return err
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

// Streaming and final assistant events only append to a conversation and finish
// their already-owned step. NextConversationSequence atomically locks/updates
// the conversation counter. READ COMMITTED lets it observe a background query
// event that committed while waiting, instead of aborting a live provider stream
// with a stale SERIALIZABLE snapshot. The task row still fences every write;
// no provider request, tool action or generic application callback is retried.
func fencedEventTransaction(ctx context.Context, store *postgres.Store, fn func(*db.Queries) error) error {
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
			// No matching row proves loss of authority. A database concurrency
			// error does not; cancelling here also prevents recording provider usage.
			if errors.Is(err, pgx.ErrNoRows) {
				guard.cancel()
			}
			return err
		}
	}
	return nil
}
