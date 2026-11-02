package argusdev

import (
	"github.com/google/uuid"
	"testing"
)

func TestPlan11GateIncludesFoundationAndBilingualRealCases(t *testing.T) {
	for _, stage := range []string{"workspace", "formal-onboarding-and-managed-collector", "kubernetes-namespace-workbench", "additional-supported-integrations", "plan11-business-discovery", "real-portal-workspace"} {
		if !plan8StageSelected("plan11", stage) {
			t.Fatal("missing full-flow stage", stage)
		}
	}
	if plan8StageSelected("all", "plan11-business-discovery") {
		t.Fatal("opt-in Plan11 changes old suite scope")
	}
	if len(plan8SelectedModelNames("plan11")) != 29 || len(plan8SelectedModelNames("models")) != 7 {
		t.Fatal("model budget inventory is inaccurate")
	}
}
func TestModelQueryOracleUnderstandsTypedScope(t *testing.T) {
	id := uuid.New()
	args := map[string]any{"scope": map[string]any{"mode": "explicit", "resources": []any{map[string]any{"type": "monitoring_target", "id": id.String()}}}}
	if !plan8ModelArgumentHasResource(args, id) || plan8ModelArgumentHasResource(args, uuid.New()) {
		t.Fatal("typed resource matching failed")
	}
	args["resource_ids"] = []any{id.String()}
	if plan8ModelArgumentHasResource(args, id) {
		t.Fatal("conflicting scope accepted by oracle")
	}
}

func TestIntegrationDataPhaseDoesNotCallTheExternalModel(t *testing.T) {
	if !plan8StageSelected("integrations", "sdk-application-protocols") || len(plan8SelectedModelNames("integrations")) != 0 {
		t.Fatal("explicit integration data phase must cover SDK protocols without real model calls")
	}
	if len(plan8SelectedModelNames("models")) != 7 {
		t.Fatal("model-key delivery coverage was removed from the model phase")
	}
}

func TestFocusedPlan11DiscoveryKeepsItsRealHostAndUIWithoutIntegrationDownloads(t *testing.T) {
	for _, stage := range []string{"workspace-device-prerequisites", "workspace", "control-prerequisite", "owned-linux-hosts", "formal-onboarding-and-managed-collector", "actual-query-and-dashboard", "normal-production-backend", "monitoring-api", "plan11-business-discovery"} {
		if !plan8StageSelected("plan11-discovery", stage) {
			t.Fatal("focused discovery lost a required actual-data prerequisite", stage)
		}
	}
	for _, stage := range []string{"sdk-application-protocols", "additional-supported-integrations", "service-instances-and-migration", "kubernetes-namespace-workbench", "real-model-workspace"} {
		if plan8StageSelected("plan11-discovery", stage) {
			t.Fatal("focused discovery requires an unrelated integration fixture", stage)
		}
	}
	if len(plan8SelectedModelNames("plan11-discovery")) != 21 || len(plan8SelectedModelNames("plan11")) != 29 {
		t.Fatal("focused discovery changed the full acceptance inventory or advertised budget")
	}
}
