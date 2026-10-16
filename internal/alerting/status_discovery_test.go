package alerting

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/toolgateway"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestPolicyStatusCategoryIncludesTransitionsWithoutReplacingConfig(t *testing.T) {
	registry := mcp.NewRegistry()
	if err := (Tools{Service: Service{Store: &postgres.Store{}}}).Register(registry); err != nil {
		t.Fatal(err)
	}
	hub, err := toolgateway.New(registry, "plan6-status-discovery")
	if err != nil {
		t.Fatal(err)
	}
	set, err := toolruntime.NewSet(toolruntime.Snapshot{NativeCatalog: hub.Revision}, hub.CoreTools())
	if err != nil {
		t.Fatal(err)
	}
	for query, expected := range map[string]string{"policy update disable": "policy.disable.preview", "policy update enable": "policy.enable.preview", "策略 修改状态 停用": "policy.disable.preview"} {
		call := toolruntime.Invocation{Principal: toolruntime.Principal{EnterpriseID: uuid.New(), UserID: uuid.New(), Permissions: []string{"alert.policy.manage"}}, Arguments: map[string]any{"category": "alert"}}
		result, err := set.Invoke(t.Context(), "tool.search", call)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, item := range result.Data["items"].([]map[string]any) {
			found = found || item["name"] == expected
			if strings.HasSuffix(item["name"].(string), ".commit") {
				t.Fatal("Commit exposed")
			}
		}
		if !found {
			t.Fatalf("%q did not discover %s", query, expected)
		}
		call.Principal.Permissions = []string{"alert.read"}
		result, err = set.Invoke(t.Context(), "tool.search", call)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range result.Data["items"].([]map[string]any) {
			if strings.HasSuffix(item["name"].(string), ".preview") {
				t.Fatal("revoked write permission still discovers a preview", item["name"])
			}
		}
	}
	for _, operation := range []string{"enable", "disable", "delete"} {
		metadata, _ := registry.Lookup("alert.policy." + operation + ".preview")
		schema, err := toolruntime.CompileInputSchema(metadata.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		input := map[string]any{"id": uuid.NewString(), "expected_version": 2}
		if err = schema.Validate(input); err != nil {
			t.Fatal(err)
		}
		input["spec"] = map[string]any{"enabled": false, "description": ""}
		if err = schema.Validate(input); err == nil {
			t.Fatal("status/delete action accepts configuration replacement", operation)
		}
	}
}
