package argusdev

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/testsupport/postgrestest"
)

func TestBusinessWorkflowPublicationPhasePreservesPrerequisitesAndBound(t *testing.T) {
	if len(plan8SelectedModelNames("plan11-publish")) != 7 {
		t.Fatal("publication cohort must declare all seven initial tasks")
	}
	for _, stage := range []string{"workspace-device-prerequisites", "workspace", "control-prerequisite", "owned-linux-hosts", "formal-onboarding-and-managed-collector", "actual-query-and-dashboard", "normal-production-backend", "monitoring-api", "plan11-business-workflows", "plan11-dashboard-authoring"} {
		if !plan8StageSelected("plan11-publish", stage) {
			t.Fatal("missing owned prerequisite", stage)
		}
	}
	for _, stage := range []string{"sdk-application-protocols", "plan11-chat-interactions", "additional-supported-integrations", "plan11-profile-comparison"} {
		if plan8StageSelected("plan11-publish", stage) {
			t.Fatal("undeclared paid or unrelated stage", stage)
		}
	}
	for _, phase := range []string{"all", "plan11-workflows", "plan11-metrics", "plan11-compare"} {
		if plan8StageSelected(phase, "plan11-dashboard-authoring") {
			t.Fatal("existing phase acquired authoring work", phase)
		}
	}
	if !plan8IndependentModelStage("plan11-business-workflows") || !plan8IndependentModelStage("plan11-dashboard-authoring") {
		t.Fatal("soft assertion failure must not suppress independent evidence")
	}
}

func TestBusinessWorkflowAuthoringOnlyAvoidsRepeatingNavigation(t *testing.T) {
	if len(plan8SelectedModelNames("plan11-authoring")) != 2 {
		t.Fatal("authoring-only budget changed")
	}
	for _, stage := range []string{"workspace", "owned-linux-hosts", "actual-query-and-dashboard", "normal-production-backend", "monitoring-api", "plan11-dashboard-authoring"} {
		if !plan8StageSelected("plan11-authoring", stage) {
			t.Fatal("missing actual prerequisite", stage)
		}
	}
	for _, stage := range []string{"plan11-navigation-ui", "plan11-business-workflows", "sdk-application-protocols", "plan11-chat-interactions", "plan11-profile-comparison"} {
		if plan8StageSelected("plan11-authoring", stage) {
			t.Fatal("authoring-only repeated unrelated task", stage)
		}
	}
}

func TestPostgresBusinessWorkflowAuthoringOracleUsesCurrentSchema(t *testing.T) {
	store := postgrestest.New(t, "ARGUS_PLAN11_TEST_DATABASE_URL", "authoring_oracle", filepath.Join("..", "..", "..", "migrations", "postgresql"))
	row := plan11AuthoringCase{Name: "Schema only", Draft: uuid.NewString(), Conversation: uuid.NewString(), Action: "not-an-action", Decision: "cancel"}
	var raw []byte
	if err := store.Pool.QueryRow(t.Context(), plan11AuthoringBeforeSQL(uuid.NewString(), uuid.NewString(), uuid.NewString(), row)).Scan(&raw); err != nil {
		t.Fatal("authoring admission SQL does not match current migrated schema", err)
	}
	var counts map[string]int
	if json.Unmarshal(raw, &counts) != nil || len(counts) != 4 {
		t.Fatal("invalid before-confirmation counts", string(raw))
	}
	for key, count := range counts {
		if count != 0 {
			t.Fatal("empty oracle invented evidence", key, count)
		}
	}
	if err := store.Pool.QueryRow(t.Context(), plan11AuthoringStateSQL(uuid.NewString(), row)).Scan(&raw); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("missing draft or final-state SQL was misclassified", err)
	}
}

