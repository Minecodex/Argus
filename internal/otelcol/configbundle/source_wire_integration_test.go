package configbundle

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	collectlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectmetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collecttraces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	_ "google.golang.org/grpc/encoding/gzip"
)

type sourceWireEvent struct {
	signal string
	attrs  []*commonpb.KeyValue
}
type metricWireSink struct {
	collectmetrics.UnimplementedMetricsServiceServer
	events chan sourceWireEvent
}
type logWireSink struct {
	collectlogs.UnimplementedLogsServiceServer
	events chan sourceWireEvent
}
type traceWireSink struct {
	collecttraces.UnimplementedTraceServiceServer
	events chan sourceWireEvent
}

func (s metricWireSink) Export(_ context.Context, r *collectmetrics.ExportMetricsServiceRequest) (*collectmetrics.ExportMetricsServiceResponse, error) {
	for _, resource := range r.ResourceMetrics {
		for _, scope := range resource.ScopeMetrics {
			for _, metric := range scope.Metrics {
				if metric.Name == "planv2_source_probe" || strings.HasPrefix(metric.Name, "otelcol_") {
					select {
					case s.events <- sourceWireEvent{"metrics", resource.Resource.Attributes}:
					default:
					}
				}
			}
		}
	}
	return &collectmetrics.ExportMetricsServiceResponse{}, nil
}
func (s logWireSink) Export(_ context.Context, r *collectlogs.ExportLogsServiceRequest) (*collectlogs.ExportLogsServiceResponse, error) {
	for _, resource := range r.ResourceLogs {
		select {
		case s.events <- sourceWireEvent{"logs", resource.Resource.Attributes}:
		default:
		}
	}
	return &collectlogs.ExportLogsServiceResponse{}, nil
}
func (s traceWireSink) Export(_ context.Context, r *collecttraces.ExportTraceServiceRequest) (*collecttraces.ExportTraceServiceResponse, error) {
	for _, resource := range r.ResourceSpans {
		select {
		case s.events <- sourceWireEvent{"traces", resource.Resource.Attributes}:
		default:
		}
	}
	return &collecttraces.ExportTraceServiceResponse{}, nil
}

