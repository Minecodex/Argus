// Package selfmonitor instruments Argus itself. It is opt-in and exports to a
// managed local Collector; application attributes never establish tenant/source identity.
package selfmonitor

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func Start(ctx context.Context, service string, logger *slog.Logger) (func(), error) {
	endpoint := strings.TrimSpace(os.Getenv("ARGUS_SELF_TRACE_ENDPOINT"))
	if endpoint == "" {
		return func() {}, nil
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("ARGUS_SELF_TRACE_ENDPOINT must be an HTTP(S) Collector URL without credentials, query or fragment")
	}
	ratio := 0.1
	if raw := os.Getenv("ARGUS_SELF_TRACE_SAMPLE_RATIO"); raw != "" {
		ratio, err = strconv.ParseFloat(raw, 64)
		if err != nil || !(ratio >= 0 && ratio <= 1) {
			return nil, fmt.Errorf("ARGUS_SELF_TRACE_SAMPLE_RATIO must be between 0 and 1")
		}
	}
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
	if err != nil {
		return nil, fmt.Errorf("initialize Argus tracing: %w", err)
	}
	instance, _ := os.Hostname()
	attrs := []attribute.KeyValue{attribute.String("service.name", service), attribute.String("service.instance.id", instance), attribute.String("telemetry.sdk.language", "go")}
	if environment := os.Getenv("ARGUS_SELF_TRACE_ENVIRONMENT"); environment != "" {
		attrs = append(attrs, attribute.String("deployment.environment.name", environment))
	}
	provider := sdktrace.NewTracerProvider(sdktrace.WithResource(resource.NewSchemaless(attrs...)), sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))), sdktrace.WithBatcher(reviewedExporter{exporter}, sdktrace.WithMaxQueueSize(1024), sdktrace.WithMaxExportBatchSize(256), sdktrace.WithBatchTimeout(time.Second), sdktrace.WithExportTimeout(5*time.Second)))
	otel.SetTracerProvider(provider)
	return func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := provider.Shutdown(shutdown); err != nil && logger != nil {
			logger.Warn("self tracing shutdown failed", "error", err)
		}
	}, nil
}

// A global provider can activate dependency instrumentation (for example the
// PromQL evaluator). Export only the reviewed, bounded Argus attributes.
type reviewedExporter struct{ sdktrace.SpanExporter }

func (exporter reviewedExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	accepted := make([]sdktrace.ReadOnlySpan, 0, len(spans))
	for _, span := range spans {
		switch span.InstrumentationScope().Name {
		case "argus.http", "argus.grpc":
			accepted = append(accepted, span)
		}
	}
	if len(accepted) == 0 {
		return nil
	}
	return exporter.SpanExporter.ExportSpans(ctx, accepted)
}
