package telemetry

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	collectlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/proto"
)

type sourceTestControl struct {
	IngestControlStore
	collector uuid.UUID
	sources   map[uuid.UUID]SourceIdentity
}

func TestMixedTargetsKeepIndependentTrustedIdentity(t *testing.T) {
	collector, first, second := uuid.New(), uuid.New(), uuid.New()
	a, b := uuid.New(), uuid.New()
	collection := uuid.New()
	control := sourceTestControl{collector: collector, sources: map[uuid.UUID]SourceIdentity{
		first:  {ID: first, Revision: 4, Type: "filelog", ResourceID: a, ResourceType: "monitoring_target", CollectionID: collection, CollectionRevision: 2, AssignmentEpoch: 3},
		second: {ID: second, Revision: 4, Type: "journald", ResourceID: b, ResourceType: "host"},
	}}
	server := IngestServer{Control: control}
	sender := TrustedIdentity{EnterpriseID: uuid.New(), CollectorID: collector, ResourceID: b, ResourceType: "host"}
	resource := func(source uuid.UUID) *resourcepb.Resource {
		return &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
			{Key: "argus.source.id", Value: stringValue(source.String())}, {Key: "argus.source.revision", Value: stringValue("4")},
			{Key: "argus.resource.id", Value: stringValue("forged")}, {Key: "argus.enterprise.id", Value: stringValue("forged")}, {Key: "argus.collection.id", Value: stringValue("forged")},
		}}
	}
	request := &collectlogs.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{Resource: resource(first)}, {Resource: resource(second)}}}
	if _, err := server.prepareLogPayload(t.Context(), sender, request); err != nil {
		t.Fatal(err)
	}
	record := &kgo.Record{Headers: []kgo.RecordHeader{{Key: "argus-normalization-version", Value: []byte("v3")}}}
	for i, id := range []uuid.UUID{a, b} {
		identity, err := recordResourceIdentity(record, sender, request.ResourceLogs[i].Resource.Attributes)
		if err != nil || identity.ResourceID != id || identity.CollectorID != collector || identity.EnterpriseID != sender.EnterpriseID {
			t.Fatalf("target normalization: %+v %v", identity, err)
		}
	}
	if attrs := attributesMap(request.ResourceLogs[0].Resource.Attributes); attrs["argus.collection.id"] != collection.String() || attrs["argus.assignment.epoch"] != "3" {
		t.Fatal("collection responsibility was not trusted")
	}
	bad := &collectlogs.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{Resource: resource(first)}, {Resource: resource(uuid.New())}}}
	before, _ := proto.Marshal(bad)
	if _, err := server.prepareLogPayload(t.Context(), sender, bad); err == nil {
		t.Fatal("partially invalid batch accepted")
	}
	after, _ := proto.Marshal(bad)
	if !bytes.Equal(before, after) {
		t.Fatal("rejected batch partially mutated")
	}
}

func (control sourceTestControl) ResolveSource(_ context.Context, identity TrustedIdentity, id uuid.UUID, revision int64, signal string) (SourceIdentity, error) {
	source, ok := control.sources[id]
	if !ok || identity.CollectorID != control.collector || source.Revision != revision || signal != "logs" {
		return SourceIdentity{}, errIngestRejected
	}
	return source, nil
}

func TestIngestSourceReferenceIsBoundToAuthenticatedCollector(t *testing.T) {
	collector := uuid.New()
	first, second := uuid.New(), uuid.New()
	control := sourceTestControl{collector: collector, sources: map[uuid.UUID]SourceIdentity{first: {ID: first, Revision: 1, Type: "filelog"}, second: {ID: second, Revision: 2, Type: "journald"}}}
	server := IngestServer{Control: control}
	identity := TrustedIdentity{CollectorID: collector, EnterpriseID: uuid.New(), ResourceID: uuid.New(), ResourceType: "host", Role: "direct"}
	attrs := func(id uuid.UUID, revision string) []*commonpb.KeyValue {
		return []*commonpb.KeyValue{{Key: "argus.source.id", Value: stringValue(id.String())}, {Key: "argus.source.revision", Value: stringValue(revision)}, {Key: "argus.source.type", Value: stringValue("forged-vendor")}}
	}
	request := &collectlogs.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{Resource: &resourcepb.Resource{Attributes: attrs(first, "1")}}, {Resource: &resourcepb.Resource{Attributes: attrs(second, "2")}}}}
	if _, err := server.prepareLogPayload(context.Background(), identity, request); err != nil {
		t.Fatal(err)
	}
	for i, typ := range []string{"filelog", "journald"} {
		source, err := recordSource(request.ResourceLogs[i].Resource.Attributes)
		if err != nil || source.Type != typ {
			t.Fatalf("mixed source origin was lost: %+v %v", source, err)
		}
	}
	for _, bad := range []struct {
		id       uuid.UUID
		revision string
		producer uuid.UUID
	}{{uuid.New(), "1", collector}, {first, "2", collector}, {first, "1", uuid.New()}} {
		request.ResourceLogs = []*logspb.ResourceLogs{{Resource: &resourcepb.Resource{Attributes: attrs(bad.id, bad.revision)}}}
		actor := identity
		actor.CollectorID = bad.producer
		if _, err := server.prepareLogPayload(context.Background(), actor, request); err == nil {
			t.Fatal("unregistered or foreign source accepted")
		}
	}
	request.ResourceLogs = []*logspb.ResourceLogs{{Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{{Key: "telemetry.sdk.name", Value: stringValue("skywalking")}}}}}
	if _, err := server.prepareLogPayload(context.Background(), identity, request); err == nil {
		t.Fatal("unregistered SDK source was accepted")
	}
}

func TestPlatformSourceAttributesDoNotConsumeUserAttributeBudget(t *testing.T) {
	attrs := []*commonpb.KeyValue{}
	for i := 0; i < maxAttributes; i++ {
		attrs = append(attrs, &commonpb.KeyValue{Key: fmt.Sprintf("user_%d", i), Value: stringValue("value")})
	}
	collector, sourceID := uuid.New(), uuid.New()
	attrs = append(attrs, &commonpb.KeyValue{Key: "argus.source.id", Value: stringValue(sourceID.String())}, &commonpb.KeyValue{Key: "argus.source.revision", Value: stringValue("1")})
	resource := &resourcepb.Resource{Attributes: attrs}
	server := IngestServer{Control: sourceTestControl{collector: collector, sources: map[uuid.UUID]SourceIdentity{sourceID: {ID: sourceID, Revision: 1, Type: "filelog"}}}}
	if err := server.applyTrustedSources(context.Background(), TrustedIdentity{EnterpriseID: uuid.New(), ResourceID: uuid.New(), CollectorID: collector}, "logs", []**resourcepb.Resource{&resource}); err != nil {
		t.Fatal(err)
	}
	if err := validateResourceAttributes(resource.Attributes); err != nil {
		t.Fatal("canonical source metadata made an accepted payload invalid")
	}
	resource.Attributes = append(resource.Attributes, &commonpb.KeyValue{Key: "extra_user_attribute", Value: stringValue("value")})
	if validateResourceAttributes(resource.Attributes) == nil {
		t.Fatal("platform allowance bypassed user attribute limit")
	}
}
