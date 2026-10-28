package httpapi

import (
	"encoding/json"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"os"
	"testing"
)

func TestModelGovernanceProviderProbe(t *testing.T) {
	file := os.Getenv("ARGUS_MODEL_GOVERNANCE_PROVIDER_FILE")
	if file == "" {
		t.Skip("requires explicitly authorized live provider")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var config modelGovernanceProvider
	if err = json.Unmarshal(raw, &config); err != nil {
		t.Fatal("invalid provider config")
	}
	clear(raw)
	policy := modelprovider.PublicEndpointPolicy{Resolver: modelGovernancePublicResolver{}}
	provider := modelprovider.Provider{Protocol: modelprovider.ProtocolChatCompletions, BaseURL: config.BaseURL, APIKey: config.APIKey, Client: policy.ModelClient()}
	usage := modelprovider.TokenUsage{}
	reason := ""
	count := 0
	err = provider.Stream(t.Context(), modelprovider.Request{Model: config.Model, Messages: []modelprovider.Message{{Role: "user", Content: modelprovider.OutputBudgetProbeMarker + ". Count from 1 to 400 separated by commas. Do not skip numbers, abbreviate or explain."}}, MaxTokens: 128, OutputTokenParameter: modelprovider.BudgetMaxTokens, ReasoningEffort: "low"}, func(event modelprovider.Event) error {
		usage.Observe(event)
		if event.Type == "completed" {
			reason = event.StopReason
		}
		count++
		return nil
	})
	t.Logf("provider stream: errorType=%T error=%v input=%d output=%d complete=%v stop=%s events=%d", err, err, usage.Input, usage.Output, usage.Complete(), reason, count)
	if err != nil {
		t.Fail()
	}
}
