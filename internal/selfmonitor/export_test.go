package selfmonitor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	collecttrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestSelfMonitoringExportsRealOTLPAndKeepsIdentityCollectorOwned(t *testing.T) {
	received := make(chan *collecttrace.ExportTraceServiceRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/traces" || r.Method != "POST" {
			t.Errorf("unexpected OTLP endpoint %s %s", r.Method, r.URL.Path)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		var request collecttrace.ExportTraceServiceRequest
		if err = proto.Unmarshal(raw, &request); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		received <- &request
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	defer server.Close()
	t.Setenv("ARGUS_SELF_TRACE_ENDPOINT", server.URL)
	t.Setenv("ARGUS_SELF_TRACE_SAMPLE_RATIO", "1")
	t.Setenv("ARGUS_SELF_TRACE_ENVIRONMENT", "self-test")
	t.Setenv("OTEL_EXPORTER_OTLP_COMPRESSION", "none")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_COMPRESSION", "none")
	prior := otel.GetTracerProvider()
	defer otel.SetTracerProvider(prior)
	stop, err := Start(context.Background(), "argus-export-test", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, span := otel.Tracer("argus.http").Start(context.Background(), "observed operation", trace.WithSpanKind(trace.SpanKindServer))
	span.End()
	_, unreviewed := otel.Tracer("").Start(context.Background(), "dependency evaluation", trace.WithAttributes(attribute.String("query", "private query input")))
	unreviewed.End()
	stop()
	select {
	case request := <-received:
		if len(request.ResourceSpans) != 1 || len(request.ResourceSpans[0].ScopeSpans[0].Spans) != 1 {
			t.Fatal("missing OTLP span")
		}
		attrs := map[string]string{}
		for _, a := range request.ResourceSpans[0].Resource.Attributes {
			attrs[a.Key] = a.Value.GetStringValue()
		}
		if attrs["service.name"] != "argus-export-test" || attrs["service.instance.id"] == "" || attrs["deployment.environment.name"] != "self-test" {
			t.Fatalf("service identity missing: %v", attrs)
		}
		for _, key := range []string{"argus.enterprise.id", "argus.resource.id", "argus.source.id"} {
			if _, ok := attrs[key]; ok {
				t.Fatal("SDK supplied Collector-owned identity")
			}
		}
	case <-time.After(time.Second):
		t.Fatal("OTLP export was not received")
	}
}
