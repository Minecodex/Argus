package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
)

func TestTraceSystemBudgetIsPartialButAuthoredPageIsPreserved(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx := context.Background()
	tenant, host, source := uuid.New(), uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	defer manager.DropTenant(ctx, tenant)
	tables, _ := router.Tables(tenant)
	at := time.Now().UTC().Add(-time.Minute)
	for _, id := range []string{"first", "second"} {
		if err := conn.Exec(ctx, "INSERT INTO `"+tables.Traces+"` (resource_id,source_id,source_revision,source_type,trace_id,span_id,service_name,operation,start_time,end_time,span_kind,status,expires_at) VALUES (?,?,1,'otlp',?,'root','api','request',?,?,2,'ok',now64(3)+INTERVAL 1 HOUR)", host, source, id, at, at.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	engine := queryengine.TraceEngine{Engine: skywalking.Engine{Conn: conn, Router: router}}
	request := queryengine.Request{Language: queryengine.LanguageTrace, Expression: `{queryTraces {total traces {traceId}}}`, Start: at.Add(-time.Second), End: at.Add(time.Minute), Scope: queryengine.Scope{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{host}, SourceKeys: []string{source.String() + ":1"}}, Budget: queryengine.Budget{MaxRows: 1, MaxScanBytes: DefaultMaxScanBytes}}
	result, err := engine.Execute(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Meta.Partial || len(result.Meta.Warnings) == 0 {
		t.Fatal("implicit budget truncation presented as complete")
	}
	request.Expression = `{queryTraces(pageSize:1){total traces{traceId}}}`
	result, err = engine.Execute(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Meta.Partial {
		t.Fatal("explicit authored page was changed into a wider query")
	}
	request.Expression = `{queryTraces(pageNum:5){total traces{traceId}}}`
	result, err = engine.Execute(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Meta.Partial {
		t.Fatal("an explicitly empty page claimed budget truncation")
	}
}
