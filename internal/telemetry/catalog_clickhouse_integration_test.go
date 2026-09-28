package telemetry

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

func TestCatalogClickHousePaginationAndPredicates(t *testing.T) {
	address := os.Getenv("ARGUS_CLICKHOUSE_TEST_ADDRESS")
	if address == "" {
		t.Skip("ClickHouse endpoint required")
	}
	conn, err := OpenClickHouse(address, "argus_telemetry", "argus", os.Getenv("ARGUS_CLICKHOUSE_TEST_PASSWORD"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	ctx := context.Background()
	tenant, host, source := uuid.New(), uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.DropTenant(ctx, tenant) })
	tables, _ := router.Tables(tenant)
	at := time.Now().UTC().Add(-time.Minute)
	for i, name := range []string{"a", "b", "c"} {
		if err := conn.Exec(ctx, "INSERT INTO `"+tables.Logs+"` (resource_id,timestamp,service_name,severity_number,event_id,expires_at,source_id,source_revision) VALUES (?,?,?,?,?,now64(3)+INTERVAL 1 HOUR,?,1)", host, at, name, uint8(i+1), name, source); err != nil {
			t.Fatal(err)
		}
	}
	request := DataCatalogRequest{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{host}, SourceKeys: []string{source.String() + ":1"}, Signal: "logs", Kind: "values", Field: "service_name", From: at.Add(-time.Second), To: at.Add(time.Second), Limit: 1, SelectedValues: []string{"a", "c", "gone"}, Budget: queryengine.Budget{MaxScanBytes: DefaultMaxScanBytes, MaxRows: 10, MaxResultBytes: 1 << 20}}
	backend := ClickHouseQuery{Conn: conn, Router: router}
	for i, name := range []string{"a", "b", "c"} {
		data, err := backend.DiscoverData(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		if len(data.Values) != 1 || data.Values[0] != name || data.Complete || data.HasMore != (i < 2) || !data.Membership["a"] || !data.Membership["c"] || data.Membership["gone"] {
			t.Fatalf("bad page: %+v", data)
		}
		request.Cursor = data.NextCursor
	}
	request.Cursor = ""
	request.Filters = []CatalogFilter{{Field: "severity_number", Operator: ">=", Values: []string{"2"}}, {Field: "service_name", Operator: "!=", Values: []string{"b"}}}
	data, err := backend.DiscoverData(ctx, request)
	if err != nil || len(data.Values) != 1 || data.Values[0] != "c" || !data.Complete || data.Membership["a"] {
		t.Fatalf("typed upstream filters failed: %+v %v", data, err)
	}
	request.Filters = nil
	request.Search = "none"
	data, err = backend.DiscoverData(ctx, request)
	if err != nil || data.Complete || len(data.Values) != 0 || !data.Membership["a"] {
		t.Fatalf("search presented as whole candidate set: %+v %v", data, err)
	}
	testCatalogFields(t, ctx, backend, request, tables, host, source, at)
}
