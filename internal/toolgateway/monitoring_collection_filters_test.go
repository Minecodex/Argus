package toolgateway

import (
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"testing"
)

func TestCollectionListToolFilters(t *testing.T) {
	registry := monitoringDomainRegistry(t, &postgres.Store{})
	tool, _ := registry.Lookup("monitoring.collection.list")
	input := map[string]any{"integration": "host_metrics", "desired_enabled": false, "apply_status": "effective", "health": "healthy", "freshness": "fresh", "limit": 1}
	if err := tool.Validate(input); err != nil {
		t.Fatalf("collection filter contract rejected: %v", err)
	}
	for field, value := range map[string]any{"desired_enabled": "false", "apply_status": "ready", "health": "good", "freshness": "recent"} {
		changed := map[string]any{}
		for k, v := range input {
			changed[k] = v
		}
		changed[field] = value
		if err := tool.Validate(changed); err == nil {
			t.Fatalf("invalid filter accepted: %s", field)
		}
	}
	other, _ := registry.Lookup("monitoring.target.list")
	if err := other.Validate(map[string]any{"desired_enabled": false}); err == nil {
		t.Fatal("collection-only filter advertised on another directory")
	}
}
