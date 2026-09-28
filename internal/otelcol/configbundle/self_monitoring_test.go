package configbundle

import (
	"testing"

	"github.com/google/uuid"
)

func TestSelfMonitoringRegistersOnlyRealInternalMetricsSources(t *testing.T) {
	for _, resource := range []string{"host", "kubernetes_cluster"} {
		input := RenderInput{CollectorID: uuid.NewString(), ResourceID: uuid.NewString(), SourceGeneration: uuid.NewString(), ConfigRevision: 1, ResourceType: resource, Role: "direct", Platform: "linux_amd64", RouteKind: "direct_argus", Transport: "direct", ProfileKeys: []string{"collector-self"}, EnrollmentEndpoint: "https://example.test/enroll", IngestGRPCEndpoint: "grpcs://example.test:4317", IngestHTTPEndpoint: "https://example.test:4318"}
		bundle, err := Render(input)
		if err != nil {
			t.Fatal(err)
		}
		sources, err := Sources(bundle)
		if err != nil {
			t.Fatal(err)
		}
		expected := 1
		targets := []string{"host"}
		if resource == "kubernetes_cluster" {
			expected = 2
			targets = []string{"kubernetes_agent", "kubernetes_gateway"}
		}
		if len(sources) != expected {
			t.Fatalf("self profile created unexpected sources: %+v", sources)
		}
		for _, source := range sources {
			if source.Type != "prometheus" || len(source.Signals) != 1 || source.Signals[0] != "metrics" {
				t.Fatalf("self profile mislabeled host metrics or logs: %+v", source)
			}
		}
		for _, target := range targets {
			config := decodeTarget(t, bundle, target)
			receiver := nestedMap(t, config, "receivers", "prometheus/collector_self", "config")
			job := receiver["scrape_configs"].([]any)[0].(map[string]any)
			if job["scrape_interval"] != "30s" {
				t.Fatal("unbounded self scrape interval")
			}
			reader := nestedMap(t, config, "service", "telemetry", "metrics")["readers"].([]any)[0].(map[string]any)
			if nestedMap(t, reader, "pull", "exporter", "prometheus")["host"] != "127.0.0.1" {
				t.Fatal("self metrics listener exposed beyond loopback")
			}
		}
	}
}
