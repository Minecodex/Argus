package nativetools

import (
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/toolgateway"
	"strings"
	"testing"
)

func TestProductionCatalogNavigationAndDelivery(t *testing.T) {
	reg, err := Catalog()
	if err != nil {
		t.Fatal(err)
	}
	entries := reg.ModelCatalog()
	if len(entries) != 133 {
		t.Fatalf("public business tools=%d", len(entries))
	}
	found := map[string]bool{}
	for _, m := range entries {
		found[m.ID] = true
		if strings.HasSuffix(m.ID, ".commit") {
			t.Fatal("commit exposed", m.ID)
		}
		if err := m.Discovery.Navigation.Validate(); err != nil {
			t.Fatalf("%s: %v", m.ID, err)
		}
		binding, err := presentation.ToolBinding(m.ID)
		if err != nil {
			t.Fatal(err)
		}
		if binding.Mode == "template" && (m.Template == nil || m.Present == nil) {
			t.Fatal("missing template", m.ID)
		}
	}
	for _, id := range []string{"observe.resources.list", "observe.capabilities.get", "observe.catalog.query", "workflow.plan.propose"} {
		if !found[id] {
			t.Fatal("missing production tool", id)
		}
	}
	gateway, err := toolgateway.New(reg, "plan11-production")
	if err != nil {
		t.Fatal(err)
	}
	if len(gateway.CoreTools()) != 3 {
		t.Fatal("top-level meta-tool set changed")
	}
}
