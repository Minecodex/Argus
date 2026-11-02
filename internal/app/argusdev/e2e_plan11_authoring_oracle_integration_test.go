package argusdev

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/testsupport/postgrestest"
)

// First test the migrated production schema in authoring_test.go; this isolated
// session then uses controlled rows to verify the acceptance SQL's distinctions.
func TestPostgresBusinessWorkflowAuthoringEquivalentCurrentVersionVerification(t *testing.T) {
	store := postgrestest.New(t, "ARGUS_PLAN11_TEST_DATABASE_URL", "authoring_equivalent", filepath.Join("..", "..", "..", "migrations", "postgresql"))
	ctx := t.Context()
	conn, err := store.Pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := conn.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	for _, sql := range []string{
		`CREATE TEMP TABLE tool_calls(id uuid,enterprise_id uuid,run_id uuid,input jsonb,status text)`,
		`CREATE TEMP TABLE tool_results(tool_call_id uuid,enterprise_id uuid,projection jsonb)`,
		`CREATE TEMP TABLE dashboard_drafts(id uuid,enterprise_id uuid,status text,draft_version bigint)`,
		`CREATE TEMP TABLE pending_actions(run_id uuid,action_type text,status text)`,
		`CREATE TEMP TABLE dashboard_revisions(enterprise_id uuid,name text)`,
	} {
		exec(sql)
	}
	enterprise, run, draft, host, call := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	row := plan11AuthoringCase{Name: "Controlled current-version verification", Draft: draft}
	exec(`INSERT INTO dashboard_drafts VALUES($1,$2,'editing',2)`, draft, enterprise)
	exec(`INSERT INTO pending_actions VALUES($1,'telemetry.dashboard.publish','awaiting_confirmation')`, run)
	input := func(name string, version int) []byte {
		raw, _ := json.Marshal(map[string]any{"category": "dashboard", "name": name, "arguments": map[string]any{"draft_id": draft, "expected_version": version}})
		return raw
	}
	exec(`INSERT INTO tool_calls VALUES($1,$2,$3,$4,'succeeded')`, call, enterprise, run, input("publish.preview", 2))
	exec(`INSERT INTO tool_results VALUES($1,$2,'{}')`, call, enterprise)
	verify := func(want int) {
		t.Helper()
		var raw []byte
		if e := conn.QueryRow(ctx, plan11AuthoringBeforeSQL(run, host, enterprise, row)).Scan(&raw); e != nil {
			t.Fatal(e)
		}
		var counts map[string]int
		if json.Unmarshal(raw, &counts) != nil || counts["validations"] != want || counts["previews"] != 1 || counts["published"] != 0 {
			t.Fatalf("verification count: %s", raw)
		}
	}
	setPreview := func(resources []map[string]string, partial bool) {
		t.Helper()
		sample, _ := json.Marshal(map[string]any{"execution": map[string]any{"resources": resources, "partial": partial}})
		projection, _ := json.Marshal(map[string]any{"summary": map[string]any{"preview": map[string]any{"validation": map[string]any{"valid": true}, "sample_json": string(sample)}}})
		exec(`UPDATE tool_results SET projection=$1 WHERE tool_call_id=$2`, projection, call)
	}
	exact := []map[string]string{{"type": "host", "id": host}}
	setPreview(exact, false)
	verify(1) // Preview itself reruns the same verifier.
	setPreview([]map[string]string{{"type": "host", "id": uuid.NewString()}}, false)
	verify(0)
	setPreview(append(exact, map[string]string{"type": "host", "id": uuid.NewString()}), false)
	verify(0)
	setPreview(exact, true)
	verify(0)
	setPreview(exact, false)
	exec(`UPDATE tool_calls SET input=$1 WHERE id=$2`, input("publish.preview", 1), call)
	verify(0) // Stale successful checks cannot certify a newer draft.
	exec(`UPDATE tool_calls SET input=$1,status='failed' WHERE id=$2`, input("publish.preview", 2), call)
	verify(0)
	exec(`UPDATE tool_calls SET status='succeeded' WHERE id=$1`, call)
	verify(1)
}
