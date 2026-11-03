package dashboardcontext

import (
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"testing"
)

func TestTaskScopeDoesNotDependOnAcquiredDashboard(t *testing.T) {
	ordinary := None()
	ordinary.Mode = "analyze"
	for _, id := range []string{"telemetry.promql.query", "telemetry.kql.query", "alert.event.query", "observe.catalog.query", "workflow.plan.propose"} {
		if !AllowsBusiness(ordinary, id) {
			t.Fatal("general diagnosis lost tool after dashboard discovery", id)
		}
		if AllowsBusiness(Snapshot{Mode: "analyze"}, id) {
			t.Fatal("legacy analysis widened", id)
		}
		if AllowsBusiness(Snapshot{Mode: "analyze", TaskScope: DashboardAnalysis}, id) {
			t.Fatal("explicit analysis widened", id)
		}
	}
	connect := toolruntime.Definition{Source: "argus", Model: modelprovider.Tool{Name: "connect"}}
	if allowsTopLevel(Snapshot{Mode: "analyze", TaskScope: DashboardAnalysis}, connect) {
		t.Fatal("remote execution bypassed restricted analysis")
	}
	if !allowsTopLevel(None(), connect) {
		t.Fatal("ordinary remote execution lost")
	}
}
func TestUserCanExplicitlyConstrainAndResetDiagnosis(t *testing.T) {
	for _, text := range []string{"/分析仪表盘 CPU", "/analyze-dashboard CPU", "只分析这张仪表盘", "only analyze this dashboard"} {
		if c := Command(text); c == nil || c.Mode != "analyze" {
			t.Fatalf("missing explicit restriction: %s", text)
		}
	}
	for _, text := range []string{"/综合诊断", "/diagnose"} {
		if c := Command(text); c == nil || c.Mode != "none" {
			t.Fatal(text)
		}
	}
	if Command("日志中包含 only analyze this dashboard") != nil {
		t.Fatal("embedded data treated as a scope command")
	}
}
