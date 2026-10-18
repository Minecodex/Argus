package remoteexecution

import (
	"context"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"time"
)

// The callback is database-only. Never place transport sends or file I/O here.
func retryStateTx(ctx context.Context, store *postgres.Store, fn func(*db.Queries) error) error {
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
