package argusdev

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
)

func TestResourceIdentityPhaseDeclaresIndependentPlacementAndClarificationTasks(t *testing.T) {
	names := plan8SelectedModelNames("plan11-identity")
	if len(names) != 12 || !plan8StageSelected("plan11-identity", "plan11-resource-identity") {
		t.Fatal("identity phase lost its twelve independent tasks")
	}
	for _, phase := range []string{"all", "plan11", "plan11-access", "plan11-mcp", "plan11-interactions"} {
		if plan8StageSelected(phase, "plan11-resource-identity") {
			t.Fatal("existing phase silently gained identity model calls", phase)
		}
	}
	for _, stage := range []string{"formal-onboarding-and-managed-collector", "actual-query-and-dashboard", "normal-production-backend", "monitoring-api"} {
		if !plan8StageSelected("plan11-identity", stage) {
			t.Fatal("identity phase lost its real environment prerequisite", stage)
		}
	}
	for _, stage := range []string{"plan11-chat-interactions", "plan11-customer-mcp", "sdk-application-protocols", "plan11-resource-navigation"} {
		if plan8StageSelected("plan11-identity", stage) {
			t.Fatal("identity phase gained unrelated provider tasks", stage)
		}
	}
	for _, locale := range []string{"zh", "en"} {
		prompt := plan11PlacementPrompt(locale, "authorized-host-id")
		if !strings.Contains(prompt, "authorized-host-id") || !strings.Contains(prompt, "production") && !strings.Contains(prompt, "生产") {
			t.Fatal("placement prompt omitted actual requested identity constraints")
		}
		if strings.Contains(plan11SameAddressQuestionPrompt(locale), "authorized-host-id") {
			t.Fatal("insufficient-intent task leaked the selected placement")
		}
	}
}

func TestPostgresPlan11IdentityEvidenceQueriesUseCurrentSchema(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable migrated PostgreSQL")
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, query := range []string{plan11ApplicationIdentityReadSQL(uuid.NewString(), uuid.New()), plan11PlacementReadSQL(uuid.NewString(), uuid.New(), uuid.NewString())} {
		var count int64
		if err := store.Pool.QueryRow(t.Context(), query).Scan(&count); err != nil || count != 0 {
			t.Fatal("current identity evidence query failed or matched another run", err, count)
		}
	}
}

func TestSameAddressQuestionsNeedAnActualDistinctIdentityFact(t *testing.T) {
	id := uuid.New()
	if plan11QuestionIdentifies(plan11IdentityEndpoint, id, []string{""}) {
		t.Fatal("empty fact or common endpoint resolved an ambiguous object")
	}
	if !plan11QuestionIdentifies("Use "+id.String(), id, nil) || !plan11QuestionIdentifies("Collector on authorized Host A", id, []string{"authorized Host A"}) {
		t.Fatal("actual ID or authorized distinct placement was not accepted")
	}
}

func TestIdentityReadTraceCannotTurnPlacementLookupIntoMutation(t *testing.T) {
	if err := plan11IdentityTraceAllowed(`[{"Tool":"tool.invoke","Input":{"Category":"application","Name":"list"}},{"Tool":"tool.invoke","Input":{"Category":"monitoring","Name":"collector.choices"}},{"Tool":"tool.invoke","Input":{"Category":"host","Name":"get"}},{"Tool":"tool.invoke","Input":{"Category":"connector","Name":"collector.list"}},{"Tool":"tool.invoke","Input":{"Category":"connector","Name":"collector.get"}}]`); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`[{"Tool":"connect"}]`, `[{"Tool":"tool.invoke","Input":{"Category":"application","Name":"change.preview"}}]`, `[{"Tool":"tool.invoke","Input":{"Category":"metric","Name":"query"}}]`} {
		if !errors.Is(plan11IdentityTraceAllowed(raw), errPlan11NavigationBoundary) {
			t.Fatal("metadata identity lookup accepted mutation or telemetry", raw)
		}
	}
}
