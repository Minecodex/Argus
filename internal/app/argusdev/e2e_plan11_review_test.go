package argusdev

import (
	"reflect"
	"testing"
)

func TestComparisonAdmissionUsesOnePhaseRuleWithoutBypassingLease(t *testing.T) {
	host := &plan6HostLoop{}
	for _, phase := range []string{"plan11-compare", "plan11-review"} {
		env := &E2EEnvironment{Options: E2EOptions{Suite: "plan8", Plan8Phase: phase}, leaseAcquired: true}
		if !plan11ComparisonPhase(phase) || !plan11ComparisonEnvironmentAllowed(env, host) {
			t.Fatal("valid comparison phase rejected", phase)
		}
		env.leaseAcquired = false
		if plan11ComparisonEnvironmentAllowed(env, host) {
			t.Fatal("phase bypassed environment lease", phase)
		}
		env.leaseAcquired = true
		env.Options.Suite = "p5"
		if plan11ComparisonEnvironmentAllowed(env, host) {
			t.Fatal("foreign suite admitted", phase)
		}
		env.Options.Suite = "plan8"
		if plan11ComparisonEnvironmentAllowed(env, nil) {
			t.Fatal("missing owned Host admitted", phase)
		}
	}
	if plan11ComparisonEnvironmentAllowed(nil, host) {
		t.Fatal("missing environment admitted")
	}
	for _, phase := range []string{"all", "models", "plan11-access", "plan11-targets", "plan11"} {
		env := &E2EEnvironment{Options: E2EOptions{Suite: "plan8", Plan8Phase: phase}, leaseAcquired: true}
		if plan11ComparisonPhase(phase) || plan11ComparisonEnvironmentAllowed(env, host) {
			t.Fatal("unselected phase admitted", phase)
		}
	}
}

func TestPlan11ReviewBudgetsAndPrerequisitesKeepTheSelectedScope(t *testing.T) {
	if len(plan11ReviewModelNames(3, nil)) != 50 || len(plan11ReviewModelNames(0, nil)) != 14 || len(plan11ReviewModelNames(1, []string{"http", "dns"})) != 27 {
		t.Fatal("review model budget lost its selected target or comparison repetitions")
	}
	for _, stage := range []string{"workspace", "formal-onboarding-and-managed-collector", "actual-query-and-dashboard", "normal-production-backend", "monitoring-api", "plan11-resource-navigation", "sdk-application-protocols", "real-model-application-revocation", "additional-supported-integrations", "plan11-profile-comparison"} {
		if !plan8StageSelected("plan11-review", stage) {
			t.Fatal("review lost an actual prerequisite", stage)
		}
	}
	for _, stage := range []string{"service-instances-and-migration", "kubernetes-namespace-workbench", "real-model-applications-and-shared-database", "plan11-business-discovery", "real-model-workspace"} {
		if plan8StageSelected("plan11-review", stage) {
			t.Fatal("review added an unrelated model scenario", stage)
		}
	}
	if plan8SDKModelKeySelected("plan11-review", "otlp_http") {
		t.Fatal("data-only SDK preparation dispatched an extra model")
	}
}

func TestPlan11ReviewFinishesMutationsBeforeFrozenComparisonAndKeepsCallbacks(t *testing.T) {
	var observed []string
	var stages []plan8DeployedStage
	for _, name := range []string{"prerequisite", "plan11-profile-comparison", "navigation", "SDK", "revocation-restore", "target-lifecycle"} {
		stages = append(stages, plan8DeployedStage{name: name, run: func() error { observed = append(observed, name); return nil }})
	}
	for _, stage := range plan8OrderedDeployedStages("plan11-review", stages) {
		if err := stage.run(); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(observed, []string{"prerequisite", "navigation", "SDK", "revocation-restore", "target-lifecycle", "plan11-profile-comparison"}) {
		t.Fatal("comparison ran before mutable fixtures settled or lost callbacks", observed)
	}
	if stages[1].name != "plan11-profile-comparison" || plan8OrderedDeployedStages("plan11-compare", stages)[1].name != "plan11-profile-comparison" {
		t.Fatal("review changed the ordinary phase sequence")
	}
}
