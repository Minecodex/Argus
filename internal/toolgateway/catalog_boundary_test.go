package toolgateway_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/alerting"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/toolgateway"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"github.com/kakj-go/Argus/internal/workspace"
)

func TestRemovedModelEntriesCannotBeDiscoveredDescribedOrInvoked(t *testing.T) {
	store := &postgres.Store{}
	base := toolgateway.ResourceTools{Store: store}
	registry := mcp.NewRegistry()
	for _, register := range []func(*mcp.Registry) error{
		base.Register, (toolgateway.LifecycleTools{Base: base}).Register,
		(toolgateway.CollectorPreviewTools{Base: base}).Register,
		(telemetry.Tools{Service: telemetry.Service{Store: store}}).Register,
		(workspace.Service{}).RegisterTools,
		(toolgateway.DashboardTools{Service: dashboard.Service{Store: store}}).Register,
		(alerting.Tools{Service: alerting.Service{Store: store}}).Register,
	} {
		if err := register(registry); err != nil {
			t.Fatal(err)
		}
	}
	gateway, err := toolgateway.New(registry, "catalog-boundary-test")
	if err != nil {
		t.Fatal(err)
	}
	set, err := toolruntime.NewSet(toolruntime.Snapshot{}, gateway.CoreTools())
	if err != nil {
		t.Fatal(err)
	}
	p := toolruntime.Principal{EnterpriseID: uuid.New(), UserID: uuid.New(), Permissions: []string{"*"}}
	expected := map[string]int{"host": 12, "k8s": 19, "metric": 2, "trace": 1, "log": 1, "connector": 11, "workflow": 4, "dashboard": 15, "alert": 38}
	found := map[string]bool{}
	for category, count := range expected {
		arguments := map[string]any{"category": category}
		result, err := set.Invoke(t.Context(), "tool.search", toolruntime.Invocation{Principal: p, Arguments: arguments})
		if err != nil {
			t.Fatal(err)
		}
		items := result.Data["items"].([]map[string]any)
		actual := len(items)
		if result.Data["complete"] != true || !result.DataIsDiscovery {
			t.Fatal("category list is not complete model metadata")
		}
		for _, item := range items {
			found[category+"/"+item["name"].(string)] = true
		}
		if actual != count {
			t.Fatalf("%s catalog=%d, want %d", category, actual, count)
		}
	}
	removed := map[string][]string{
		"host": {"windows_rdp.enable.preview"}, "workflow": {"list"},
		"dashboard": {"budget.get", "convert", "draft.drilldowns"},
		"alert": {"settings.create.preview", "settings.update.preview", "settings.delete.preview",
			"channel.create.preview", "channel.update.preview", "channel.delete.preview", "channel.enable.preview", "channel.disable.preview",
			"grouping.create.preview", "grouping.update.preview", "grouping.delete.preview",
			"template.create.preview", "template.update.preview", "template.delete.preview"},
	}
	for category, names := range removed {
		for _, name := range names {
			if found[category+"/"+name] {
				t.Fatalf("removed entry is searchable: %s/%s", category, name)
			}
			for _, tool := range []string{"tool.describe", "tool.invoke"} {
				arguments := map[string]any{"category": category, "name": name}
				if tool == "tool.invoke" {
					arguments["arguments"] = map[string]any{}
				}
				_, err := set.Invoke(t.Context(), tool, toolruntime.Invocation{Principal: p, Arguments: arguments})
				var failure toolruntime.Error
				if !errors.As(err, &failure) || failure.Kind != "TOOL_NOT_FOUND" {
					t.Fatalf("%s still accepts %s/%s: %v", tool, category, name, err)
				}
			}
		}
	}
	for _, id := range []string{"alert.settings.list", "alert.channel.list", "alert.grouping.list", "alert.template.list", "alert.notification.test.preview", "alert.notification.resend.preview", "workflow.import_result", "workflow.publish_file", "pending_action.get", "pending_action.cancel"} {
		if _, ok := registry.Lookup(id); !ok {
			t.Fatalf("required read/handling/file entry was removed: %s", id)
		}
	}
	for _, metadata := range registry.ModelCatalog() {
		if strings.HasSuffix(metadata.ID, ".commit") {
			t.Fatal("commit exposed to model")
		}
	}
}
