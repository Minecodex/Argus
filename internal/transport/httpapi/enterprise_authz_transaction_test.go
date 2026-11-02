package httpapi

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kakj-go/Argus/internal/authorization"
)

func TestGrantWriteErrorContractDistinguishesAbortedTransactionsFromStaleAndUnknown(t *testing.T) {
	for _, state := range []string{"40001", "40P01"} {
		err := &pgconn.PgError{Code: state, Message: "private database diagnostics"}
		body := authzError(context.Background(), err)
		if grantUpdateErrorStatus(err) != http.StatusConflict || body.Code != "VERSION_CONFLICT" || body.Retryable == nil || !*body.Retryable || body.MessageKey != "errors.common.version_conflict" {
			t.Fatal("confirmed aborted transaction advertised as an unknown permanent failure", state, body)
		}
		if body.Message != nil && *body.Message != "" {
			t.Fatal("raw transaction diagnostics leaked")
		}
	}
	stale := authzError(context.Background(), authorization.ErrAuthorizationConflict)
	if grantUpdateErrorStatus(authorization.ErrAuthorizationConflict) != http.StatusConflict || stale.Code != "AUTHORIZATION_VERSION_STALE" {
		t.Fatal("explicit stale authorization version was confused with internal retry", stale)
	}
	unknown := errors.New("unknown grant operation result")
	if grantUpdateErrorStatus(unknown) != http.StatusInternalServerError || authzError(context.Background(), unknown).Code != "INTERNAL_ERROR" {
		t.Fatal("unknown failure incorrectly advertised as a version conflict")
	}
	if grantUpdateErrorStatus(authorization.ErrInvalidResource) != http.StatusUnprocessableEntity {
		t.Fatal("invalid resource error status changed")
	}
}
