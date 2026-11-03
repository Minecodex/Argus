package telemetry

import (
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/observationscope"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry/metricmeta"
)

func TestMetricQuantitySemanticsAreSourceTypeUnitAndScopeQualified(t *testing.T) {
	id := uuid.New()
	key := id.String() + ":2"
	for _, change := range []string{"none", "sdk", "unknown_metric", "wrong_type", "wrong_unit", "foreign_source", "nonhost", "not_selected"} {
		t.Run(change, func(t *testing.T) {
			input := metricmeta.Descriptor{Name: "system_memory_utilization", Type: "gauge", Unit: "1", SourceKey: key}
			row := db.ListObservationSourcesRow{ID: id, SourceType: "hostmetrics", ResourceType: "host"}
			keys := []string{key}
			switch change {
			case "sdk":
				row.SourceType = "otlp"
			case "unknown_metric":
				input.Name = "status_code"
			case "wrong_type":
				input.Type = "counter"
			case "wrong_unit":
				input.Unit = "By"
			case "foreign_source":
				input.SourceKey = uuid.NewString() + ":2"
			case "nonhost":
				row.ResourceType = "application"
			case "not_selected":
				keys = nil
			}
			snapshot := observationscope.Snapshot{Query: &observationscope.SourceResult{Keys: keys, Rows: []db.ListObservationSourcesRow{row}}}
			got := queryInputMetricSemantics([]metricmeta.Descriptor{input}, snapshot)
			if (len(got) == 1) != (change == "none") {
				t.Fatal("unit/name alone inherited ratio", change, got)
			}
			if len(got) > 0 && (got[0].SourceKey != key || got[0].Unit != "1" || got[0].Semantics.QuantityKind != "ratio") {
				t.Fatal("raw metadata changed", got)
			}
		})
	}
	for _, name := range []string{"system_cpu_utilization", "system_memory_utilization"} {
		if builtincollection.InputMetricSemantics("hostmetrics", name, "gauge", "1") == nil {
			t.Fatal("known builtin ratio missing", name)
		}
	}
	m := MetricDescriptor{Name: "system_memory_utilization", Type: "gauge", Unit: "1", SourceType: "hostmetrics", SourceKeys: []string{key}}
	if observedMetricSemantics(m, []ObservationSource{{SourceType: "hostmetrics", Keys: []string{key}}}) == nil || observedMetricSemantics(m, []ObservationSource{{SourceType: "otlp", Keys: []string{key}}}) != nil {
		t.Fatal("catalog semantics bypassed actual source binding")
	}
}
