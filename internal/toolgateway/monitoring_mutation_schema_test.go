package toolgateway

import (
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"testing"
)

func TestCollectionEnableContractRequiresExplicitNestedState(t *testing.T) {
	reg := monitoringDomainRegistry(t, &postgres.Store{})
	tool, _ := reg.Lookup("monitoring.collection.change.preview")
	for _, enabled := range []bool{false, true} {
		input := map[string]any{"kind": "collection", "operation": "set_enabled", "id": uuid.NewString(), "expected_version": 1, "collection": map[string]any{"enabled": enabled}}
		if err := tool.Validate(input); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"id", "expected_version", "collection"} {
			copy := map[string]any{}
			for k, v := range input {
				copy[k] = v
			}
			delete(copy, field)
			if tool.Validate(copy) == nil {
				t.Fatalf("missing %s accepted", field)
			}
		}
		input["collection"] = map[string]any{}
		if tool.Validate(input) == nil {
			t.Fatal("absent enabled silently disabled a collection")
		}
		input["collection"] = map[string]any{"enabled": enabled, "target": map[string]any{"kind": "monitoring_target", "id": uuid.NewString()}}
		if tool.Validate(input) == nil {
			t.Fatal("set_enabled accepted unrelated changes")
		}
	}
}
