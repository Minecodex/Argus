package tooltemplatefixture

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/presentation"
	"strings"
	"time"
)

var host = uuid.MustParse("10000000-0000-4000-8000-000000000001")
var cluster = uuid.MustParse("10000000-0000-4000-8000-000000000002")
var now = time.Date(2026, 10, 13, 1, 30, 0, 0, time.UTC)

func object(value any) map[string]any {
	raw, _ := json.Marshal(value)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}
func series() []any {
	rows := []any{}
	for index := 0; index < 3; index++ {
		id := host.String()
		database := "orders"
		if index == 1 {
			database = "warehouse"
		}
		if index == 2 {
			id = cluster.String()
		}
		rows = append(rows, map[string]any{"metric": map[string]any{"__name__": "db_connections", "resource_id": id, "database": database, "service_name": "orders-api", "unit": "connections", "source_key": "source-db-1"}, "values": []any{[]any{1791853200, "12"}, []any{1791853230, "18"}, []any{1791853260, "15"}}})
	}
	return rows
}
func Normal(entry string) map[string]any {
	resources := []map[string]any{{"id": host.String(), "type": "host", "name": "db-prod-01"}, {"id": cluster.String(), "type": "kubernetes_cluster", "name": "orders-production"}}
	base := map[string]any{"resources": resources, "observed_at": now, "partial": false}
	item := map[string]any{"id": host, "name": "db-prod-01", "hostname": "db-prod-01.internal", "platform": "linux", "address": "10.0.10.24", "status": "ready", "connection_status": "connected", "resource_version": 3, "namespace": "orders", "resource_id": host, "last_heartbeat_at": now, "labels": map[string]string{"env": "production"}, "inventory": map[string]any{"observed_at": now, "operating_system": "Ubuntu 24.04", "cpu_cores": 8, "memory_bytes": 16 << 30, "collection_status": "observed"}, "desired_version": 3, "observed_version": 3, "convergence_status": "converged"}
	query := map[string]any{"query": "db_connections{database=~\"orders|warehouse\"}", "from": now.Add(-30 * time.Minute), "to": now, "step_seconds": 30}
	switch {
	case entry == "metric-query":
		base["query_context"], base["data"], base["status"] = query, map[string]any{"resultType": "matrix", "result": series()}, "success"
	case entry == "log-query":
		query["query"] = "* | limit 10"
		base["query_context"], base["result_type"], base["data"] = query, "log_entries", []any{map[string]any{"timestamp": now, "severity_text": "ERROR", "service_name": "orders-api", "resource_id": host, "body": "Database connection timed out <retry> & recover", "trace_id": "trace-001", "span_id": "span-001"}}
	case entry == "pod-logs":
		base["namespace"], base["pod"], base["container"], base["cluster_id"], base["content"], base["bytes"] = "orders", "orders-api-7cc8b", "api", cluster, "INFO request started\nERROR database timeout\nINFO connection recovered", 72
	case entry == "trace-result":
		delete(query, "query")
		query["document"] = "query { queryTrace(traceId: \"trace-001\") { traceId rootService rootOperation startTime duration spanCount errorCount spans { spanId parentSpanId serviceName operationName startTime duration status } } }"
		base["query_context"] = query
		base["data"] = map[string]any{"queryTrace": map[string]any{"traceId": "trace-001", "rootService": "orders-api", "rootOperation": "POST /orders", "startTime": "2026-10-13T01:00:00Z", "duration": 43, "spanCount": 2, "errorCount": 1, "completeness": "complete", "spans": []any{map[string]any{"spanId": "a", "sourceId": "20000000-0000-4000-8000-000000000001", "resourceId": host.String(), "serviceName": "orders-api", "operationName": "POST /orders", "startTime": "2026-10-13T01:00:00Z", "duration": 43, "status": "error", "isError": true}, map[string]any{"spanId": "b", "parentSpanId": "a", "sourceId": "20000000-0000-4000-8000-000000000001", "resourceId": host.String(), "serviceName": "postgresql", "operationName": "SELECT orders", "startTime": "2026-10-13T01:00:00.005Z", "duration": 25, "status": "error", "isError": true}}}}
	case entry == "telemetry-summary":
		for key, value := range map[string]any{"resource_count": 2, "healthy_collectors": 2, "degraded_collectors": 0, "metric_points": 240, "log_records": 82, "spans": 15, "window_seconds": 1800} {
			base[key] = value
		}
	case entry == "change-preview" || entry == "action-detail" || entry == "action-list":
		for key, value := range map[string]any{"title": "Update orders host labels", "summary": "Change environment label on db-prod-01", "risk": "write", "status": "awaiting_confirmation", "action_type": "host.update", "preview": map[string]any{"name": "db-prod-01", "affected_resources": 1, "labels": map[string]string{"env": "staging"}, "changes": []any{map[string]any{"path": "labels.env", "before": "production", "after": "staging"}}}, "diff": []any{map[string]any{"kind": "change", "text": "Update host labels"}}, "expires_at": now.Add(10 * time.Minute)} {
			base[key] = value
		}
		if entry == "action-list" {
			base["items"] = []any{object(base)}
		}
	case strings.HasPrefix(entry, "alert-"):
		return alertFixture(entry, "policy")
	case strings.HasPrefix(entry, "dashboard-") || entry == "telemetry-catalog":
		spec := dashboard.Spec{SchemaVersion: dashboard.SchemaVersion, Layout: dashboard.Grid{Columns: 12, RowHeight: 40}, DefaultTimeRange: dashboard.TimeRange{Kind: "relative", Seconds: 1800}, Panels: []dashboard.Panel{}}
		types := []string{"timeseries", "stat", "bar", "pie", "gauge", "bar_gauge", "histogram", "heatmap", "scatter", "state_timeline", "table"}
		panels := []any{}
		for index, kind := range types {
			id := "panel-" + kind
			definition := dashboard.Panel{ID: id, Title: "Database · " + kind, Signal: "metrics", Type: kind, Unit: "connections", Decimals: 1, Legend: true, Layout: dashboard.Rectangle{X: (index % 2) * 6, Y: (index / 2) * 8, W: 6, H: 8}, Thresholds: []dashboard.Threshold{{Value: 17, Tone: "warning"}}}
			definition.SourceBinding = dashboard.SourceBinding{SourceType: "prometheus", CapabilityVersion: "v1"}
			definition.Targets = []dashboard.Target{{ID: "q", Language: "promql", QueryMode: "range", SourceDefinition: dashboard.Definition{DSL: &dashboard.DSL{Expression: "db_connections{database=~\"orders|warehouse\"}"}}, RangeStepPolicy: dashboard.StepPolicy{Kind: "fixed", Seconds: 30}}}
			spec.Panels = append(spec.Panels, definition)
			panel := object(definition)
			panel["name"] = definition.Title
			panel["status"] = "success"
			rows := series()
			if kind == "histogram" {
				rows = []any{map[string]any{"metric": map[string]string{"__name__": "latency_bucket", "le": "10"}, "value": []any{1791853200, "3"}}, map[string]any{"metric": map[string]string{"__name__": "latency_bucket", "le": "+Inf"}, "value": []any{1791853200, "8"}}}
			}
			panel["targets"] = []any{map[string]any{"id": "q", "status": "success", "result_type": "matrix", "step_seconds": 30, "data": map[string]any{"result": rows}}}
			panels = append(panels, panel)
		}
		base["name"], base["id"], base["revision_number"], base["spec"] = "Database overview", uuid.NewSHA1(host, []byte("dashboard")), 3, object(spec)
		if entry == "dashboard-result" {
			base["panels"], base["layout"], base["job_id"], base["attempt_id"], base["status"], base["from"], base["to"], base["timezone"], base["variables"] = panels, spec.Layout, "query-001", "attempt-001", "complete", now.Add(-30*time.Minute), now, "Asia/Shanghai", map[string]any{"database": map[string]any{"values": []string{"orders"}}}
		}
		if entry == "dashboard-list" || entry == "telemetry-catalog" {
			base["items"] = []any{map[string]any{"id": host, "name": "Database overview", "description": "CPU / database / storage", "revision_number": 3, "updated_at": now}}
		}
		if entry == "dashboard-draft-validation" {
			base["status"], base["sample_status"], base["issues"] = "invalid", "not_sampled", []any{map[string]any{"path": "panels[0].targets[0]", "severity": "error", "message": "Metric source binding is required"}}
		}
	case entry == "tool-search" || entry == "tool-description":
		row := map[string]any{"name": "get", "category": "host", "risk": "read", "version": "host.get/v1", "discovery": map[string]any{"title": "主机获取 / Get host", "description": "Read host inventory and collection scope"}, "input_schema": map[string]any{"type": "object", "required": []string{"host_id"}, "properties": map[string]any{"host_id": map[string]any{"type": "string", "format": "uuid"}}}}
		if entry == "tool-description" {
			return row
		}
		base["items"] = []any{row}
		base["complete"] = true
	default:
		if strings.HasSuffix(entry, "-detail") {
			if entry == "collector-detail" {
				base["collector"] = item
			} else {
				for key, value := range item {
					base[key] = value
				}
			}
		} else {
			base["items"] = []any{item}
		}
	}
	result := presentation.PublicSnapshot(base)
	return result
}
