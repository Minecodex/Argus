package selfmonitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/grpc"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestHTTPToGRPCPreservesTraceAndDoesNotExportPrivateRequestData(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	prior := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()); otel.SetTracerProvider(prior) })
	router := chi.NewRouter()
	router.Use(HTTP)
	router.Get("/objects/{id}", func(w http.ResponseWriter, r *http.Request) {
		err := UnaryClient(r.Context(), "/argus.telemetry.Query/Execute", nil, nil, nil, func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			md, _ := metadata.FromOutgoingContext(ctx)
			_, e := UnaryServer(metadata.NewIncomingContext(context.Background(), md), nil, &grpc.UnaryServerInfo{FullMethod: "/argus.telemetry.Query/Execute"}, func(context.Context, any) (any, error) {
				return nil, status.Error(grpcCodes.Unavailable, "private-query-text")
			})
			return e
		})
		if err != nil {
			w.WriteHeader(503)
		}
	})
	r := httptest.NewRequest("GET", "/objects/private-object?password=secret", nil)
	r.Header.Set("Authorization", "Bearer secret")
	router.ServeHTTP(httptest.NewRecorder(), r)
	spans := recorder.Ended()
	if len(spans) != 3 {
		t.Fatalf("want HTTP/client/server spans, got %d", len(spans))
	}
	for _, span := range spans {
		if span.SpanContext().TraceID() != spans[0].SpanContext().TraceID() || span.Status().Code != codes.Error {
			t.Fatal("trace propagation/status lost")
		}
		if strings.Contains(span.Name(), "private") || strings.Contains(span.Status().Description, "private") {
			t.Fatal("private request data recorded")
		}
		for _, a := range span.Attributes() {
			if strings.Contains(a.Value.AsString(), "secret") || strings.Contains(a.Value.AsString(), "private") {
				t.Fatal("private attribute recorded")
			}
		}
	}
	if spans[0].Parent().SpanID() != spans[1].SpanContext().SpanID() || spans[1].Parent().SpanID() != spans[2].SpanContext().SpanID() || spans[2].Name() != "GET /objects/{id}" {
		t.Fatal("HTTP/RPC hierarchy or route template lost")
	}
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/healthz", nil))
	if len(recorder.Ended()) != 3 {
		t.Fatal("health requests generate recursive/noisy telemetry")
	}
}

func TestSelfTracingRejectsInvalidConfiguration(t *testing.T) {
	for _, endpoint := range []string{"bad", "https://user:pass@collector", "http://collector?token=secret"} {
		t.Setenv("ARGUS_SELF_TRACE_ENDPOINT", endpoint)
		if _, err := Start(context.Background(), "test", nil); err == nil {
			t.Fatal("invalid endpoint accepted")
		}
	}
	t.Setenv("ARGUS_SELF_TRACE_ENDPOINT", "http://127.0.0.1:4318")
	t.Setenv("ARGUS_SELF_TRACE_SAMPLE_RATIO", "NaN")
	if _, err := Start(context.Background(), "test", nil); err == nil {
		t.Fatal("invalid sample ratio accepted")
	}
	t.Setenv("ARGUS_SELF_TRACE_ENDPOINT", "")
	stop, err := Start(context.Background(), "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	stop()
}
