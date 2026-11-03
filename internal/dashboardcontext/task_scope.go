package dashboardcontext

import "github.com/kakj-go/Argus/internal/toolruntime"

const (
	GeneralDiagnosis   = "general_diagnosis"
	DashboardAnalysis  = "dashboard_analysis"
	DashboardAuthoring = "dashboard_authoring"
)

// EffectiveTaskScope preserves pre-Plan11 snapshots. Only new admission writes
// an explicit task scope; replay cannot silently widen an old analysis Run.
func EffectiveTaskScope(s Snapshot) string {
	if s.TaskScope != "" {
		return s.TaskScope
	}
	return scopeForMode(s.Mode)
}
func scopeForMode(mode string) string {
	switch mode {
	case "analyze":
		return DashboardAnalysis
	case "create":
		return DashboardAuthoring
	default:
		return GeneralDiagnosis
	}
}
func allowsTopLevel(s Snapshot, t toolruntime.Definition) bool {
	if EffectiveTaskScope(s) == GeneralDiagnosis {
		return true
	}
	if t.Source == "sandbox" {
		return true
	}
	if t.Source != "argus" {
		return false
	}
	switch t.Model.Name {
	case "tool.search", "tool.describe", "tool.invoke", "request_user_input":
		return true
	}
	return false
}
