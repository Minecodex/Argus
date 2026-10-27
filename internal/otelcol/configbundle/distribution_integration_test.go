package configbundle

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"sigs.k8s.io/yaml"
)

// This gate loads complete rendered targets in the actual locked distribution.
// It does not substitute YAML shape checks for Collector component validation.
// Run in a disposable Linux Kubernetes Pod with the same read-only kubelet CA
// and service-account mounts as the real agent. ARGUS_COLLECTOR_TEST_BINARY
// must point at a freshly built distribution.
func TestRenderedTargetsLoadRealCollectorDistribution(t *testing.T) {
	binary := os.Getenv("ARGUS_COLLECTOR_TEST_BINARY")
	if binary == "" {
		t.Skip("requires freshly built Collector binary")
	}
	for _, scenario := range []struct {
		name, resource, role, target string
		profiles                     []string
	}{
		{"host_control_only", "host", "direct", "host", nil},
		{"host_fixed_metrics", "host", "direct", "host", nil},
		{"cluster_fixed_agent", "kubernetes_cluster", "direct", "kubernetes_agent", nil},
		{"cluster_fixed_gateway", "kubernetes_cluster", "direct", "kubernetes_gateway", nil},
		{"application_ingest", "host", "direct", "host", nil},
		{"host_plugins", "host", "direct", "host", []string{"host-basic", "otlp-receiver", "file-log", "linux-journald", "prometheus-endpoint", "collector-self"}},
		{"host_gateway", "host", "edge_gateway", "host", []string{"host-basic", "otlp-receiver"}},
		{"native_traces", "host", "direct", "host", []string{"otlp-receiver", "jaeger-receiver"}},
		{"cluster_agent", "kubernetes_cluster", "direct", "kubernetes_agent", []string{"k8s-node-container", "k8s-cluster", "k8s-otlp-gateway", "collector-self"}},
		{"cluster_gateway", "kubernetes_cluster", "direct", "kubernetes_gateway", []string{"k8s-node-container", "k8s-cluster", "k8s-otlp-gateway", "collector-self"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			input := RenderInput{CollectorID: uuid.NewString(), ResourceID: uuid.NewString(), SourceGeneration: uuid.NewString(), ConfigRevision: 2,
				ResourceType: scenario.resource, Role: scenario.role, Platform: "linux_amd64", RouteKind: "direct_argus", Transport: "direct", ProfileKeys: scenario.profiles,
				EnrollmentEndpoint: "https://enterprise.example.test/enroll", IngestGRPCEndpoint: "grpcs://ingest.example.test:4317", IngestHTTPEndpoint: "https://ingest.example.test:4318"}
			if scenario.name == "application_ingest" {
				for _, protocol := range []string{"otlp", "jaeger"} {
					input.ApplicationEndpoints = append(input.ApplicationEndpoints, ApplicationEndpoint{ID: uuid.NewString(), ApplicationID: uuid.NewString(), Protocol: protocol, Signals: []string{"traces"}, Version: 1})
				}
			}
			if strings.Contains(scenario.name, "fixed") {
				keys := []string{"collector_metrics", "host_metrics"}
				if scenario.resource == "kubernetes_cluster" {
					keys = []string{"collector_metrics", "kubernetes_node_metrics", "kubernetes_cluster_metrics"}
				}
				for _, key := range keys {
					input.Collections = append(input.Collections, Collection{ID: uuid.NewString(), Integration: key, TargetType: scenario.resource, TargetID: input.ResourceID, Config: json.RawMessage(`{}`), Revision: 1, AssignmentEpoch: 1, Enabled: true})
				}
			}
			bundle, err := Render(input)
			if err != nil {
				t.Fatal(err)
			}
			config, err := Extract(bundle, scenario.target)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "collector.json")
			if err := os.WriteFile(path, config, 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, binary, "validate", "--config=file:"+path)
			command.Env = append(os.Environ(), "K8S_NODE_NAME=fixture-node", "POD_IP=127.0.0.1")
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("actual Collector rejected %s: %v\n%s", scenario.name, err, output)
			}
		})
	}
}

func TestRealCollectorComponentsMatchDistributionRegistry(t *testing.T) {
	binary := os.Getenv("ARGUS_COLLECTOR_TEST_BINARY")
	if binary == "" {
		t.Skip("requires freshly built Collector binary")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(ctx, binary, "components").Output()
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct{ Receivers, Processors, Exporters, Extensions []struct{ Name string } }
	if err := yaml.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, group := range [][]struct{ Name string }{manifest.Receivers, manifest.Processors, manifest.Exporters, manifest.Extensions} {
		for _, component := range group {
			ids[component.Name] = true
		}
	}
	actual := []string{}
	for id := range ids {
		actual = append(actual, id)
	}
	slices.Sort(actual)
	expected := DistributionRuntimeComponents(runtime.GOOS + "_" + runtime.GOARCH)
	if !slices.Equal(actual, expected) {
		t.Fatalf("binary and advertised components differ: %v vs %v", actual, expected)
	}
}
