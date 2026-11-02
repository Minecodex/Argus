package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/model"
)

// This opt-in probe uses the production streaming/tool/schema compatibility
// checks. Credentials are supplied only through the test process environment.
func TestPlan11LiveProviderCompatibility(t *testing.T) {
	key := os.Getenv("ARGUS_PLAN11_API_KEY")
	if key == "" || os.Getenv("ARGUS_PLAN11_PROVIDER_PROBE") != "1" {
		t.Skip("requires explicitly authorized live provider credential")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Minute)
	defer cancel()
	effort := os.Getenv("ARGUS_PLAN11_REASONING_EFFORT")
	if effort == "" {
		effort = "low"
	}
	if _, err := modelprovider.NormalizeReasoningEffort(modelprovider.ProtocolChatCompletions, effort); err != nil {
		t.Fatal("invalid explicitly selected live reasoning profile")
	}
	policy := modelprovider.PublicEndpointPolicy{Resolver: modelGovernancePublicResolver{}}
	result := (model.ProviderTester{Policy: policy}).Test(ctx, model.Input{BaseURL: "https://api.apimart.ai/v1", ProviderModelID: "glm-5.3-flash", Protocol: "chat_completions", APIKey: key, ContextWindow: 131072, MaxOutput: 8192, ReasoningEffort: effort})
	key = ""
	t.Logf("explicit reasoning profile: %s; max output: 8192", effort)
	if path := os.Getenv("ARGUS_PLAN11_PROVIDER_RESULT"); path != "" {
		raw, _ := json.MarshalIndent(result, "", "  ")
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, check := range result.Checks {
		t.Logf("%s: %s (%s)", check.Name, check.Status, check.ErrorCode)
		if check.Status != "passed" {
			t.Fail()
		}
	}
}
