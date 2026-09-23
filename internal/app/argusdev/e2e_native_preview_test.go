package argusdev

import (
	"context"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	argusopenapi "github.com/kakj-go/Argus/api/openapi"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/toolgateway"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestP5RequestKeysFollowThePublicContractForLongScenarioNames(t *testing.T) {
	document, err := openapi3.NewLoader().LoadFromData(argusopenapi.BundledJSON)
	if err != nil {
		t.Fatal(err)
	}
	parameter := document.Components.Parameters["IdempotencyKey"]
	if parameter == nil || parameter.Value == nil || parameter.Value.Schema == nil {
		t.Fatal("Idempotency-Key contract missing")
	}
	for _, prefix := range []string{"p5-conversation", "p5-message", "p5-preview-cancel"} {
		name := "m7-collector-kubernetes-cluster-" + strings.Repeat("long-scenario-", 100) + "配置\n"
		key := p5RequestKey(prefix, name)
		if err := parameter.Value.Schema.Value.VisitJSON(key); err != nil {
			t.Fatalf("generated key violates public contract: %v", err)
		}
		// enterpriseHeaders appends the run identity after the bounded prefix.
		if err := parameter.Value.Schema.Value.VisitJSON(key + "-" + strings.Repeat("r", 64)); err != nil {
			t.Fatalf("run-scoped key violates public contract: %v", err)
		}
		if key != p5RequestKey(prefix, name) || key == p5RequestKey(prefix, name+"next") {
			t.Fatal("request key is unstable or aliases another scenario")
		}
	}
}

func TestP5RecoveryPreviewUsesTheFullHTTPContract(t *testing.T) {
	document, err := openapi3.NewLoader().LoadFromData(argusopenapi.BundledJSON)
	if err != nil {
		t.Fatal(err)
	}
	if err := document.Components.Schemas["HostPreviewCreate"].Value.VisitJSON(p5RecoveryPreviewInput(strings.Repeat("r", 64))); err != nil {
		t.Fatal(err)
	}
}

func TestM7CollectorIdentityUsesTheActualGatewayCategory(t *testing.T) {
	for _, resource := range []string{"host", "kubernetes-cluster"} {
		category, _ := m7NativeCollectorIdentity(resource)
		prefix := "host"
		if resource == "kubernetes-cluster" {
			prefix = "kubernetes"
		}
		registry := mcp.NewRegistry()
		if err := registry.Register(mcp.Metadata{ID: prefix + ".collector.install.preview", Discovery: mcp.Discovery{SchemaVersion: "argus.tool_discovery/v1", Revision: "1", Title: "Collector install preview", Description: "Preview installation of a collector for category mapping tests.", Keywords: []string{"install", "collector"}, ResultDescription: "Returns a public pending action.", Preconditions: []string{"Uses an empty synthetic input schema."}, Examples: []mcp.DiscoveryExample{{Request: "Install a collector", Guidance: "Describe the fixture without executing it."}}}, Risk: "write", Visibility: mcp.Visible, InputVersion: "1", OutputVersion: "1", MaxResultBytes: 1024, InputSchema: map[string]any{"type": "object", "additionalProperties": false}, Execute: func(context.Context, mcp.Call) (mcp.Result, error) {
			t.Fatal("Describe executed the business operation")
			return mcp.Result{}, nil
		}}); err != nil {
			t.Fatal(err)
		}
		gateway, err := toolgateway.New(registry, "category-contract")
		if err != nil {
			t.Fatal(err)
		}
		set, err := toolruntime.NewSet(toolruntime.Snapshot{NativeCatalog: gateway.Revision}, gateway.CoreTools())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := set.Invoke(t.Context(), "tool.describe", toolruntime.Invocation{Arguments: map[string]any{"category": category, "name": "collector.install.preview"}}); err != nil {
			t.Fatalf("%s fixture used an invalid model category: %v", resource, err)
		}
	}
}
