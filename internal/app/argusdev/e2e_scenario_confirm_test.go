package argusdev

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/toolruntime"
	"sigs.k8s.io/yaml"
)

func TestScenarioConfirmRepeatedOperationLabelBindsActionReference(t *testing.T) {
	env := &E2EEnvironment{State: NewScenarioState("plan8-key-regression"), Options: E2EOptions{RunID: "plan8-key-regression"}, Endpoints: &E2EEndpoints{EnterpriseOrigin: "https://enterprise.example.test"}}
	env.State.Values["enterprise_csrf"] = "fixture-csrf"
	first := pendingConfirmationHeaders(env, "scale-same-deployment", "act_111111111111111111111111")
	retry := pendingConfirmationHeaders(env, "scale-same-deployment", "act_111111111111111111111111")
	renamed := pendingConfirmationHeaders(env, "different-label-same-approved-action", "act_111111111111111111111111")
	second := pendingConfirmationHeaders(env, "scale-same-deployment", "act_222222222222222222222222")
	if first["Idempotency-Key"] == "" || first["Idempotency-Key"] != retry["Idempotency-Key"] || first["Idempotency-Key"] != renamed["Idempotency-Key"] || first["Idempotency-Key"] == second["Idempotency-Key"] {
		t.Fatal("confirmation identity depends on display label, changed on retry or collided with another action")
	}
	if first["X-CSRF-Token"] != "fixture-csrf" || second["X-CSRF-Token"] != "fixture-csrf" {
		t.Fatal("binding a confirmation dropped authenticated CSRF context")
	}
	env.Options.RunID = "another-owned-run"
	if first["Idempotency-Key"] == pendingConfirmationHeaders(env, "scale-same-deployment", "act_111111111111111111111111")["Idempotency-Key"] {
		t.Fatal("key did not retain run identity")
	}
}

func TestScenarioConfirmLongLabelsAndRunIDsHonorCurrentOpenAPIKeyContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "api", "openapi", "components", "common.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Components struct{ Schemas map[string]map[string]any }
	}
	if err = yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	schema := doc.Components.Schemas["IdempotencyKey"]
	if schema == nil {
		t.Fatal("missing authoritative idempotency schema")
	}
	compiled, err := toolruntime.CompileInputSchema(map[string]any{"type": "object", "properties": map[string]any{"key": schema}, "required": []string{"key"}, "additionalProperties": false})
	if err != nil {
		t.Fatal(err)
	}
	env := &E2EEnvironment{State: NewScenarioState("owned"), Options: E2EOptions{RunID: strings.Repeat("r", 64)}, Endpoints: &E2EEndpoints{EnterpriseOrigin: "https://enterprise.example.test"}}
	for _, name := range []string{"plan8-service-confirm-file-log-final-stop-with-gateway-loss", strings.Repeat("long-label-", 200), "中文场景名称"} {
		key := pendingConfirmationHeaders(env, name, "act_111111111111111111111111")["Idempotency-Key"]
		if err = compiled.Validate(map[string]any{"key": key}); err != nil {
			t.Fatalf("confirmation key failed actual OpenAPI contract: %v", err)
		}
	}
}
