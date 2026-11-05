package main

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	jaegerapi "github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

func jaegerExporters(ctx context.Context, cfg configuration, secure *tls.Config, headers map[string]string, service string, res *resource.Resource) (sdktrace.SpanExporter, sdklog.Exporter, sdkmetric.Exporter, func() error, error) {
	if cfg.AuxEndpoint == "" || cfg.AuxKey == "" {
		return nil, nil, nil, nil, errors.New("OTLP metrics and logs endpoint is required")
	}
	traceConn, err := grpc.NewClient(cfg.Endpoint, grpc.WithTransportCredentials(credentials.NewTLS(secure)))
	if err != nil {
		return nil, nil, nil, nil, err
	}
	auxConn, err := dialOTLPGRPC(cfg.AuxEndpoint, secure)
	if err != nil {
		return nil, nil, nil, nil, errors.Join(err, traceConn.Close())
	}
	closeConnections := func() error { return errors.Join(traceConn.Close(), auxConn.Close()) }
	logs, metrics, err := grpcLogMetricExporters(ctx, auxConn, map[string]string{"Authorization": "Bearer " + cfg.AuxKey})
	if err != nil {
		return nil, nil, nil, nil, errors.Join(err, closeConnections())
	}
	traces := &jaegerSpanExporter{
		client:  jaegerapi.NewCollectorServiceClient(traceConn),
		headers: metadata.New(headers),
		process: &model.Process{ServiceName: service, Tags: jaegerTags(res.Attributes())},
	}
	return traces, logs, metrics, closeConnections, nil
}

// Adapt the SDK's finished request spans to Jaeger's native collector protocol.
type jaegerSpanExporter struct {
	client  jaegerapi.CollectorServiceClient
	headers metadata.MD
	process *model.Process
	ack     sync.Once
}

func (e *jaegerSpanExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	if len(spans) == 0 {
		return nil
	}
	batch := model.Batch{Process: e.process, Spans: make([]*model.Span, 0, len(spans))}
	for _, span := range spans {
		batch.Spans = append(batch.Spans, jaegerSpan(span))
	}
	var remote peer.Peer
	_, err := e.client.PostSpans(metadata.NewOutgoingContext(ctx, e.headers), &jaegerapi.PostSpansRequest{Batch: batch}, grpc.Peer(&remote))
	if err == nil {
		if tlsInfo, ok := remote.AuthInfo.(credentials.TLSInfo); ok {
			e.ack.Do(func() {
				proof, _ := json.Marshal(map[string]any{
					"rpc": "/jaeger.api_v2.CollectorService/PostSpans", "spans": len(spans),
					"tls_version": tlsInfo.State.Version, "alpn": tlsInfo.State.NegotiatedProtocol,
					"verified_chains": len(tlsInfo.State.VerifiedChains),
				})
				fmt.Println("jaeger_grpc_ack " + string(proof))
			})
		}
	}
	return err
}

func (*jaegerSpanExporter) Shutdown(context.Context) error { return nil }

func jaegerSpan(span sdktrace.ReadOnlySpan) *model.Span {
	sc := span.SpanContext()
	traceID, spanID := sc.TraceID(), sc.SpanID()
	value := &model.Span{
		TraceID: jaegerTraceID(traceID), SpanID: model.SpanID(binary.BigEndian.Uint64(spanID[:])),
		OperationName: span.Name(), StartTime: span.StartTime(), Duration: span.EndTime().Sub(span.StartTime()),
		Tags: append(jaegerTags(span.Attributes()), model.String("span.kind", span.SpanKind().String())),
	}
	if parent := span.Parent(); parent.IsValid() {
		id := parent.SpanID()
		value.References = []model.SpanRef{model.NewChildOfRef(jaegerTraceID(parent.TraceID()), model.SpanID(binary.BigEndian.Uint64(id[:])))}
	}
	if sc.IsSampled() {
		value.Flags = 1
	}
	if status := span.Status(); status.Code == codes.Error {
		value.Tags = append(value.Tags, model.Bool("error", true), model.String("otel.status_code", "ERROR"), model.String("otel.status_description", status.Description))
	}
	for _, event := range span.Events() {
		value.Logs = append(value.Logs, model.Log{Timestamp: event.Time, Fields: append(jaegerTags(event.Attributes), model.String("event", event.Name))})
	}
	return value
}

func jaegerTraceID(id trace.TraceID) model.TraceID {
	return model.TraceID{High: binary.BigEndian.Uint64(id[:8]), Low: binary.BigEndian.Uint64(id[8:])}
}

func jaegerTags(attributes []attribute.KeyValue) []model.KeyValue {
	tags := make([]model.KeyValue, 0, len(attributes))
	for _, item := range attributes {
		key := string(item.Key)
		switch item.Value.Type() {
		case attribute.BOOL:
			tags = append(tags, model.Bool(key, item.Value.AsBool()))
		case attribute.INT64:
			tags = append(tags, model.Int64(key, item.Value.AsInt64()))
		case attribute.FLOAT64:
			tags = append(tags, model.Float64(key, item.Value.AsFloat64()))
		default:
			tags = append(tags, model.String(key, item.Value.Emit()))
		}
	}
	return tags
}
