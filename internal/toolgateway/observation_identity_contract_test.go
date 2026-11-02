package toolgateway

import (
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/storage/postgres"
)

func TestObservationIdentityAnchorsChangeOutputWithoutAddingInputFilters(t *testing.T) {
	registry := mcp.NewRegistry()
	if err := (ObservationTools{Base: ResourceTools{Store: &postgres.Store{}}}).Register(registry); err != nil {
		t.Fatal(err)
	}
	metadata, ok := registry.Lookup("observe.capabilities.get")
	if !ok || metadata.Discovery.Revision != "7" || metadata.OutputVersion != "argus.observe_result/v5" {
		t.Fatal("identity output changed without a new frozen contract")
	}
	properties := metadata.InputSchema["properties"].(map[string]any)
	if len(properties) != 4 || properties["resources"] == nil || properties["signals"] == nil || properties["sections"] == nil || properties["cursor"] == nil {
		t.Fatal("Collector identity anchors became additional capability input filters")
	}
	for _, fact := range []string{"configured_collector_id", "effective_collector_id", "requires_authorized_resolution", "NOT verified", "ask rather than guess"} {
		if !strings.Contains(metadata.Discovery.Description, fact) {
			t.Fatal("identity anchor semantics missing from model discovery", fact)
		}
	}
}
