package alerting

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"strings"
	"testing"
)

func TestNativeHubAlertCatalogAndStrictRules(t *testing.T) {
	registry := mcp.NewRegistry()
	if err := (Tools{Service: Service{Store: &postgres.Store{}}}).Register(registry); err != nil {
		t.Fatal(err)
	}
	if err := registry.ValidatePairs(); err != nil {
		t.Fatal(err)
	}
	for _, metadata := range registry.ModelCatalog() {
		if strings.HasSuffix(metadata.ID, ".commit") {
			t.Fatal("commit exposed to model")
		}
		if metadata.Template == nil || len(metadata.Discovery.Examples) == 0 {
			t.Fatal(metadata.ID, "missing presentation or discovery")
		}
	}
	metadata, ok := registry.Lookup("alert.policy.create.preview")
	if !ok {
		t.Fatal("missing create tool")
	}
	schema, err := toolruntime.CompileInputSchema(metadata.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(PolicySpec{Enabled: true, Resources: ResourceSelector{Mode: "dynamic", Types: []string{"host"}}, Rules: []Rule{{Name: "CPU", Expression: "host_cpu_usage_percent", QueryMode: "numeric", Operator: ">", Threshold: 85, RecoveryThreshold: 75, Severity: "critical"}}, Mode: "independent", IntervalSeconds: 30, ForSeconds: 300, RecoverySeconds: 180, NoDataBehavior: "report", ErrorBehavior: "keep"})
	normalized, e := normalizeMutation(Mutation{Kind: "policy", Operation: "create", Name: "CPU", Spec: raw})
	if e != nil {
		t.Fatal(e)
	}
	raw = normalized.Spec
	var spec any
	_ = json.Unmarshal(raw, &spec)
	if err = schema.Validate(map[string]any{"name": "CPU", "spec": spec}); err != nil {
		t.Fatal(err)
	}
	update, ok := registry.Lookup("alert.policy.update.preview")
	if !ok {
		t.Fatal("missing update tool")
	}
	updateSchema, err := toolruntime.CompileInputSchema(update.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"id": uuid.NewString(), "name": "CPU", "expected_version": 1, "spec": spec}
	if err = updateSchema.Validate(input); err != nil {
		t.Fatal("explicit rule identity rejected", err)
	}
	rule := spec.(map[string]any)["rules"].([]any)[0].(map[string]any)
	identity := rule["id"]
	delete(rule, "id")
	if err = updateSchema.Validate(input); err == nil {
		t.Fatal("update schema permits missing rule identity")
	}
	if err = schema.Validate(map[string]any{"name": "CPU", "spec": spec}); err != nil {
		t.Fatal("update requirements leaked into create schema", err)
	}
	rule["id"] = identity
	spec.(map[string]any)["private_token"] = "forbidden"
	if err = schema.Validate(map[string]any{"name": "CPU", "spec": spec}); err == nil {
		t.Fatal("unknown field accepted")
	}
}
func TestPreviewTemplateVariablesAndSecretReferences(t *testing.T) {
	pattern := normalizeTemplate("[{{severity}}] {{policy}} {{.value}}")
	if pattern != "[{{.severity}}] {{.policy}} {{.value}}" {
		t.Fatal(pattern)
	}
	got := previewSpec([]byte(`{"credential_id":"reference-only","display_address":"operations","enabled":true}`))
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "credential_id") || !strings.Contains(string(raw), "reference_id") {
		t.Fatal(string(raw))
	}
}
