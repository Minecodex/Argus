package argusdev

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
)

func TestPlan11InteractionsPhaseKeepsExplicitModernScope(t *testing.T) {
	for _, stage := range []string{"workspace", "owned-linux-hosts", "formal-onboarding-and-managed-collector", "actual-query-and-dashboard", "normal-production-backend", "monitoring-api", "plan11-chat-interactions"} {
		if !plan8StageSelected("plan11-interactions", stage) {
			t.Fatal("modern interaction phase lost actual prerequisite", stage)
		}
	}
	for _, stage := range []string{"plan11-profile-comparison", "plan11-resource-navigation", "sdk-application-protocols", "additional-supported-integrations", "real-model-application-revocation", "plan11-read-only-dashboards"} {
		if plan8StageSelected("plan11-interactions", stage) {
			t.Fatal("modern interaction phase gained an unrelated model task", stage)
		}
	}
	for _, phase := range []string{"all", "plan11", "plan11-compare", "plan11-access", "plan11-review", "integrations", "models"} {
		if plan8StageSelected(phase, "plan11-chat-interactions") {
			t.Fatal("existing phase silently acquired four new provider workflows", phase)
		}
	}
	if !slices.Equal(plan8SelectedModelNames("plan11-interactions"), []string{"plan11-interaction-request-approval", "plan11-interaction-full-access", "plan11-stop-request-approval", "plan11-stop-full-access"}) {
		t.Fatal("interaction workflow budget does not match the explicit cases")
	}
}

func TestPostgresPlan11InteractionEvidenceQueryMatchesCurrentSchema(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable migrated PostgreSQL")
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var raw []byte
	if err = store.Pool.QueryRow(t.Context(), plan11InteractionTraceSQL(uuid.NewString())).Scan(&raw); err != nil || !json.Valid(raw) || string(raw) != "[]" {
		t.Fatalf("current evidence SQL did not match schema: %v", err)
	}
}
