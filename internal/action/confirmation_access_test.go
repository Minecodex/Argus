package action

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type domainConflict struct{}

func (domainConflict) Error() string                { return "private detail" }
func (domainConflict) ActionValidationCode() string { return "DASHBOARD_VERSION_CONFLICT" }
func TestConfirmationConflictIsVisibleOnlyToItsCreator(t *testing.T) {
	owner := uuid.New()
	a := db.PendingAction{CreatorSubjectID: owner, CreatorSubjectType: "user"}
	cause := errors.Join(ErrInvalidated, domainConflict{})
	if resource.ActionValidationCode(confirmationAccessError(a, owner, cause)) != "DASHBOARD_VERSION_CONFLICT" {
		t.Fatal("owner lost useful rejection")
	}
	if resource.ActionValidationCode(confirmationAccessError(a, uuid.New(), cause)) != "" {
		t.Fatal("another user learned domain rejection")
	}
	a.CreatorSubjectType = "service"
	if !errors.Is(confirmationAccessError(a, owner, cause), ErrInvalidated) || resource.ActionValidationCode(confirmationAccessError(a, owner, cause)) != "" {
		t.Fatal("service subject bypassed confirmation ownership")
	}
}
func TestExecutorRetainsRegisteredConflictAfterWrapping(t *testing.T) {
	err := runtime.Error{ErrorCode: "DASHBOARD_VERSION_CONFLICT", Cause: domainConflict{}, Permanent: true}
	if executionFailureCode(err) != "DASHBOARD_VERSION_CONFLICT" {
		t.Fatal("async execution lost conflict reason")
	}
}
