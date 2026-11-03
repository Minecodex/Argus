package dashboard

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboardparams"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestPostgresAnalysisRequiresAttributedResourcesBeforeSavingOrExecuting(t *testing.T) {
	f := newAnalysisFixture(t)
	f.begin(t, "analyze the recent half-hour")
	ctx := t.Context()
	input := AnalysisResolveInput{DashboardID: f.b, Changes: dashboardparams.Patch{Time: &TimeRange{Kind: "relative", Seconds: 1800}}, Evidence: []dashboardparams.Evidence{{Path: "/time", Quote: "recent half-hour"}}}
	_, err := f.service.ResolveAnalysis(ctx, f.actor, f.conversation, input, uuid.New())
	var failure toolruntime.Error
	if !errors.As(err, &failure) || failure.Kind != "DASHBOARD_RESOURCE_SCOPE_REQUIRED" {
		t.Fatalf("initial omission silently admitted all resources: %v", err)
	}
	state, err := f.store.Queries.GetDashboardRunParameters(ctx, db.GetDashboardRunParametersParams{RunID: f.actor.RunID, DashboardID: f.b, EnterpriseID: f.actor.EnterpriseID, OwnerUserID: f.actor.SubjectID})
	if err != nil || state.Version != 0 {
		t.Fatalf("failed resolution persisted a partial condition: %v", err)
	}
	var count int
	if err = f.store.Pool.QueryRow(ctx, "SELECT count(*) FROM dashboard_analysis_contexts WHERE run_id=$1", f.actor.RunID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed resolution created a context: %d %v", count, err)
	}
	input = allAnalysisResourcesForTest(input)
	resolved, err := f.service.ResolveAnalysis(ctx, f.actor, f.conversation, input, uuid.New())
	if err != nil || resolved.Parameters.ResourceMode != "all_authorized" {
		t.Fatalf("explicit attributed all-resource choice rejected: %v", err)
	}
	inherited, err := f.service.ResolveAnalysis(ctx, f.actor, f.conversation, AnalysisResolveInput{DashboardID: f.b, ExpectedVersion: resolved.ConditionVersion}, uuid.New())
	if err != nil || inherited.Conditions.Evidence["/resources"] != resolved.Conditions.Evidence["/resources"] {
		t.Fatalf("no-change follow-up lost the confirmed resource condition: %v", err)
	}
	// Reproduce a pre-v3 immutable context with implicit all. A root query must
	// reject it too, even when a caller already holds its otherwise valid ID.
	row, err := f.store.Queries.GetDashboardAnalysisContext(ctx, db.GetDashboardAnalysisContextParams{ID: resolved.ID, EnterpriseID: f.actor.EnterpriseID, ConversationID: f.conversation, RunID: f.actor.RunID, OwnerUserID: f.actor.SubjectID})
	if err != nil {
		t.Fatal(err)
	}
	legacy, _ := json.Marshal(dashboardparams.Empty())
	old, err := f.store.Queries.CreateDashboardAnalysisContext(ctx, db.CreateDashboardAnalysisContextParams{ID: uuid.New(), EnterpriseID: row.EnterpriseID, ConversationID: row.ConversationID, RunID: row.RunID, DashboardID: row.DashboardID, OwnerUserID: row.OwnerUserID, RevisionID: row.RevisionID, ConditionVersion: row.ConditionVersion, InputHash: "legacy-implicit-all", UserEventID: row.UserEventID, State: legacy, Parameters: row.Parameters})
	if err != nil {
		t.Fatal(err)
	}
	jobs := QueryJobs{Runtime: Runtime{Store: f.store, Backend: conditionBackend{}}, Objects: conditionObjects{}}
	_, err = jobs.StartAnalysis(ctx, f.actor, f.conversation, AnalysisQueryInput{DashboardID: f.b, ContextRef: old.ID}, uuid.NewString())
	if !errors.As(err, &failure) || failure.Kind != "DASHBOARD_RESOURCE_SCOPE_REQUIRED" {
		t.Fatalf("legacy implicit-all context reached query execution: %v", err)
	}
	if err = f.store.Pool.QueryRow(ctx, "SELECT count(*) FROM dashboard_query_jobs WHERE run_id=$1", f.actor.RunID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rejected root created query work: %d %v", count, err)
	}
}
