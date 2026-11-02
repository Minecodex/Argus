package argusdev

import "testing"

func TestSDKKeyDispatchMatchesDeclaredModelBudget(t *testing.T) {
	for _, phase := range []string{"integrations", "models", "plan11", "plan11-business", "all"} {
		declared := false
		for _, name := range plan8SelectedModelNames(phase) {
			declared = declared || name == "sdk_private_key_delivery"
		}
		dispatched := false
		for _, protocol := range []string{"otlp_http", "otlp_grpc", "jaeger"} {
			selected := plan8SDKModelKeySelected(phase, protocol)
			if phase == "integrations" && selected {
				t.Fatal("data-only SDK admission would dispatch an external model", protocol)
			}
			if protocol != "otlp_http" && selected {
				t.Fatal("SDK model admission expanded beyond the declared key-delivery task", protocol)
			}
			dispatched = dispatched || selected
		}
		if declared != dispatched || !plan8StageSelected(phase, "sdk-application-protocols") {
			t.Fatal("advertised model budget and actual SDK key dispatch disagree", phase)
		}
	}
}

func TestPlan11BusinessPhaseCombinesDeepModelAndKubernetesDependencies(t *testing.T) {
	for _, stage := range []string{"formal-onboarding-and-managed-collector", "sdk-application-protocols", "service-instances-and-migration", "kubernetes-namespace-workbench", "real-model-applications-and-shared-database", "real-model-application-revocation", "real-model-workspace"} {
		if !plan8StageSelected("plan11-business", stage) {
			t.Fatal("combined business phase lost prerequisite", stage)
		}
	}
	if plan8StageSelected("plan11-business", "additional-supported-integrations") || len(plan8SelectedModelNames("plan11-business")) != 8 {
		t.Fatal("combined business phase budget/scope changed")
	}
}

func TestPlan8PartialPhasesKeepPrerequisitesWithoutClaimingModelCoverage(t *testing.T) {
	for _, name := range []string{"formal-onboarding-and-managed-collector", "normal-production-backend", "actual-query-and-dashboard"} {
		if !plan8StageSelected("integrations", name) || !plan8StageSelected("kubernetes", name) || !plan8StageSelected("workspace", name) {
			t.Fatal("partial phase lost real enrollment prerequisites", name)
		}
	}
	if !plan8StageSelected("integrations", "additional-supported-integrations") || plan8StageSelected("integrations", "real-model-applications-and-shared-database") || plan8StageSelected("integrations", "existing-target-authorization") || plan8StageSelected("integrations", "kubernetes-namespace-workbench") {
		t.Fatal("integration phase scope broadened or needs missing prior data")
	}
	if !plan8StageSelected("kubernetes", "kubernetes-namespace-workbench") || plan8StageSelected("kubernetes", "additional-supported-integrations") || plan8StageSelected("invalid", "formal-onboarding-and-managed-collector") {
		t.Fatal("invalid Kubernetes phase scope")
	}
	if !plan8StageSelected("workspace", "real-model-workspace") || plan8StageSelected("workspace", "additional-supported-integrations") || plan8StageSelected("workspace", "kubernetes-namespace-workbench") || plan8StageSelected("workspace", "real-model-secure-form") {
		t.Fatal("workspace phase must use actual Host/dashboard prerequisites and only the selected model scenario")
	}
	for _, name := range []string{"real-model-secure-form", "existing-target-authorization", "additional-supported-integrations", "kubernetes-namespace-workbench"} {
		if !plan8StageSelected("all", name) {
			t.Fatal("all gate silently skipped a phase", name)
		}
	}
	for _, input := range []string{"postgresql,postgresql", "unavailable", "postgresql, oracle", ","} {
		if _, err := plan8ParseIntegrationGroups(input); err == nil {
			t.Fatal("invalid group selection accepted", input)
		}
	}
	if groups, err := plan8ParseIntegrationGroups("postgresql,oracle,http-tcp-tls-dns-icmp"); err != nil || len(groups) != 3 {
		t.Fatal("explicit actual integration groups rejected", err)
	}
}

func TestPlan8ModelPhaseKeepsRealDataDependenciesAndRemainsPartial(t *testing.T) {
	for _, name := range []string{"workspace-device-prerequisites", "workspace", "control-prerequisite", "owned-linux-hosts", "formal-onboarding-and-managed-collector", "actual-query-and-dashboard", "normal-production-backend", "monitoring-api", "service-instances-and-migration", "sdk-application-protocols", "real-model-applications-and-shared-database", "real-model-secure-form", "real-model-collection-alert-health", "real-model-application-revocation", "real-model-workspace"} {
		if !plan8StageSelected("models", name) || !plan8StageSelected("all", name) {
			t.Fatal("model phase or full phase lost required real data/model stage", name)
		}
	}
	for _, name := range []string{"host-application-files-and-system-journal", "kubernetes-namespace-workbench", "additional-services-and-managed-dashboards", "existing-target-authorization", "additional-supported-integrations", "real-portal-workspace"} {
		if plan8StageSelected("models", name) || !plan8StageSelected("all", name) {
			t.Fatal("partial model diagnostic altered the full release inventory", name)
		}
	}
}
