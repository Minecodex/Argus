package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"time"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	jaeger "github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	logapi "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	traceapi "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

type input struct {
	Endpoint, Key, CA, ServerName, Protocol, Service, Namespace, Instance, Marker string
	ExpectedAccept                                                                bool
}
type output struct {
	Accepted       bool     `json:"accepted"`
	TraceID        string   `json:"trace_id"`
	Signals        []string `json:"signals"`
	ExpectationMet bool     `json:"expectation_met"`
}

func main() {
	var in input
	if json.NewDecoder(io.LimitReader(os.Stdin, 64<<10)).Decode(&in) != nil || in.Endpoint == "" || in.Key == "" || in.ServerName == "" || in.Service == "" || in.Marker == "" {
		fmt.Fprintln(os.Stderr, "invalid private SDK test input")
		os.Exit(2)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(in.CA)) {
		fmt.Fprintln(os.Stderr, "SDK test trust bundle invalid")
		os.Exit(2)
	}
	tlsConfig := &tls.Config{RootCAs: pool, ServerName: in.ServerName, MinVersion: tls.VersionTLS13}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out := output{Signals: []string{"traces"}}
	if in.Protocol == "jaeger" {
		connection, err := grpc.NewClient(in.Endpoint, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
		if err == nil {
			id := model.TraceID{High: 1, Low: uint64(time.Now().UnixNano())}
			out.TraceID = id.String()
			_, err = jaeger.NewCollectorServiceClient(connection).PostSpans(metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+in.Key)), &jaeger.PostSpansRequest{Batch: model.Batch{Process: &model.Process{ServiceName: in.Service, Tags: []model.KeyValue{{Key: "service.namespace", VType: model.ValueType_STRING, VStr: in.Namespace}, {Key: "service.instance.id", VType: model.ValueType_STRING, VStr: in.Instance}}}, Spans: []*model.Span{{TraceID: id, SpanID: 3, OperationName: in.Marker, StartTime: time.Now(), Duration: time.Millisecond, Tags: []model.KeyValue{{Key: "span.kind", VType: model.ValueType_STRING, VStr: "server"}}}}}})
			connection.Close()
		}
		out.Accepted = err == nil
	} else {
		var failed atomic.Bool
		otel.SetErrorHandler(otel.ErrorHandlerFunc(func(error) { failed.Store(true) }))
		var exporter *otlptrace.Exporter
		var err error
		if in.Protocol == "otlp_grpc" {
			exporter, err = otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(in.Endpoint), otlptracegrpc.WithTLSCredentials(credentials.NewTLS(tlsConfig)), otlptracegrpc.WithHeaders(map[string]string{"Authorization": "Bearer " + in.Key}), otlptracegrpc.WithRetry(otlptracegrpc.RetryConfig{Enabled: false}))
		} else if in.Protocol == "otlp_http" {
			exporter, err = otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(in.Endpoint), otlptracehttp.WithTLSClientConfig(tlsConfig), otlptracehttp.WithHeaders(map[string]string{"Authorization": "Bearer " + in.Key}), otlptracehttp.WithRetry(otlptracehttp.RetryConfig{Enabled: false}))
		} else {
			fmt.Fprintln(os.Stderr, "unsupported SDK test protocol")
			os.Exit(2)
		}
		if err == nil {
			res := resource.NewSchemaless(attribute.String("service.name", in.Service), attribute.String("service.namespace", in.Namespace), attribute.String("service.instance.id", in.Instance))
			provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter), sdktrace.WithResource(res))
			spanCtx, span := provider.Tracer("argus.plan8.actual-sdk").Start(ctx, in.Marker, traceapi.WithSpanKind(traceapi.SpanKindServer))
			out.TraceID = span.SpanContext().TraceID().String()
			if in.Protocol == "otlp_http" {
				out.Signals = []string{"traces", "logs", "metrics"}
				logs, e := otlploghttp.New(ctx, otlploghttp.WithEndpoint(in.Endpoint), otlploghttp.WithTLSClientConfig(tlsConfig), otlploghttp.WithHeaders(map[string]string{"Authorization": "Bearer " + in.Key}), otlploghttp.WithRetry(otlploghttp.RetryConfig{Enabled: false}))
				if e != nil {
					failed.Store(true)
				} else {
					logProvider := sdklog.NewLoggerProvider(sdklog.WithResource(res), sdklog.WithProcessor(sdklog.NewSimpleProcessor(logs)))
					var record logapi.Record
					record.SetTimestamp(time.Now())
					record.SetBody(logapi.StringValue(in.Marker))
					record.SetSeverity(logapi.SeverityInfo)
					logProvider.Logger("argus.plan8.actual-sdk").Emit(spanCtx, record)
					if logProvider.Shutdown(ctx) != nil {
						failed.Store(true)
					}
				}
				metrics, e := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpoint(in.Endpoint), otlpmetrichttp.WithTLSClientConfig(tlsConfig), otlpmetrichttp.WithHeaders(map[string]string{"Authorization": "Bearer " + in.Key}), otlpmetrichttp.WithRetry(otlpmetrichttp.RetryConfig{Enabled: false}))
				if e != nil {
					failed.Store(true)
				} else {
					reader := sdkmetric.NewManualReader()
					mp := sdkmetric.NewMeterProvider(sdkmetric.WithResource(res), sdkmetric.WithReader(reader))
					counter, e := mp.Meter("argus.plan8.actual-sdk").Int64Counter("argus_plan8_sdk_requests")
					if e != nil {
						failed.Store(true)
					} else {
						counter.Add(spanCtx, 1)
					}
					var data metricdata.ResourceMetrics
					if reader.Collect(ctx, &data) != nil || metrics.Export(ctx, &data) != nil {
						failed.Store(true)
					}
					if mp.Shutdown(ctx) != nil || metrics.Shutdown(ctx) != nil {
						failed.Store(true)
					}
				}
			}
			span.End()
			if provider.Shutdown(ctx) != nil {
				failed.Store(true)
			}
		} else {
			failed.Store(true)
		}
		out.Accepted = !failed.Load()
	}
	out.ExpectationMet = out.Accepted == in.ExpectedAccept
	if json.NewEncoder(os.Stdout).Encode(out) != nil || !out.ExpectationMet {
		os.Exit(1)
	}
}
