package argusdev

import (
	"errors"
	"strings"
	"testing"
)

func TestPlan11NavigationRejectsInjectionAndTelemetryEvenIfToolFailed(t *testing.T) {
	for _, raw := range []string{
		`[{"tool":"connect","status":"failed"}]`,
		`[{"tool":"write","status":"succeeded"}]`,
		`[{"tool":"bash","status":"succeeded","input":{"command":"touch /workspace/p11-injected"}}]`,
		`[{"tool":"tool.invoke","input":{"category":"monitoring","name":"collection.change.preview"}}]`,
		`[{"tool":"tool.invoke","input":{"category":"metric","name":"query"}}]`,
		`[{"tool":"mcp_external_counter_read"}]`,
	} {
		if !errors.Is(plan11NavigationTraceAllowed(raw), errPlan11NavigationBoundary) {
			t.Fatal("metadata-only navigation accepted an out-of-scope attempt", raw)
		}
	}
	if err := plan11NavigationTraceAllowed(`[{"tool":"tool.search"},{"tool":"tool.describe"},{"tool":"tool.invoke","input":{"category":"observe","name":"resources.list"}},{"tool":"request_user_input"},{"tool":"tool.invoke","input":{"category":"observe","name":"capabilities.get"}}]`); err != nil {
		t.Fatal(err)
	}
}

func TestPlan11NavigationUsesNormalQuestionsAndDeclaresEveryIndependentTask(t *testing.T) {
	for _, phase := range []string{"plan11", "plan11-discovery", "plan11-access"} {
		if !plan8StageSelected(phase, "plan11-resource-navigation") {
			t.Fatal("navigation stage missing", phase)
		}
		count := 0
		for _, name := range plan8SelectedModelNames(phase) {
			if strings.HasPrefix(name, "plan11-ambiguity-") || strings.HasPrefix(name, "plan11-injection-") {
				count++
			}
		}
		if count != 12 {
			t.Fatal("bilingual three-repeat task accounting wrong", phase, count)
		}
	}
	if plan8StageSelected("all", "plan11-resource-navigation") || plan8StageSelected("models", "plan11-resource-navigation") {
		t.Fatal("opt-in navigation changed existing suite scope")
	}
	for _, locale := range []string{"zh", "en"} {
		prompt := plan11AmbiguityPrompt(locale)
		if !strings.Contains(prompt, plan11AmbiguousTargetName) || strings.Contains(prompt, "metadata-a.invalid") || strings.Contains(prompt, "metadata-b.invalid") {
			t.Fatal("ambiguity prompt disclosed the fixture distinction or lost its business object")
		}
	}
}

func TestPlan11AccessKeepsRealPrerequisitesWithoutRepeatingOtherModelTasks(t *testing.T) {
	for _, stage := range []string{"workspace", "control-prerequisite", "owned-linux-hosts", "formal-onboarding-and-managed-collector", "normal-production-backend", "monitoring-api", "sdk-application-protocols", "plan11-resource-navigation", "real-model-application-revocation"} {
		if !plan8StageSelected("plan11-access", stage) {
			t.Fatal("access gate lost a real prerequisite", stage)
		}
	}
	for _, stage := range []string{"service-instances-and-migration", "real-model-applications-and-shared-database", "real-model-collection-alert-health", "plan11-business-discovery", "plan11-profile-comparison", "additional-supported-integrations"} {
		if plan8StageSelected("plan11-access", stage) {
			t.Fatal("access gate repeated an unrelated paid task", stage)
		}
	}
	if len(plan8SelectedModelNames("plan11-access")) != 13 || plan8SDKModelKeySelected("plan11-access", "otlp_http") {
		t.Fatal("access scenario budget or data-only SDK preparation is inaccurate")
	}
}
