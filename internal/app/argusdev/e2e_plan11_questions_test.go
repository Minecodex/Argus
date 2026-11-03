package argusdev

import "testing"

func TestFocusedComparisonDeclaresExactlyItsQuestionCohort(t *testing.T) {
	selected, err := plan11ParseComparisonQuestions("dashboard", "plan8", "plan11-compare")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan11ComparisonModelNames(1, selected...)) != 4 || len(plan11ComparisonModelNames(3, selected...)) != 12 {
		t.Fatal("focused scenario budget differs from execution")
	}
	for _, test := range []struct{ raw, suite, phase string }{
		{"dashboard,dashboard", "plan8", "plan11-compare"},
		{"dashboard,unknown", "plan8", "plan11-compare"},
		{"dashboard,", "plan8", "plan11-compare"},
		{"dashboard", "plan8", "plan11-review"},
		{"dashboard", "plan7", "plan11-compare"},
	} {
		if _, err := plan11ParseComparisonQuestions(test.raw, test.suite, test.phase); err == nil {
			t.Fatal("invalid focused comparison accepted", test)
		}
	}
	var cases []map[string]any
	for _, profile := range plan11ComparisonProfiles() {
		for range 2 {
			cases = append(cases, map[string]any{"profile": profile, "business_passed": true, "passed": true, "call_trace_complete": true, "sample": p5BenchmarkSample{ModelCalls: 1, UsageComplete: true, CachedUsageComplete: true, DurationMS: 1}})
		}
	}
	metrics, err := plan11ComparisonMetrics(cases, 1, "dashboard")
	if err != nil || metrics["full_question_set"] != false || metrics["planned_repetitions_complete"] != false || metrics["whole_plan_release_eligible"] != false {
		t.Fatal("subset was promoted to complete comparison", metrics, err)
	}
	if _, err = plan11ComparisonMetrics(cases[:3], 1, "dashboard"); err == nil {
		t.Fatal("missing subset attempt was ignored")
	}
}
