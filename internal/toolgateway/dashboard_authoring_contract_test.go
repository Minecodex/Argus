package toolgateway

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestDashboardAuthoringContractExplainsVersionTimeAndStepPolicy(t *testing.T) {
	registry := mcp.NewRegistry()
	if err := (DashboardTools{Service: dashboard.Service{Store: &postgres.Store{}}}).Register(registry); err != nil {
		t.Fatal(err)
	}
	metadata, _ := registry.Lookup("telemetry.dashboard.draft.create")
	compiled, err := toolruntime.CompileInputSchema(metadata.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"", "short_version", "invented_time_kind"} {
		spec := dashboard.EmptySpec()
		if invalid == "short_version" {
			spec.SchemaVersion = "v1"
		}
		if invalid == "invented_time_kind" {
			spec.DefaultTimeRange.Kind = "last_seconds"
		}
		raw, _ := json.Marshal(map[string]any{"name": "Contract", "description": "", "spec": spec, "proposed_bindings": []any{}})
		var input any
		_ = json.Unmarshal(raw, &input)
		if err := compiled.Validate(input); (err == nil) != (invalid == "") {
			t.Fatalf("authoring schema mismatch for %s: %v", invalid, err)
		}
	}
	doc, err := nativePreviewDocument()
	if err != nil {
		t.Fatal(err)
	}
	step, err := previewSchema(doc, "DashboardStepPolicy", "")
	if err != nil {
		t.Fatal(err)
	}
	properties := step["properties"].(map[string]any)
	description := properties["kind"].(map[string]any)["description"].(string)
	for _, field := range []string{"fixed", "seconds", "auto", "target_points", "min_step_seconds"} {
		if !strings.Contains(description, field) {
			t.Fatalf("range-step constraint %s is undiscoverable", field)
		}
	}
	example, _ := json.Marshal(step["example"])
	var policy dashboard.StepPolicy
	if json.Unmarshal(example, &policy) != nil {
		t.Fatal("step example is not a domain value")
	}
	if _, err := policy.Resolve(time.Unix(0, 0), time.Unix(3600, 0)); err != nil {
		t.Fatal("advertised step example fails the actual compiler")
	}
	if _, ok := registry.Lookup("telemetry.dashboard.draft.validate"); !ok {
		t.Fatal("editor validation is not exposed to creation tools")
	}
}
