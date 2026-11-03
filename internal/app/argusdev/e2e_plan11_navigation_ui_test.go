package argusdev

import "testing"

func TestBusinessWorkflowNavigationCohortAvoidsRepeatedAnalysis(t *testing.T) {
	for _, phase := range []string{"plan11-navigation-ui", "plan11-navigation-publish"} {
		want, bound := 4, 72
		if phase == "plan11-navigation-publish" {
			want, bound = 6, 136
		}
		if len(plan8SelectedModelNames(phase)) != want || plan11NavigationCallBound(phase) != bound {
			t.Fatal("navigation budget does not match actual selected tasks", phase)
		}
		for _, stage := range []string{"workspace", "owned-linux-hosts", "formal-onboarding-and-managed-collector", "actual-query-and-dashboard", "normal-production-backend", "monitoring-api", "plan11-navigation-ui"} {
			if !plan8StageSelected(phase, stage) {
				t.Fatal("owned data/UI prerequisite missing", phase, stage)
			}
		}
		for _, stage := range []string{"plan11-business-workflows", "sdk-application-protocols", "plan11-chat-interactions", "plan11-profile-comparison", "additional-supported-integrations"} {
			if plan8StageSelected(phase, stage) {
				t.Fatal("unrelated or repeated paid workflow selected", phase, stage)
			}
		}
		if plan8StageSelected(phase, "plan11-dashboard-authoring") != (phase == "plan11-navigation-publish") {
			t.Fatal("authoring must be explicitly selected")
		}
	}
	for _, phase := range []string{"all", "plan11-workflows", "plan11-publish", "plan11-metrics", "plan11-compare"} {
		if plan8StageSelected(phase, "plan11-navigation-ui") {
			t.Fatal("existing phase acquired extra paid tasks", phase)
		}
	}
}
