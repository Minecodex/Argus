package telemetry

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	collectmetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type rotationIdentityControl struct {
	IngestControlStore
	identity TrustedIdentity
	uri      string
	err      error
	read     string
}

func (control *rotationIdentityControl) ResolveCollectorIdentity(_ context.Context, serial string) (TrustedIdentity, string, error) {
	control.read = serial
	return control.identity, control.uri, control.err
}

func TestQueuedSelfTelemetryAcceptsOnlyRegisteredOverlappingLeaf(t *testing.T) {
	for _, role := range []string{"daemonset", "edge_gateway"} {
		t.Run(role, func(t *testing.T) {
			gateway := TrustedIdentity{EnterpriseID: uuid.New(), ResourceID: uuid.New(), CollectorID: uuid.New(),
				ResourceType: "kubernetes_cluster", Role: role, CertificateSerial: "new-leaf", AuthorizationVersion: 7}
			previous := gateway
			previous.CertificateSerial = "old-leaf"
			control := &rotationIdentityControl{identity: previous, uri: CollectorCertificateURI(gateway.CollectorID)}
			server := IngestServer{Control: control}
			request := &collectmetrics.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
				{Key: "argus.downstream.collector.id", Value: stringValue(gateway.CollectorID.String())},
				{Key: "argus.downstream.certificate.serial", Value: stringValue("OLD-LEAF")},
			}}}}}
			encoded, err := proto.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			replayed := &collectmetrics.ExportMetricsServiceRequest{}
			if err := proto.Unmarshal(encoded, replayed); err != nil {
				t.Fatal(err)
			}
			identity, err := server.resolveMetricPayloadIdentity(t.Context(), gateway, replayed)
			if err != nil || identity != gateway || control.read != "old-leaf" {
				t.Fatalf("valid certificate handover was rejected: %v", err)
			}
			// Once the registry retires that leaf, the very same queued request
			// must be rejected even though its logical Collector ID still matches.
			control.err = errIngestRejected
			if _, err := server.resolveMetricPayloadIdentity(t.Context(), gateway, replayed); err == nil {
				t.Fatal("retired same-Collector leaf accepted")
			}
		})
	}
}

func TestSameCollectorRotationCannotChangeIdentityOrAuthorization(t *testing.T) {
	gateway := TrustedIdentity{EnterpriseID: uuid.New(), ResourceID: uuid.New(), CollectorID: uuid.New(), ResourceType: "kubernetes_cluster",
		Role: "daemonset", CertificateSerial: "new-leaf", AuthorizationVersion: 5}
	for _, field := range []string{"serial", "collector", "enterprise", "resource", "resource-type", "role", "authorization", "uri", "fenced", "control-unavailable"} {
		t.Run(field, func(t *testing.T) {
			previous := gateway
			previous.CertificateSerial = "old-leaf"
			control := &rotationIdentityControl{identity: previous, uri: CollectorCertificateURI(gateway.CollectorID)}
			switch field {
			case "serial":
				control.identity.CertificateSerial = "other-leaf"
			case "collector":
				control.identity.CollectorID = uuid.New()
			case "enterprise":
				control.identity.EnterpriseID = uuid.New()
			case "resource":
				control.identity.ResourceID = uuid.New()
			case "resource-type":
				control.identity.ResourceType = "host"
			case "role":
				control.identity.Role = "edge_gateway"
			case "authorization":
				control.identity.AuthorizationVersion++
			case "uri":
				control.uri = CollectorCertificateURI(uuid.New())
			case "fenced":
				control.err = errIngestRejected
			case "control-unavailable":
				control.err = ErrUnavailable
			}
			server := IngestServer{Control: control}
			_, err := server.resolveDownstreamIdentity(t.Context(), gateway,
				downstreamReference{collectorID: gateway.CollectorID, serial: "old-leaf", found: true})
			if err == nil {
				t.Fatal("same-ID reference widened the authenticated identity")
			}
			code := status.Code(admissionRPCError(err, "identity rejected"))
			if field == "control-unavailable" {
				if !errors.Is(err, ErrUnavailable) || code != codes.Unavailable {
					t.Fatal("control outage became a permanent data rejection")
				}
			} else if code != codes.PermissionDenied {
				t.Fatal("fenced identity became a retryable admission")
			}
		})
	}
	if _, err := (&IngestServer{}).resolveDownstreamIdentity(t.Context(), gateway,
		downstreamReference{collectorID: gateway.CollectorID, serial: "old-leaf", found: true}); err == nil {
		t.Fatal("unknown certificate accepted without a registry")
	}
}
