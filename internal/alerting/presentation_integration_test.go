package alerting

import (
	"encoding/json"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/telemetry"
	"testing"
)

func TestPostgresToolPresentationResourceNamesAndRevocation(t *testing.T) {
	f := newAlertFixture(t)
	registry := mcp.NewRegistry()
	if err := (Tools{Service: f.s}).Register(registry); err != nil {
		t.Fatal(err)
	}
	if err := (telemetry.Tools{Service: telemetry.Service{Store: f.s.Store}}).Register(registry); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"alert.event.query", "telemetry.promql.query"} {
		metadata, ok := registry.Lookup(tool)
		if !ok {
			t.Fatal("missing tool", tool)
		}
		call := mcp.Call{ToolID: tool, Enterprise: f.a.EnterpriseID.String(), Subject: f.a.SubjectID.String(), SubjectType: "user", Input: map[string]any{"resource_ids": []string{f.host.String()}, "query": "cpu", "from": "2026-10-12T13:40:00Z", "to": "2026-10-12T13:41:00Z"}}
		result := mcp.Result{Structured: map[string]any{"page": map[string]any{"resource_ids": []string{f.host.String(), f.host.String()}, "items": []any{}}}}
		presentation, err := metadata.Present(t.Context(), call, result)
		if err != nil {
			t.Fatal(tool, err)
		}
		if len(presentation.Resources) != 1 || presentation.Resources[0].Type != "host" || presentation.Resources[0].ID != f.host.String() {
			t.Fatalf("%s: unexpected references %#v", tool, presentation.Resources)
		}
		// Public projections normalize slices through JSON. Assert the wire
		// resource/name contract rather than one Go slice representation.
		var names []map[string]any
		raw, marshalErr := json.Marshal(presentation.Data["resources"])
		if marshalErr != nil || json.Unmarshal(raw, &names) != nil || len(names) != 1 || names[0]["name"] != "web-prod-01" {
			t.Fatalf("%s: missing actual name %#v", tool, presentation.Data["resources"])
		}
	}
	f.exec("DELETE FROM data_authorization_grants WHERE subject_id=$1 AND resource_id=$2", f.a.SubjectID, f.host)
	for _, tool := range []string{"alert.event.query", "telemetry.promql.query"} {
		metadata, _ := registry.Lookup(tool)
		call := mcp.Call{ToolID: tool, Enterprise: f.a.EnterpriseID.String(), Subject: f.a.SubjectID.String(), SubjectType: "user", Input: map[string]any{"resource_ids": []string{f.host.String()}}}
		result := mcp.Result{Structured: map[string]any{"page": map[string]any{"resource_ids": []string{f.host.String()}}}}
		if value, err := metadata.Present(t.Context(), call, result); err == nil || value != nil {
			t.Fatalf("%s: revoked resource name leaked: %#v %v", tool, value, err)
		}
	}
}
