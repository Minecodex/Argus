package argusapplicationidentity

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.opentelemetry.io/collector/client"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
)

func TestServiceValidationRejectsWholeBatchWithoutStampingOrForwarding(t *testing.T) {
	endpoint, source, key := uuid.NewString(), uuid.NewString(), uuid.NewString()
	cfg := &Config{Bindings: []Binding{{EndpointID: endpoint, EndpointVersion: 1, SourceID: source, SourceRevision: 1}}}
	ctx := client.NewContext(t.Context(), client.Info{Auth: testAuth{"key_id": key, "endpoint_id": endpoint, "endpoint_version": "1", "signals": "traces"}})
	forwarded := 0
	sink, err := consumer.NewTraces(func(context.Context, ptrace.Traces) error { forwarded++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	set := processor.Settings{ID: component.MustNewID("argus_application_identity"), TelemetrySettings: component.TelemetrySettings{Logger: zap.NewNop(), MeterProvider: metricnoop.NewMeterProvider(), TracerProvider: tracenoop.NewTracerProvider()}}
	receiver, err := createTraces(ctx, set, cfg, sink)
	if err != nil {
		t.Fatal(err)
	}
	value := ptrace.NewTraces()
	first := value.ResourceSpans().AppendEmpty()
	first.Resource().Attributes().PutStr("service.name", "orders")
	first.Resource().Attributes().PutStr("argus.resource.id", "original-untrusted-value")
	second := value.ResourceSpans().AppendEmpty()
	if err = receiver.ConsumeTraces(ctx, value); err == nil || !consumererror.IsPermanent(err) || forwarded != 0 {
		t.Fatal("invalid second Resource reached batch/export or was accepted", err)
	}
	if _, stamped := first.Resource().Attributes().Get("argus.source.id"); stamped {
		t.Fatal("failed whole batch partly stamped valid Resource")
	}
	if original, _ := first.Resource().Attributes().Get("argus.resource.id"); original.Str() != "original-untrusted-value" {
		t.Fatal("failed batch partly mutated input")
	}
	second.Resource().Attributes().PutStr("service.name", "inventory")
	if err = receiver.ConsumeTraces(ctx, value); err != nil || forwarded != 1 {
		t.Fatal("valid batch did not reach consumer", err)
	}
	if stamped, _ := first.Resource().Attributes().Get("argus.source.id"); stamped.Str() != source {
		t.Fatal("valid batch lost trusted source")
	}
}
