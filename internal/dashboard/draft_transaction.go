package dashboard

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func draftLookupError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// These callbacks contain database work only. A retry uses a new serializable
// snapshot and repeats every permission/lifecycle/expected-version check.
// Explicit editor conflicts and unique constraints are never retried.
func (service Service) draftTransaction(ctx context.Context, callback func(*db.Queries) error) error {
	return retryDraftTransaction(ctx, func() error { return service.Store.InTx(ctx, callback) })
}
func retryDraftTransaction(ctx context.Context, attempt func() error) error {
	var err error
	// A view may append audit records for many panels while the user opens an
	// editor. Leave a bounded backoff window for that burst to drain.
	for n := 0; n < 8; n++ {
		if err = ctx.Err(); err != nil {
			return err
		}
		err = attempt()
		var state interface{ SQLState() string }
		if !errors.As(err, &state) || (state.SQLState() != "40001" && state.SQLState() != "40P01") || n == 7 {
			return err
		}
		timer := time.NewTimer(time.Duration(1<<n) * 20 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}
