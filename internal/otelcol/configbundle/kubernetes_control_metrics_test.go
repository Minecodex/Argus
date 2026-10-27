package configbundle

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestControlPlaneCollectionsUseScopedAuthenticatedAPIMetricsAndIndependentSources(t *testing.T) {
	in := RenderInput{CollectorID: uuid.NewString(), ResourceID: uuid.NewString(), ResourceType: "kubernetes_cluster", Role: "direct", Platform: "linux_amd64", SourceGeneration: uuid.NewString(), ConfigRevision: 1, RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: "https://control.test/enroll", IngestGRPCEndpoint: "https://ingest.test:4317", IngestHTTPEndpoint: "https://ingest.test"}
	for key := range controlMetricEndpoints {
		in.Collections = append(in.Collections, Collection{ID: uuid.NewString(), Integration: key, TargetType: "kubernetes_cluster", TargetID: in.ResourceID, Config: json.RawMessage(`{"interval":"5s"}`), Enabled: true, Revision: 1, AssignmentEpoch: 1})
	}
	raw, err := Render(in)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := Sources(raw)
	if err != nil || len(sources) != 3 {
		t.Fatalf("independent control plane sources: %v %v", sources, err)
	}
	for _, source := range sources {
		if !strings.HasPrefix(source.Key, "kubernetes_gateway") || source.ResourceID != in.ResourceID || source.CollectionID == "" {
			t.Fatal("control-plane source escaped gateway/cluster responsibility")
		}
	}
	config := decodeTarget(t, raw, "kubernetes_gateway")
	for _, value := range nestedMap(t, config, "receivers") {
		r, ok := value.(map[string]any)
		if !ok {
			continue
		}
		settings, ok := r["config"].(map[string]any)
		if !ok {
			continue
		}
		for _, raw := range settings["scrape_configs"].([]any) {
			job := raw.(map[string]any)
			if job["scheme"] != "https" {
				t.Fatal("API TLS disabled")
			}
			tls := job["tls_config"].(map[string]any)
			if tls["insecure_skip_verify"] != false {
				t.Fatal("API certificate validation bypassed")
			}
			if job["authorization"].(map[string]any)["credentials_file"] != "/var/run/secrets/kubernetes.io/serviceaccount/token" {
				t.Fatal("separate exporter credentials used")
			}
			discovery := job["kubernetes_sd_configs"].([]any)[0].(map[string]any)
			names := discovery["namespaces"].(map[string]any)["names"].([]any)
			if len(names) != 1 || names[0] != "kube-system" {
				t.Fatal("component discovery widened to arbitrary namespaces")
			}
		}
	}
	for i := range in.Collections {
		in.Collections[i].Enabled = false
	}
	raw, err = Render(in)
	if err != nil {
		t.Fatal(err)
	}
	sources, _ = Sources(raw)
	if len(sources) != 0 {
		t.Fatal("disabled components still produced sources")
	}
}
