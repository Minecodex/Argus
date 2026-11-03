package argusdev

import "testing"

func TestFocusedRevocationKeepsRealDataAndNoUnrelatedModelStages(t *testing.T) {
	for _, name := range []string{"workspace", "formal-onboarding-and-managed-collector", "normal-production-backend", "sdk-application-protocols", "real-model-application-revocation"} {
		if !plan8StageSelected("plan11-revocation", name) {
			t.Fatal("missing prerequisite", name)
		}
	}
	for _, name := range []string{"plan11-profile-comparison", "plan11-resource-navigation", "additional-supported-integrations", "real-model-workspace"} {
		if plan8StageSelected("plan11-revocation", name) {
			t.Fatal("unrelated stage selected", name)
		}
	}
	if plan8SDKModelKeySelected("plan11-revocation", "otlp_http") {
		t.Fatal("revocation fixture repeated model key delivery")
	}
	names := plan8SelectedModelNames("plan11-revocation")
	if len(names) != 1 || names[0] != "same_conversation_application_revocation" {
		t.Fatal("wrong composite model scenario declaration", names)
	}
}
