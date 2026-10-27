package telemetry

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	promqlengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
	"github.com/prometheus/prometheus/promql"
	"github.com/twmb/franz-go/pkg/kgo"
	metricsrpc "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/proto"
)

type namespaceMetricTestControl struct {
	nativeTemplateSourceControl
	namespaces map[string]string
}

func (c namespaceMetricTestControl) ResolveMetricNamespace(_ context.Context, _ TrustedIdentity, source SourceIdentity, name, uid string) (string, error) {
	if source.ResourceType != "kubernetes_cluster" || c.namespaces[name] != uid || uid == "" {
		return "", errIngestRejected
	}
	return uid, nil
}

func TestClickHouseNamespaceMetricsFilterBeforeAggregationCatalogAndOverview(t *testing.T) {
	testClickHouseNamespaceMetricsWithMemberCount(t, 1)
}
func TestClickHouseMaximumNamespaceMetricsFilterBeforeAggregationCatalogAndOverview(t *testing.T) {
	testClickHouseNamespaceMetricsWithMemberCount(t, 10000)
}
func testClickHouseNamespaceMetricsWithMemberCount(t *testing.T, sourceCount int) {
	conn := sourceTestClickHouse(t)
	ctx := t.Context()
	enterprise, cluster, collector, sourceID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, enterprise); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.DropTenant(context.Background(), enterprise) })
	nsA, nsB := uuid.NewString(), uuid.NewString()
	source := SourceIdentity{ID: sourceID, Revision: 1, Type: "k8s_cluster", ResourceType: "kubernetes_cluster", ResourceID: cluster, CollectionID: uuid.New(), CollectionRevision: 1, AssignmentEpoch: 1}
	control := namespaceMetricTestControl{nativeTemplateSourceControl{sourceTestControl{collector: collector, sources: map[uuid.UUID]SourceIdentity{sourceID: source}}}, map[string]string{"orders": nsA, "payments": nsB}}
	server := IngestServer{Control: control}
	identity := TrustedIdentity{EnterpriseID: enterprise, CollectorID: collector, ResourceID: cluster, ResourceType: "kubernetes_cluster"}
	writer := Writer{ClickHouse: conn, Router: router}
	at := time.Now().UTC().Truncate(time.Second)
	requestFor := func(name, uid, metric string, value float64) *metricsrpc.ExportMetricsServiceRequest {
		attrs := []*commonpb.KeyValue{{Key: "argus.source.id", Value: stringValue(sourceID.String())}, {Key: "argus.source.revision", Value: stringValue("1")}, {Key: "k8s.namespace.name", Value: stringValue(name)}, {Key: "k8s.namespace.uid", Value: stringValue(uid)}, {Key: "argus.namespace.uid", Value: stringValue("forged")}}
		point := &metricspb.NumberDataPoint{TimeUnixNano: uint64(at.UnixNano()), Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: value}}
		return &metricsrpc.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{Resource: &resourcepb.Resource{Attributes: attrs}, ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: []*metricspb.Metric{{Name: metric, Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{point}}}}}}}}}}
	}
	for i, request := range []*metricsrpc.ExportMetricsServiceRequest{requestFor("orders", nsA, "namespace_load", 1), requestFor("payments", nsB, "namespace_load", 100), requestFor("payments", nsB, "foreign_only", 7)} {
		if _, err := server.prepareMetricPayload(ctx, identity, request); err != nil {
			t.Fatal(err)
		}
		if attrs := attributesMap(request.ResourceMetrics[0].Resource.Attributes); attrs["argus.namespace.uid"] == "forged" || attrs["argus.namespace.uid"] == "" {
			t.Fatal("Namespace identity was not canonicalized")
		}
		if _, err := writer.writeMetrics(ctx, &kgo.Record{Topic: "otlp-metrics", Offset: int64(i + 1), Timestamp: at, Headers: []kgo.RecordHeader{{Key: "argus-normalization-version", Value: []byte("v3")}}}, identity, request, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	// Same claimed name and forged reserved UID cannot make an observation
	// without trusted UID proof visible to the Namespace reader.
	for i, metric := range []string{"namespace_load", "unverified_only"} {
		request := requestFor("orders", "", metric, 1000)
		attrs := request.ResourceMetrics[0].Resource.Attributes
		request.ResourceMetrics[0].Resource.Attributes = append(attrs[:3], attrs[4:]...)
		if _, err := server.prepareMetricPayload(ctx, identity, request); err != nil {
			t.Fatal(err)
		}
		actual := attributesMap(request.ResourceMetrics[0].Resource.Attributes)
		if actual["argus.namespace.uid"] != "" || actual["argus.namespace.attribution_status"] != "unverified" {
			t.Fatal("missing proof acquired Namespace authority", actual)
		}
		if _, err := writer.writeMetrics(ctx, &kgo.Record{Topic: "otlp-metrics", Offset: int64(4 + i), Timestamp: at, Headers: []kgo.RecordHeader{{Key: "argus-normalization-version", Value: []byte("v3")}}}, identity, request, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range []string{source.Key() + "@" + nsA, source.Key() + "@" + nsB, source.Key() + "@" + uuid.NewString(), source.Key()} {
		want := float64(0)
		switch key {
		case source.Key() + "@" + nsA:
			want = 1
		case source.Key() + "@" + nsB:
			want = 100
		case source.Key():
			want = 1101
		}
		result, err := promqlengine.NewEngine(conn, router, nil).Execute(ctx, promqlengine.Request{Expression: "sum(namespace_load)", Instant: true, Start: at.Add(-time.Minute), End: at.Add(time.Second), Scope: promqlengine.Scope{EnterpriseID: enterprise, ResourceIDs: []uuid.UUID{cluster}, SourceKeys: paddedNamespaceKeys(key, sourceCount)}})
		if err != nil {
			t.Fatal(err)
		}
		vector, ok := result.Value.(promql.Vector)
		if !ok || want == 0 && len(vector) != 0 || want != 0 && (len(vector) != 1 || vector[0].F != want) {
			t.Fatal("Namespace metrics escaped before aggregation", result.Value)
		}
	}
	query := ClickHouseQuery{Conn: conn, Router: router}
	keys := paddedNamespaceKeys(source.Key()+"@"+nsA, sourceCount)
	catalog, err := query.DiscoverData(ctx, DataCatalogRequest{EnterpriseID: enterprise, ResourceIDs: []uuid.UUID{cluster}, SourceKeys: keys, Signal: "metrics", Kind: "metrics", From: at.Add(-time.Minute), To: at.Add(time.Minute), Limit: 10})
	if err != nil || len(catalog.Metrics) != 1 || catalog.Metrics[0].Name != "namespace_load" {
		t.Fatal("foreign Namespace influenced metric candidates", catalog, err)
	}
	overview, err := query.Overview(ctx, OverviewRequest{EnterpriseID: enterprise, ResourceIDs: []uuid.UUID{cluster}, SourceKeys: keys, From: at.Add(-time.Minute), To: at.Add(time.Minute), Timeout: 10 * time.Second, MaxScanBytes: 64 << 20})
	if err != nil || overview.MetricPoints != 1 {
		t.Fatal("Namespace overview counted foreign rows", overview, err)
	}
	bad := requestFor("orders", nsB, "namespace_load", 1)
	before, _ := proto.Marshal(bad)
	if _, err := server.prepareMetricPayload(ctx, identity, bad); err == nil {
		t.Fatal("recreated/forged namespace UID accepted")
	}
	after, _ := proto.Marshal(bad)
	if !bytes.Equal(before, after) {
		t.Fatal("rejected Namespace batch was partly normalized")
	}
}

func paddedNamespaceKeys(key string, sourceCount int) []string {
	keys := []string{key}
	for len(keys) < sourceCount {
		value := uuid.NewString() + ":1"
		if len(keys)%2 == 0 {
			value += "@" + uuid.NewString()
		}
		keys = append(keys, value)
	}
	return keys
}
