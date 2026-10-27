package argusdev

import (
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/monitoring"
	"testing"
)

func TestDefaultAggregationRequiresTrustedExecutionScopeAndRealAggregate(t *testing.T) {
	target, collection, source := uuid.New(), uuid.New(), uuid.New()
	row := monitoring.Collection{ID: collection}
	row.Target.ID = target
	makeExecution := func() map[string]any {
		return map[string]any{"scope_signature": "verified-scope", "scope_empty": false, "resources": []any{map[string]any{"type": "monitoring_target", "id": target.String()}}, "panels": []any{map[string]any{"status": "success", "sources": []any{map[string]any{"id": source.String(), "revision": 1, "resource_type": "monitoring_target", "resource_id": target.String()}}}}}
	}
	if err := plan8DefaultViewScope(makeExecution(), row, []string{source.String() + ":1"}); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(map[string]any){func(x map[string]any) { x["scope_signature"] = "" }, func(x map[string]any) {
		x["resources"] = []any{map[string]any{"type": "monitoring_target", "id": uuid.NewString()}}
	}, func(x map[string]any) {
		x["panels"].([]any)[0].(map[string]any)["sources"] = []any{map[string]any{"id": uuid.NewString(), "revision": 1, "resource_id": target.String(), "resource_type": "monitoring_target"}}
	}} {
		x := makeExecution()
		change(x)
		if err := plan8DefaultViewScope(x, row, []string{source.String() + ":1"}); err == nil {
			t.Fatal("untrusted default view scope passed")
		}
	}
	if !plan8DefaultTargetAggregates("http", "success", "a") || !plan8DefaultTargetAggregates("tls", "minimum_days", "a") || plan8DefaultTargetAggregates("http", "duration", "a") || plan8DefaultTargetAggregates("http", "success", "other") {
		t.Fatal("frozen query aggregation classification incorrect")
	}
	data := map[string]any{"metric": map[string]any{}, "value": []any{1, "1"}}
	if _, err := plan8ExtraMetricValues(data, target.String(), collection.String()); err == nil {
		t.Fatal("raw series lost identity labels")
	}
	if values, err := plan8MetricValues(data, target.String(), collection.String(), true); err != nil || len(values) != 1 {
		t.Fatal("verified scoped aggregate was rejected", err)
	}
	data["metric"] = map[string]any{"argus_resource_id": uuid.NewString(), "argus_collection_id": collection.String()}
	if _, err := plan8MetricValues(data, target.String(), collection.String(), true); err == nil {
		t.Fatal("aggregation excused foreign identity labels")
	}
}
