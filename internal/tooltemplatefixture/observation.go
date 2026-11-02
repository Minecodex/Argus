package tooltemplatefixture

import (
	"fmt"
	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/telemetry"
)

func observationFixture(tool string) map[string]any {
	ref := map[string]any{"type": "monitoring_target", "id": host.String()}
	switch tool {
	case "observe.resources.list":
		return map[string]any{"items": []any{map[string]any{"ref": ref, "id": host.String(), "name": "orders-mysql", "type": "monitoring_target", "environment": "production", "family": "database", "integrations": []string{"mysql"}}}, "complete": true, "has_more": false}
	case "observe.capabilities.get":
		return map[string]any{"items": []any{map[string]any{"ref": ref, "id": host.String(), "name": "orders-mysql", "type": "monitoring_target", "sections": map[string]any{
			"collection_state": map[string]any{"status": "available", "complete": true, "coverage": "collection_instances_only", "scope_note": "Collection instances only, not application SDK endpoints or all signal sources.", "items": []any{map[string]any{"name": "MySQL collection", "configured_collector_id": "a1111111-1111-4111-8111-111111111111", "effective_collector_id": "b2222222-2222-4222-8222-222222222222", "placement_state": "requires_authorized_resolution", "desired_enabled": true, "desired_revision": 4, "effective_revision": 3, "health": "unknown", "freshness": "no_data"}}},
			"sources":          map[string]any{"status": "available", "complete": true, "entry_kind": "source_version_selectors", "page_source_count": 1, "page_entry_count": 3, "items": sourceVersionFixtures()},
			"dashboards":       map[string]any{"status": "unavailable", "complete": false, "items": []any{}},
		}}}, "complete": false, "sampling_performed": false, "data_state": "not_checked", "data_state_reason": "This capability request reads metadata only; no telemetry sample query was performed. This does not prove historical data absence."}
	case "observe.catalog.query":
		return map[string]any{"metrics": []any{map[string]any{"name": "mysql_threads", "description": "Database threads", "type": "gauge", "unit": "threads", "source_type": "mysql", "observed": true, "recipes": []any{map[string]any{"expression": "mysql_threads", "unit": "threads"}}}, histogramMethodFixture("request_duration", "histogram", "s"), histogramMethodFixture("request_size", "exponential_histogram", "By"), map[string]any{"name": "system_memory_utilization", "type": "gauge", "unit": "1", "source_type": "hostmetrics", "observed": true, "semantics": builtincollection.InputMetricSemantics("hostmetrics", "system_memory_utilization", "gauge", "1")}}, "fields": []any{}, "values": []any{}, "sources": []any{}, "complete": true, "has_more": false}
	}
	return nil
}

func histogramMethodFixture(name, kind, unit string) map[string]any {
	metric := telemetry.MetricDescriptor{Name: name, Type: kind, Unit: unit, SourceType: "otlp"}
	return map[string]any{"name": name, "type": kind, "unit": unit, "source_type": "otlp", "observed": true, "query_methods": telemetry.ObservationMethodsForMetric(metric)}
}

func sourceVersionFixtures() []any {
	const id = "c3333333-3333-4333-8333-333333333333"
	rows := []any{}
	for revision := 1; revision <= 3; revision++ {
		rows = append(rows, map[string]any{"source_id": id, "source_key": fmt.Sprintf("%s:%d", id, revision), "source_type": "mysql", "config_revision": revision, "entry_kind": "source_version_selector", "installation_scope": "collector_generation_only", "current_installation": true, "data_state": "not_checked", "sampling_performed": false})
	}
	return rows
}
