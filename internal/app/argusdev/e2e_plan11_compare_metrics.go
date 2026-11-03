package argusdev

import (
	"encoding/json"
	"errors"
	"math"
	"slices"
	"sort"
)

func plan11ComparisonMetrics(cases []map[string]any, repeats int, selected ...string) (map[string]any, error) {
	return plan11ComparisonMetricsForProfiles(cases, repeats, nil, selected)
}

func plan11ComparisonMetricsForProfiles(cases []map[string]any, repeats int, selectedProfiles, selected []string) (map[string]any, error) {
	questions := plan11ComparisonQuestions(selected)
	profiles := plan11ComparisonProfiles(selectedProfiles...)
	for _, row := range cases {
		profile, ok := row["profile"].(string)
		if !ok || !slices.Contains(profiles, profile) {
			return nil, errors.New("comparison contains an undeclared profile attempt")
		}
		if question, ok := row["question"].(string); ok && !slices.Contains(questions, question) {
			return nil, errors.New("comparison contains an undeclared question attempt")
		}
	}
	result := map[string]any{
		"sample_scope":                "three question styles, two languages, serial profiles",
		"whole_plan_release_eligible": false,
		"first_capability_accuracy":   "pending_trace_review; first category alone is not a correctness oracle",
		"candidate_gate_policy":       "business >= 90%; complete provider usage and call traces for every attempt; zero hard boundary failures",
		"first_effective_definition":  "first successful tool result excluding tool.search/tool.describe; this clock alone does not prove correct resource/source selection or useful telemetry",
	}
	result["questions"] = questions
	result["full_question_set"] = len(questions) == 3
	result["sample_scope"] = "declared bilingual questions, serial profiles"
	result["profiles"] = profiles
	result["complete_comparative_cohort"] = false
	result["candidate_machine_gates_complete"] = false
	if repeats == 0 {
		result["status"] = "infrastructure_only_no_model_cases"
		return result, nil
	}
	var failures []error
	for _, profile := range profiles {
		count, business, complete, invalid, incompleteTraces := 0, 0, 0, 0, 0
		usageComplete, accountingUnknown, hardFailures := 0, 0, 0
		modelCalls, input, output, cached := 0, int64(0), int64(0), int64(0)
		durations, first := []int64{}, []int64{}
		for _, row := range cases {
			if row["profile"] != profile {
				continue
			}
			count++
			if row["call_trace_complete"] != true {
				incompleteTraces++
			}
			if row["hard_failure"] == true {
				hardFailures++
			}
			if row["business_passed"] == true {
				business++
			}
			if row["passed"] == true {
				complete++
			}
			if n, ok := row["invalid_calls"].(int); ok {
				invalid += n
			}
			var sample p5BenchmarkSample
			raw, err := json.Marshal(row["sample"])
			if err != nil || json.Unmarshal(raw, &sample) != nil || sample.ModelCalls < 1 {
				accountingUnknown++
				continue
			}
			modelCalls += sample.ModelCalls
			input += sample.InputTokens
			output += sample.OutputTokens
			cached += sample.CachedInputTokens
			if sample.UsageComplete && sample.CachedUsageComplete {
				usageComplete++
			}
			if sample.DurationMS > 0 && !math.IsNaN(sample.DurationMS) && !math.IsInf(sample.DurationMS, 0) {
				durations = append(durations, int64(math.Round(sample.DurationMS)))
			}
			if sample.FirstEffectiveResultMS != nil && *sample.FirstEffectiveResultMS > 0 && !math.IsNaN(*sample.FirstEffectiveResultMS) && !math.IsInf(*sample.FirstEffectiveResultMS, 0) {
				first = append(first, int64(math.Round(*sample.FirstEffectiveResultMS)))
			}
		}
		if count != 2*len(questions)*repeats {
			return result, errors.New("comparison profile case set incomplete")
		}
		metrics := map[string]any{
			"scenarios": count, "business_passed": business, "business_completion_rate": float64(business) / float64(count),
			"all_gates_passed": complete, "not_all_gates_passed": count - complete,
			"known_invalid_calls": invalid, "call_trace_incomplete_cases": incompleteTraces, "hard_boundary_failures": hardFailures,
			"usage_complete_cases": usageComplete, "usage_incomplete_cases": count - usageComplete, "accounting_unknown_cases": accountingUnknown,
			"model_calls": modelCalls, "model_call_totals_complete": accountingUnknown == 0,
			"known_input_tokens": input, "known_output_tokens": output, "known_cached_input_tokens": cached,
			"duration_known_cases": len(durations), "duration_missing_cases": count - len(durations),
			"duration_p50_ms": nil, "duration_p95_ms": nil,
			"known_duration_p50_ms": plan11Percentile(durations, 0.5), "known_duration_p95_ms": plan11Percentile(durations, 0.95),
			"first_effective_known_cases": len(first), "first_effective_missing_cases": count - len(first),
			"first_effective_p50_ms": nil, "first_effective_p95_ms": nil,
			"known_first_effective_p50_ms": plan11Percentile(first, 0.5), "known_first_effective_p95_ms": plan11Percentile(first, 0.95),
			"pricing": "provider billing is not inferred from test ledger prices",
		}
		if len(durations) == count {
			metrics["duration_p50_ms"], metrics["duration_p95_ms"] = plan11Percentile(durations, 0.5), plan11Percentile(durations, 0.95)
		}
		if len(first) == count {
			metrics["first_effective_p50_ms"], metrics["first_effective_p95_ms"] = plan11Percentile(first, 0.5), plan11Percentile(first, 0.95)
		}
		result[profile] = metrics
		if profile == "candidate" && (float64(business)/float64(count) < 0.9 || usageComplete != count || incompleteTraces != 0 || hardFailures != 0) {
			failures = append(failures, errors.New("candidate comparison business threshold, provider usage, call trace or hard boundary gate failed"))
		}
	}
	result["status"] = "executed_pending_initial_route_and_whole_matrix_review"
	result["planned_repetitions_complete"] = repeats == 3 && len(questions) == 3
	result["complete_comparative_cohort"] = repeats == 3 && len(questions) == 3 && slices.Contains(profiles, "candidate") && slices.Contains(profiles, "legacy")
	result["candidate_machine_gates_complete"] = repeats == 3 && len(questions) == 3 && slices.Contains(profiles, "candidate") && len(failures) == 0
	return result, errors.Join(failures...)
}

func plan11Percentile(values []int64, p float64) int64 {
	if len(values) == 0 {
		return 0
	}
	copy := append([]int64{}, values...)
	sort.Slice(copy, func(i, j int) bool { return copy[i] < copy[j] })
	i := int(math.Ceil(float64(len(copy))*p)) - 1
	if i < 0 {
		i = 0
	}
	return copy[i]
}
