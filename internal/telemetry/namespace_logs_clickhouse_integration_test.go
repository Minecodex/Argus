package telemetry

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/kql"
	"github.com/twmb/franz-go/pkg/kgo"
	collectlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

// Real canonicalization -> Writer -> ClickHouse query gate. Source admission is
// controlled here; PostgreSQL authorization and Kafka/network E2E are separate.
func TestClickHouseNamespaceLogSourcesFilterBeforeAggregationAndCandidates(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx := t.Context()
	enterprise, collector, cluster, appA, appB := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, enterprise); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.DropTenant(context.Background(), enterprise); err != nil {
			t.Error(err)
		}
	})
	nsA, nsB := uuid.NewString(), uuid.NewString()
	sources := []SourceIdentity{
		{ID: uuid.New(), Revision: 1, Type: "filelog", ResourceType: "kubernetes_cluster", ResourceID: cluster, NamespaceUID: nsA, CollectionID: uuid.New(), CollectionRevision: 1, AssignmentEpoch: 1},
		{ID: uuid.New(), Revision: 1, Type: "filelog", ResourceType: "application", ResourceID: appA, NamespaceUID: nsA, CollectionID: uuid.New(), CollectionRevision: 1, AssignmentEpoch: 1, ServiceID: uuid.New(), ServiceName: "orders", ServiceNamespace: "business", Environment: "production"},
		{ID: uuid.New(), Revision: 1, Type: "filelog", ResourceType: "application", ResourceID: appB, NamespaceUID: nsB, CollectionID: uuid.New(), CollectionRevision: 1, AssignmentEpoch: 1, ServiceID: uuid.New(), ServiceName: "orders", ServiceNamespace: "business", Environment: "testing"},
	}
	control := sourceTestControl{collector: collector, sources: map[uuid.UUID]SourceIdentity{}}
	for _, source := range sources {
		control.sources[source.ID] = source
	}
	server := IngestServer{Control: control}
	sender := TrustedIdentity{EnterpriseID: enterprise, CollectorID: collector, ResourceID: cluster, ResourceType: "kubernetes_cluster"}
	writer := Writer{ClickHouse: conn, Router: router}
	at := time.Now().UTC().Truncate(time.Second)
	for i, source := range sources {
		body := "namespace-a system"
		if i == 1 {
			body = `{"message":"namespace-a business","password":"private-namespace-test-only"}`
		}
		if i == 2 {
			body = "namespace-b hidden"
		}
		request := &collectlogs.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{{Key: "argus.source.id", Value: stringValue(source.ID.String())}, {Key: "argus.source.revision", Value: stringValue("1")}, {Key: "argus.application.id", Value: stringValue("spoofed")}, {Key: "argus.namespace.uid", Value: stringValue("spoofed")}}}, ScopeLogs: []*logspb.ScopeLogs{{LogRecords: []*logspb.LogRecord{{TimeUnixNano: uint64(at.UnixNano()), Body: stringValue(body)}}}}}}}
		if _, err := server.prepareLogPayload(ctx, sender, request); err != nil {
			t.Fatal(err)
		}
		attrs := attributesMap(request.ResourceLogs[0].Resource.Attributes)
		if attrs["argus.namespace.uid"] != source.NamespaceUID {
			t.Fatal("trusted Namespace provenance lost")
		}
		if _, err := writer.writeLogs(ctx, &kgo.Record{Topic: "otlp-logs", Offset: int64(i + 1), Timestamp: at, Headers: []kgo.RecordHeader{{Key: "argus-normalization-version", Value: []byte("v3")}}}, sender, request, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	keys := []string{sources[0].Key(), sources[1].Key()}
	scope := kql.Scope{EnterpriseID: enterprise, ResourceIDs: []uuid.UUID{cluster, appA, appB}, SourceKeys: keys}
	request := kql.Request{Expression: "*", Start: at.Add(-time.Minute), End: at.Add(time.Minute), Scope: scope, Budget: kql.Budget{MaxRows: 100, Timeout: 10 * time.Second}}
	logs, err := kql.Execute(ctx, conn, router, request)
	if err != nil || len(logs.Data) != 2 {
		t.Fatalf("Namespace raw rows: %+v %v", logs.Data, err)
	}
	for _, row := range logs.Data {
		body, _ := row["body"].(string)
		if strings.Contains(body, "namespace-b") || strings.Contains(body, "private-namespace-test-only") {
			t.Fatal("foreign or unredacted row returned")
		}
	}
	request.Expression = "* | stats count()"
	count, err := kql.Execute(ctx, conn, router, request)
	if err != nil || len(count.Data) != 1 {
		t.Fatalf("Namespace count: %+v %v", count.Data, err)
	}
	if len(count.Data[0]) != 1 {
		t.Fatalf("unexpected count shape: %+v", count.Data)
	}
	for _, value := range count.Data[0] {
		if fmt.Sprint(value) != "2" {
			t.Fatalf("foreign source influenced count: %v", value)
		}
	}
	candidates, err := (ClickHouseQuery{Conn: conn, Router: router}).DiscoverData(ctx, DataCatalogRequest{EnterpriseID: enterprise, ResourceIDs: scope.ResourceIDs, SourceKeys: keys, Signal: "logs", Kind: "values", Field: "body", From: request.Start, To: request.End, Limit: 20, Budget: queryengine.Budget{MaxRows: 100, MaxScanBytes: 1 << 30, MaxResultBytes: 8 << 20, Timeout: 10 * time.Second}})
	if err != nil || len(candidates.Values) != 2 {
		t.Fatalf("Namespace candidates: %+v %v", candidates.Values, err)
	}
	for _, value := range candidates.Values {
		if strings.Contains(value, "namespace-b") {
			t.Fatal("foreign Namespace influenced candidate lookup")
		}
	}
}
