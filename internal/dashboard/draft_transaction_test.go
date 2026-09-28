package dashboard

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kakj-go/Argus/internal/resource"
)

func TestDraftWriteRetriesOnlyAbortedDatabaseTransactions(t *testing.T) {
	calls := 0
	err := retryDraftTransaction(context.Background(), func() error {
		calls++
		if calls <= 2 {
			return &pgconn.PgError{Code: "40001"}
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("transient audit contention not retried: %d %v", calls, err)
	}
	for _, expected := range []error{ErrConflict, ErrDenied, ErrArchived, &pgconn.PgError{Code: "23505"}} {
		calls = 0
		err = retryDraftTransaction(context.Background(), func() error { calls++; return expected })
		if !errors.Is(err, expected) || calls != 1 {
			t.Fatal("semantic conflict or permission failure retried")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	calls = 0
	err = retryDraftTransaction(ctx, func() error { calls++; cancel(); return &pgconn.PgError{Code: "40P01"} })
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatal("cancelled retry continued")
	}
}

func TestDatabaseContentionDoesNotPretendToBeAnEditorConflict(t *testing.T) {
	for _, code := range []string{"40001", "40P01"} {
		if !errors.Is(translateConflict(&pgconn.PgError{Code: code}), ErrUnavailable) {
			t.Fatal("database contention reported as editor conflict")
		}
	}
	if !errors.Is(translateConflict(ErrConflict), ErrConflict) || !errors.Is(translateConflict(&pgconn.PgError{Code: "23505"}), ErrConflict) {
		t.Fatal("real conflict lost")
	}
}

func TestDraftWriteAllowsPanelAuditBurstToDrain(t *testing.T) {
	calls := 0
	err := retryDraftTransaction(context.Background(), func() error {
		calls++
		if calls <= 5 {
			return &pgconn.PgError{Code: "40001"}
		}
		return ErrConflict // A concurrent editor won while the database was busy.
	})
	if !errors.Is(err, ErrConflict) || calls != 6 {
		t.Fatalf("audit burst hid a real editor conflict: calls=%d err=%v", calls, err)
	}
}

func TestDraftLookupDoesNotHideDatabaseContention(t *testing.T) {
	for _, err := range []error{&pgconn.PgError{Code: "40001"}, context.Canceled} {
		if !errors.Is(draftLookupError(err), err) {
			t.Fatal("lookup replaced operational failure with missing draft")
		}
	}
	if !errors.Is(draftLookupError(pgx.ErrNoRows), ErrNotFound) {
		t.Fatal("missing draft no longer maps to not found")
	}
}

func TestPublicationConflictRetainsActionAndDomainIdentity(t *testing.T) {
	err := actionFailure(ErrConflict)
	if !errors.Is(err, ErrConflict) {
		t.Fatal("domain conflict identity lost")
	}
	if code := resource.ActionValidationCode(err); code != "DASHBOARD_VERSION_CONFLICT" {
		t.Fatal(code)
	}
	if resource.ActionValidationCode(actionFailure(ErrDenied)) != "" {
		t.Fatal("permission failure mislabeled as conflict")
	}
}
