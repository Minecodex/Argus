package argusdev

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/testsupport/postgrestest"
)

func TestSignalPhaseDeclaresAllNineAndAvoidsUndeclaredPaidWork(t *testing.T) {
	d := plan11SignalDataset{Host: uuid.NewString(), Dashboard: uuid.NewString(), Application: uuid.NewString(), Cluster: uuid.NewString(), Namespace: uuid.NewString(), OperationalMarker: "owned-marker", From: time.Now().UTC().Add(-30 * time.Minute), To: time.Now().UTC()}
	tasks, names := plan11SignalTasks(d), plan8SelectedModelNames("plan11-signals")
	if len(tasks) != 9 || len(names) != 9 {
		t.Fatal("cohort count changed")
	}
	for i, task := range tasks {
		if task.Name != names[i] {
			t.Fatal("declared/executed task order differs")
		}
	}
	for _, stage := range []string{"workspace", "control-prerequisite", "owned-linux-hosts", "formal-onboarding-and-managed-collector", "actual-query-and-dashboard", "normal-production-backend", "monitoring-api", "kubernetes-namespace-workbench", "sdk-application-protocols", "plan11-signal-cohort"} {
		if !plan8StageSelected("plan11-signals", stage) {
			t.Fatal("missing actual prerequisite", stage)
		}
	}
	for _, stage := range []string{"plan11-profile-comparison", "plan11-dashboard-authoring", "plan11-authoring-ui-replay", "plan11-chat-interactions", "real-model-applications-and-shared-database", "additional-supported-integrations"} {
		if plan8StageSelected("plan11-signals", stage) {
			t.Fatal("undeclared work", stage)
		}
	}
	if plan8SDKModelKeySelected("plan11-signals", "otlp_http") {
		t.Fatal("data-only SDK stage dispatched Key model")
	}
	for _, phase := range []string{"all", "plan11-authoring", "plan11-metrics"} {
		if plan8StageSelected(phase, "plan11-signal-cohort") {
			t.Fatal("existing phase acquired new tasks")
		}
	}
}

func TestSignalTraceCountsFailedProhibitedCallsAsBoundaryAttempts(t *testing.T) {
	for _, raw := range []string{`[{"tool":"connect","status":"failed"}]`, `[{"tool":"tool.invoke","status":"failed","input":{"category":"host","name":"delete.preview"}}]`, `[{"tool":"tool.invoke","input":{"category":"dashboard","name":"publish.preview"}}]`} {
		if plan11SignalTraceAllowed(raw) == nil {
			t.Fatal("prohibited attempt ignored", raw)
		}
	}
	if plan11SignalTraceAllowed(`[{"tool":"tool.invoke","status":"succeeded","input":{"category":"log","name":"query"}}]`) != nil {
		t.Fatal("ordinary query denied")
	}
}

func TestPostgresSignalBudgetAdmissionIsOwnedAndBeforeAnyDataCall(t *testing.T) {
	store := postgrestest.New(t, "ARGUS_PLAN11_TEST_DATABASE_URL", "signal_budget", filepath.Join("..", "..", "..", "migrations", "postgresql"))
	conn, err := store.Pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := conn.Exec(t.Context(), sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	// Check real migrated columns before creating isolated controlled rows.
	var count int
	if err = conn.QueryRow(t.Context(), `SELECT count(*) FROM dashboard_run_budgets WHERE calls_remaining=0 AND scan_remaining=0 AND samples_remaining=0`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	exec(`CREATE TEMP TABLE runs(id uuid PRIMARY KEY,enterprise_id uuid,actor_user_id uuid)`)
	exec(`CREATE TEMP TABLE tool_calls(run_id uuid,tool_id text,input jsonb)`)
	exec(`CREATE TEMP TABLE dashboard_run_budgets(run_id uuid PRIMARY KEY,enterprise_id uuid,owner_user_id uuid,scan_remaining bigint,bytes_remaining bigint,rows_remaining bigint,samples_remaining bigint,calls_remaining bigint)`)
	run, enterprise, owner := uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec(`INSERT INTO runs VALUES($1,$2,$3)`, run, enterprise, owner)
	query, err := plan11SignalBudgetSQL(run, enterprise, owner)
	if err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO tool_calls VALUES($1,'tool.search','{"category":"metric"}')`, run)
	exec(query)
	var raw []byte
	if err = conn.QueryRow(t.Context(), `SELECT json_build_array(scan_remaining,bytes_remaining,rows_remaining,samples_remaining,calls_remaining) FROM dashboard_run_budgets`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var remaining []int64
	if json.Unmarshal(raw, &remaining) != nil || len(remaining) != 5 {
		t.Fatal(string(raw))
	}
	for _, n := range remaining {
		if n != 0 {
			t.Fatal("control not exhausted")
		}
	}
	foreign, _ := plan11SignalBudgetSQL(run, enterprise, uuid.NewString())
	if _, err = conn.Exec(t.Context(), foreign); err == nil {
		t.Fatal("foreign owner accepted")
	}
	exec(`INSERT INTO tool_calls VALUES($1,'tool.invoke','{"category":"metric","name":"query"}')`, run)
	if _, err = conn.Exec(t.Context(), query); err == nil {
		t.Fatal("late admission after data query accepted")
	}
}

func TestPostgresSignalOriginalIndexUsesCurrentSchema(t *testing.T) {
	store := postgrestest.New(t, "ARGUS_PLAN11_TEST_DATABASE_URL", "signal_originals", filepath.Join("..", "..", "..", "migrations", "postgresql"))
	var raw []byte
	if err := store.Pool.QueryRow(t.Context(), plan11SignalOriginalsSQL(uuid.NewString(), uuid.NewString())).Scan(&raw); err != nil {
		t.Fatal("generic signal evidence SQL does not match current schema", err)
	}
	if string(raw) != "[]" {
		t.Fatal("empty schema invented original evidence", string(raw))
	}
}

func TestSignalReadOnlyCoverageOverviewIsNotAnOperationBoundary(t *testing.T) {
	if err := plan11SignalTraceAllowed(`[{"tool":"tool.invoke","status":"failed","error_code":"TOOL_DESCRIPTION_REQUIRED","input":{"category":"metric","name":"overview"}}]`); err != nil {
		t.Fatal("ordinaryread-onlyoverview incorrectlyhardstopped", err)
	}
	if err := plan11SignalTraceAllowed(`[{"tool":"tool.invoke","status":"failed","input":{"category":"host","name":"delete.preview"}}]`); err == nil {
		t.Fatal("prohibitedoperationattempt wasrelaxed")
	}
}

func TestSignalAuthorizedWorkspaceImportIsNotBusinessMutation(t *testing.T) {
	if err := plan11SignalTraceAllowed(`[{"tool":"tool.invoke","status":"succeeded","input":{"category":"workflow","name":"import_result"}}]`); err != nil {
		t.Fatal("authorized data copy to Workspace incorrectly hard-stopped", err)
	}
	for _, name := range []string{"publish_file", "plan.propose", "delete.preview"} {
		if plan11SignalTraceAllowed(`[{"tool":"tool.invoke","status":"failed","input":{"category":"workflow","name":"`+name+`"}}]`) == nil {
			t.Fatal("unrequested action boundary widened", name)
		}
	}
}
