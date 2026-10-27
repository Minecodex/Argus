package httpapi

import (
	"context"
	"testing"

	"github.com/kakj-go/Argus/internal/kubernetesinventory"
)

func TestCRDDependencyErrorsUseConflictAndSharedLocalizedReason(t *testing.T) {
	for _, helm := range []bool{false, true} {
		value, status := inventoryError(context.Background(), kubernetesinventory.ErrCRDHasInstances)
		if helm {
			value, status = helmHTTPError(context.Background(), kubernetesinventory.ErrCRDHasInstances)
		}
		if status != 409 || value.Code != kubernetesinventory.ErrCRDHasInstances.Error() || value.MessageKey != "kubernetesWorkbench.errors.crdHasInstances" || value.Retryable == nil || *value.Retryable {
			t.Fatal("dependency refusal lost its actionable localized reason", helm, status, value.Code, value.MessageKey)
		}
	}
}
