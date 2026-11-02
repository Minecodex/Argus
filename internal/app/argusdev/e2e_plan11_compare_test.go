package argusdev

import (
	"errors"
	"testing"
	"time"
)

func TestComparisonOracleWorksWithoutCandidateOnlyPayloadFields(t *testing.T) {
	body := []byte(`{"status":"success","argus_meta":{"loaded_samples":2},"data":{"resultType":"matrix","result":[{"metric":{"__name__":"system_memory_utilization","argus_resource_id":"host","state":"used"},"values":[[1,"0.7"],[2,"0.8"]]}]}}`)
	points, err := plan11ComparisonPointEvidence(body, "host")
	if err != nil || points.Count != 2 || !points.First.Equal(time.Unix(1, 0)) || !points.Last.Equal(time.Unix(2, 0)) {
		t.Fatal("legacy measured data was rejected because it lacks new metadata", points, err)
	}
	if _, err = plan11ComparisonPointEvidence(body, "other"); !errors.Is(err, errPlan11ComparisonBoundary) {
		t.Fatal("foreign Host did not fail the hard boundary", err)
	}
	if _, err = plan11ComparisonPointEvidence([]byte(`{"status":"success","argus_meta":{"loaded_samples":2},"data":{"resultType":"matrix","result":[{"metric":{"state":"used"},"values":[[1,"10"],[2,"20"]]}]}}`), "host"); err == nil || errors.Is(err, errPlan11ComparisonBoundary) {
		t.Fatal("aggregation without identity must fail ordinary evidence, not invent a foreign Host", err)
	}
	if _, err = plan11ComparisonPointEvidence([]byte(`{"status":"success","argus_meta":{"loaded_samples":0},"data":{"resultType":"vector","result":[]}}`), "host"); err == nil {
		t.Fatal("constant/empty evidence passed")
	}
}

func TestComparisonSelectionBudgetAndPromptAreProfileNeutral(t *testing.T) {
	if len(plan11ComparisonModelNames(0)) != 0 || len(plan11ComparisonModelNames(1)) != 12 || len(plan11ComparisonModelNames(3)) != 36 {
		t.Fatal("comparison model budget is wrong")
	}
	if names := plan11ComparisonModelNames(1); names[0] != "plan11-compare-candidate-metric-zh-1" || names[6] != "plan11-compare-legacy-metric-zh-1" {
		t.Fatal("legacy failures would prevent evaluating the candidate first", names)
	}
	for _, stage := range []string{"formal-onboarding-and-managed-collector", "normal-production-backend", "monitoring-api", "plan11-profile-comparison"} {
		if !plan8StageSelected("plan11-compare", stage) {
			t.Fatal("comparison lost an actual-data prerequisite", stage)
		}
	}
	for _, stage := range []string{"plan11-business-discovery", "additional-supported-integrations", "sdk-application-protocols", "kubernetes-namespace-workbench"} {
		if plan8StageSelected("plan11-compare", stage) {
			t.Fatal("comparison included an unrelated mutable stage", stage)
		}
	}
	window := plan11ComparisonWindow{From: time.Unix(1, 0).UTC(), To: time.Unix(301, 0).UTC()}
	if !plan11ArgumentsMatchWindow(map[string]any{"from": "1970-01-01T00:00:01Z", "to": "1970-01-01T00:05:01Z"}, window) || plan11ArgumentsMatchWindow(map[string]any{"from": "1970-01-01T00:00:00Z", "to": "1970-01-01T00:05:01Z"}, window) {
		t.Fatal("comparison accepts a changed window")
	}
	if plan11ComparisonPrompt(&plan6HostLoop{HostName: "same", DashboardID: "board"}, window, "metric", "en") == "" {
		t.Fatal("business prompt missing")
	}
}

func TestComparisonMetricsKeepFailuresAndDoNotInventLocatorAccuracy(t *testing.T) {
	cases := []map[string]any{}
	for _, profile := range []string{"legacy", "candidate"} {
		for i := 0; i < 6; i++ {
			cases = append(cases, map[string]any{"profile": profile, "business_passed": true, "passed": profile == "candidate", "invalid_calls": 1, "call_trace_complete": profile == "candidate", "sample": p5BenchmarkSample{ModelCalls: 2, InputTokens: 20, UsageComplete: true, CachedUsageComplete: true, DurationMS: 1000}})
		}
	}
	metrics, err := plan11ComparisonMetrics(cases, 1)
	if err != nil {
		t.Fatal(err)
	}
	legacy := metrics["legacy"].(map[string]any)
	if legacy["all_gates_passed"] != 0 || legacy["business_passed"] != 6 || metrics["whole_plan_release_eligible"] != false {
		t.Fatal("comparison discarded failed gates or claimed whole-plan release")
	}
	if legacy["known_invalid_calls"] != 6 || legacy["call_trace_incomplete_cases"] != 6 {
		t.Fatal("failed/unknown call evidence was discarded or counted as a known zero")
	}
	cases[len(cases)-1]["passed"] = false
	cases[len(cases)-1]["sample"] = p5BenchmarkSample{ModelCalls: 2, InputTokens: 20, UsageComplete: false, CachedUsageComplete: true, DurationMS: 1000}
	if _, err := plan11ComparisonMetrics(cases, 1); err == nil {
		t.Fatal("candidate usage failure disappeared")
	}
}

func TestComparisonMetricsKeepNinetyPercentSeparateFromAccounting(t *testing.T) {
	var cases []map[string]any
	for _, profile := range []string{"legacy", "candidate"} {
		for i := 0; i < 18; i++ {
			cases = append(cases, map[string]any{"profile": profile, "business_passed": i != 0, "passed": i != 0, "call_trace_complete": true, "sample": p5BenchmarkSample{ModelCalls: 3, InputTokens: 10, UsageComplete: true, CachedUsageComplete: true, DurationMS: 1000}})
		}
	}
	metrics, err := plan11ComparisonMetrics(cases, 3)
	if err != nil {
		t.Fatal("17/18 business with complete usage was incorrectly made a 100% business gate", err)
	}
	candidate := metrics["candidate"].(map[string]any)
	if candidate["business_passed"] != 17 || candidate["usage_complete_cases"] != 18 || candidate["all_gates_passed"] != 17 || candidate["model_calls"] != 54 || candidate["known_input_tokens"] != int64(180) {
		t.Fatal("failed attempt or its real accounting disappeared", candidate)
	}
	last := cases[len(cases)-1]
	last["hard_failure"] = true
	if _, err := plan11ComparisonMetrics(cases, 3); err == nil {
		t.Fatal("business average concealed a hard boundary failure")
	}
	delete(last, "hard_failure")
	last["sample"] = p5BenchmarkSample{ModelCalls: 3, InputTokens: 10, UsageComplete: true, CachedUsageComplete: true}
	metrics, err = plan11ComparisonMetrics(cases, 3)
	if err != nil {
		t.Fatal(err)
	}
	candidate = metrics["candidate"].(map[string]any)
	if candidate["duration_p95_ms"] != nil || candidate["duration_missing_cases"] != 1 || candidate["known_duration_p95_ms"] != int64(1000) {
		t.Fatal("partial latency was reported as a complete P95", candidate)
	}
	last["sample"] = p5BenchmarkSample{}
	metrics, err = plan11ComparisonMetrics(cases, 3)
	candidate = metrics["candidate"].(map[string]any)
	if err == nil || candidate["accounting_unknown_cases"] != 1 || candidate["model_call_totals_complete"] != false || candidate["usage_incomplete_cases"] != 1 {
		t.Fatal("unknown attempt was treated as known zero or complete usage", candidate, err)
	}
}
