package toolruntime

import (
	"strings"
	"testing"
)

func TestMetadataEvidenceScopeIsSharedByAdmissionAndLaterInference(t *testing.T) {
	for _, mode := range []string{"execute", "plan"} {
		for _, turn := range []int{0, 10, MaxInferenceTurns - 1} {
			instructions := ModelSystemInstructionsForTurn(mode, turn)
			if !strings.Contains(instructions, MetadataEvidenceScopeInstructions) {
				t.Fatal("metadata identity/filter boundaries disappeared after admission or turn update", mode, turn)
			}
			if !strings.Contains(instructions, MetadataCountInstructions) {
				t.Fatal("catalog count units disappeared after admission or turn update", mode, turn)
			}
			if !strings.Contains(instructions, ObservationWorkflowInstructions) {
				t.Fatal("metadata-to-measurement workflow disappeared", mode, turn)
			}
		}
	}
}
