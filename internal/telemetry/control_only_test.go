package telemetry

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
)

func TestControlOnlyProfileSelectionDoesNotEnableAnImplicitSource(t *testing.T) {
	// No store is needed for an explicit empty set: there are no profile IDs
	// to resolve, and no default receiver should be silently introduced.
	keys, err := (Service{}).profiles(t.Context(), []uuid.UUID{}, configbundle.CatalogConfigSchemaVersion, "supported", "linux_amd64")
	if err != nil || keys == nil || len(keys) != 0 {
		t.Fatalf("explicit empty profiles: %v %v", keys, err)
	}
	for _, tc := range []struct{ schema, status string }{{"unsupported", "supported"}, {configbundle.CatalogConfigSchemaVersion, "retired"}, {configbundle.CatalogConfigSchemaVersion, "validation_pending"}} {
		if _, err := (Service{}).profiles(t.Context(), nil, tc.schema, tc.status, "linux_amd64"); !errors.Is(err, ErrQueryInvalid) {
			t.Fatalf("unsupported control-only contract: %v", err)
		}
	}
	if _, err := (Service{}).profiles(t.Context(), make([]uuid.UUID, 33), configbundle.CatalogConfigSchemaVersion, "supported", "linux_amd64"); !errors.Is(err, ErrQueryInvalid) {
		t.Fatal("profile budget bypassed")
	}
}
