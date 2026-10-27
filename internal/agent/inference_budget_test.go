package agent

import (
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestInferenceBudgetRetainsAdmissionPrefixAndReservesFinalResponse(t *testing.T) {
	for _, mode := range []string{"analyze", "act", "plan", "execute"} {
		messages := []modelprovider.Message{{Role: "system", Content: toolruntime.ModelSystemInstructions(mode)}, {Role: "user", Content: "Only use verified owned data."}}
		tools := []modelprovider.Tool{{Name: "read", Schema: map[string]any{"type": "object"}}}
		first, firstTools, err := inferenceTurnContext(mode, 0, messages, tools)
		if err != nil || first[0].Content != messages[0].Content || len(firstTools) != 1 {
			t.Fatal("admission and first runtime prefix diverged")
		}
		penultimate, lastTools, err := inferenceTurnContext(mode, toolruntime.MaxInferenceTurns-2, messages, tools)
		if err != nil || len(lastTools) != 1 || !strings.Contains(penultimate[0].Content, "2 remain") {
			t.Fatal("last evidence-gathering turn unavailable")
		}
		last, finalTools, err := inferenceTurnContext(mode, toolruntime.MaxInferenceTurns-1, messages, tools)
		if err != nil || len(finalTools) != 0 || !strings.Contains(last[0].Content, "1 remain") || !strings.Contains(last[0].Content, "No tools are available") {
			t.Fatal("final response was not reserved")
		}
		if messages[0].Content != toolruntime.ModelSystemInstructions(mode) || len(tools) != 1 || last[1].Content != messages[1].Content {
			t.Fatal("immutable history or tool snapshot changed")
		}
		if _, _, err = inferenceTurnContext(mode, toolruntime.MaxInferenceTurns, messages, tools); err == nil {
			t.Fatal("an extra inference turn was admitted")
		}
	}
}
