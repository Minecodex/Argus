package collectormanager_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	metricsrpc "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type builtinMetricSink struct {
	metricsrpc.UnimplementedMetricsServiceServer
	mu       sync.Mutex
	token    string
	samples  map[string]map[string]string
	requests []*metricsrpc.ExportMetricsServiceRequest
}

func (s *builtinMetricSink) Export(ctx context.Context, in *metricsrpc.ExportMetricsServiceRequest) (*metricsrpc.ExportMetricsServiceResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	if values := md.Get("x-plan8-builtin"); len(values) != 1 || values[0] != s.token {
		return nil, status.Error(codes.Unauthenticated, "test sink authentication")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, resource := range in.ResourceMetrics {
		source := ""
		for _, attr := range resource.Resource.GetAttributes() {
			if attr.Key == "argus.source.id" {
				source = attr.Value.GetStringValue()
			}
		}
		if source == "" {
			return nil, status.Error(codes.InvalidArgument, "native source missing")
		}
		if s.samples[source] == nil {
			s.samples[source] = map[string]string{}
		}
		for _, scope := range resource.ScopeMetrics {
			for _, metric := range scope.Metrics {
				for _, point := range metric.GetHistogram().GetDataPoints() {
					if math.IsNaN(point.GetSum()) || math.IsInf(point.GetSum(), 0) {
						return nil, status.Error(codes.InvalidArgument, "non-finite histogram")
					}
					s.samples[source][metric.Name] = metric.Unit
				}
				for _, point := range append(metric.GetGauge().GetDataPoints(), metric.GetSum().GetDataPoints()...) {
					value := point.GetAsDouble()
					if point.GetAsInt() != 0 {
						value = float64(point.GetAsInt())
					}
					if math.IsNaN(value) || math.IsInf(value, 0) {
						return nil, status.Error(codes.InvalidArgument, "non-finite native sample")
					}
					s.samples[source][metric.Name] = metric.Unit
				}
			}
		}
	}
	s.requests = append(s.requests, proto.Clone(in).(*metricsrpc.ExportMetricsServiceRequest))
	return &metricsrpc.ExportMetricsServiceResponse{}, nil
}
func (s *builtinMetricSink) snapshot() map[string]map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]map[string]string{}
	for id, metrics := range s.samples {
		out[id] = map[string]string{}
		for key, unit := range metrics {
			out[id][key] = unit
		}
	}
	return out
}
func (s *builtinMetricSink) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.samples = map[string]map[string]string{}
}

func (s *builtinMetricSink) requestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

func (s *builtinMetricSink) gaugeAfter(source, name string, start int) (float64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.requests) - 1; i >= start; i-- {
		for _, resource := range s.requests[i].ResourceMetrics {
			id := ""
			for _, attr := range resource.Resource.GetAttributes() {
				if attr.Key == "argus.source.id" {
					id = attr.Value.GetStringValue()
				}
			}
			if id != source {
				continue
			}
			for _, scope := range resource.ScopeMetrics {
				for _, metric := range scope.Metrics {
					if metric.Name != name {
						continue
					}
					for _, point := range metric.GetGauge().GetDataPoints() {
						return point.GetAsDouble() + float64(point.GetAsInt()), true
					}
				}
			}
		}
	}
	return 0, false
}

