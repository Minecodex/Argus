package dashboard

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboardcontext"
	"github.com/kakj-go/Argus/internal/dashboardparams"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresAutomaticAnalysisCorrectionAndFrozenPresentation(t *testing.T) {
	f := newAnalysisFixture(t)
	ctx := t.Context()
	presentationTime := func(data map[string]any, key string) time.Time {
		t.Helper()
		value, err := time.Parse(time.RFC3339Nano, data[key].(string))
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	begin := func(message string) {
		t.Helper()
		if f.actor.RunID != uuid.Nil {
			_, err := f.store.Pool.Exec(ctx, "UPDATE runs SET status='succeeded' WHERE id=$1", f.actor.RunID)
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.store.Pool.Exec(ctx, "UPDATE runtime_tasks SET status='succeeded' WHERE run_id=$1", f.actor.RunID)
			if err != nil {
				t.Fatal(err)
			}
		}
		accepted, err := f.convo.AddMessage(ctx, f.actor.SubjectID.String(), f.actor.EnterpriseID, f.actor.SubjectID, f.conversation, 1, "zh-CN", message, nil, uuid.NewString(), nil)
		if err != nil {
			t.Fatal(err)
		}
		f.actor.RunID = accepted.Run.ID
	}
	begin("看看仪表盘是否存在异常")
	principal, err := f.convo.ToolPrincipal(ctx, f.actor.EnterpriseID, f.actor.SubjectID, f.conversation)
	if err != nil {
		t.Fatal(err)
	}
	acquired, err := dashboardcontext.Acquire(ctx, f.store, principal, f.actor.RunID, f.b)
	if err != nil || acquired.Mode != "analyze" || len(acquired.DashboardIDs) != 1 {
		t.Fatalf("automatic discovery: %+v %v", acquired, err)
	}
	if _, err = dashboardcontext.Acquire(ctx, f.store, principal, f.actor.RunID, f.b); err != nil {
		t.Fatal(err)
	}
	seed, err := f.store.Queries.GetRunDashboardContext(ctx, db.GetRunDashboardContextParams{RunID: f.actor.RunID, EnterpriseID: f.actor.EnterpriseID, ConversationID: f.conversation, OwnerUserID: f.actor.SubjectID})
	if err != nil {
		t.Fatal(err)
	}
	var admitted dashboardcontext.Snapshot
	if json.Unmarshal(seed.Selection, &admitted) != nil || admitted.Mode != "none" || len(admitted.DashboardIDs) != 0 {
		t.Fatal("automatic discovery rewrote the admission fact")
	}
	input := AnalysisResolveInput{DashboardID: f.b, ExpectedVersion: 0, Changes: dashboardparams.Patch{Time: &TimeRange{Kind: "relative", Seconds: 1800}}, Evidence: []dashboardparams.Evidence{{Path: "/time", Origin: "ai_selection", Reason: "Use a recent half-hour for an initial diagnostic overview"}}}
	resolved, err := f.service.ResolveAnalysis(ctx, f.actor, f.conversation, input, uuid.New())
	if err != nil || resolved.Conditions.Evidence["/time"].Origin != "ai_selection" {
		t.Fatalf("AI conditions: %+v %v", resolved, err)
	}
	jobs := QueryJobs{Runtime: Runtime{Store: f.store, Backend: conditionBackend{}}, Objects: conditionObjects{}}
	first, err := jobs.StartAnalysis(ctx, f.actor, f.conversation, AnalysisQueryInput{DashboardID: f.b, ContextRef: resolved.ID}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	before, err := jobs.PresentQuery(ctx, f.actor, f.conversation, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	from, to := presentationTime(before.Data, "from"), presentationTime(before.Data, "to")
	if to.Sub(from) != 30*time.Minute {
		t.Fatal("presentation did not use the admitted absolute range")
	}
	begin("时间改成最近一小时")
	if _, err = dashboardcontext.Acquire(ctx, f.store, principal, f.actor.RunID, f.b); err != nil {
		t.Fatal(err)
	}
	input.ExpectedVersion = 1
	input.Changes.Time.Seconds = 3600
	input.Evidence = []dashboardparams.Evidence{{Path: "/time", Quote: "最近一小时"}}
	corrected, err := f.service.ResolveAnalysis(ctx, f.actor, f.conversation, input, uuid.New())
	if err != nil || corrected.Conditions.Evidence["/time"].Origin != "message_interpretation" {
		t.Fatalf("user correction: %+v %v", corrected, err)
	}
	second, err := jobs.StartAnalysis(ctx, f.actor, f.conversation, AnalysisQueryInput{DashboardID: f.b, ContextRef: corrected.ID}, uuid.NewString())
	if err != nil || second.ID == first.ID {
		t.Fatalf("correction did not admit a new query: %v", err)
	}
	after, err := jobs.PresentQuery(ctx, f.actor, f.conversation, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if presentationTime(after.Data, "to").Sub(presentationTime(after.Data, "from")) != time.Hour {
		t.Fatal("corrected query retained old parameters")
	}
	history, err := jobs.PresentQuery(ctx, f.actor, f.conversation, first.ID)
	if err != nil || !presentationTime(history.Data, "from").Equal(from) || !presentationTime(history.Data, "to").Equal(to) {
		t.Fatalf("historical query range changed: %v", err)
	}
	forged := principal
	forged.UserID = uuid.New()
	if _, err = dashboardcontext.Acquire(ctx, f.store, forged, f.actor.RunID, f.b); err == nil {
		t.Fatal("foreign actor acquired dashboard")
	}
	if _, err = dashboardcontext.Acquire(ctx, f.store, principal, f.actor.RunID, uuid.New()); err == nil {
		t.Fatal("invented dashboard acquired")
	}
}
