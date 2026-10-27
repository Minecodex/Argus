package telemetry

import (
	"bytes"
	collectlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestLogCredentialsAreRemovedBeforeQueuedProtobuf(t *testing.T) {
	request := &collectlogs.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{{Key: "api_key", Value: stringValue("resource-canary")}, {Key: "argus.application.key.id", Value: stringValue("reference-id")}}}, ScopeLogs: []*logspb.ScopeLogs{{LogRecords: []*logspb.LogRecord{{Body: stringValue(`{"password":"body-canary","message":"business event"}`), Attributes: []*commonpb.KeyValue{{Key: "Authorization", Value: stringValue("Bearer header-canary")}, {Key: "nested", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_KvlistValue{KvlistValue: &commonpb.KeyValueList{Values: []*commonpb.KeyValue{{Key: "name", Value: stringValue("DATABASE_PASSWORD")}, {Key: "value", Value: stringValue("nested-canary")}}}}}}, {Key: "count", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 9223372036854775806}}}}}}}}}}}
	if err := redactLogPayload(request); err != nil {
		t.Fatal(err)
	}
	raw, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"resource-canary", "body-canary", "header-canary", "nested-canary"} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatal("credential retained in queued payload")
		}
	}
	if !bytes.Contains(raw, []byte("reference-id")) || !bytes.Contains(raw, []byte("business event")) {
		t.Fatal("non-secret provenance was lost")
	}
	if got := request.ResourceLogs[0].ScopeLogs[0].LogRecords[0].Attributes[2].Value.GetIntValue(); got != 9223372036854775806 {
		t.Fatal("integer precision changed")
	}
}
