package collectormanager_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	logapi "go.opentelemetry.io/otel/log"
	metricapi "go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	traceapi "go.opentelemetry.io/otel/trace"
)

type sdkTraceExport struct {
	sdktrace.SpanExporter
	mu   sync.Mutex
	last error
}

func (e *sdkTraceExport) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	err := e.SpanExporter.ExportSpans(ctx, spans)
	e.mu.Lock()
	e.last = err
	e.mu.Unlock()
	return err
}
func (e *sdkTraceExport) failure() error { e.mu.Lock(); defer e.mu.Unlock(); return e.last }

type sdkLogExport struct {
	sdklog.Exporter
	mu   sync.Mutex
	last error
}

func (e *sdkLogExport) Export(ctx context.Context, records []sdklog.Record) error {
	err := e.Exporter.Export(ctx, records)
	e.mu.Lock()
	e.last = err
	e.mu.Unlock()
	return err
}
func (e *sdkLogExport) failure() error { e.mu.Lock(); defer e.mu.Unlock(); return e.last }

type actualSDKService struct {
	trace        *sdktrace.TracerProvider
	logs         *sdklog.LoggerProvider
	traceExport  *sdkTraceExport
	logExport    *sdkLogExport
	metrics      *sdkmetric.MeterProvider
	metricReader *sdkmetric.ManualReader
	metricExport sdkmetric.Exporter
	counter      metricapi.Int64Counter
}

func newActualSDKService(t *testing.T, f *nativeSDKPipeline, app sdkPipelineApplication, namespace string) *actualSDKService {
	t.Helper()
	res, err := resource.New(f.ctx, resource.WithTelemetrySDK(), resource.WithAttributes(
		attribute.String("service.name", "api"), attribute.String("service.namespace", namespace),
		attribute.String("service.instance.id", "replica-one"),
		// An SDK cannot select either the application or the execution host.
		attribute.String("argus.resource.id", f.host.String()),
		attribute.String("argus.application.id", f.apps[1].app.String()),
	))
	if err != nil {
		t.Fatal(err)
	}
	traceExporter, err := otlptracehttp.New(f.ctx, otlptracehttp.WithEndpoint(f.address), otlptracehttp.WithTLSClientConfig(f.sdkTLS()), otlptracehttp.WithHeaders(map[string]string{"Authorization": "Bearer " + app.token}))
	if err != nil {
		t.Fatal(err)
	}
	logExporter, err := otlploghttp.New(f.ctx, otlploghttp.WithEndpoint(f.address), otlploghttp.WithTLSClientConfig(f.sdkTLS()), otlploghttp.WithHeaders(map[string]string{"Authorization": "Bearer " + app.token}))
	if err != nil {
		t.Fatal(err)
	}
	service := &actualSDKService{traceExport: &sdkTraceExport{SpanExporter: traceExporter}, logExport: &sdkLogExport{Exporter: logExporter}}
	service.trace = sdktrace.NewTracerProvider(sdktrace.WithSyncer(service.traceExport), sdktrace.WithResource(res), sdktrace.WithSampler(sdktrace.AlwaysSample()))
	service.logs = sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(service.logExport)), sdklog.WithResource(res))
	service.metricReader = sdkmetric.NewManualReader()
	service.metrics = sdkmetric.NewMeterProvider(sdkmetric.WithReader(service.metricReader), sdkmetric.WithResource(res))
	service.metricExport, err = otlpmetrichttp.New(f.ctx, otlpmetrichttp.WithEndpoint(f.address), otlpmetrichttp.WithTLSClientConfig(f.sdkTLS()), otlpmetrichttp.WithHeaders(map[string]string{"Authorization": "Bearer " + app.token}))
	if err != nil {
		t.Fatal(err)
	}
	service.counter, err = service.metrics.Meter("plan8.actual-go-metric-sdk").Int64Counter("plan8_sdk_request_count")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := service.trace.Shutdown(ctx); err != nil {
			t.Error(err)
		}
		if err := service.logs.Shutdown(ctx); err != nil {
			t.Error(err)
		}
		if err := service.metrics.Shutdown(ctx); err != nil {
			t.Error(err)
		}
		if err := service.metricExport.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	return service
}

func (s *actualSDKService) log(ctx context.Context, message, private string) {
	var record logapi.Record
	record.SetTimestamp(time.Now())
	record.SetSeverity(logapi.SeverityInfo)
	record.SetBody(logapi.StringValue(message))
	record.AddAttributes(logapi.String("password", private))
	s.logs.Logger("plan8.actual-go-log-sdk").Emit(ctx, record)
	s.counter.Add(ctx, 1, metricapi.WithAttributes(attribute.String("password", private)))
}

func (s *actualSDKService) flushMetrics(ctx context.Context) error {
	var data metricdata.ResourceMetrics
	if err := s.metricReader.Collect(ctx, &data); err != nil {
		return err
	}
	return s.metricExport.Export(ctx, &data)
}
func (s *actualSDKService) start(ctx context.Context, operation string, kind traceapi.SpanKind, private string) (context.Context, traceapi.Span) {
	return s.trace.Tracer("plan8.actual-go-trace-sdk").Start(ctx, operation, traceapi.WithSpanKind(kind), traceapi.WithAttributes(attribute.String("password", private)))
}
