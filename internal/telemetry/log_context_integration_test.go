package telemetry

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/kql"
)

func TestKQLClickHouseLogContextKeepsStreamAndTime(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx := context.Background()
	tenant, host, source, otherHost := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.DropTenant(ctx, tenant) })
	tables, _ := router.Tables(tenant)
	at := time.Now().UTC().Add(-time.Minute)
	insert := func(resource uuid.UUID, index int, file string) {
		t.Helper()
		if err := conn.Exec(ctx, "INSERT INTO `"+tables.Logs+"` (resource_id,source_id,source_revision,source_type,timestamp,body,event_id,service_name,stream_labels,structured_metadata,expires_at) VALUES (?,?,1,'otlp',?,?,?,'api',?,?,now64(3)+INTERVAL 1 HOUR)", resource, source, at.Add(time.Duration(index)*time.Second), fmt.Sprint(index), fmt.Sprintf("event-%d-%s", index, file), map[string]string{"service": "api"}, map[string]string{"log.file.path": file}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 6; i++ {
		insert(host, i, "main")
	}
	insert(host, 2, "other-file")
	insert(otherHost, 2, "main")
	request := kql.Request{Expression: `* | context "event-3-main" before 1 after 1`, Start: at.Add(-time.Second), End: at.Add(10 * time.Second), Scope: kql.Scope{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{host}, SourceKeys: []string{source.String() + ":1"}}, Budget: kql.Budget{MaxRows: 10, MaxScanBytes: 256 << 20, Timeout: 10 * time.Second}}
	result, err := kql.Execute(ctx, conn, router, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Data) != 3 || result.Partial || result.Data[0]["body"] != "2" || result.Data[1]["body"] != "3" || result.Data[2]["body"] != "4" {
		t.Fatalf("context ordering or stream boundary failed: %+v", result)
	}
	request.End = at.Add(4 * time.Second)
	result, err = kql.Execute(ctx, conn, router, request)
	if err != nil || len(result.Data) != 2 {
		t.Fatalf("context widened the time window: %+v %v", result, err)
	}
	request.Expression = `* | context "not-there" before 1 after 1`
	result, err = kql.Execute(ctx, conn, router, request)
	if err != nil || len(result.Data) != 0 {
		t.Fatalf("missing anchor became an unrestricted query: %+v %v", result, err)
	}
	// A busy authorized source must not be rescanned for every neighbor side.
	// The unrelated file shares resource, source, service and event time.
	const noiseRows = 8000
	if err := conn.Exec(ctx, "INSERT INTO `"+tables.Logs+"` (resource_id,source_id,source_revision,source_type,timestamp,body,event_id,service_name,stream_labels,structured_metadata,expires_at) SELECT ?,?,1,'otlp',?,repeat('x',4096),concat('noise-',toString(number)),'api',map('service','api'),map('log.file.path','other-file'),now64(3)+INTERVAL 1 HOUR FROM numbers(8000)", host, source, at.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	request.End = at.Add(10 * time.Second)
	request.Expression = `* | context "event-3-main" before 1 after 1`
	request.Budget.MaxScanBytes = 96 << 20
	result, err = kql.Execute(ctx, conn, router, request)
	if err != nil || len(result.Data) != 3 || result.ScannedRows > 3*(noiseRows+8) || result.ScannedBytes > request.Budget.MaxScanBytes {
		t.Fatalf("context repeated a scoped dataset scan: rows=%d bytes=%d records=%d err=%v", result.ScannedRows, result.ScannedBytes, len(result.Data), err)
	}
	for _, side := range []string{"a", "z"} {
		if err := conn.Exec(ctx, "INSERT INTO `"+tables.Logs+"` (resource_id,source_id,source_revision,source_type,timestamp,body,event_id,service_name,stream_labels,structured_metadata,expires_at) VALUES (?,?,1,'otlp',?,?,?,'api',?,?,now64(3)+INTERVAL 1 HOUR)", host, source, at.Add(3*time.Second), "tie-"+side, "event-3-"+side, map[string]string{"service": "api"}, map[string]string{"log.file.path": "main"}); err != nil {
			t.Fatal(err)
		}
	}
	result, err = kql.Execute(ctx, conn, router, request)
	if err != nil || len(result.Data) != 3 || result.Data[0]["body"] != "tie-a" || result.Data[1]["body"] != "3" || result.Data[2]["body"] != "tie-z" {
		t.Fatalf("context event-id ties are unstable: %+v %v", result.Data, err)
	}
	request.Expression = `* | context "event-3-main" before 0 after 0`
	result, err = kql.Execute(ctx, conn, router, request)
	if err != nil || len(result.Data) != 1 || result.Data[0]["body"] != "3" {
		t.Fatalf("zero neighbors did not retain exactly the anchor: %+v %v", result.Data, err)
	}
}