// Exercises real receiver -> resource stamping -> batch -> OTLP exporter code.
// Only endpoints, TLS/enrollment and persistent queue are replaced by local test
// transport. This is not an Ingest authorization or Kafka durability test.
func TestRealCollectorPreservesReceiverSourcesOnWire(t *testing.T) {
	binary := os.Getenv("ARGUS_COLLECTOR_TEST_BINARY")
	if binary == "" {
		t.Skip("requires freshly built Collector binary")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	events := make(chan sourceWireEvent, 64)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	sink := grpc.NewServer()
	collectmetrics.RegisterMetricsServiceServer(sink, metricWireSink{events: events})
	collectlogs.RegisterLogsServiceServer(sink, logWireSink{events: events})
	collecttraces.RegisterTraceServiceServer(sink, traceWireSink{events: events})
	go func() { _ = sink.Serve(listener) }()
	t.Cleanup(sink.Stop)
	prometheus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = fmt.Fprintln(w, "# TYPE planv2_source_probe gauge\nplanv2_source_probe 7")
	}))
	t.Cleanup(prometheus.Close)
	port, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := port.Addr().String()
	_ = port.Close()
	input := RenderInput{CollectorID: uuid.NewString(), ResourceID: uuid.NewString(), SourceGeneration: uuid.NewString(), ConfigRevision: 2, ResourceType: "host", Role: "direct", Platform: "linux_amd64", RouteKind: "direct_argus", Transport: "direct", ProfileKeys: []string{"otlp-receiver", "prometheus-endpoint", "collector-self"}, EnrollmentEndpoint: "https://example.test/enroll", IngestGRPCEndpoint: "grpcs://example.test:4317", IngestHTTPEndpoint: "https://example.test:4318"}
	bundle, err := Render(input)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := Sources(bundle)
	if err != nil || len(sources) != 3 {
		t.Fatal("expected independent receivers", sources, err)
	}
	ids := map[string]string{}
	for _, source := range sources {
		ids[strings.TrimPrefix(source.Key, "host/")] = source.ID
	}
	config := decodeTarget(t, bundle, "host")
	delete(config, "extensions")
	delete(config["service"].(map[string]any), "extensions")
	config["exporters"] = map[string]any{"otlp/argus": map[string]any{"endpoint": listener.Addr().String(), "tls": map[string]any{"insecure": true}, "sending_queue": map[string]any{"enabled": false}}}
	receivers := config["receivers"].(map[string]any)
	receivers["otlp"] = map[string]any{"protocols": map[string]any{"grpc": map[string]any{"endpoint": address}}}
	receivers["prometheus"] = map[string]any{"config": map[string]any{"scrape_configs": []any{map[string]any{"job_name": "source-probe", "scrape_interval": "1s", "scrape_timeout": "500ms", "static_configs": []any{map[string]any{"targets": []string{strings.TrimPrefix(prometheus.URL, "http://")}}}}}}}
	self := receivers["prometheus/collector_self"].(map[string]any)["config"].(map[string]any)["scrape_configs"].([]any)[0].(map[string]any)
	self["scrape_interval"], self["scrape_timeout"] = "1s", "500ms"
	dir := t.TempDir()
	raw, _ := json.Marshal(config)
	path := filepath.Join(dir, "collector.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	output, err := os.Create(filepath.Join(dir, "collector.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	command := exec.CommandContext(ctx, binary, "--config=file:"+path)
	command.Stdout, command.Stderr = output, output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = command.Wait(); close(done) }()
	t.Cleanup(func() {
		cancel()
		<-done
		if t.Failed() {
			logs, _ := os.ReadFile(output.Name())
			if len(logs) > 16<<10 {
				logs = logs[:16<<10]
			}
			t.Log(string(logs))
		}
	})
	client, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	value := func(s string) *commonpb.AnyValue {
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: s}}
	}
	resource := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{{Key: "argus.source.id", Value: value(uuid.NewString())}, {Key: "argus.source.revision", Value: value("999")}, {Key: "service.name", Value: value("same-service")}}}
	now := uint64(time.Now().UnixNano())
	metricRequest := &collectmetrics.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{Resource: resource, ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: []*metricspb.Metric{{Name: "planv2_source_probe", Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{{TimeUnixNano: now, Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: 3}}}}}}}}}}}}
	// The process becomes ready asynchronously. WaitForReady bounds all retries
	// by this test's context without depending on log text or startup timing.
	if _, err := collectmetrics.NewMetricsServiceClient(client).Export(ctx, metricRequest, grpc.WaitForReady(true)); err != nil {
		t.Fatal(err)
	}
	if _, err := collectlogs.NewLogsServiceClient(client).Export(ctx, &collectlogs.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{Resource: resource, ScopeLogs: []*logspb.ScopeLogs{{LogRecords: []*logspb.LogRecord{{TimeUnixNano: now, Body: value("source probe")}}}}}}}, grpc.WaitForReady(true)); err != nil {
		t.Fatal(err)
	}
	if _, err := collecttraces.NewTraceServiceClient(client).Export(ctx, &collecttraces.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{Resource: resource, ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{{TraceId: []byte("0123456789abcdef"), SpanId: []byte("01234567"), Name: "source probe", StartTimeUnixNano: now, EndTimeUnixNano: now + 1000}}}}}}}, grpc.WaitForReady(true)); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for len(seen) < 5 {
		select {
		case event := <-events:
			attrs := map[string]string{}
			for _, attr := range event.attrs {
				attrs[attr.Key] = attr.Value.GetStringValue()
			}
			id := attrs["argus.source.id"]
			if attrs["argus.source.revision"] != "2" || (id != ids["otlp"] && id != ids["prometheus"] && id != ids["prometheus/collector_self"]) || (event.signal != "metrics" && id != ids["otlp"]) {
				t.Fatalf("receiver source stamp lost or spoofed: %+v %+v", event, attrs)
			}
			seen[event.signal+"/"+id] = true
		case <-done:
			t.Fatal("Collector exited before all receiver signals arrived")
		case <-ctx.Done():
			t.Fatalf("missing source/signal evidence: %v", seen)
		}
	}
}
