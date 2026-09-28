package dashboard

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestFileDrilldownContextBindsImmutableAttemptAndDepth(t *testing.T) {
	parent := db.DashboardQueryJob{ID: uuid.New(), DashboardID: uuid.New(), RevisionID: uuid.New(), AttemptID: nullID(uuid.New())}
	m := QueryManifest{Schema: "argus.dashboard_query_manifest/v1", JobID: parent.ID, AttemptID: parent.AttemptID.UUID, Compiler: CompilerVersion, Execution: Execution{DashboardID: parent.DashboardID, RevisionID: parent.RevisionID, Panels: []PanelExecution{{ID: "trace", Targets: []TargetExecution{{ID: "detail"}}}}}, Drilldown: &QueryDrilldownContext{Depth: 16, PanelID: "trace", TargetID: "detail"}}
	frozen, err := fileExecutionContext(parent, m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (Runtime{}).prepareDrilldown(context.Background(), Actor{}, EmptySpec(), frozen, DrilldownInput{}, nil); !errors.Is(err, ErrContextExpired) {
		t.Fatal("persistent files bypassed chain depth")
	}
	for _, change := range []func(*QueryManifest){
		func(m *QueryManifest) { m.AttemptID = uuid.New() }, func(m *QueryManifest) { m.Execution.RevisionID = uuid.New() }, func(m *QueryManifest) { m.Schema = "unknown" }, func(m *QueryManifest) { m.Compiler = "old" }, func(m *QueryManifest) {
			m.Drilldown = &QueryDrilldownContext{Depth: 1, PanelID: "trace", TargetID: "another"}
		},
	} {
		copy := m
		change(&copy)
		if _, err := fileExecutionContext(parent, copy); err == nil {
			t.Fatal("foreign file context accepted")
		}
	}
	if hasQueryOverrides(ExecutionInput{}) {
		t.Fatal("empty input is not an override")
	}
	if !hasQueryOverrides(ExecutionInput{LocalValues: map[string]map[string]Selection{"trace": {}}}) {
		t.Fatal("explicit local parameter override ignored")
	}
}
