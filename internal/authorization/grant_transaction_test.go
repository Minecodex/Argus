package authorization

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestGrantTransactionRetriesOnlyConfirmedAbortsAndKeepsCallerConflicts(t *testing.T) {
	for _, code := range []string{"40001", "40P01"} {
		calls := 0
		err := retryGrantTransaction(t.Context(), func() error {
			calls++
			if calls < 3 {
				return &pgconn.PgError{Code: code}
			}
			return nil
		})
		if err != nil || calls != 3 {
			t.Fatal("known aborted transaction was not retried", code, calls, err)
		}
	}
	for _, want := range []error{ErrAuthorizationConflict, errors.New("commit result unknown"), &pgconn.PgError{Code: "08006"}, &pgconn.PgError{Code: "23505"}} {
		calls := 0
		err := retryGrantTransaction(t.Context(), func() error { calls++; return want })
		if err != want || calls != 1 {
			t.Fatal("unknown outcome or semantic conflict was replayed", calls, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	calls := 0
	err := retryGrantTransaction(ctx, func() error { calls++; cancel(); return &pgconn.PgError{Code: "40001"} })
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatal("cancelled grant request continued retrying", calls, err)
	}
}
