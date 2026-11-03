package toolruntime

import (
	"strings"
	"testing"
)

func TestMetricRateCoverageRuleSurvivesAdmissionAndLaterInference(t *testing.T) {
	for _, mode := range []string{"execute", "plan"} {
		for _, turn := range []int{0, 10, MaxInferenceTurns - 1} {
			if !strings.Contains(ModelSystemInstructionsForTurn(mode, turn), MetricRateEvidenceInstructions) {
				t.Fatal("counter rate/extrapolation scope disappeared across inference turns", mode, turn)
			}
		}
	}
}
