package collectormanager_test

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	promengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
	metricapi "go.opentelemetry.io/otel/metric"
)

// Instruments are scoped to this acceptance scenario. They use the same actual
// SDK exporter/Collector/ingest pipeline as the application's existing counter.
func emitObservationMetricKinds(t *testing.T, f *nativeSDKPipeline, app *actualSDKService) {
	t.Helper()
	meter := app.metrics.Meter("plan11.actual-metric-semantics")
	counter, err := meter.Int64Counter("plan11_sdk_completed", metricapi.WithUnit("{request}"))
	if err != nil {
		t.Fatal(err)
	}
	nativeHistogram, err := meter.Float64Histogram("plan11_sdk_native_latency", metricapi.WithUnit("s"))
	if err != nil {
		t.Fatal(err)
	}
	gauge, err := meter.Float64Gauge("plan11_sdk_queue", metricapi.WithUnit("{request}"))
	if err != nil {
		t.Fatal(err)
	}
	histogram, err := meter.Float64Histogram("plan11_sdk_latency", metricapi.WithUnit("s"), metricapi.WithExplicitBucketBoundaries(.1, .5, 1))
	if err != nil {
		t.Fatal(err)
	}
	variantA, err := app.metrics.Meter("plan11.variant-a").Float64Gauge("plan11_sdk_variant", metricapi.WithUnit("By"))
	if err != nil {
		t.Fatal(err)
	}
	variantB, err := app.metrics.Meter("plan11.variant-b").Float64Gauge("plan11_sdk_variant", metricapi.WithUnit("s"))
	if err != nil {
		t.Fatal(err)
	}
	variantC, err := app.metrics.Meter("plan11.variant-c").Int64Counter("plan11_sdk_variant", metricapi.WithUnit("By"))
	if err != nil {
		t.Fatal(err)
	}
	variantA.Record(f.ctx, 1024)
	variantB.Record(f.ctx, .4)
	variantC.Add(f.ctx, 64)
	counter.Add(f.ctx, 2)
	gauge.Record(f.ctx, 9)
	histogram.Record(f.ctx, .1)
	histogram.Record(f.ctx, .3)
	nativeHistogram.Record(f.ctx, .1)
	nativeHistogram.Record(f.ctx, .3)
	if err := app.flushMetrics(f.ctx); err != nil {
		t.Fatal(err)
	}
	// Two actual cumulative observations are required for a rate. No backdated
	// sample is inserted directly into storage to manufacture that prerequisite.
	time.Sleep(1100 * time.Millisecond)
	counter.Add(f.ctx, 3)
	gauge.Record(f.ctx, 7)
	histogram.Record(f.ctx, .2)
	nativeHistogram.Record(f.ctx, .2)
	if err := app.flushMetrics(f.ctx); err != nil {
		t.Fatal(err)
	}
}

