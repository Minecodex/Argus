package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	taskruntime "github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/objectstore"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func testPublishedFileDrilldowns(t *testing.T, ctx context.Context, rt Runtime, actor Actor, id, a, b, c, grantB uuid.UUID, spec Spec, traceID, childID string, selected map[string]string) {
	endpoint := os.Getenv("ARGUS_QUERY_TEST_OBJECT_ENDPOINT")
	if endpoint == "" {
		t.Skip("query object endpoint required")
	}
	access, secret := queryTestObjectCredentials()
	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(access, secret, ""), Secure: false, Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	bucket := "drill-" + uuid.NewString()
	if err = client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: "us-east-1"}); err != nil {
		t.Fatal(err)
	}
	objects, err := objectstore.Open(ctx, "http://"+endpoint, bucket, access, secret)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = objects.DeletePrefix(context.Background(), "enterprises/"+actor.EnterpriseID.String()+"/")
		_ = client.RemoveBucket(context.Background(), bucket)
	})
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := rt.Store.Pool.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	model, conversation, workspace := uuid.New(), uuid.New(), uuid.New()
	exec(`INSERT INTO role_permissions(role_id,permission_id) SELECT role_id,'workspace.use' FROM role_bindings WHERE subject_id=$1 AND subject_type='user' ON CONFLICT DO NOTHING`, actor.SubjectID)
	exec(`INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,health_status) VALUES($1,$2,'File drill','https://model.example.test','model','chat_completions',32768,1024,0,0,'healthy')`, model, actor.EnterpriseID)
	exec(`INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'File drill',$4)`, conversation, actor.EnterpriseID, actor.SubjectID, model)
	exec(`INSERT INTO workspaces(id,enterprise_id,conversation_id,pvc_name,namespace,status,capacity_bytes,environment_version) VALUES($1,$2,$3,$4,'test-file-drill','ready',2147483648,'test')`, workspace, actor.EnterpriseID, conversation, "workspace-"+workspace.String())
	backend := &countedQueryBackend{RuntimeBackend: rt.Backend}
	rt.Backend = backend
	jobs := QueryJobs{Runtime: rt, Objects: objects}
	delivery := &queryTestDelivery{store: rt.Store, workspace: workspace, sources: jobs, contents: map[string][]byte{}}
	jobs.Delivery = delivery
	start := func(input QueryJobInput) QueryJobView {
		t.Helper()
		v, e := jobs.Start(ctx, actor, conversation, input, uuid.NewString())
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	claim := func(job QueryJobView) taskruntime.Task {
		t.Helper()
		task, e := rt.Store.Queries.ClaimRuntimeTask(ctx, db.ClaimRuntimeTaskParams{Queue: "dashboard_query", LeaseOwner: pgtype.Text{String: uuid.NewString(), Valid: true}, Column3: pgtype.Interval{Microseconds: 120_000_000, Valid: true}})
		if e != nil {
			t.Fatal(e)
		}
		stored, e := rt.Store.Queries.GetDashboardQueryJob(ctx, db.GetDashboardQueryJobParams{ID: job.ID, EnterpriseID: actor.EnterpriseID})
		if e != nil || stored.TaskID != task.ID {
			t.Fatalf("wrong task: %v", e)
		}
		task, e = rt.Store.Queries.StartRuntimeTask(ctx, db.StartRuntimeTaskParams{ID: task.ID, LeaseOwner: task.LeaseOwner, FenceToken: task.FenceToken})
		if e != nil {
			t.Fatal(e)
		}
		return taskruntime.Task{RuntimeTask: task}
	}
	finish := func(task taskruntime.Task, state string) {
		t.Helper()
		if _, e := rt.Store.Queries.FinishRuntimeTask(ctx, db.FinishRuntimeTaskParams{ID: task.ID, LeaseOwner: task.LeaseOwner, FenceToken: task.FenceToken, Status: state}); e != nil {
			t.Fatal(e)
		}
	}
	run := func(job QueryJobView) QueryJobView {
		t.Helper()
		task := claim(job)
		if e := jobs.Handle(ctx, task); e != nil {
			t.Fatal(e)
		}
		finish(task, "succeeded")
		v, e := jobs.Get(ctx, actor, conversation, job.ID)
		if e != nil {
			t.Fatal(e)
		}
		if v.Status != "complete" {
			t.Fatalf("unexpected result: %+v", v)
		}
		return v
	}
	data := func(job QueryJobView) any {
		t.Helper()
		file := job.Manifest.Files[0]
		record, e := rt.Store.Queries.GetDashboardQueryFile(ctx, db.GetDashboardQueryFileParams{ID: file.ID, EnterpriseID: actor.EnterpriseID})
		if e != nil {
			t.Fatal(e)
		}
		reader, e := newQueryFileReader(ctx, objects, record, func() error { return nil })
		if e != nil {
			t.Fatal(e)
		}
		var v any
		if e = json.NewDecoder(reader).Decode(&v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	find := func(origin, kind string) Drilldown {
		t.Helper()
		for _, d := range spec.Panels[0].Drilldowns {
			if d.OriginQueryRef == origin && d.Kind == kind {
				return d
			}
		}
		t.Fatalf("missing %s", kind)
		return Drilldown{}
	}
	input := func(parent QueryJobView, drill Drilldown, values map[string]string, expand bool) QueryJobInput {
		return QueryJobInput{DashboardID: id, Drilldown: &QueryDrilldownInput{ParentJobID: parent.ID, PanelID: "traces", DrilldownID: drill.ID, Values: values, ExpandAuthorizedResources: expand}}
	}
	baseQueued := start(QueryJobInput{DashboardID: id, Parameters: ExecutionInput{ResourceIDs: []uuid.UUID{a}}})
	detail := find("list", "trace_details")
	if _, e := jobs.Start(ctx, actor, conversation, input(baseQueued, detail, selected, false), uuid.NewString()); !errors.Is(e, ErrSelectionStale) {
		t.Fatalf("unsealed parent accepted: %v", e)
	}
	base := run(baseQueued)
	t.Run("new Run must acquire a new root", func(t *testing.T) {
		oldRun, newRun := uuid.New(), uuid.New()
		for _, r := range []uuid.UUID{oldRun, newRun} {
			exec(`INSERT INTO runs(id,enterprise_id,conversation_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'en-US',1,'succeeded')`, r, actor.EnterpriseID, conversation, actor.SubjectID, model)
		}
		parent := run(start(QueryJobInput{DashboardID: id, RunID: oldRun, Parameters: ExecutionInput{ResourceIDs: []uuid.UUID{a}}}))
		child := input(parent, detail, selected, false)
		child.RunID = newRun
		if _, e := jobs.Start(ctx, actor, conversation, child, uuid.NewString()); !errors.Is(e, ErrContextExpired) {
			t.Fatalf("new Run reused old execution: %v", e)
		}
		child.RunID = uuid.Nil
		if _, e := jobs.Start(ctx, actor, conversation, child, uuid.NewString()); !errors.Is(e, ErrContextExpired) {
			t.Fatal("omitting Run bypassed its binding")
		}
		child.RunID = oldRun
		run(start(child))
	})
	parentRecord, e := rt.Store.Queries.GetDashboardQueryJob(ctx, db.GetDashboardQueryJobParams{ID: base.ID, EnterpriseID: actor.EnterpriseID})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = jobs.readFileRowEvidence(ctx, actor, conversation, parentRecord, *base.Manifest, "traces", "list", &executionBudget{bytes: 1, rows: 100, scan: 1 << 20, samples: 100, pending: 1}); !errors.Is(e, telemetry.ErrQueryBudget) {
		t.Fatalf("row proof ignored budget: %v", e)
	}
	for _, bad := range []QueryJobInput{
		{DashboardID: id, Drilldown: &QueryDrilldownInput{ParentJobID: base.ID, PanelID: "wrong", DrilldownID: detail.ID, Values: selected}},
		{DashboardID: id, Parameters: ExecutionInput{ResourceIDs: []uuid.UUID{b}}, Drilldown: input(base, detail, selected, false).Drilldown},
		input(base, detail, map[string]string{"trace_id": "forged", "source_id": selected["source_id"], "resource_id": selected["resource_id"]}, false),
	} {
		if _, e := jobs.Start(ctx, actor, conversation, bad, uuid.NewString()); e == nil {
			t.Fatal("invalid selection/scope accepted")
		}
	}
	// Workspace content cannot authenticate a row; proof comes from verified S3.
	delivery.contents[strings.TrimPrefix(base.Manifest.Files[0].Path, "/workspace/")] = []byte(`{"traceId":"forged"}`)
	reads := backend.calls.Load()
	ordinaryQueued := start(input(base, detail, selected, false))
	if backend.calls.Load() != reads {
		t.Fatal("file row proof reran the origin query")
	}
	ordinary := run(ordinaryQueued)
	assertSpans := func(job QueryJobView, count int) {
		t.Helper()
		v := data(job).(map[string]any)["queryTraceGraph"].(map[string]any)
		if len(v["spans"].([]any)) != count {
			t.Fatalf("bad graph: %+v", v)
		}
	}
	assertSpans(ordinary, 1)
	full := find(detail.DetailQueryRef, "full_trace")
	if _, e := jobs.Start(ctx, actor, conversation, input(ordinary, full, selected, false), uuid.NewString()); !errors.Is(e, ErrInvalid) {
		t.Fatal("implicit expansion accepted")
	}
	if _, e := jobs.Start(ctx, actor, conversation, input(base, full, selected, true), uuid.NewString()); !errors.Is(e, ErrInvalid) {
		t.Fatal("skipped published origin accepted")
	}
	fullQueued := start(input(ordinary, full, selected, true))
	grantC := uuid.New()
	exec(`INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'host',$4)`, grantC, actor.EnterpriseID, actor.SubjectID, c)
	expanded := run(fullQueued)
	assertSpans(expanded, 2)
	exec(`DELETE FROM data_authorization_grants WHERE id=$1`, grantC)
	if expanded.Manifest.Drilldown.Depth != 2 || expanded.Manifest.Drilldown.ParentAttemptID != ordinary.Manifest.AttemptID || len(expanded.Manifest.Execution.Resources) != 2 {
		t.Fatal("detail lineage/scope lost")
	}
	logs := find(full.DetailQueryRef, "span_logs")
	childSource := ""
	for _, row := range data(expanded).(map[string]any)["queryTraceGraph"].(map[string]any)["spans"].([]any) {
		span := row.(map[string]any)
		if span["spanId"] == childID && span["resourceId"] == b.String() {
			childSource = span["sourceId"].(string)
		}
	}
	if childSource == "" {
		t.Fatal("missing child source identity")
	}
	forged := input(expanded, logs, map[string]string{"trace_id": traceID, "span_id": childID, "resource_id": a.String(), "source_id": childSource}, false)
	if _, e := jobs.Start(ctx, actor, conversation, forged, uuid.NewString()); !errors.Is(e, ErrSelectionStale) {
		t.Fatalf("cross-row splice accepted: %v", e)
	}
	logInput := input(expanded, logs, map[string]string{"trace_id": traceID, "span_id": childID, "resource_id": b.String(), "source_id": childSource}, false)
	logQueued := start(logInput)
	task := claim(logQueued)
	delivery.failAfter = delivery.calls + 1
	if e := jobs.Handle(ctx, task); e == nil {
		t.Fatal("delivery fault disappeared")
	}
	mid, e := jobs.Get(ctx, actor, conversation, logQueued.ID)
	if e != nil {
		t.Fatal(e)
	}
	attempt := mid.Manifest.AttemptID
	calls := backend.calls.Load()
	delivery.failAfter = 0
	if _, e = rt.Store.Queries.RequeueRuntimeTask(ctx, db.RequeueRuntimeTaskParams{ID: task.ID, LeaseOwner: task.LeaseOwner, FenceToken: task.FenceToken, AvailableAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}); e != nil {
		t.Fatal(e)
	}
	logResult := run(logQueued)
	if backend.calls.Load() != calls || logResult.Manifest.AttemptID != attempt {
		t.Fatal("detail delivery queried again")
	}
	rows := data(logResult).([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["body"] != "correlated evidence" {
		t.Fatalf("incorrect logs: %v", rows)
	}
	contextDrill := find(logs.DetailQueryRef, "log_context")
	contextResult := run(start(input(logResult, contextDrill, map[string]string{"event": rows[0].(map[string]any)["event_id"].(string)}, false)))
	if len(data(contextResult).([]any)) != 1 || contextResult.Manifest.Drilldown.Depth != 4 {
		t.Fatal("context chain lost published scope")
	}
	t.Run("parent revision survives newer publication", func(t *testing.T) {
		_, old, e := (Service{Store: rt.Store}).Get(ctx, actor, id)
		if e != nil {
			t.Fatal(e)
		}
		changed, e := DecodeSpec(old.Spec)
		if e != nil {
			t.Fatal(e)
		}
		changed.Panels[0].Targets[0].SourceDefinition.Builder.Filters = []Filter{{Field: "serviceName", Operator: "=", Value: "not-received"}}
		raw, _ := json.Marshal(changed)
		next, e := rt.Store.Queries.CreateDashboardRevision(ctx, db.CreateDashboardRevisionParams{ID: uuid.New(), EnterpriseID: actor.EnterpriseID, DashboardID: id, RevisionNumber: old.RevisionNumber + 1, SchemaVersion: SchemaVersion, Name: old.Name, Description: old.Description, FolderID: old.FolderID, Spec: raw, SpecHash: queryHash(raw), ValidationReport: old.ValidationReport, SampleReport: old.SampleReport, CreatedBy: actor.SubjectID})
		if e != nil {
			t.Fatal(e)
		}
		exec(`UPDATE dashboards SET active_revision_id=$2,version=version+1 WHERE id=$1`, id, next.ID)
		continued := run(start(input(base, detail, selected, false)))
		if continued.RevisionID != base.RevisionID {
			t.Fatal("file continuation mixed revisions")
		}
		assertSpans(continued, 1)
		fresh := run(start(QueryJobInput{DashboardID: id, Parameters: ExecutionInput{ResourceIDs: []uuid.UUID{a}}}))
		if fresh.RevisionID != next.ID || fresh.Manifest.Execution.Panels[0].Status != "no_data" {
			t.Fatal("fresh request did not use latest publication")
		}
	})
	// The old sealed context remains self-contained after browser tokens expire.
	exec(`UPDATE dashboard_query_jobs SET created_at=now()-interval '2 hours' WHERE id=$1`, base.ID)
	if v := run(start(input(base, detail, selected, false))); v.RevisionID != base.RevisionID {
		t.Fatal("parent revision changed")
	}
	// Corruption anywhere in the parent file must prevent a new task.
	parentFile, e := rt.Store.Queries.GetDashboardQueryFile(ctx, db.GetDashboardQueryFileParams{ID: base.Manifest.Files[0].ID, EnterpriseID: actor.EnterpriseID})
	if e != nil {
		t.Fatal(e)
	}
	var parts []queryChunk
	_ = json.Unmarshal(parentFile.Chunks, &parts)
	old, e := objects.ReadFile(ctx, parts[0].Key, 0, 0)
	if e != nil {
		t.Fatal(e)
	}
	saved, e := io.ReadAll(old)
	_ = old.Close()
	if e != nil {
		t.Fatal(e)
	}
	if e = objects.PutFile(ctx, parts[0].Key, strings.NewReader("corrupt"), 7, "application/json"); e != nil {
		t.Fatal(e)
	}
	if _, e = jobs.Start(ctx, actor, conversation, input(base, detail, selected, false), uuid.NewString()); e == nil {
		t.Fatal("corrupt evidence accepted")
	}
	if e = objects.PutFile(ctx, parts[0].Key, strings.NewReader(string(saved)), int64(len(saved)), "application/json"); e != nil {
		t.Fatal(e)
	}
	revokeQueued := start(input(ordinary, full, selected, true))
	exec(`UPDATE data_authorization_grants SET status='disabled' WHERE id=$1`, grantB)
	task = claim(revokeQueued)
	if e = jobs.Handle(ctx, task); !errors.Is(e, ErrDenied) {
		t.Fatalf("revoked frozen expansion accepted: %v", e)
	}
	finish(task, "failed")
	if _, e = jobs.Start(ctx, actor, conversation, logInput, uuid.NewString()); !errors.Is(e, ErrDenied) {
		t.Fatal("revoked parent scope accepted")
	}
	p := toolruntime.Principal{EnterpriseID: actor.EnterpriseID, UserID: actor.SubjectID, ConversationID: conversation, AuthorizationVersion: actor.AuthorizationVersion}
	if _, e = jobs.AuthorizeSource(ctx, p, expanded.Manifest.Files[0].SourceRef); !errors.Is(e, ErrDenied) {
		t.Fatal("expanded evidence escaped revoke")
	}
	exec(`UPDATE data_authorization_grants SET status='active' WHERE id=$1`, grantB)
	wrongConversation := uuid.New()
	if _, e = jobs.Start(ctx, actor, wrongConversation, input(base, detail, selected, false), uuid.NewString()); e == nil {
		t.Fatal("foreign conversation accepted")
	}
}
