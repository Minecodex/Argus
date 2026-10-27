package telemetry

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/proto"
)

type traceBoundaryControl struct{ sourceTestControl }

func (c traceBoundaryControl) ResolveSource(ctx context.Context, identity TrustedIdentity, id uuid.UUID, revision int64, signal string) (SourceIdentity, error) {
	if signal != "traces" {
		return SourceIdentity{}, errIngestRejected
	}
	return c.sourceTestControl.ResolveSource(ctx, identity, id, revision, "logs")
}

func TestTraceCannotBypassApplicationEndpointThroughLegacySources(t *testing.T) {
	collector := uuid.New()
	for _, kind := range []string{"unknown", "otlp", "jaeger", "skywalking"} {
		t.Run(kind, func(t *testing.T) {
			id := uuid.New()
			source := SourceIdentity{ID: id, Revision: 1, Type: kind, ResourceID: uuid.New(), ResourceType: "host"}
			control := traceBoundaryControl{sourceTestControl{collector: collector, sources: map[uuid.UUID]SourceIdentity{id: source}}}
			server := IngestServer{Control: control}
			resource := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{{Key: "service.name", Value: stringValue("forged-application")}, {Key: "argus.application.id", Value: stringValue(uuid.NewString())}, {Key: "argus.application.key.id", Value: stringValue(uuid.NewString())}}}
			if kind != "unknown" {
				resource.Attributes = append(resource.Attributes, &commonpb.KeyValue{Key: "argus.source.id", Value: stringValue(id.String())}, &commonpb.KeyValue{Key: "argus.source.revision", Value: stringValue("1")})
			}
			before, _ := proto.Marshal(resource)
			if err := server.applyTrustedSources(t.Context(), TrustedIdentity{EnterpriseID: uuid.New(), CollectorID: collector}, "traces", []**resourcepb.Resource{&resource}); err == nil {
				t.Fatal("unkeyed trace acquired application authority")
			}
			after, _ := proto.Marshal(resource)
			if !bytes.Equal(before, after) {
				t.Fatal("rejected trace mutated before validation")
			}
		})
	}
}