func verifyObservationMetricKinds(t *testing.T, f *nativeSDKPipeline, entries []any, call func(string, string, any) (int, map[string]any), scope map[string]any, from time.Time) {
	t.Helper()
	expected := map[string]struct{ kind, unit string }{
		"plan11_sdk_completed":      {"counter", "{request}"},
		"plan11_sdk_queue":          {"gauge", "{request}"},
		"plan11_sdk_latency":        {"histogram", "s"},
		"plan11_sdk_native_latency": {"exponential_histogram", "s"},
	}
	variants := map[string]bool{}
	countExpression, meanExpression := "", ""
	nativeCountExpression, nativeMeanExpression := "", ""
	for _, entry := range entries {
		m := entry.(map[string]any)
		name, _ := m["name"].(string)
		if name == "plan11_sdk_variant" {
			if m["observed"] != true {
				t.Fatal("actually collected metric variant was not observed", m)
			}
			variants[fmt.Sprint(m["type"])+"/"+fmt.Sprint(m["unit"])] = true
		}
		if name == "plan11_sdk_latency" {
			methods, ok := m["query_methods"].(map[string]any)
			if !ok || methods["representation"] != "classic_histogram" || methods["count_unit"] != "{observation}" || methods["mean_unit"] != "s" {
				t.Fatal("actual SDK HTTP catalog lost classic histogram query representation/units", m)
			}
			countExpression, _ = methods["count_expression"].(string)
			meanExpression, _ = methods["mean_expression"].(string)
		}
		if name == "plan11_sdk_native_latency" {
			methods, ok := m["query_methods"].(map[string]any)
			if !ok || methods["representation"] != "native_histogram" || methods["count_unit"] != "{observation}" || methods["mean_unit"] != "s" {
				t.Fatal("actual SDK HTTP catalog lost native histogram query representation/units", m)
			}
			nativeCountExpression, _ = methods["count_expression"].(string)
			nativeMeanExpression, _ = methods["mean_expression"].(string)
		}
		want, ok := expected[name]
		if !ok {
			continue
		}
		keys, _ := m["source_keys"].([]any)
		if m["observed"] != true || m["type"] != want.kind || m["unit"] != want.unit || len(keys) != 1 {
			t.Fatalf("actual SDK catalog changed type/unit/source: %v", m)
		}
		delete(expected, name)
	}
	if len(expected) != 0 {
		t.Fatalf("actual SDK metric kinds absent from authorized catalog: %v", expected)
	}
	if len(variants) != 3 || !variants["counter/By"] || !variants["gauge/By"] || !variants["gauge/s"] {
		t.Fatal("actual SDK same-name source/type/unit variants were merged", variants)
	}
	t.Log("Actual SDK same-name metric retains counter/By, gauge/By and gauge/s as separate observed catalog entries")
	if countExpression == "" || meanExpression == "" || nativeCountExpression == "" || nativeMeanExpression == "" {
		t.Fatal("actual SDK histogram directory did not supply query methods")
	}
	for _, test := range []struct {
		expression, name, kind, unit string
		want                         float64
	}{
		{"plan11_sdk_completed", "plan11_sdk_completed", "counter", "{request}", 5},
		{"plan11_sdk_queue", "plan11_sdk_queue", "gauge", "{request}", 7},
		{countExpression, "plan11_sdk_latency", "histogram", "s", 3},
		{meanExpression, "plan11_sdk_latency", "histogram", "s", .2},
		{nativeCountExpression, "plan11_sdk_native_latency", "exponential_histogram", "s", 3},
		{nativeMeanExpression, "plan11_sdk_native_latency", "exponential_histogram", "s", .2},
		{"rate(plan11_sdk_completed[5m])", "plan11_sdk_completed", "counter", "{request}", -1},
	} {
		t.Run(test.expression, func(t *testing.T) {
			to := time.Now()
			body := map[string]any{"scope": scope, "query": test.expression, "time_range": map[string]any{"from": from, "to": to}}
			status, out := call("POST", "/enterprise/metrics/query", body)
			if status != 200 {
				t.Fatalf("actual SDK semantics HTTP %d: %v", status, out)
			}
			raw, _ := json.Marshal(out)
			var typed struct {
				Data struct {
					Result []struct {
						Value []json.RawMessage `json:"value"`
					} `json:"result"`
				} `json:"data"`
			}
			if json.Unmarshal(raw, &typed) != nil || len(typed.Data.Result) != 1 || len(typed.Data.Result[0].Value) != 2 {
				t.Fatalf("actual SDK semantics missing instant value: %s", raw)
			}
			var value string
			if json.Unmarshal(typed.Data.Result[0].Value[1], &value) != nil {
				t.Fatalf("invalid PromQL value: %s", raw)
			}
			actual, err := strconv.ParseFloat(value, 64)
			if err != nil || math.IsNaN(actual) || math.IsInf(actual, 0) || test.want >= 0 && math.Abs(actual-test.want) > 1e-9 || test.want < 0 && actual <= 0 {
				t.Fatalf("wrong actual SDK semantic value: %g want %g", actual, test.want)
			}
			// Input metadata stays the original source kind/unit for a derived
			// mean or rate; it does not assert that the output is a raw counter.
			// The public legacy HTTP response does not carry input descriptors.
			// Verify those independently on the same real-data query engine used
			// by native tools; do not pretend they came from that HTTP response.
			engine := promengine.NewEngine(f.conn, f.router, nil)
			result, err := engine.Execute(f.ctx, promengine.Request{Expression: test.expression, Instant: true, Start: from, End: to, Scope: promengine.Scope{EnterpriseID: f.enterprise, ResourceIDs: []uuid.UUID{f.apps[0].app}, SourceKeys: []string{f.apps[0].source + ":1"}}, MaxSamples: 10000, MaxSeries: 100, MaxScanBytes: 32 << 20, Timeout: 5 * time.Second})
			if err != nil || !result.InputMetricsComplete || len(result.InputMetrics) != 1 {
				t.Fatalf("missing original engine input metadata: %+v %v", result.InputMetrics, err)
			}
			input := result.InputMetrics[0]
			if input.Name != test.name || input.Type != test.kind || input.Unit != test.unit || input.SourceKey != f.apps[0].source+":1" {
				t.Fatalf("input metadata overwritten by output semantics: %+v", input)
			}
			t.Logf("actual SDK expression=%s value=%g input=%s/%s", test.expression, actual, input.Type, input.Unit)
		})
	}
}
