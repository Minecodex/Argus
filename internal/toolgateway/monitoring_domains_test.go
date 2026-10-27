package toolgateway

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func monitoringDomainRegistry(t *testing.T, store *postgres.Store) *mcp.Registry {
	t.Helper()
	reg := mcp.NewRegistry()
	if err := (MonitoringTools{Base: ResourceTools{Store: store}, Monitoring: monitoring.Service{Store: store}, Kubernetes: kubernetesworkbench.Service{Store: store}}).Register(reg); err != nil {
		t.Fatal(err)
	}
	return reg
}

func TestMonitoringDomainsDiscoveryUsesSharedCategoriesAndHiddenCommits(t *testing.T) {
	reg := monitoringDomainRegistry(t, &postgres.Store{})
	g, err := New(reg, "plan8-domain-test")
	if err != nil {
		t.Fatal(err)
	}
	if err = reg.ValidatePairs(); err != nil {
		t.Fatal(err)
	}
	set, err := toolruntime.NewSet(toolruntime.Snapshot{}, g.CoreTools())
	if err != nil {
		t.Fatal(err)
	}
	p := toolruntime.Principal{EnterpriseID: uuid.New(), UserID: uuid.New(), Permissions: []string{"*"}}
	for _, category := range []string{"application", "monitoring", "k8s"} {
		result, e := set.Invoke(t.Context(), "tool.search", toolruntime.Invocation{Principal: p, Arguments: map[string]any{"category": category}})
		if e != nil {
			t.Fatal(e)
		}
		items := result.Data["items"].([]map[string]any)
		want := 7
		if category == "monitoring" {
			want = 9
		}
		if category == "k8s" {
			want = 10
		}
		if len(items) != want {
			t.Fatalf("%s returned %d tools", category, len(items))
		}
		for _, item := range items {
			if strings.Contains(item["name"].(string), "commit") {
				t.Fatal("commit disclosed")
			}
		}
	}
	for _, m := range reg.ModelCatalog() {
		m, _ = reg.Lookup(m.ID)
		if m.Template == nil || m.Present == nil {
			t.Fatalf("%s missing shared presentation", m.ID)
		}
		if e := m.Validate(map[string]any{"credential_draft_id": uuid.NewString()}); e == nil {
			t.Fatalf("%s accepted forged private channel", m.ID)
		}
	}
	meta, _ := reg.Lookup("kubernetes.object.list")
	empty := map[string]any{"cluster_id": uuid.NewString(), "type": "pod", "namespaces": map[string]any{"mode": "explicit", "ids": []any{}}}
	if err = meta.Validate(empty); err != nil {
		t.Fatalf("explicit empty namespace selection rejected: %v", err)
	}
	delete(empty, "namespaces")
	if meta.Validate(empty) == nil {
		t.Fatal("implicit namespace scope accepted")
	}
	preview, _ := reg.Lookup("monitoring.collection.change.preview")
	if preview.Validate(map[string]any{"kind": "collection", "operation": "create", "credentials": map[string]any{"password": "must-not-enter-model-tool"}}) == nil {
		t.Fatal("raw credentials accepted")
	}
	if preview.Validate(map[string]any{"kind": "application", "operation": "archive"}) == nil {
		t.Fatal("collection tool changed another domain")
	}
	encoded, _ := json.Marshal(preview.InputSchema)
	if strings.Contains(string(encoded), `"credentials":`) || strings.Contains(string(encoded), `"credential_draft_id":`) {
		t.Fatal("private channel advertised")
	}
}

func TestHelmToolSchemaKeepsPrivateMaterialOutOfModelContext(t *testing.T) {
	reg := monitoringDomainRegistry(t, &postgres.Store{})
	meta, ok := reg.Lookup("kubernetes.helm.change.preview")
	if !ok {
		t.Fatal("Helm tool missing")
	}
	input := map[string]any{"cluster_id": uuid.NewString(), "namespace_id": uuid.NewString(), "name": "sample", "operation": "rollback", "expected_revision": 2, "rollback_revision": 1, "reason": "Restore observed revision"}
	if e := meta.Validate(input); e != nil {
		t.Fatal(e)
	}
	for _, key := range []string{"chart_archive", "values", "private_material_json", "private_input_id", "sealed_plan", "source_credentials", "encrypted_chart"} {
		next := map[string]any{}
		for k, v := range input {
			next[k] = v
		}
		next[key] = "private-canary"
		if e := meta.Validate(next); e == nil {
			t.Fatal("model supplied protected Helm input", key)
		}
	}
	input["operation"] = "install"
	if e := meta.Validate(input); e == nil {
		t.Fatal("model bypassed trusted install form")
	}
}

func TestMonitoringPrivateInputRejectedAtEveryNestingLevel(t *testing.T) {
	if rejectPrivateDomainInput(map[string]any{"object": map[string]any{"env": []any{map[string]any{"name": "password", "value": "private-value"}}}}) == nil {
		t.Fatal("name/value credential bypassed common data policy")
	}
	for _, in := range []any{map[string]any{"object": map[string]any{"spec": map[string]any{"credentials": "x"}}}, map[string]any{"config": []any{map[string]any{"PASSWORD": "x"}}}, map[string]any{"secret_values": map[string]any{}}} {
		if rejectPrivateDomainInput(in) == nil {
			t.Fatal("nested private input accepted")
		}
	}
	if err := rejectPrivateDomainInput(map[string]any{"kind": "collection", "config": map[string]any{"interval": "30s"}}); err != nil {
		t.Fatal(err)
	}
}
