package telemetry

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
	metricsrpc "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/proto"
)

type attributionTestControl struct {
	nativeTemplateSourceControl
	uid     string
	failure error
}

func (c attributionTestControl) ResolveMetricObjectNamespace(_ context.Context, _ TrustedIdentity, _ SourceIdentity, name string, _ []MetricNamespaceObject) (string, error) {
	if name == "verified" {
		return c.uid, nil
	}
	return "", c.failure
}

func TestMixedNamespaceAttributionRetainsClusterDataWithoutGrantingNamespace(t *testing.T) {
	collector, id, cluster, ns := uuid.New(), uuid.New(), uuid.New(), uuid.NewString()
	source := SourceIdentity{ID: id, Revision: 1, Type: "k8s_cluster", ResourceID: cluster, ResourceType: "kubernetes_cluster"}
	control := attributionTestControl{nativeTemplateSourceControl{sourceTestControl{collector: collector, sources: map[uuid.UUID]SourceIdentity{id: source}}}, ns, errNamespaceAttributionUnverified}
	identity := TrustedIdentity{EnterpriseID: uuid.New(), CollectorID: collector, ResourceID: cluster, ResourceType: "kubernetes_cluster"}
	request := &metricsrpc.ExportMetricsServiceRequest{}
	for _, name := range []string{"verified", "unobserved", "name-only", ""} {
		attrs := []*commonpb.KeyValue{{Key: "argus.source.id", Value: stringValue(id.String())}, {Key: "argus.source.revision", Value: stringValue("1")}, {Key: "argus.namespace.uid", Value: stringValue("forged")}, {Key: "argus.namespace.attribution_status", Value: stringValue("forged")}}
		if name != "" {
			attrs = append(attrs, &commonpb.KeyValue{Key: "k8s.namespace.name", Value: stringValue(name)})
		}
		if name == "verified" || name == "unobserved" {
			attrs = append(attrs, &commonpb.KeyValue{Key: "k8s.pod.uid", Value: stringValue(uuid.NewString())})
		}
		request.ResourceMetrics = append(request.ResourceMetrics, &metricspb.ResourceMetrics{Resource: &resourcepb.Resource{Attributes: attrs}})
	}
	before := proto.Clone(request).(*metricsrpc.ExportMetricsServiceRequest)
	server := &IngestServer{Control: control}
	if _, err := server.prepareMetricPayload(t.Context(), identity, request); err != nil {
		t.Fatal(err)
	}
	for i, block := range request.ResourceMetrics {
		attrs := attributesMap(block.Resource.Attributes)
		if attrs["argus.resource.id"] != cluster.String() || attrs["argus.source.id"] != id.String() {
			t.Fatal("trusted Cluster source was not preserved", attrs)
		}
		if i == 0 {
			if attrs["argus.namespace.uid"] != ns || attrs["argus.namespace.attribution_status"] != "" {
				t.Fatal("verified namespace changed", attrs)
			}
		} else if attrs["argus.namespace.uid"] != "" || (i < 3 && attrs["argus.namespace.attribution_status"] != "unverified") || (i == 3 && attrs["argus.namespace.attribution_status"] != "") {
			t.Fatal("unverified or Cluster-only observation acquired Namespace authority", attrs)
		}
	}
	for _, failure := range []error{ErrSourceAwaitingApply, ErrUnavailable, errIngestRejected} {
		control.failure = failure
		server.Control = control
		rejected := proto.Clone(before).(*metricsrpc.ExportMetricsServiceRequest)
		initial, _ := proto.Marshal(rejected)
		if _, err := server.prepareMetricPayload(t.Context(), identity, rejected); err == nil {
			t.Fatal("control failure was incorrectly downgraded")
		}
		after, _ := proto.Marshal(rejected)
		if !bytes.Equal(initial, after) {
			t.Fatal("rejected mixed batch was partially mutated")
		}
	}
}
