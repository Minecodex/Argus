package argusdev

import (
	"slices"
	"testing"
)

func TestReadOnlyDashboardAcceptanceKeepsDistinctActorAndNoManagementPrivileges(t *testing.T) {
	if plan11DashboardReaderSpec.Prefix == plan8TargetReaderSpec.Prefix || plan11DashboardReaderSpec.Alias == plan8TargetReaderSpec.Alias {
		t.Fatal("dashboard and data-only target readers share mutable session state")
	}
	for _, permission := range []string{"*", "telemetry.dashboard.manage", "model.usage.read", "role.manage", "user.manage"} {
		if slices.Contains(plan11DashboardReaderSpec.Permissions, permission) {
			t.Fatal("read-only fixture acquired a private/management permission", permission)
		}
	}
	if !slices.Equal(plan8TargetReaderSpec.Permissions, []string{"monitoring.read"}) {
		t.Fatal("existing target-only authorization acceptance was widened")
	}
	for _, phase := range []string{"plan11-compare", "plan11-review", "plan11", "all"} {
		if !plan8StageSelected(phase, "plan11-read-only-dashboards") {
			t.Fatal("read-only portal acceptance missing from its declared phase", phase)
		}
	}
	for _, phase := range []string{"plan11-revocation", "plan11-access", "models", "integrations", "kubernetes"} {
		if plan8StageSelected(phase, "plan11-read-only-dashboards") {
			t.Fatal("unrelated acceptance phase acquired an extra scenario", phase)
		}
	}
}
