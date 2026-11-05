package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	logapi "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type configuration struct {
	Endpoint, Key, CA, ServerName, RedisPassword, Run, Transport string
	AuxEndpoint, AuxKey                                          string
}
type telemetry struct {
	tracer             trace.Tracer
	logger             logapi.Logger
	requests, failures metric.Int64Counter
	active             metric.Int64UpDownCounter
	duration           metric.Float64Histogram
	shutdown           func(context.Context) error
}

func instrument(ctx context.Context, cfg configuration, service string) (*telemetry, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(cfg.CA)) {
		return nil, errors.New("invalid SDK trust bundle")
	}
	secure := &tls.Config{RootCAs: pool, ServerName: cfg.ServerName, MinVersion: tls.VersionTLS13}
	headers := map[string]string{"Authorization": "Bearer " + cfg.Key}
	namespace := "plan9-b07a"
	if cfg.Transport == "grpc" {
		namespace = "plan9-b07b"
	} else if cfg.Transport == "jaeger" {
		namespace = "plan9-b07c"
	}
	res := resource.NewSchemaless(attribute.String("service.name", service), attribute.String("service.namespace", namespace), attribute.String("service.instance.id", cfg.Run+"-"+service), attribute.String("deployment.environment.name", "testing"))
	otel.SetTextMapPropagator(propagation.TraceContext{})
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(error) { println("SDK export failed; inspect protected Collector diagnostics") }))
	te, le, me, closeTransport, err := sdkExporters(ctx, cfg, secure, headers, service, res)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithSampler(sdktrace.AlwaysSample()), sdktrace.WithBatcher(te, sdktrace.WithBatchTimeout(time.Second), sdktrace.WithMaxQueueSize(2048)))
	lp := sdklog.NewLoggerProvider(sdklog.WithResource(res), sdklog.WithProcessor(sdklog.NewBatchProcessor(le, sdklog.WithExportInterval(time.Second))))
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithResource(res), sdkmetric.WithReader(sdkmetric.NewPeriodicReader(me, sdkmetric.WithInterval(10*time.Second))), sdkmetric.WithView(sdkmetric.NewView(sdkmetric.Instrument{Name: "plan9_http_duration_seconds"}, sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: []float64{.01, .05, .1, .25, .5, 1, 2, 5}}})))
	meter := mp.Meter("argus.plan9.real-http")
	requests, err := meter.Int64Counter("plan9_http_requests_total")
	if err != nil {
		return nil, err
	}
	failures, err := meter.Int64Counter("plan9_http_errors_total")
	if err != nil {
		return nil, err
	}
	active, err := meter.Int64UpDownCounter("plan9_http_active_requests")
	if err != nil {
		return nil, err
	}
	duration, err := meter.Float64Histogram("plan9_http_duration_seconds", metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}
	started := time.Now()
	_, err = meter.Float64ObservableGauge("plan9_service_uptime_seconds", metric.WithUnit("s"), metric.WithFloat64Callback(func(ctx context.Context, o metric.Float64Observer) error {
		o.Observe(time.Since(started).Seconds())
		return nil
	}))
	if err != nil {
		return nil, err
	}
	return &telemetry{tracer: tp.Tracer("argus.plan9.real-http"), logger: lp.Logger("argus.plan9.business"), requests: requests, failures: failures, active: active, duration: duration, shutdown: func(c context.Context) error {
		return errors.Join(tp.Shutdown(c), lp.Shutdown(c), mp.Shutdown(c), closeTransport())
	}}, nil
}

func sdkExporters(ctx context.Context, cfg configuration, secure *tls.Config, headers map[string]string, service string, res *resource.Resource) (sdktrace.SpanExporter, sdklog.Exporter, sdkmetric.Exporter, func() error, error) {
	switch cfg.Transport {
	case "jaeger":
		return jaegerExporters(ctx, cfg, secure, headers, service, res)
	case "grpc":
		return grpcExporters(ctx, cfg.Endpoint, secure, headers)
	case "", "http": // Existing B07a configuration files omit Transport.
	default:
		return nil, nil, nil, nil, errors.New("unsupported SDK transport")
	}
	traces, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(cfg.Endpoint), otlptracehttp.WithTLSClientConfig(secure), otlptracehttp.WithHeaders(headers))
	if err != nil {
		return nil, nil, nil, nil, err
	}
	logs, err := otlploghttp.New(ctx, otlploghttp.WithEndpoint(cfg.Endpoint), otlploghttp.WithTLSClientConfig(secure), otlploghttp.WithHeaders(headers))
	if err != nil {
		return nil, nil, nil, nil, errors.Join(err, traces.Shutdown(ctx))
	}
	metrics, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpoint(cfg.Endpoint), otlpmetrichttp.WithTLSClientConfig(secure), otlpmetrichttp.WithHeaders(headers))
	if err != nil {
		return nil, nil, nil, nil, errors.Join(err, traces.Shutdown(ctx), logs.Shutdown(ctx))
	}
	return traces, logs, metrics, func() error { return nil }, nil
}
