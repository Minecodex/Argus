package toolgateway

import (
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestDashboardAuthoringScopeIsExplicitAndCannotDefaultToAll(t *testing.T) {
	registry := mcp.NewRegistry()
	if err := (DashboardTools{Service: dashboard.Service{Store: &postgres.Store{}}}).Register(registry); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"draft.validate", "publish.preview"} {
		metadata, _ := registry.Lookup("telemetry.dashboard." + name)
		compiled, err := toolruntime.CompileInputSchema(metadata.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		params := map[string]any{"resource_mode": "explicit", "resource_types": []any{"host"}, "resource_ids": []any{uuid.NewString()}}
		input := map[string]any{"draft_id": uuid.NewString(), "expected_version": 1, "parameters": params}
		if err = compiled.Validate(input); err != nil {
			t.Fatal("valid bounded scope rejected", name, err)
		}
		delete(input, "parameters")
		if compiled.Validate(input) == nil {
			t.Fatal("dropped parameters defaulted to all", name)
		}
		input["parameters"] = params
		params["resource_mode"] = "all_authorized"
		if compiled.Validate(input) == nil {
			t.Fatal("native model selected implicit/all scope", name)
		}
		params["resource_mode"] = "explicit"
		params["resource_ids"] = nil
		if compiled.Validate(input) == nil {
			t.Fatal("nil resources accepted as bounded choice", name)
		}
		params["resource_ids"] = []any{}
		if err = compiled.Validate(input); err != nil {
			t.Fatal("explicit empty must remain no resources", err)
		}
		value, err := modelAuthoringParameters(input)
		if err != nil || value.ResourceIDs == nil || len(value.ResourceIDs) != 0 {
			t.Fatal("explicit empty widened", value, err)
		}
	}
}
