package sdkacceptance

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaegertracing/jaeger-idl/model/v1"
	jaeger "github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	tracesrpc "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	_ "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	agent "skywalking.apache.org/repo/goapi/collect/language/agent/v3"
)

type capture struct {
	tracesrpc.UnimplementedTraceServiceServer
	mu      sync.Mutex
	sources map[string]string
}

func (c *capture) Export(_ context.Context, value *tracesrpc.ExportTraceServiceRequest) (*tracesrpc.ExportTraceServiceResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, r := range value.ResourceSpans {
		attrs := map[string]string{}
		for _, a := range r.Resource.GetAttributes() {
			attrs[a.Key] = a.Value.GetStringValue()
		}
		if attrs["argus.application.key.id"] == "" || attrs["argus.source.id"] == "" || attrs["argus.resource.id"] != "" {
			return nil, fmt.Errorf("application source identity missing or forged ownership survived")
		}
		c.sources[attrs["service.name"]] = attrs["argus.source.id"] + ":" + attrs["argus.application.key.id"]
	}
	return &tracesrpc.ExportTraceServiceResponse{}, nil
}

// Native protocol/authentication gate only: the control authority and OTLP sink
// are test endpoints. PostgreSQL discovery and full Kafka/platform gates remain
// separate. OTLP uses the actual Go SDK; Jaeger/SkyWalking use their native RPC clients.
func TestNativeSDKProtocolAuthentication(t *testing.T) {
	binary := os.Getenv("ARGUS_SDK_COLLECTOR_BINARY")
	if binary == "" {
		t.Skip("requires built Linux Collector and Docker")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	collector, rootID, generation := uuid.NewString(), uuid.NewString(), uuid.NewString()
	directory := t.TempDir()
	pool, certificate := fixtureCertificates(t, directory, collector)
	tlsServer := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool}
	type keyBinding struct {
		key, endpoint, protocol string
		revoked                 bool
		version                 string
	}
	var mutex sync.Mutex
	unavailable := false
	bindings := map[string]*keyBinding{}
	endpoints := []configbundle.ApplicationEndpoint{}
	for _, protocol := range []string{"otlp", "otlp", "jaeger", "skywalking"} {
		endpoint, key := uuid.NewString(), uuid.NewString()
		token := "argus_app." + key + "." + strings.Repeat("x", 43)
		bindings[token] = &keyBinding{key: key, endpoint: endpoint, protocol: protocol, version: "1"}
		endpoints = append(endpoints, configbundle.ApplicationEndpoint{ID: endpoint, ApplicationID: uuid.NewString(), Protocol: protocol, Signals: []string{"traces"}, Version: 1})
	}
	authority := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/identity/trust-bundle" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path != "/v1/application-ingest/authorize" {
			http.NotFound(w, r)
			return
		}
		var input struct {
			Protocol string `json:"protocol"`
			Key      string `json:"key"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			w.WriteHeader(400)
			return
		}
		mutex.Lock()
		defer mutex.Unlock()
		if unavailable {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		binding := bindings[input.Key]
		if binding == nil || binding.revoked || binding.protocol != input.Protocol {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"key_id": binding.key, "endpoint_id": binding.endpoint, "endpoint_version": binding.version, "signals": "traces"})
	}))
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	authority.Listener.Close()
	authority.Listener = listener
	authority.TLS = tlsServer
	authority.StartTLS()
	defer authority.Close()
	authorityAddress := "https://host.docker.internal:" + fmt.Sprint(listener.Addr().(*net.TCPAddr).Port)
	sink := &capture{sources: map[string]string{}}
	grpcListener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsServer)))
	tracesrpc.RegisterTraceServiceServer(server, sink)
	go server.Serve(grpcListener)
	defer server.Stop()
	input := configbundle.RenderInput{CollectorID: collector, ResourceID: rootID, ResourceType: "host", Role: "leaf", SourceGeneration: generation, ConfigRevision: 1, Platform: "linux_amd64", RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: authorityAddress + "/v1/identity/enroll", IngestHTTPEndpoint: authorityAddress, IngestGRPCEndpoint: fmt.Sprintf("grpcs://host.docker.internal:%d", grpcListener.Addr().(*net.TCPAddr).Port), ApplicationEndpoints: endpoints}
	bundle, err := configbundle.Render(input)
	if err != nil {
		t.Fatal(err)
	}
	config, err := configbundle.Extract(bundle, "host")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, "config.json"), config, 0600); err != nil {
		t.Fatal(err)
	}
	owner := "plan8-sdk-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	docker := func(args ...string) (string, error) {
		command := exec.CommandContext(ctx, "docker", args...)
		out, e := command.CombinedOutput()
		return strings.TrimSpace(string(out)), e
	}
	out, err := docker("run", "--detach", "--rm", "--name", owner, "--label", "argus.owner="+owner, "--memory=384m", "--cpus=1", "--publish", "127.0.0.1::24318", "--publish", "127.0.0.1::24250", "--publish", "127.0.0.1::21800", "--mount", "type=bind,source="+binary+",target=/argus,readonly", "--mount", "type=bind,source="+directory+",target=/etc/argus-otelcol", "--mount", "type=bind,source="+directory+",target=/var/lib/argus-otelcol/identity", "alpine:3.21", "/argus", "--config=/etc/argus-otelcol/config.json")
	if err != nil {
		t.Fatalf("start native Collector: %v %s", err, out)
	}
	t.Cleanup(func() {
		cleanup, c := context.WithTimeout(context.Background(), 20*time.Second)
		defer c()
		label, _ := exec.CommandContext(cleanup, "docker", "inspect", "--format", `{{index .Config.Labels "argus.owner"}}`, owner).Output()
		if strings.TrimSpace(string(label)) == owner {
			_ = exec.CommandContext(cleanup, "docker", "rm", "--force", owner).Run()
		}
	})
	port := func(number int) string {
		t.Helper()
		raw, e := docker("port", owner, fmt.Sprintf("%d/tcp", number))
		if e != nil {
			t.Fatal(e)
		}
		return strings.TrimSpace(raw)
	}
	address := port(24318)
	clientTLS := &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, ServerName: "collector-" + collector + ".argus.telemetry"}
	httpClient := &http.Client{Timeout: time.Second, Transport: &http.Transport{TLSClientConfig: clientTLS}}
	deadline := time.Now().Add(15 * time.Second)
	for {
		response, e := httpClient.Get("https://" + address + "/v1/traces")
		if e == nil {
			response.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			logs, _ := docker("logs", owner)
			t.Fatalf("native Collector not ready: %s", logs)
		}
		time.Sleep(100 * time.Millisecond)
	}
	nameIndex := 0
	expected := map[string]string{}
	for token, binding := range bindings {
		nameIndex++
		name := fmt.Sprintf("%s-service-%d", binding.protocol, nameIndex)
		source := uuid.NewSHA1(uuid.MustParse(generation), []byte("application/"+binding.endpoint)).String()
		expected[name] = source + ":" + binding.key
		switch binding.protocol {
		case "otlp":
			exporter, e := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(address), otlptracehttp.WithTLSClientConfig(clientTLS), otlptracehttp.WithHeaders(map[string]string{"Authorization": "Bearer " + token}))
			if e != nil {
				t.Fatal(e)
			}
			provider := trace.NewTracerProvider(trace.WithSyncer(exporter), trace.WithResource(resource.NewSchemaless(attribute.String("service.name", name), attribute.String("argus.resource.id", "forged"))))
			_, span := provider.Tracer("plan8.actual-go-sdk").Start(ctx, "request")
			span.End()
			if e = provider.Shutdown(ctx); e != nil {
				t.Fatal(e)
			}
		case "jaeger":
			connection, e := grpc.NewClient(port(24250), grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)))
			if e != nil {
				t.Fatal(e)
			}
			_, e = jaeger.NewCollectorServiceClient(connection).PostSpans(metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token)), &jaeger.PostSpansRequest{Batch: model.Batch{Process: &model.Process{ServiceName: name}, Spans: []*model.Span{{TraceID: model.TraceID{High: 1, Low: 2}, SpanID: 3, OperationName: "request", StartTime: time.Now(), Duration: time.Millisecond}}}})
			connection.Close()
			if e != nil {
				t.Fatal(e)
			}
		case "skywalking":
			connection, e := grpc.NewClient(port(21800), grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)))
			if e != nil {
				t.Fatal(e)
			}
			client := agent.NewTraceSegmentReportServiceClient(connection)
			stream, e := client.Collect(metadata.NewOutgoingContext(ctx, metadata.Pairs("authentication", token)))
			if e != nil {
				t.Fatal(e)
			}
			e = stream.Send(&agent.SegmentObject{TraceId: "1.2.3", TraceSegmentId: "1.2.4", Service: name, ServiceInstance: "replica-a", Spans: []*agent.SpanObject{{SpanId: 0, ParentSpanId: -1, StartTime: time.Now().UnixMilli(), EndTime: time.Now().Add(time.Millisecond).UnixMilli(), OperationName: "request", SpanType: agent.SpanType_Entry, SpanLayer: agent.SpanLayer_Http}}})
			if e == nil {
				_, e = stream.CloseAndRecv()
			}
			connection.Close()
			if e != nil {
				t.Fatal(e)
			}
		}
	}
	deadline = time.Now().Add(15 * time.Second)
	for {
		sink.mu.Lock()
		count := len(sink.sources)
		sink.mu.Unlock()
		if count == len(expected) {
			break
		}
		if time.Now().After(deadline) {
			logs, _ := docker("logs", owner)
			t.Fatalf("native sources missing: %d/%d %s", count, len(expected), logs)
		}
		time.Sleep(100 * time.Millisecond)
	}
	sink.mu.Lock()
	for name, value := range expected {
		if sink.sources[name] != value {
			t.Errorf("source for %s did not preserve authenticated application", name)
		}
	}
	sink.mu.Unlock()
	for token, binding := range bindings {
		if binding.protocol != "otlp" {
			continue
		}
		span := &tracepb.Span{TraceId: []byte("0123456789abcdef"), SpanId: []byte("12345678"), Name: "auth-recovery", StartTimeUnixNano: uint64(time.Now().UnixNano()), EndTimeUnixNano: uint64(time.Now().Add(time.Millisecond).UnixNano())}
		metadata := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "auth-recovery"}}}}}
		payload, _ := proto.Marshal(&tracesrpc.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{Resource: metadata, ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{span}}}}}})
		post := func(want int) {
			t.Helper()
			request, e := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+address+"/v1/traces", bytes.NewReader(payload))
			if e != nil {
				t.Fatal(e)
			}
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("Content-Type", "application/x-protobuf")
			response, e := httpClient.Do(request)
			if e != nil {
				t.Fatal(e)
			}
			_, _ = io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode != want {
				t.Fatalf("native HTTP auth status: got %d want %d", response.StatusCode, want)
			}
		}
		mutex.Lock()
		unavailable = true
		mutex.Unlock()
		post(http.StatusServiceUnavailable)
		mutex.Lock()
		unavailable = false
		binding.revoked = true
		mutex.Unlock()
		post(http.StatusUnauthorized)
		mutex.Lock()
		binding.revoked = false
		mutex.Unlock()
		post(http.StatusOK)
		break
	}
	// Reproduce the upstream admission failure: a mismatched endpoint version
	// is rejected by the identity processor, but CollectInSync still reports
	// success and prints the raw segment. This is exclusion evidence, not a
	// supported SkyWalking acceptance result.
	for token, binding := range bindings {
		if binding.protocol != "skywalking" {
			continue
		}
		mutex.Lock()
		binding.version = "2"
		mutex.Unlock()
		connection, e := grpc.NewClient(port(21800), grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)))
		if e != nil {
			t.Fatal(e)
		}
		_, e = agent.NewTraceSegmentReportServiceClient(connection).CollectInSync(metadata.NewOutgoingContext(ctx, metadata.Pairs("authentication", token)), &agent.SegmentCollection{Segments: []*agent.SegmentObject{{TraceId: "1.2.8", TraceSegmentId: "1.2.9", Service: "plan8-rejected-sensitive-canary", ServiceInstance: "replica-a", Spans: []*agent.SpanObject{{SpanId: 0, ParentSpanId: -1, StartTime: time.Now().UnixMilli(), EndTime: time.Now().Add(time.Millisecond).UnixMilli(), OperationName: strings.Repeat("plan8-rejected-sensitive-canary-", 1024), SpanType: agent.SpanType_Entry}}}}})
		connection.Close()
		if e != nil {
			t.Fatalf("upstream behavior changed; re-evaluate SkyWalking admission: %v", e)
		}
	}
	logs, e := docker("logs", owner)
	if e != nil {
		t.Fatal(e)
	}
	for token := range bindings {
		if strings.Contains(logs, token) {
			t.Fatal("SDK key leaked into Collector logs")
		}
	}
	if !strings.Contains(logs, "plan8-rejected-sensitive-canary") {
		t.Fatal("expected upstream raw-payload logging behavior changed; re-evaluate SkyWalking admission")
	}
	sink.mu.Lock()
	_, accepted := sink.sources["plan8-rejected-sensitive-canary"]
	sink.mu.Unlock()
	if accepted {
		t.Fatal("rejected endpoint version reached the exporter")
	}
	deadline = time.Now().Add(10 * time.Second)
	for {
		sink.mu.Lock()
		_, recovered := sink.sources["auth-recovery"]
		sink.mu.Unlock()
		if recovered {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("restored authentication did not resume actual export")
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Log("OTLP HTTP auth outage returned 503, revoked key returned 401, and restoration resumed actual export")
	t.Log("SkyWalking admission FAILED: rejected CollectInSync returned success and printed raw payload; not eligible for product enablement")
	t.Log("actual Go OTLP SDK, native Jaeger and SkyWalking RPCs preserved four application source bindings")
}
