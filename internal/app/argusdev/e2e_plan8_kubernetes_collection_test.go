package argusdev

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/logcollection"
	"github.com/kakj-go/Argus/internal/monitoring"
)

func TestPlan8KubernetesCollectionCandidateUsesCurrentPublicMutation(t *testing.T) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	doc, err := loader.LoadFromFile(filepath.Join("..", "..", "..", "api", "openapi", "generated", "monitoringapi.bundle.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	app, service, collector := uuid.New(), uuid.New(), uuid.New()
	config := plan8KubernetesLogConfig(uuid.New(), "plan8-native-http", uuid.NewString())
	input := map[string]any{"kind": "collection", "operation": "create", "collection": map[string]any{"name": "Actual Deployment container logs", "integration": "k8slog", "collector_id": collector, "application_id": app, "service_id": service, "enabled": true, "credential_update": "keep", "config": config}}
	raw, _ := json.Marshal(input)
	var wire any
	if err = json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if err = doc.Components.Schemas["MonitoringMutation"].Value.VisitJSON(wire); err != nil {
		t.Fatal(err)
	}
	var mutation monitoring.Mutation
	if err = json.Unmarshal(raw, &mutation); err != nil {
		t.Fatal(err)
	}
	if mutation.Collection == nil || mutation.Collection.ApplicationID != app || mutation.Collection.CollectorID != collector || mutation.Collection.ServiceID != service {
		t.Fatal("typed workload ownership lost")
	}
	var normalized logcollection.Config
	if err = json.Unmarshal(mutation.Collection.Config, &normalized); err != nil {
		t.Fatal(err)
	}
	if normalized.Workload == nil || normalized.Workload.Type != "deployment" || normalized.Workload.NamespaceID == uuid.Nil || normalized.Workload.UID == "" {
		t.Fatal("workload selection lost its stable identities")
	}
	metricInput := map[string]any{"target": monitoring.ResourceRef{Kind: "kubernetes_cluster", ID: uuid.New()}, "key": "kubernetes_cluster_metrics", "enabled": true, "expected_version": int64(0), "config": map[string]string{"interval": "5s"}}
	raw, _ = json.Marshal(metricInput)
	_ = json.Unmarshal(raw, &wire)
	if err = doc.Components.Schemas["MonitoringBuiltinInput"].Value.VisitJSON(wire); err != nil {
		t.Fatal(err)
	}
	telemetryDoc, err := loader.LoadFromFile(filepath.Join("..", "..", "..", "api", "openapi", "generated", "telemetryapi.bundle.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, uninstall := range []bool{false, true} {
		body := m7CollectorPreviewBody(uuid.NewString(), []string{}, "registry.example.test/argus/otelcol:acceptance")
		if uninstall {
			body["expected_version"] = int64(2)
		}
		raw, _ = json.Marshal(body)
		_ = json.Unmarshal(raw, &wire)
		if err = telemetryDoc.Components.Schemas["CollectorPreview"].Value.VisitJSON(wire); err != nil {
			t.Fatal(err)
		}
	}
}
