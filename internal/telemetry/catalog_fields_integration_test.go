package telemetry

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

func testCatalogFields(t *testing.T, ctx context.Context, backend ClickHouseQuery, request DataCatalogRequest, tables TenantTables, host, source uuid.UUID, at time.Time) {
	t.Helper()
	for _, row := range []struct {
		host, source uuid.UUID
		at           time.Time
		key, service string
	}{
		{host, source, at, "code", "a"}, {host, source, at, "empty", "b"},
		{uuid.New(), source, at, "hidden_resource", "hidden"},
		{host, uuid.New(), at, "hidden_source", "hidden"},
		{host, source, at.Add(-time.Hour), "old_only", "old"},
	} {
		if err := backend.Conn.Exec(ctx, "INSERT INTO `"+tables.Logs+"` (resource_id,timestamp,service_name,structured_metadata,source_id,source_revision,event_id,expires_at) VALUES (?,?,?,?,?,1,?,now64(3)+INTERVAL 1 HOUR)", row.host, row.at, row.service, map[string]string{row.key: "200"}, row.source, uuid.NewString()); err != nil {
			t.Fatal(err)
		}
	}
	request.Kind, request.Search, request.Cursor = "fields", "", ""
	request.SelectedValues = nil
	request.Limit = 2
	fields := map[string]string{}
	for page := 0; ; page++ {
		data, err := backend.DiscoverData(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		if page > 20 {
			t.Fatal("field pagination did not end")
		}
		for _, f := range data.Fields {
			if _, exists := fields[f.Name]; exists {
				t.Fatal("field repeated across pages")
			}
			fields[f.Name] = f.Type
		}
		if !data.HasMore {
			break
		}
		if data.Complete || data.NextCursor == "" {
			t.Fatal("incomplete field catalog claimed complete")
		}
		request.Cursor = data.NextCursor
	}
	if fields["structured_metadata.code"] != "string" || fields["severity_number"] != "number" || fields["timestamp"] != "datetime" || fields["structured_metadata.hidden_resource"] != "" || fields["structured_metadata.hidden_source"] != "" || fields["structured_metadata.old_only"] != "" {
		t.Fatalf("field scope/types: %+v", fields)
	}
	request.Cursor = ""
	request.Limit = 100
	request.Search = "structured_metadata"
	request.Filters = []CatalogFilter{{Field: "service_name", Operator: "=", Values: []string{"a"}}}
	data, err := backend.DiscoverData(ctx, request)
	if err != nil || len(data.Fields) != 1 || data.Fields[0].Name != "structured_metadata.code" || data.Complete {
		t.Fatalf("filtered fields: %+v %v", data, err)
	}
	request.Kind = "values"
	request.Field = data.Fields[0].Name
	request.Search = ""
	data, err = backend.DiscoverData(ctx, request)
	if err != nil || !slices.Equal(data.Values, []string{"200"}) {
		t.Fatalf("discovered field not queryable: %+v %v", data, err)
	}
	request.Kind = "fields"
	request.Filters = nil
	request.From = at.Add(time.Hour)
	request.To = at.Add(2 * time.Hour)
	data, err = backend.DiscoverData(ctx, request)
	if err != nil || len(data.Fields) != 0 || !data.Complete {
		t.Fatalf("empty scope fabricated fields: %+v %v", data, err)
	}
	request.From = at.Add(-time.Second)
	request.To = at.Add(time.Second)
	request.Budget.MaxRows = 1
	_, err = backend.DiscoverData(ctx, request)
	if !errors.Is(err, queryengine.ErrBudget) && !errors.Is(err, ErrQueryBudget) {
		t.Fatalf("field budget ignored: %v", err)
	}
	request.Budget.MaxRows = 100
	if err := backend.Conn.Exec(ctx, "INSERT INTO `"+tables.Traces+"` (resource_id,source_id,source_revision,trace_id,span_id,start_time,span_kind,attributes,expires_at) VALUES (?,?,1,'trace','span',?,2,?,now64(3)+INTERVAL 1 HOUR)", host, source, at, map[string]string{"http.status_code": "200"}); err != nil {
		t.Fatal(err)
	}
	request.Signal = "traces"
	data, err = backend.DiscoverData(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(data.Fields, CatalogField{Name: "span_kind", Type: "number"}) || !slices.Contains(data.Fields, CatalogField{Name: "attributes.http.status_code", Type: "string"}) {
		t.Fatalf("trace types: %+v", data.Fields)
	}
	for field, want := range map[string]string{"source_id": source.String(), "resource_id": host.String()} {
		if !slices.Contains(data.Fields, CatalogField{Name: field, Type: "string"}) {
			t.Fatalf("missing scoped identity field %s", field)
		}
		values := request
		values.Kind, values.Field = "values", field
		values.SelectedValues = []string{want, uuid.NewString()}
		found, e := backend.DiscoverData(ctx, values)
		if e != nil || !slices.Equal(found.Values, []string{want}) || !found.Membership[want] || len(found.Membership) != 2 {
			t.Fatalf("scoped identity candidates: %+v %v", found, e)
		}
	}
	series := uuid.New()
	if err := backend.Conn.Exec(ctx, "INSERT INTO `"+tables.MetricSeries+"` (resource_id,series_id,metric_name,labels,source_id,source_revision,expires_at) VALUES (?,?,'catalog_cpu',?,?,1,now64(3)+INTERVAL 1 HOUR)", host, series, map[string]string{"region": "east"}, source); err != nil {
		t.Fatal(err)
	}
	if err := backend.Conn.Exec(ctx, "INSERT INTO `"+tables.MetricSamples+"` (resource_id,series_id,metric_name,timestamp,value,source_id,source_revision,expires_at) VALUES (?,?,'catalog_cpu',?,1,?,1,now64(3)+INTERVAL 1 HOUR)", host, series, at, source); err != nil {
		t.Fatal(err)
	}
	request.Signal = "metrics"
	request.Metric = "catalog_cpu"
	data, err = backend.DiscoverData(ctx, request)
	if err != nil || !slices.Contains(data.Fields, CatalogField{Name: "region", Type: "string"}) {
		t.Fatalf("metric fields: %+v %v", data, err)
	}
	request.Metric = "not_received"
	data, err = backend.DiscoverData(ctx, request)
	if err != nil || len(data.Fields) != 0 {
		t.Fatalf("metric without samples exposed fields: %+v %v", data, err)
	}
}
