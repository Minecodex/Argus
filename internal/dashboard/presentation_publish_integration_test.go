package dashboard

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboardparams"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresPresentationSealingIdempotencyFailureAndAttemptIsolation(t *testing.T) {
	f := newAnalysisFixture(t)
	f.begin(t, "Presentation sealing fixture")
	ctx := t.Context()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.store.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("UPDATE runtime_tasks SET status='cancelled' WHERE enterprise_id=$1 AND queue='agent'", f.actor.EnterpriseID)
	resolved, err := f.service.ResolveAnalysis(ctx, f.actor, f.conversation, AnalysisResolveInput{DashboardID: f.b, Changes: dashboardparams.Patch{}, Evidence: []dashboardparams.Evidence{}}, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	jobs := QueryJobs{Runtime: Runtime{Store: f.store, Backend: conditionBackend{}}, Objects: conditionObjects{}}
	job, err := jobs.StartAnalysis(ctx, f.actor, f.conversation, AnalysisQueryInput{DashboardID: f.b, ContextRef: resolved.ID}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	exec("UPDATE runtime_tasks SET status='cancelled' WHERE enterprise_id=$1 AND queue='dashboard_query'", f.actor.EnterpriseID)
	pending, err := jobs.PresentQuery(ctx, f.actor, f.conversation, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	asset := QueryTemplate()
	if err = f.store.Queries.CreateTemplateAsset(ctx, db.CreateTemplateAssetParams{Hash: asset.Hash, Source: asset.Source, Runtime: asset.Runtime}); err != nil {
		t.Fatal(err)
	}
	oldAttempt, newAttempt := uuid.New(), uuid.New()
	exec("INSERT INTO dashboard_query_attempts(id,enterprise_id,job_id,ordinal,status) VALUES($1,$3,$4,1,'abandoned'),($2,$3,$4,2,'materialized')", oldAttempt, newAttempt, f.actor.EnterpriseID, job.ID)
	ids := []uuid.UUID{}
	for index, attempt := range []string{"", oldAttempt.String()} {
		call, step, id := uuid.New(), uuid.New(), uuid.New()
		exec("INSERT INTO run_steps(id,run_id,enterprise_id,sequence,step_type,status) VALUES($1,$2,$3,$4,'tool_call','succeeded')", step, f.actor.RunID, f.actor.EnterpriseID, index+100)
		exec("INSERT INTO tool_calls(id,call_id,enterprise_id,run_id,step_id,tool_id,input,input_hash,status) VALUES($1,$2,$3,$4,$5,'telemetry.dashboard.query','{}',decode(repeat('00',32),'hex'),'succeeded')", call, call.String(), f.actor.EnterpriseID, f.actor.RunID, step)
		details, _ := json.Marshal(pending.Data)
		_, err := f.store.Queries.CreateToolPresentation(ctx, db.CreateToolPresentationParams{ID: id, EnterpriseID: f.actor.EnterpriseID, ConversationID: f.conversation, ToolCallID: call, ToolVersion: asset.Version, AuthorizationScope: pending.AuthorizationScope, TemplateHash: asset.Hash, DetailData: details, ResourceRefs: []byte(`[]`), Status: "ready"})
		if err != nil {
			t.Fatal(err)
		}
		binding, _ := json.Marshal(map[string]any{"kind": "dashboard_query", "job_id": job.ID.String(), "attempt_id": attempt})
		if err = f.store.Queries.SetPresentationReadBinding(ctx, db.SetPresentationReadBindingParams{ID: id, EnterpriseID: f.actor.EnterpriseID, ReadOperations: asset.ReadOperations, ResultBinding: binding}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	// A fresh publisher instance discovers persisted pending presentations after
	// a failed acquisition. An older bound attempt must never receive its data.
	exec("UPDATE dashboard_query_jobs SET status='failed',attempt_id=$2,error_code='QUERY_UNAVAILABLE',version=version+1 WHERE id=$1", job.ID, newAttempt)
	first := map[uuid.UUID]string{}
	for pass := 0; pass < 3; pass++ {
		restarted := QueryJobs{Runtime: jobs.Runtime, Objects: jobs.Objects}
		if err = restarted.PublishPresentations(ctx, f.actor.EnterpriseID, job.ID); err != nil {
			t.Fatal(err)
		}
		for index, id := range ids {
			snapshot, err := f.store.Queries.GetPresentationSnapshot(ctx, db.GetPresentationSnapshotParams{PresentationID: id, EnterpriseID: f.actor.EnterpriseID})
			if err != nil {
				t.Fatal(err)
			}
			data := map[string]any{}
			_ = json.Unmarshal(snapshot.Data, &data)
			if data["status"] != "failed" {
				t.Fatal("failed query remained pending", data)
			}
			if index == 1 && data["error_code"] != "DASHBOARD_ATTEMPT_REPLACED" {
				t.Fatal("replaced attempt received new facts", data)
			}
			if pass == 0 {
				first[id] = snapshot.SourceVersion
			} else if first[id] != snapshot.SourceVersion {
				t.Fatal("historical snapshot changed on publisher replay")
			}
			if snapshot.SourceVersion != presentation.SnapshotVersion(snapshot.Data) {
				// PostgreSQL JSONB canonicalizes whitespace; compare parsed content.
				raw, _ := json.Marshal(data)
				if snapshot.SourceVersion != presentation.SnapshotVersion(raw) {
					t.Fatal("snapshot source version does not describe its facts")
				}
			}
		}
	}
	var count int
	if err = f.store.Pool.QueryRow(ctx, "SELECT count(*) FROM conversation_events WHERE conversation_id=$1 AND event_type='tool_presentation' AND payload->>'status'='data_ready'", f.conversation).Scan(&count); err != nil || count != 2 {
		t.Fatal("ready events must publish once for each presentation", count, err)
	}
}
