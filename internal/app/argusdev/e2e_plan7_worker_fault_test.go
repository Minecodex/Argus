package argusdev

import (
	"github.com/kakj-go/Argus/internal/remoteexecution"
	"testing"
)

func TestPlan7WorkerFaultDistinguishesUnsentContextLossFromUnknownEffect(t *testing.T) {
	for _, tc := range []struct {
		name, status, second, last string
		valid                      bool
		commands                   int
	}{
		{"known completion", "succeeded", "succeeded", "succeeded", true, 3},
		{"missing receipt", "result_unknown", "result_unknown", "skipped", true, 2},
		{"next preparation lost context", "paused_for_recovery", "succeeded", "paused_for_recovery", true, 2},
		{"continued after unknown", "result_unknown", "result_unknown", "succeeded", false, 0},
		{"unresolved pending", "paused_for_recovery", "succeeded", "pending", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := remoteexecution.View{Status: tc.status, Results: []remoteexecution.StepResult{{Status: "succeeded"}, {Status: tc.second}, {Status: tc.last}}}
			_, commands, err := plan7WorkerFaultExpectation(v)
			if (err == nil) != tc.valid || commands != tc.commands {
				t.Fatalf("commands=%d err=%v", commands, err)
			}
		})
	}
}
