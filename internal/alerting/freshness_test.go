package alerting

import (
	"testing"
	"time"
)

func TestEvaluationFreshnessRetainsSuccessfulValuesAcrossFailures(t *testing.T) {
	first := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	obs := Observation{Value: 91, Rule: Rule{Expression: "cpu", Threshold: 85}, Members: []MemberValue{{Value: 91}}}
	_, state, _ := advance("firing", InstanceState{}, obs, PolicySpec{}, first)
	state = evaluationFailure(state)
	state.LastSlot = first.Add(time.Minute)
	if state.LastSuccessfulAt == nil || !state.LastSuccessfulAt.Equal(first) || *state.Value != 91 || state.Members[0].Value != 91 {
		t.Fatal("failed attempt must retain successful value provenance", state)
	}
	obs.Value, obs.Members[0].Value = 96, 96
	next := first.Add(2 * time.Minute)
	_, state, _ = advance("firing", state, obs, PolicySpec{}, next)
	if state.LastSuccessfulAt == nil || !state.LastSuccessfulAt.Equal(next) || *state.Value != 96 || state.Members[0].Value != 96 {
		t.Fatal("successful recovery must replace value provenance", state)
	}
}
