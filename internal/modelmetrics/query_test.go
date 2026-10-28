package modelmetrics

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
)

func TestPrivateMetricsPreserveUnknownUsageAndExactLatency(t *testing.T) {
	at := time.Now().UTC()
	id := uuid.New()
	d := dataset{Names: map[string]string{}}
	calls := []Call{{ModelID: id, Status: "succeeded", Input: 100, Output: 10, Cached: 20, InputSource: "provider", OutputSource: "provider", CachedSource: "provider", Amount: "1.25", Dispatched: true, LatencyMS: 100},
		{ModelID: id, Status: "failed", Input: 50, Output: 5, InputSource: "estimated", OutputSource: "missing", CachedSource: "missing", Amount: "0.25", Dispatched: true, LatencyMS: 300}}
	engine := promql.NewEngine(promql.EngineOpts{MaxSamples: 10000, Timeout: time.Second, LookbackDelta: time.Minute, Parser: parser.NewParser(parser.Options{})})
	q, err := d.metrics(calls, at, false)
	if err != nil {
		t.Fatal(err)
	}
	for expression, want := range map[string]float64{"sum(argus_model_requests)": 2, "sum(argus_model_input_tokens)": 100, "sum(argus_model_cached_tokens)": 20, "sum(argus_model_amount)": 1.5, "argus_model_latency_p95_ms": 300, "sum(argus_model_successes)/sum(argus_model_requests)": .5} {
		value, err := evaluate(t.Context(), engine, q, expression, at)
		if err != nil || len(value) != 1 || math.Abs(value[0].F-want) > 1e-9 {
			t.Fatalf("%s: %v %v", expression, value, err)
		}
	}
	unknown, err := d.metrics(calls[1:], at, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, metric := range []string{"argus_model_input_tokens", "argus_model_output_tokens", "argus_model_cached_tokens", "argus_model_tool_failures"} {
		value, err := evaluate(context.Background(), engine, unknown, "sum("+metric+")", at)
		if err != nil || len(value) != 0 {
			t.Fatalf("unknown value became a measurement: %s %v %v", metric, value, err)
		}
	}
	empty, _ := d.metrics(nil, at, false)
	value, err := evaluate(t.Context(), engine, empty, "sum(argus_model_successes)/sum(argus_model_requests)", at)
	if err != nil || len(value) != 0 {
		t.Fatal("empty success rate must be unavailable", value, err)
	}
	// Display names are not identities: two users with the same name must
	// remain separate ranking rows.
	first, second := uuid.New(), uuid.New()
	calls[0].UserID = first
	calls[1].UserID = second
	d.Names[first.String()], d.Names[second.String()] = "Same name", "Same name"
	q, _ = d.metrics(calls, at, false)
	ranking, err := evaluate(t.Context(), engine, q, "sum by (user_id, user) (argus_model_amount)", at)
	if err != nil || len(ranking) != 2 {
		t.Fatal("same-name users were merged", ranking, err)
	}
}

func TestQuotaMetricsUseReservationLedgerRatherThanCallCompletion(t *testing.T) {
	at := time.Now()
	modelID, department := uuid.New(), uuid.New()
	d := dataset{Names: map[string]string{}, Quotas: []Quota{{ModelID: modelID, SubjectID: department, SubjectType: "department", Amount: "20"}}, Reservations: []Reservation{
		{ModelID: modelID, DepartmentID: department, Status: "settled", Reserved: "10", Settled: "2"},
		{ModelID: modelID, DepartmentID: department, Status: "active", Reserved: "3", Settled: "0"},
		{ModelID: modelID, DepartmentID: department, Status: "released", Reserved: "7", Settled: "0"},
	}}
	q, err := d.metrics([]Call{{ModelID: modelID, DepartmentID: department, Amount: "999", Status: "succeeded"}}, at, true)
	if err != nil {
		t.Fatal(err)
	}
	engine := promql.NewEngine(promql.EngineOpts{MaxSamples: 1000, Timeout: time.Second, LookbackDelta: time.Minute, Parser: parser.NewParser(parser.Options{})})
	value, err := evaluate(t.Context(), engine, q, "argus_model_quota_used / argus_model_quota_limit", at)
	if err != nil || len(value) != 1 || value[0].F != .25 {
		t.Fatal("quota must include settled and active reservations only", value, err)
	}
}
