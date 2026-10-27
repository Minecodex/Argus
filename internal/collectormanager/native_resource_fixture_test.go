package collectormanager

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	metricsrpc "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	metrics "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/grpc"
)

type nativeResourceFixture struct {
	t                                                  *testing.T
	owner, directory, binary, network, host, collector string
	collections                                        []configbundle.Collection
	credentials                                        map[string]map[string]string
	capture                                            *nativeMetricCapture
	listener                                           net.Listener
}

func (c *nativeMetricCapture) latestValue(target, name string, attributes map[string]string) (float64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var at uint64
	var value float64
	for _, request := range c.requests {
		for _, resource := range request.ResourceMetrics {
			match := false
			for _, a := range resource.Resource.GetAttributes() {
				if a.Key == "argus.resource.id" && a.Value.GetStringValue() == target {
					match = true
				}
			}
			if !match {
				continue
			}
			for _, scope := range resource.ScopeMetrics {
				for _, metric := range scope.Metrics {
					if metric.Name != name {
						continue
					}
					for _, point := range append(metric.GetGauge().GetDataPoints(), metric.GetSum().GetDataPoints()...) {
						matched := true
						for key, want := range attributes {
							found := false
							for _, a := range resource.Resource.GetAttributes() {
								if a.Key == key {
									found = a.Value.GetStringValue() == want
								}
							}
							for _, a := range point.Attributes {
								if a.Key == key {
									found = a.Value.GetStringValue() == want
								}
							}
							matched = matched && found
						}
						if matched && point.TimeUnixNano >= at {
							at = point.TimeUnixNano
							value = point.GetAsDouble()
							if _, ok := point.Value.(*metrics.NumberDataPoint_AsInt); ok {
								value = float64(point.GetAsInt())
							}
						}
					}
				}
			}
		}
	}
	return value, at != 0
}

