package model

import (
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"testing"
)

func TestModelUpdatesPreserveOrExplicitlyClearReasoningStrength(t *testing.T) {
	current := db.AiModel{ApiProtocol: "chat_completions", Capabilities: []byte(`{"reasoning_effort":"low"}`)}
	for _, example := range []struct{ patch, expected string }{{"", "low"}, {"high", "high"}, {"provider_default", ""}} {
		merged := mergeInput(current, Input{ReasoningEffort: example.patch})
		stored := capabilitiesJSON(merged, CompatibilityResult{OutputTokenParameter: modelprovider.BudgetMaxTokens})
		effort, err := modelprovider.ReasoningEffortForCapabilities(modelprovider.ProtocolChatCompletions, stored)
		if err != nil || effort != example.expected {
			t.Fatalf("reasoning profile changed unintentionally: %q %v", effort, err)
		}
	}
}
