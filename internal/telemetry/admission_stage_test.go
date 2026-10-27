package telemetry

import (
	"bytes"
	"context"
	"errors"
	"github.com/google/uuid"
	metricsrpc "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdmissionStagesKeepAuthorizationAndPrivateErrorBoundaries(t *testing.T) {
	for _, row := range []struct {
		stage  string
		cause  error
		reason string
	}{{"source", ErrSourceAwaitingApply, "SOURCE_METADATA_PENDING"}, {"namespace_object", ErrSourceAwaitingApply, "NAMESPACE_OBJECT_METADATA_PENDING"}, {"namespace_uid", ErrUnavailable, "NAMESPACE_UID_CONTROL_UNAVAILABLE"}} {
		err := &admissionStageError{Stage: row.stage, Cause: errors.Join(row.cause, errors.New("password=private-canary https://private:secret@target"))}
		rpc := admissionRPCError(err, "rejected")
		if status.Code(rpc) != codes.Unavailable || !strings.Contains(status.Convert(rpc).Message(), row.reason) || strings.Contains(rpc.Error(), "private-canary") || strings.Contains(rpc.Error(), "target") {
			t.Fatal("unsafe or ambiguous admission reason", row.stage)
		}
		w := httptest.NewRecorder()
		admissionHTTPError(w, err, "rejected", 403)
		if w.Code != 503 || w.Header().Get("Retry-After") != "1" || !strings.Contains(w.Body.String(), row.reason) || strings.Contains(w.Body.String(), "private-canary") {
			t.Fatal("HTTP diagnostic changed retry or privacy boundary")
		}
	}
	if status.Code(admissionRPCError(&admissionStageError{Stage: "namespace_object", Cause: errIngestRejected}, "rejected")) != codes.PermissionDenied {
		t.Fatal("fixed reason changed permanent denial into retry")
	}
	if admissionPublicReason(&admissionStageError{Stage: "private-model-input", Cause: ErrSourceAwaitingApply}) != "" {
		t.Fatal("unrecognized stage escaped")
	}
}

type pendingNamespaceStageControl struct{ nativeTemplateSourceControl }

func (c pendingNamespaceStageControl) ResolveMetricObjectNamespace(context.Context, TrustedIdentity, SourceIdentity, string, []MetricNamespaceObject) (string, error) {
	return "", ErrSourceAwaitingApply
}
func TestActualAdmissionPathNamesNamespaceStageWithoutMutatingRejectedBatch(t *testing.T) {
	collector, sourceID, cluster := uuid.New(), uuid.New(), uuid.New()
	source := SourceIdentity{ID: sourceID, Revision: 1, Type: "k8s_cluster", ResourceType: "kubernetes_cluster", ResourceID: cluster}
	control := pendingNamespaceStageControl{nativeTemplateSourceControl{sourceTestControl{collector: collector, sources: map[uuid.UUID]SourceIdentity{sourceID: source}}}}
	attrs := []*commonpb.KeyValue{{Key: "argus.source.id", Value: stringValue(sourceID.String())}, {Key: "argus.source.revision", Value: stringValue("1")}, {Key: "k8s.namespace.name", Value: stringValue("private-namespace")}, {Key: "k8s.pod.uid", Value: stringValue(uuid.NewString())}}
	request := &metricsrpc.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{Resource: &resourcepb.Resource{Attributes: attrs}}}}
	before, _ := proto.Marshal(request)
	_, err := (&IngestServer{Control: control}).prepareMetricPayload(t.Context(), TrustedIdentity{EnterpriseID: uuid.New(), CollectorID: collector, ResourceID: cluster, ResourceType: "kubernetes_cluster"}, request)
	if !errors.Is(err, ErrSourceAwaitingApply) || admissionPublicReason(err) != "NAMESPACE_OBJECT_METADATA_PENDING" {
		t.Fatal("actual namespace path was misclassified", err)
	}
	after, _ := proto.Marshal(request)
	if !bytes.Equal(before, after) {
		t.Fatal("rejected request was partially normalized")
	}
}
