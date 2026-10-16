package tooltemplatefixture

import (
	"math"
	"time"

	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
)

// These controlled observations use Prometheus' production Matrix serializer,
// then the same bounded view used by detail and series-page reads.
func metricSamplingScenarios() map[string]Fixture {
	out := map[string]Fixture{}
	for _, kind := range []string{"peak", "gap", "gap-budget", "multi"} {
		matrix := promql.Matrix{}
		groups := 1
		if kind == "multi" {
			groups = 2
		}
		for group := 0; group < groups; group++ {
			points := make([]promql.FPoint, 600)
			for i := range points {
				points[i] = promql.FPoint{T: now.Add(time.Duration(i-600) * time.Second).UnixMilli(), F: 1}
			}
			points[2].F = 10000
			switch kind {
			case "peak", "multi":
				points[1].F = math.NaN()
			case "gap":
				points[3].F = math.NaN()
			case "gap-budget":
				for i := 1; i < 600; i += 2 {
					points[i].F = math.NaN()
				}
			}
			database := "orders"
			if group == 1 {
				database = "payments"
			}
			matrix = append(matrix, promql.Series{Metric: labels.FromStrings("__name__", "template_sampling_"+kind, "argus_resource_id", host.String(), "db_name", database), Floats: points})
		}
		data := object(map[string]any{"query_context": map[string]any{"query": "template_sampling_" + kind, "from": now.Add(-600 * time.Second), "to": now.Add(-time.Second), "step_seconds": 1}, "resources": []any{map[string]any{"id": host, "name": "db-prod-01", "type": "host"}}, "data": map[string]any{"resultType": "matrix", "result": matrix}})
		out["metric-sampling-"+kind] = Fixture{Tool: "telemetry.promql.query", Data: presentation.ViewData(data), Expected: []string{"template_sampling_" + kind, "db-prod-01"}}
		dashboard := Normal("dashboard-result")
		dashboard["panels"] = []any{map[string]any{"id": "sampling", "name": "Frozen metric sampling " + kind, "type": "timeseries", "signal": "metrics", "status": "success", "legend": true, "layout": map[string]any{"x": 0, "y": 0, "w": 12, "h": 1}, "targets": []any{map[string]any{"id": "metric", "status": "success", "result_type": "matrix", "data": data["data"]}}}}
		out["dashboard-sampling-"+kind] = Fixture{Tool: "telemetry.dashboard.query", Data: presentation.ViewData(dashboard), Expected: []string{"Frozen metric sampling " + kind}}
	}
	return out
}
