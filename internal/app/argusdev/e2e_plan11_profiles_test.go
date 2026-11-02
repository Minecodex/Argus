package argusdev

import "testing"

func TestCandidateAcceptanceIsNotPromotedToCompleteComparison(t *testing.T) {
	profiles, err := plan11ParseComparisonProfiles("candidate", "plan8", "plan11-compare")
	if err != nil {
		t.Fatal(err)
	}
	if names := plan11ComparisonSelectedModelNames(3, profiles, nil); len(names) != 18 {
		t.Fatal("candidate-only budget", len(names))
	}
	for _, raw := range []string{"candidate,candidate", "candidate,", "unknown"} {
		if _, err := plan11ParseComparisonProfiles(raw, "plan8", "plan11-compare"); err == nil {
			t.Fatal("invalid profiles accepted", raw)
		}
	}
	if _, err := plan11ParseComparisonProfiles("candidate", "plan8", "plan11-review"); err == nil {
		t.Fatal("profile selection drifted combined review budget")
	}
	var cases []map[string]any
	for i := 0; i < 18; i++ {
		cases = append(cases, map[string]any{"profile": "candidate", "business_passed": true, "passed": true, "call_trace_complete": true, "sample": p5BenchmarkSample{ModelCalls: 1, UsageComplete: true, CachedUsageComplete: true, DurationMS: 1}})
	}
	metrics, err := plan11ComparisonMetricsForProfiles(cases, 3, profiles, nil)
	if err != nil || metrics["candidate_machine_gates_complete"] != true || metrics["complete_comparative_cohort"] != false || metrics["whole_plan_release_eligible"] != false {
		t.Fatal("candidate quality confused with comparative or whole-plan completeness", metrics, err)
	}
	cases[0]["hard_failure"] = true
	if _, err := plan11ComparisonMetricsForProfiles(cases, 3, profiles, nil); err == nil {
		t.Fatal("candidate hard boundary ignored")
	}
	delete(cases[0], "hard_failure")
	cases[0]["profile"] = "legacy"
	if _, err := plan11ComparisonMetricsForProfiles(cases, 3, profiles, nil); err == nil {
		t.Fatal("undeclared baseline attempt omitted from denominator")
	}
}
