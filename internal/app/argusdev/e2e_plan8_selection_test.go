package argusdev

import "testing"

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
