package argusdev

import (
	"encoding/json"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/monitoring"
	"path/filepath"
	"testing"
)

func TestPlan8ServiceUpdateUsesCurrentPublicRequestContract(t *testing.T) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	document, err := loader.LoadFromFile(filepath.Join("..", "..", "..", "api", "openapi", "generated", "monitoringapi.bundle.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	schema := document.Components.Schemas["MonitoringMutation"].Value
	app, service := uuid.New(), uuid.New()
	for _, associated := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			row := monitoring.Collection{ID: uuid.New(), Name: "Actual database", Integration: "mysql", Target: monitoring.ResourceRef{Kind: "monitoring_target", ID: uuid.New()}, Config: json.RawMessage(`{}`), Version: 2}
			if associated {
				row.ApplicationID = &app
				row.ServiceID = &service
			}
			raw, err := json.Marshal(plan8ServiceUpdateBody(row, uuid.New(), enabled))
			if err != nil {
				t.Fatal(err)
			}
			var wire any
			if err = json.Unmarshal(raw, &wire); err != nil {
				t.Fatal(err)
			}
			if err = schema.VisitJSON(wire); err != nil {
				t.Fatalf("public service update violates current API: %v", err)
			}
			wire.(map[string]any)["collection"].(map[string]any)["credential_draft_id"] = uuid.NewString()
			if err = schema.VisitJSON(wire); err == nil {
				t.Fatal("public contract admitted a private credential draft reference")
			}
		}
	}
}

func TestPlan8BusinessLogUpdateUsesWritableConfigAndMinimalToggle(t *testing.T) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	document, err := loader.LoadFromFile(filepath.Join("..", "..", "..", "api", "openapi", "generated", "monitoringapi.bundle.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	namespace, application, service := uuid.New(), uuid.New(), uuid.New()
	config, _ := json.Marshal(map[string]any{"scope": "business", "workload": map[string]any{"namespace_id": namespace, "namespace": "observed-name", "namespace_uid": "attested-uid", "ancestors": []string{"attested-owner"}, "type": "deployment", "name": "orders", "uid": "deployment-uid", "containers": []string{"http"}}})
	row := monitoring.Collection{ID: uuid.New(), Name: "Business logs", Integration: "k8slog", Target: monitoring.ResourceRef{Kind: "application", ID: application}, ApplicationID: &application, ServiceID: &service, CollectorID: uuid.New(), Config: config, Version: 1, DesiredEnabled: true}
	raw, err := json.Marshal(plan8ServiceUpdateBody(row, row.CollectorID, false))
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err = json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	schema := document.Components.Schemas["MonitoringMutation"].Value
	if err = schema.VisitJSON(wire); err != nil {
		t.Fatalf("business application target was not accepted: %v", err)
	}
	w := wire["collection"].(map[string]any)["config"].(map[string]any)["workload"].(map[string]any)
	for _, key := range []string{"namespace", "namespace_uid", "ancestors"} {
		if _, exists := w[key]; exists {
			t.Fatalf("request copied server-only %s", key)
		}
		w[key] = "forged"
		if err = schema.VisitJSON(wire); err == nil {
			t.Fatalf("public input admitted server-only %s", key)
		}
		delete(w, key)
	}
	if w["namespace_id"] != namespace.String() || w["uid"] != "deployment-uid" {
		t.Fatal("projection removed user-selected stable identity")
	}
	toggle := plan8ServiceEnabledBody(row, false)
	if toggle["operation"] != "set_enabled" || len(toggle["collection"].(map[string]any)) != 1 {
		t.Fatal("toggle copied configuration instead of using the state transition")
	}
	if row.Config == nil || !document.Components.Schemas["MonitoringLogWorkload"].Value.Properties["namespace_uid"].Value.ReadOnly {
		t.Fatal("server observation metadata was removed or made writable")
	}
}