func newNativeResourceFixture(t *testing.T) *nativeResourceFixture {
	t.Helper()
	if os.Getenv("ARGUS_MONITORING_NATIVE_TARGETS") != "1" {
		t.Skip("requires explicitly owned native targets")
	}
	f := &nativeResourceFixture{t: t, owner: "plan8-resource-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12], directory: t.TempDir(), host: uuid.NewString(), collector: uuid.NewString(), credentials: map[string]map[string]string{}}
	var err error
	f.binary, err = filepath.Abs(os.Getenv("ARGUS_MONITORING_COLLECTOR_BINARY"))
	if err != nil {
		t.Fatal(err)
	}
	f.network = f.must("", "network", "create", "--label", "argus.owner="+f.owner, f.owner)
	t.Cleanup(func() { _, _ = f.command("", "network", "rm", f.network) })
	f.listener, err = net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	f.capture = &nativeMetricCapture{token: uuid.NewString(), targets: map[string]map[string]bool{}}
	server := grpc.NewServer()
	metricsrpc.RegisterMetricsServiceServer(server, f.capture)
	go server.Serve(f.listener)
	t.Cleanup(server.Stop)
	return f
}
func (f *nativeResourceFixture) command(input string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", args...)
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
func (f *nativeResourceFixture) must(input string, args ...string) string {
	f.t.Helper()
	out, err := f.command(input, args...)
	if err != nil {
		f.t.Fatalf("owned resource Docker operation: %v: %s", err, out)
	}
	return out
}
func (f *nativeResourceFixture) write(name string, value []byte) string {
	f.t.Helper()
	path := filepath.Join(f.directory, name)
	if err := os.WriteFile(path, value, 0600); err != nil {
		f.t.Fatal(err)
	}
	return path
}
func (f *nativeResourceFixture) start(name string, args ...string) string {
	f.t.Helper()
	fullName := f.owner + "-" + name
	// Register cleanup before dispatch: an interrupted docker run can have an
	// unknown outcome even when no container ID reached this client.
	f.t.Cleanup(func() {
		if owner, err := f.command("", "inspect", "--format", `{{index .Config.Labels "argus.owner"}}`, fullName); err == nil && owner == f.owner {
			_, _ = f.command("", "rm", "-f", "-v", fullName)
		}
	})
	base := []string{"run", "-d", "--name", fullName, "--label", "argus.owner=" + f.owner, "--network", f.network}
	id := f.must("", append(base, args...)...)
	return id
}
func (f *nativeResourceFixture) add(key, endpoint, database string, credentials map[string]string) {
	id := uuid.NewString()
	identity, _ := json.Marshal(map[string]string{"endpoint": endpoint, "database": database})
	f.collections = append(f.collections, configbundle.Collection{ID: id, Integration: key, TargetID: uuid.NewString(), TargetType: "monitoring_target", Identity: identity, Config: json.RawMessage(`{"interval":"2s","timeout":"2s"}`), Revision: 1, AssignmentEpoch: 1, Enabled: true, HasCredential: len(credentials) > 0})
	if len(credentials) > 0 {
		f.credentials[id] = credentials
	}
}
func (f *nativeResourceFixture) bundle(revision int64) []byte {
	f.t.Helper()
	raw, err := configbundle.Render(configbundle.RenderInput{CollectorID: f.collector, ResourceID: f.host, ResourceType: "host", Role: "leaf", SourceGeneration: uuid.NewSHA1(uuid.NameSpaceOID, []byte(f.owner)).String(), ConfigRevision: revision, Platform: "linux_amd64", RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: "https://control.test/enroll", IngestGRPCEndpoint: "https://ingest.test:4317", IngestHTTPEndpoint: "https://ingest.test", Collections: f.collections})
	if err != nil {
		f.t.Fatal(err)
	}
	return raw
}
func (f *nativeResourceFixture) run(revision int64) string {
	return f.runOptions(revision)
}

func (f *nativeResourceFixture) runOptions(revision int64, options ...string) string {
	f.t.Helper()
	bundle := f.bundle(revision)
	raw, err := configbundle.Extract(bundle, "host")
	if err != nil {
		f.t.Fatal(err)
	}
	var config map[string]any
	if err = json.Unmarshal(raw, &config); err != nil {
		f.t.Fatal(err)
	}
	delete(config, "extensions")
	delete(config["service"].(map[string]any), "extensions")
	exporters := config["exporters"].(map[string]any)
	exporters["otlp/argus"] = map[string]any{"endpoint": fmt.Sprintf("host.docker.internal:%d", f.listener.Addr().(*net.TCPAddr).Port), "tls": map[string]bool{"insecure": true}, "headers": map[string]string{"x-plan8-native-test": f.capture.token}}
	public, _ := json.Marshal(config)
	privateReceivers := map[string]map[string]string{}
	for _, collection := range f.collections {
		if collection.Enabled && collection.HasCredential {
			entry, exists := monitoringcatalog.Lookup(collection.Integration)
			if !exists {
				f.t.Fatal("unknown native resource integration")
			}
			privateReceivers[entry.Component+"/collection_"+strings.ReplaceAll(collection.ID, "-", "")] = f.credentials[collection.ID]
		}
	}
	privateInput, _ := json.Marshal(map[string]any{"receivers": privateReceivers})
	private, err := monitoringPrivateConfig(&connectorv1.CollectorManagementCommand{RenderedConfig: bundle, PrivateMonitoringConfig: privateInput})
	if err != nil {
		f.t.Fatal(err)
	}
	publicPath := f.write(fmt.Sprintf("public-%d.json", revision), public)
	privatePath := f.write(fmt.Sprintf("private-%d.json", revision), private)
	clear(private)
	args := []string{"--memory=512m", "--cpus=1", "--read-only", "--cap-drop=ALL", "--mount", "type=bind,source=" + f.binary + ",target=/argus,readonly", "--mount", "type=bind,source=" + publicPath + ",target=/config.json,readonly", "--mount", "type=bind,source=" + privatePath + ",target=/private.json,readonly", "--entrypoint", "/argus"}
	args = append(args, options...)
	return f.start(fmt.Sprintf("collector-%d", revision), append(args, "alpine:3.21", "--config=/config.json", "--config=/private.json")...)
}

func (f *nativeResourceFixture) waitValue(index int, name string, attributes map[string]string, expected float64) {
	f.t.Helper()
	deadline := time.Now().Add(18 * time.Second)
	for time.Now().Before(deadline) {
		if value, ok := f.capture.latestValue(f.collections[index].TargetID, name, attributes); ok && value == expected {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	f.t.Fatalf("native %s %s did not reach actual value %v", f.collections[index].Integration, name, expected)
}
func (f *nativeResourceFixture) wait(container string, count int) map[string][]string {
	f.t.Helper()
	deadline := time.Now().Add(35 * time.Second)
	for time.Now().Before(deadline) {
		samples := f.capture.snapshot()
		if len(samples) == count {
			return samples
		}
		time.Sleep(200 * time.Millisecond)
	}
	logs, _ := f.command("", "logs", "--tail", "80", container)
	f.t.Fatalf("native resource count %d want %d, metrics=%v: %s", len(f.capture.snapshot()), count, f.capture.snapshot(), logs)
	return nil
}
func (f *nativeResourceFixture) save(path string, revision int64) {
	f.t.Helper()
	if path == "" {
		return
	}
	sources, err := configbundle.Sources(f.bundle(revision))
	if err != nil {
		f.t.Fatal(err)
	}
	raw, err := json.MarshalIndent(map[string]any{"scope": "actual_dedicated_native_receivers_not_platform_enrollment", "collector_id": f.collector, "host_id": f.host, "captured_at": time.Now().UTC(), "sources": sources, "requests": f.capture.payloads()}, "", "  ")
	if err != nil {
		f.t.Fatal(err)
	}
	if err = os.WriteFile(path, raw, 0600); err != nil {
		f.t.Fatal(err)
	}
}
