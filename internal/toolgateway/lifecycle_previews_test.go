package toolgateway

import (
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/telemetry"
)

func TestAllFormalPreviewFamiliesHaveStrictTemplatesAndHiddenCommits(t *testing.T) {
	base := ResourceTools{Store: &postgres.Store{}}
	registry := mcp.NewRegistry()
	for _, register := range []func(*mcp.Registry) error{base.Register, (LifecycleTools{Base: base}).Register, (CollectorPreviewTools{Base: base, Service: telemetry.Service{Store: base.Store}}).Register} {
		if err := register(registry); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := New(registry, "test-release"); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, item := range registry.ModelCatalog() {
		if strings.HasSuffix(item.ID, ".commit") {
			t.Fatalf("commit visible: %s", item.ID)
		}
		if !strings.HasSuffix(item.ID, ".preview") {
			continue
		}
		count++
		if item.Template == nil || item.Present == nil || item.InputSchema["additionalProperties"] != false {
			t.Fatalf("incomplete Preview: %s", item.ID)
		}
		commit, exists := registry.Lookup(strings.TrimSuffix(item.ID, ".preview") + ".commit")
		if !exists || commit.Visibility != mcp.Hidden {
			t.Fatalf("missing hidden commit for %s", item.ID)
		}
	}
	if count != 27 {
		t.Fatalf("got %d Previews; want all 27 formal operation variants", count)
	}
	for _, id := range []string{"host.removal.preview", "host.install.retry.preview", "host.windows_rdp.enable.preview", "connector.bastion.enrollment_rotate.preview", "connector.bastion.replacement.preview", "connector.install.retry.preview", "connector.uninstall.preview", "host.collector.install.preview", "kubernetes.collector.uninstall.preview", "kubernetes.node_host_binding.confirm.preview"} {
		if _, exists := registry.Lookup(id); !exists {
			t.Fatalf("missing %s", id)
		}
	}
}
