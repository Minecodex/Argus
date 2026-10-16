package alerting

import (
	"context"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"time"
)

// Only transactional database work is retried. External sends are never inside
// this boundary and cannot be duplicated by serialization/deadlock recovery.
func alertTransaction(ctx context.Context, store *postgres.Store, fn func(*db.Queries) error) error {
	for attempt := 0; ; attempt++ {
		err := store.InTx(ctx, fn)
		if attempt >= 7 || !postgres.IsRetryableTransactionError(err) {
			return err
		}
		timer := time.NewTimer(time.Duration(20*(attempt+1)) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// Preview and handling use the existing idempotent transaction service. A
// serialization retry repeats only those database operations, never a send.
func retryAlertWrite[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	for attempt := 0; ; attempt++ {
		value, err := fn()
		if attempt >= 7 || !postgres.IsRetryableTransactionError(err) {
			return value, err
		}
		timer := time.NewTimer(time.Duration(20*(attempt+1)) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			var empty T
			return empty, ctx.Err()
		case <-timer.C:
		}
	}
}
