package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/kql"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
	"github.com/twmb/franz-go/pkg/kgo"
	logsrpc "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	rpc "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	common "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func TestAPMStableServiceIdentitySeparatesSameNamedNamespaces(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx := t.Context()
	tenant, app, source := uuid.New(), uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.DropTenant(context.Background(), tenant); err != nil {
			t.Error(err)
		}
	})
	at := time.Now().UTC().Add(-time.Minute)
	ids := []uuid.UUID{uuid.New(), uuid.New()}
	request := &rpc.ExportTraceServiceRequest{}
	for i, namespace := range []string{"payments", "inventory"} {
		attrs := []*common.KeyValue{{Key: "service.name", Value: stringValue("api")}, {Key: "service.namespace", Value: stringValue(namespace)}, {Key: "argus.service.id", Value: stringValue(ids[i].String())}, {Key: "argus.source.id", Value: stringValue(source.String())}, {Key: "argus.source.revision", Value: stringValue("1")}, {Key: "argus.source.type", Value: stringValue("otlp")}}
		spanID := []byte("span0001")
		spanID[7] += byte(i)
		request.ResourceSpans = append(request.ResourceSpans, &tracepb.ResourceSpans{Resource: &resourcepb.Resource{Attributes: attrs}, ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{{TraceId: []byte("0123456789abcdef"), SpanId: spanID, Name: "GET /", Kind: tracepb.Span_SPAN_KIND_SERVER, StartTimeUnixNano: uint64(at.UnixNano()), EndTimeUnixNano: uint64(at.Add(10 * time.Millisecond).UnixNano())}}}}})
	}
	if _, err := (&Writer{ClickHouse: conn, Router: router}).writeTraces(ctx, &kgo.Record{Topic: "otlp-traces", Offset: 1}, TrustedIdentity{EnterpriseID: tenant, ResourceID: app, ResourceType: "application", CollectorID: uuid.New()}, request, time.Hour); err != nil {
		t.Fatal(err)
	}
	logRequest := &logsrpc.ExportLogsServiceRequest{}
	for _, block := range request.ResourceSpans {
		logRequest.ResourceLogs = append(logRequest.ResourceLogs, &logspb.ResourceLogs{Resource: block.Resource, ScopeLogs: []*logspb.ScopeLogs{{LogRecords: []*logspb.LogRecord{{TimeUnixNano: uint64(at.UnixNano()), Body: stringValue("same-named service log")}}}}})
	}
	if _, err := (&Writer{ClickHouse: conn, Router: router}).writeLogs(ctx, &kgo.Record{Topic: "otlp-logs", Offset: 1}, TrustedIdentity{EnterpriseID: tenant, ResourceID: app, ResourceType: "application", CollectorID: uuid.New()}, logRequest, time.Hour); err != nil {
		t.Fatal(err)
	}
	logResult, err := kql.Execute(ctx, conn, router, kql.Request{Expression: `resource_attributes.argus.service.id = "` + ids[1].String() + `"`, Pipeline: "limit 100", Start: at.Add(-time.Second), End: at.Add(time.Second), Scope: kql.Scope{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{app}, SourceKeys: []string{source.String() + ":1"}}, Budget: kql.Budget{MaxRows: 100, MaxScanBytes: 256 << 20, Timeout: 10 * time.Second}})
	if err != nil || len(logResult.Data) != 1 {
		t.Fatalf("application log service identity filter: rows=%+v err=%v", logResult.Data, err)
	}
	engine := skywalking.Engine{Conn: conn, Router: router}
	query := skywalking.Request{Document: `query{queryAPMServices{status coverage{observedSpanCount requestSampleCount} rows{serviceId serviceNamespace serviceName sampleCount}}}`, Start: at.Add(-time.Second), End: at.Add(time.Second), Scope: skywalking.Scope{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{app}, SourceKeys: []string{source.String() + ":1"}}, Budget: skywalking.Budget{MaxRows: 100, MaxScanBytes: 256 << 20, Timeout: 10 * time.Second}}
	result, err := engine.Execute(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	rows := result.Data["queryAPMServices"].(map[string]any)["rows"].([]any)
	if len(rows) != 2 {
		tables, _ := router.Tables(tenant)
		var count uint64
		_ = conn.QueryRow(ctx, "SELECT count() FROM "+tables.Traces).Scan(&count)
		var resources, keys, services []string
		var first, last time.Time
		var matched uint64
		matchErr := conn.QueryRow(ctx, "SELECT count() FROM "+tables.Traces+" WHERE start_time>=? AND start_time<? AND resource_id IN (?) AND source_key IN (?)", query.Start, query.End, query.Scope.ResourceIDs, query.Scope.SourceKeys).Scan(&matched)
		var timesOnly, resourcesOnly, keysOnly uint64
		_ = conn.QueryRow(ctx, "SELECT countIf(start_time>=? AND start_time<?),countIf(resource_id IN (?)),countIf(source_key IN (?)) FROM "+tables.Traces, query.Start, query.End, query.Scope.ResourceIDs, query.Scope.SourceKeys).Scan(&timesOnly, &resourcesOnly, &keysOnly)
		t.Logf("predicate counts time=%d resource=%d keys=%d expected scope=%+v", timesOnly, resourcesOnly, keysOnly, query.Scope)
		var resourceWhere, timeWhere, keyWhere, noPrewhere uint64
		_ = conn.QueryRow(ctx, "SELECT count() FROM "+tables.Traces+" WHERE resource_id IN (?)", query.Scope.ResourceIDs).Scan(&resourceWhere)
		_ = conn.QueryRow(ctx, "SELECT count() FROM "+tables.Traces+" WHERE start_time>=? AND start_time<?", query.Start, query.End).Scan(&timeWhere)
		_ = conn.QueryRow(ctx, "SELECT count() FROM "+tables.Traces+" WHERE source_key IN (?)", query.Scope.SourceKeys).Scan(&keyWhere)
		_ = conn.QueryRow(ctx, "SELECT count() FROM "+tables.Traces+" WHERE start_time>=? AND start_time<? AND resource_id IN (?) AND source_key IN (?) SETTINGS optimize_move_to_prewhere=0, use_skip_indexes=0", query.Start, query.End, query.Scope.ResourceIDs, query.Scope.SourceKeys).Scan(&noPrewhere)
		t.Logf("WHERE resource=%d time=%d key=%d no-prewhere=%d", resourceWhere, timeWhere, keyWhere, noPrewhere)
		var nanosWhere uint64
		nanosErr := conn.QueryRow(ctx, "SELECT count() FROM "+tables.Traces+" WHERE start_time>=fromUnixTimestamp64Nano(?) AND start_time<fromUnixTimestamp64Nano(?)", query.Start.UnixNano(), query.End.UnixNano()).Scan(&nanosWhere)
		t.Logf("explicit DateTime64 nanos boundary: count=%d error=%v", nanosWhere, nanosErr)
		diagnosticErr := conn.QueryRow(ctx, "SELECT groupUniqArray(toString(resource_id)),groupUniqArray(source_key),groupUniqArray(service_name),min(start_time),max(start_time) FROM "+tables.Traces).Scan(&resources, &keys, &services, &first, &last)
		t.Fatalf("same named services merged: result=%+v raw_count=%d matched=%d matchErr=%v resources=%v keys=%v services=%v times=%v/%v expected=%v/%v diagnostic=%v", result.Data, count, matched, matchErr, resources, keys, services, first, last, query.Start, query.End, diagnosticErr)
	}
	seen := map[string]string{}
	for _, raw := range rows {
		row := raw.(map[string]any)
		seen[row["serviceNamespace"].(string)] = row["serviceId"].(string)
		if row["sampleCount"] != float64(1) {
			t.Fatal("namespace grouping changed sample count")
		}
	}
	if seen["payments"] != ids[0].String() || seen["inventory"] != ids[1].String() {
		t.Fatal("stable service identities lost")
	}
	topologyQuery := query
	topologyQuery.Document = skywalking.ApplicationAPMQueries()["topology"]
	topologyQuery.Variables = map[string]any{"serviceIds": nil}
	topology, topologyErr := engine.Execute(ctx, topologyQuery)
	if topologyErr != nil {
		t.Fatal(topologyErr)
	}
	if nodes := topology.Data["queryAPMTopology"].(map[string]any)["nodes"].([]any); len(nodes) != 2 {
		t.Fatalf("topology merged same-named namespaces: %+v", nodes)
	}
	query.Document = `query($id:[String!]){queryAPMServices(filters:[{resource:true,key:"argus.service.id",values:$id}]){rows{serviceId serviceNamespace sampleCount}}}`
	query.Variables = map[string]any{"id": []any{ids[1].String()}}
	result, err = engine.Execute(ctx, query)
	if err != nil {
		t.Fatalf("service filter: %v %+v", err, result.Errors)
	}
	rows = result.Data["queryAPMServices"].(map[string]any)["rows"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["serviceId"] != ids[1].String() {
		t.Fatal("stable service filter broadened")
	}
}