func TestBusinessWorkflowAuthoringTraceKeepsConfirmationBoundary(t *testing.T) {
	for _, raw := range []string{
		`[{"tool":"tool.search","input":{"category":"dashboard"}},{"tool":"tool.invoke","status":"succeeded","input":{"category":"observe","name":"catalog.query"}},{"tool":"tool.invoke","status":"succeeded","input":{"category":"dashboard","name":"draft.validate"}},{"tool":"tool.invoke","status":"succeeded","input":{"category":"dashboard","name":"publish.preview"}}]`,
		`[{"tool":"bash","status":"succeeded"}]`,
	} {
		if err := plan11AuthoringTraceAllowed(raw); err != nil {
			t.Fatal(err)
		}
	}
	for _, raw := range []string{
		`[{"tool":"connect","status":"failed"}]`,
		`[{"tool":"mcp_customer_write","status":"failed"}]`,
		`[{"tool":"tool.invoke","input":{"category":"dashboard","name":"publish.commit"}}]`,
		`[{"tool":"tool.invoke","input":{"category":"observe","name":"publish.preview"}}]`,
		`[{"tool":"tool.invoke","input":{"category":"host","name":"create.preview"}}]`,
	} {
		if !errors.Is(plan11AuthoringTraceAllowed(raw), errPlan11NavigationBoundary) {
			t.Fatal("unrelated execution/hidden commit was not rejected", raw)
		}
	}
}

func TestBusinessWorkflowAuthoringUIReplayHasNoAutonomousModelTasks(t *testing.T) {
	const phase = "plan11-authoring-ui-replay"
	if len(plan8SelectedModelNames(phase)) != 0 {
		t.Fatal("UI-only replay added a paid model task")
	}
	for _, name := range []string{"workspace", "control-prerequisite", "owned-linux-hosts", "formal-onboarding-and-managed-collector", "actual-query-and-dashboard", "normal-production-backend", "monitoring-api", phase} {
		if !plan8StageSelected(phase, name) {
			t.Fatal("missing actual prerequisite", name)
		}
	}
	for _, name := range []string{"plan11-dashboard-authoring", "sdk-application-protocols", "plan11-chat-interactions", "plan11-profile-comparison", "plan11-navigation-ui", "plan11-business-workflows"} {
		if plan8StageSelected(phase, name) {
			t.Fatal("UI replay selected an unrelated/paid stage", name)
		}
	}
	for _, other := range []string{"all", "plan11-authoring", "plan11-publish", "plan11-workflows"} {
		if plan8StageSelected(other, phase) {
			t.Fatal("existing phase silently acquired replay", other)
		}
	}
	rows, err := plan11RecordedAuthoringSpecs()
	if err != nil || len(rows) != 2 {
		t.Fatal("recorded real-model specifications unavailable", err)
	}
	for _, row := range rows {
		if !dashboard.Validate(row.Spec).Valid {
			t.Fatal("recorded specification invalid")
		}
	}
}

func TestBusinessWorkflowAuthoringReplayPrecedesNormalBackendPolicy(t *testing.T) {
	stages := []plan8DeployedStage{{name: "actual-query-and-dashboard"}, {name: "normal-production-backend"}, {name: "monitoring-api"}, {name: "plan11-authoring-ui-replay"}, {name: "plan11-authoring-ui-production-readback"}}
	actual := plan8OrderedDeployedStages("plan11-authoring-ui-replay", stages)
	want := []string{"actual-query-and-dashboard", "plan11-authoring-ui-replay", "normal-production-backend", "monitoring-api", "plan11-authoring-ui-production-readback"}
	if len(actual) != len(want) {
		t.Fatal("ordered replay lost a prerequisite")
	}
	for i, name := range want {
		if actual[i].name != name {
			t.Fatal("private fixture attempted after normal endpoint policy", i, actual[i].name)
		}
	}
	if !plan8StageSelected("plan11-authoring-ui-replay", "plan11-authoring-ui-production-readback") {
		t.Fatal("normal readback omitted")
	}
}