func TestNativeOwnHostAndCollectorMetricsHaveIndependentLifecycle(t *testing.T) {
	if os.Getenv("ARGUS_MONITORING_NATIVE_TARGETS") != "1" {
		t.Skip("requires Docker and locked native Collector")
	}
	binary, err := filepath.Abs(os.Getenv("ARGUS_MONITORING_COLLECTOR_BINARY"))
	if err != nil {
		t.Fatal(err)
	}
	d := newPipelineDocker(t)
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	sink := &builtinMetricSink{token: uuid.NewString(), samples: map[string]map[string]string{}}
	server := grpc.NewServer()
	metricsrpc.RegisterMetricsServiceServer(server, sink)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	host, collector, generation := uuid.NewString(), uuid.NewString(), uuid.NewString()
	collections := []configbundle.Collection{}
	for _, key := range []string{"host_metrics", "collector_metrics"} {
		collections = append(collections, configbundle.Collection{ID: uuid.NewString(), Integration: key, TargetType: "host", TargetID: host, Config: json.RawMessage(`{"interval":"5s"}`), Revision: 1, AssignmentEpoch: 1, Enabled: true})
	}
	run := func(name string, revision int64) (string, []configbundle.Source) {
		t.Helper()
		raw, err := configbundle.Render(configbundle.RenderInput{CollectorID: collector, ResourceID: host, ResourceType: "host", Role: "direct", Platform: "linux_amd64", SourceGeneration: generation, ConfigRevision: revision, RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: "https://control.test/enroll", IngestGRPCEndpoint: "https://ingest.test:4317", IngestHTTPEndpoint: "https://ingest.test", Collections: collections})
		if err != nil {
			t.Fatal(err)
		}
		sources, _ := configbundle.Sources(raw)
		public, _ := configbundle.Extract(raw, "host")
		var config map[string]any
		_ = json.Unmarshal(public, &config)
		config["extensions"] = map[string]any{"file_storage": map[string]any{"directory": "/tmp/storage", "create_directory": true}, "health_check": map[string]any{"endpoint": "0.0.0.0:13133"}}
		config["service"].(map[string]any)["extensions"] = []string{"file_storage", "health_check"}
		config["exporters"].(map[string]any)["otlp/argus"] = map[string]any{"endpoint": fmt.Sprintf("host.docker.internal:%d", listener.Addr().(*net.TCPAddr).Port), "tls": map[string]any{"insecure": true}, "headers": map[string]string{"x-plan8-builtin": sink.token}}
		value, _ := json.Marshal(config)
		path := d.file(name+".json", value)
		return d.start(name, "--memory=512m", "--cpus=1", "--read-only", "--tmpfs", "/tmp", "--cap-drop=ALL", "--mount", "type=bind,source="+binary+",target=/argus,readonly", "--mount", "type=bind,source="+path+",target=/config.json,readonly", "--entrypoint", "/argus", "alpine:3.21", "--config=/config.json"), sources
	}
	wait := func(count int, container string) map[string]map[string]string {
		t.Helper()
		deadline := time.Now().Add(25 * time.Second)
		for time.Now().Before(deadline) {
			value := sink.snapshot()
			if len(value) == count {
				return value
			}
			time.Sleep(200 * time.Millisecond)
		}
		logs, _ := d.command("logs", "--tail", "15", container)
		t.Fatalf("native own metrics: got %d sources want %d: %s", len(sink.snapshot()), count, logs)
		return nil
	}
	first, sources := run("both", 1)
	samples := wait(2, first)
	time.Sleep(6 * time.Second)
	sink.mu.Lock()
	payloads := make([][]byte, len(sink.requests))
	for i, request := range sink.requests {
		payloads[i], _ = proto.Marshal(request)
	}
	sink.mu.Unlock()
	for _, source := range sources {
		if source.Type == "hostmetrics" {
			for _, metric := range []string{"system.cpu.time", "system.memory.usage", "system.filesystem.usage", "system.network.io"} {
				if _, ok := samples[source.ID][metric]; !ok {
					t.Fatalf("native host metric missing: %s", metric)
				}
			}
		}
		if source.CollectionID == "" {
			t.Fatal("fixed metrics lack their own collection responsibility")
		}
	}
	d.must("stop", "--time", "3", first)
	sink.reset()
	collections[0].Enabled = false
	collections[0].Revision = 2
	second, remaining := run("self-only", 2)
	wait(1, second)
	if len(remaining) != 1 || remaining[0].Type != "collector_self" {
		t.Fatal("host toggle stopped or relabeled Collector metrics")
	}
	d.must("stop", "--time", "3", second)
	sink.reset()
	collections[1].Enabled = false
	collections[1].Revision = 2
	third, remaining := run("no-metrics", 3)
	time.Sleep(2 * time.Second)
	if len(remaining) != 0 || len(sink.snapshot()) != 0 || d.must("inspect", "--format", "{{.State.Running}}", third) != "true" {
		t.Fatal("all metrics disabled stopped control runtime or produced implicit data")
	}
	if path := os.Getenv("ARGUS_BUILTIN_METRIC_EVIDENCE"); path != "" {
		output, _ := json.MarshalIndent(map[string]any{"scope": "native_linux_container_not_formal_host_enrollment", "collector_version": configbundle.CollectorVersion, "captured_at": time.Now().UTC(), "collector_id": collector, "host_id": host, "sources": sources, "metric_names_and_units": samples, "requests": payloads}, "", "  ")
		if err = os.WriteFile(path, output, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("native own metrics: host and self sources=%d; self-only=1; all-off=0 with process running; owner=%s", len(samples), strings.TrimSpace(d.owner))
}
