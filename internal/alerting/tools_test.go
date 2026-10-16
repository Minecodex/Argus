package alerting

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/toolgateway"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"strings"
	"testing"
)

func TestAlertSilenceCategoryIncludesAuthorizedCreation(t *testing.T) {
	registry := mcp.NewRegistry()
	if err := (Tools{Service: Service{Store: &postgres.Store{}}}).Register(registry); err != nil {
		t.Fatal(err)
	}
	hub, err := toolgateway.New(registry, "plan6-discovery-regression")
	if err != nil {
		t.Fatal(err)
	}
	set, err := toolruntime.NewSet(toolruntime.Snapshot{NativeCatalog: hub.Revision}, hub.CoreTools())
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"mute silence suppress alert temporary", "create maintenance window downtime", "创建 临时 屏蔽", "temporary silence"} {
		call := toolruntime.Invocation{Principal: toolruntime.Principal{EnterpriseID: uuid.New(), UserID: uuid.New(), Permissions: []string{"alert.silence.manage"}}, Arguments: map[string]any{"category": "alert"}}
		result, err := set.Invoke(t.Context(), "tool.search", call)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, item := range result.Data["items"].([]map[string]any) {
			found = found || item["name"] == "silence.create.preview"
			if strings.HasSuffix(item["name"].(string), ".commit") {
				t.Fatal("private commit discovered")
			}
		}
		if !found {
			t.Fatalf("failed-model query %q cannot discover silence creation", query)
		}
		call.Principal.Permissions = []string{"alert.read"}
		result, err = set.Invoke(t.Context(), "tool.search", call)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range result.Data["items"].([]map[string]any) {
			if item["name"] == "silence.create.preview" {
				t.Fatal("discovery bypassed revoked write permission")
			}
		}
	}
}

func TestCurrentAlertCategoryIncludesEventsAndHealth(t *testing.T) {
	registry := mcp.NewRegistry()
	if err := (Tools{Service: Service{Store: &postgres.Store{}}}).Register(registry); err != nil {
		t.Fatal(err)
	}
	hub, err := toolgateway.New(registry, "plan6-current-alert-discovery")
	if err != nil {
		t.Fatal(err)
	}
	set, err := toolruntime.NewSet(toolruntime.Snapshot{NativeCatalog: hub.Revision}, hub.CoreTools())
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"cpu", "cluster alerts list", "cluster CPU", "当前告警 评估健康", "memory no data"} {
		call := toolruntime.Invocation{Principal: toolruntime.Principal{EnterpriseID: uuid.New(), UserID: uuid.New(), Permissions: []string{"alert.read"}}, Arguments: map[string]any{"category": "alert"}}
		result, err := set.Invoke(t.Context(), "tool.search", call)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, item := range result.Data["items"].([]map[string]any) {
			if item["name"] == "event.query" {
				found = true
			}
			if strings.HasSuffix(item["name"].(string), ".commit") {
				t.Fatal("private commit discovered")
			}
		}
		if !found {
			t.Fatalf("current-alert question %q only discovers configuration tools", query)
		}
	}
}

func TestNativeHubAlertCatalogAndStrictRules(t *testing.T) {
	registry := mcp.NewRegistry()
	if err := (Tools{Service: Service{Store: &postgres.Store{}}}).Register(registry); err != nil {
		t.Fatal(err)
	}
	if err := registry.ValidatePairs(); err != nil {
		t.Fatal(err)
	}
	if _, err := toolgateway.New(registry, "plan6-test"); err != nil {
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
