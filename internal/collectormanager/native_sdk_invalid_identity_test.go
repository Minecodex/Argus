package collectormanager_test

import (
	"bytes"
	"io"
	"net/http"
	"testing"
	"time"

	collecttraces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestNativeSDKRejectsInvalidServiceIdentityBeforeBatching(t *testing.T) {
	f := newNativeSDKPipeline(t)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: f.sdkTLS()}, Timeout: 10 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	for _, name := range []string{"", "unknown_service:plan8-test"} {
		attrs := []*commonpb.KeyValue{}
		if name != "" {
			attrs = append(attrs, &commonpb.KeyValue{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: name}}})
		}
		now := time.Now()
		request := &collecttraces.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{Resource: &resourcepb.Resource{Attributes: attrs}, ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{{TraceId: bytes.Repeat([]byte{1}, 16), SpanId: bytes.Repeat([]byte{2}, 8), Name: "plan8-invalid-service", Kind: tracepb.Span_SPAN_KIND_SERVER, StartTimeUnixNano: uint64(now.UnixNano()), EndTimeUnixNano: uint64(now.Add(time.Millisecond).UnixNano())}}}}}}}
		raw, err := proto.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		call, err := http.NewRequestWithContext(f.ctx, http.MethodPost, "https://"+f.address+"/v1/traces", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		call.Header.Set("Content-Type", "application/x-protobuf")
		call.Header.Set("Authorization", "Bearer "+f.apps[0].token)
		response, err := client.Do(call)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusBadRequest {
			t.Errorf("invalid service identity acknowledged before batching: HTTP %d", response.StatusCode)
		}
	}
}
