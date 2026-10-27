package telemetry

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
	collectlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/proto"
)

func TestNativeBusinessLogsStampTrustedServiceBeforePublish(t *testing.T) {
	collector, sourceID, app, service := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	source := SourceIdentity{ID: sourceID, Revision: 1, Type: "filelog", ResourceID: app, ResourceType: "application", CollectionID: uuid.New(), CollectionRevision: 1, AssignmentEpoch: 1, ServiceID: service, ServiceName: "orders", ServiceNamespace: "business", Environment: "production"}
	control := sourceTestControl{collector: collector, sources: map[uuid.UUID]SourceIdentity{sourceID: source}}
	server := IngestServer{Control: control}
	sender := TrustedIdentity{EnterpriseID: uuid.New(), ResourceID: uuid.New(), ResourceType: "host", CollectorID: collector}
	resource := func() *resourcepb.Resource {
		return &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
			{Key: "argus.source.id", Value: stringValue(sourceID.String())}, {Key: "argus.source.revision", Value: stringValue("1")},
			{Key: "service.name", Value: stringValue("spoofed")}, {Key: "deployment.environment.name", Value: stringValue("testing")},
			{Key: "argus.application.key.id", Value: stringValue(uuid.NewString())},
		}}
	}
	request := &collectlogs.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{Resource: resource(), ScopeLogs: []*logspb.ScopeLogs{{LogRecords: []*logspb.LogRecord{{Body: stringValue(`{"password":"private-test-only","message":"hello"}`)}}}}}}}
	if _, err := server.prepareLogPayload(t.Context(), sender, request); err != nil {
		t.Fatal(err)
	}
	attrs := attributesMap(request.ResourceLogs[0].Resource.Attributes)
	for key, value := range map[string]string{"argus.application.id": app.String(), "argus.service.id": service.String(), "service.name": "orders", "service.namespace": "business", "deployment.environment.name": "production"} {
		if attrs[key] != value {
			t.Fatalf("%s = %q", key, attrs[key])
		}
	}
	if attrs["argus.application.key.id"] != "" || attrs["argus.application.endpoint.id"] != "" {
		t.Fatal("native rule acquired SDK credential identity")
	}
	raw, _ := proto.Marshal(request)
	if bytes.Contains(raw, []byte("private-test-only")) {
		t.Fatal("raw secret reached queue payload")
	}
	// Reject an invalid later resource without mutating the valid first resource.
	invalid := uuid.New()
	control.sources[invalid] = SourceIdentity{ID: invalid, Revision: 1, Type: "filelog", ResourceID: app, ResourceType: "application"}
	bad := resource()
	bad.Attributes[0].Value = stringValue(invalid.String())
	request.ResourceLogs = []*logspb.ResourceLogs{{Resource: resource()}, {Resource: bad}}
	before, _ := proto.Marshal(request)
	if _, err := server.prepareLogPayload(t.Context(), sender, request); err == nil {
		t.Fatal("unmapped application logs accepted")
	}
	after, _ := proto.Marshal(request)
	if !bytes.Equal(before, after) {
		t.Fatal("rejected batch partially mutated")
	}
}
