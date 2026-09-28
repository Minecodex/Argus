package resource

import (
	"errors"
	"testing"
)

type validationFailure struct{ code string }

func (e validationFailure) Error() string                { return "private worker detail" }
func (e validationFailure) ActionValidationCode() string { return e.code }
func TestActionValidationCodeRequiresExplicitSafeMarker(t *testing.T) {
	if ActionValidationCode(errors.New("DASHBOARD_VERSION_CONFLICT")) != "" {
		t.Fatal("raw error text became a public code")
	}
	if ActionValidationCode(validationFailure{"PRIVATE_WORKER_SECRET"}) != "" {
		t.Fatal("unregistered code exposed")
	}
	if got := ActionValidationCode(errors.Join(ErrActionInvalidated, validationFailure{"DASHBOARD_VERSION_CONFLICT"})); got != "DASHBOARD_VERSION_CONFLICT" {
		t.Fatal(got)
	}
}
