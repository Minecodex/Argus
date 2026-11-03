package authorization

import (
	"context"
	"time"

	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Grant callbacks contain database work only, including the audit/outbox rows.
// Retrying confirmed PostgreSQL aborts repeats all existing subject, resource
// and caller-supplied version checks in a fresh serializable snapshot.
func (service Service) grantTransaction(ctx context.Context, callback func(*db.Queries) error) error {
	return retryGrantTransaction(ctx, func() error { return service.Store.InTx(ctx, callback) })
}

func retryGrantTransaction(ctx context.Context, attempt func() error) error {
	for n := 0; ; n++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := attempt()
		if !postgres.IsRetryableTransactionError(err) || n == 7 {
			return err
		}
		// Unknown commit results, transport failures and explicit stale caller
		// versions never reach this branch and are not replayed.
		timer := time.NewTimer(time.Duration(1<<n) * 20 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
