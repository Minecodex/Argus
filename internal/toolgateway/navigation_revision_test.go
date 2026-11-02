package toolgateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestNavigationMetadataInvalidatesNativeSnapshot(t *testing.T) {
	document := fixtureDiscovery("navigation revision")
	document.Navigation = mcp.Navigation{ResourceTypes: []string{"host"}, Signals: []string{"metrics"}, OperationKind: "discover", ResultKind: []string{"resource_list"}}
	base := discoveryFixtureGateway(t, document, false)
	principal := toolruntime.Principal{EnterpriseID: uuid.New(), UserID: uuid.New()}
	original, err := (toolruntime.CompositeFactory{Providers: []toolruntime.Provider{base}}).Build(t.Context(), principal)
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*mcp.Discovery){
		"resource_types": func(d *mcp.Discovery) { d.ResourceTypes = []string{"application"} },
		"signals":        func(d *mcp.Discovery) { d.Signals = []string{"logs"} },
		"operation_kind": func(d *mcp.Discovery) { d.OperationKind = "query" },
		"result_kind":    func(d *mcp.Discovery) { d.ResultKind = []string{"metric_catalog"} },
	} {
		t.Run(name, func(t *testing.T) {
			changed := document
			change(&changed)
			current := discoveryFixtureGateway(t, changed, false)
			if current.Revision == base.Revision || current.manifests[0].Version == base.manifests[0].Version || current.manifests[0].DiscoveryHash == base.manifests[0].DiscoveryHash {
				t.Fatal("changed navigation reused an earlier discovery, tool or catalog version")
			}
			restored, err := (toolruntime.CompositeFactory{Providers: []toolruntime.Provider{current}}).Restore(t.Context(), principal, original.Snapshot)
			if err != nil {
				t.Fatal(err)
			}
			_, err = restored.Invoke(t.Context(), "tool.search", toolruntime.Invocation{Principal: principal, Arguments: map[string]any{"category": "host"}})
			var unavailable toolruntime.Error
			if !errors.As(err, &unavailable) || unavailable.Kind != "TOOL_VERSION_UNAVAILABLE" {
				t.Fatal("restored Run silently used changed navigation", err)
			}
		})
	}
	before, _ := json.Marshal(base.manifests)
	document.ResourceTypes[0] = "application"
	document.Signals[0] = "logs"
	document.ResultKind[0] = "metric_catalog"
	after, _ := json.Marshal(base.manifests)
	if !bytes.Equal(before, after) {
		t.Fatal("caller mutation changed frozen navigation")
	}
}
