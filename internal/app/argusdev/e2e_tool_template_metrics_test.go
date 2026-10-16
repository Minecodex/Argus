package argusdev

import (
	"context"
	"testing"
	"time"

	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
)

func TestMetricSamplingFixturesExecuteInPrometheus(t *testing.T) {
	engine := promql.NewEngine(promql.EngineOpts{MaxSamples: 100000, Timeout: time.Minute, EnableAtModifier: true, EnableNegativeOffset: true})
	for _, kind := range []string{"peak", "gap", "gap-budget"} {
		t.Run(kind, func(t *testing.T) {
			// Frozen relative dashboard windows retain subsecond timestamps.
			// Equality against time()%600 would never select spikes or gaps.
			start := time.Unix(600, 137000000)
			query, err := engine.NewRangeQuery(context.Background(), &storage.MockQueryable{MockQuerier: &storage.MockQuerier{}}, promql.NewPrometheusQueryOpts(false, time.Minute), templateSamplingExpression(kind), start, start.Add(599*time.Second), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer query.Close()
			result := query.Exec(context.Background())
			if result.Err != nil {
				t.Fatal(result.Err)
			}
			matrix := result.Value.(promql.Matrix)
			if len(matrix) != 1 || len(matrix[0].Floats) != 600 {
				t.Fatal("invalid actual samples")
			}
			data := presentation.ViewData(map[string]any{"data": map[string]any{"result": matrix}})
			proof, err := validateTemplateMetricPreview(data)
			if err != nil || len(proof) != 1 {
				t.Fatal(proof, err)
			}
		})
	}
}
