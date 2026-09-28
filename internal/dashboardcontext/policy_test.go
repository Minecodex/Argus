package dashboardcontext

import "testing"

func TestStructuredModesDoNotGrantUnrelatedCapabilities(t *testing.T) {
	for _, id := range []string{"telemetry.promql.query", "telemetry.kql.query", "telemetry.skywalking.trace", "host.get", "host.update.preview"} {
		if !AllowsBusiness(None(), id) {
			t.Fatalf("ordinary chat lost %s", id)
		}
		for _, mode := range []string{"analyze", "create"} {
			if AllowsBusiness(Snapshot{Mode: mode}, id) {
				t.Fatalf("%s allowed %s", mode, id)
			}
		}
	}
	for _, id := range []string{"telemetry.dashboard.list", "telemetry.dashboard.get", "telemetry.dashboard.query", "telemetry.dashboard.query.resume", "workflow.publish_file"} {
		if !AllowsBusiness(Snapshot{Mode: "analyze"}, id) {
			t.Fatalf("analysis lost %s", id)
		}
	}
	for _, id := range []string{"telemetry.dashboard.draft.create", "telemetry.dashboard.draft.validate", "telemetry.dashboard.publish.preview", "telemetry.dashboard.catalog"} {
		if AllowsBusiness(Snapshot{Mode: "analyze"}, id) || !AllowsBusiness(Snapshot{Mode: "create"}, id) {
			t.Fatalf("creation boundary missing: %s", id)
		}
	}
}
func TestCreationCommandRequiresAnExactLeadingToken(t *testing.T) {
	for _, text := range []string{"/创建仪表盘", " /create-dashboard new ", "/创建仪表盘\tCPU", "/创建仪表盘\nCPU"} {
		if Command(text) == nil {
			t.Fatalf("command not recognized: %q", text)
		}
	}
	for _, text := range []string{"explain /创建仪表盘", "/创建仪表盘xxx", "/create-dashboard-other"} {
		if Command(text) != nil {
			t.Fatalf("accidental activation: %q", text)
		}
	}
}
