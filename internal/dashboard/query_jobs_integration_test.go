package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	conversationservice "github.com/kakj-go/Argus/internal/conversation"
	taskruntime "github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/objectstore"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type countedQueryBackend struct {
	RuntimeBackend
	calls atomic.Int64
	block chan struct{}
}

func (b *countedQueryBackend) ExecuteEngineQuery(ctx context.Context, r queryengine.Request) (queryengine.Result, error) {
	b.calls.Add(1)
	if b.block != nil {
		close(b.block)
		<-ctx.Done()
		return queryengine.Result{}, ctx.Err()
	}
	return b.RuntimeBackend.ExecuteEngineQuery(ctx, r)
}

type queryTestObjects struct {
	QueryObjectStore
	failAfter int
	writes    int
}

func (s *queryTestObjects) PutFile(ctx context.Context, key string, r io.Reader, size int64, media string) error {
	s.writes++
	if s.failAfter > 0 && s.writes > s.failAfter {
		return errors.New("injected object storage interruption")
	}
	return s.QueryObjectStore.PutFile(ctx, key, r, size, media)
}

// This adapter injects Workspace failures while persisting real file metadata.
// Workspace's lease/atomic-RPC path has separate tests; it is not simulated E2E.
type queryTestDelivery struct {
	store     *postgres.Store
	workspace uuid.UUID
	sources   QueryJobs
	failAfter int
	calls     int
	contents  map[string][]byte
}

func (d *queryTestDelivery) ImportQueryFile(ctx context.Context, p toolruntime.Principal, run uuid.UUID, ref, path string, r io.Reader, size int64, digest string) (db.WorkspaceFile, error) {
	d.calls++
	if d.failAfter > 0 && d.calls > d.failAfter {
		return db.WorkspaceFile{}, toolruntime.Error{Kind: "WORKSPACE_QUOTA_EXCEEDED"}
	}
	if handled, err := d.sources.AuthorizeSource(ctx, p, ref); !handled || err != nil {
		return db.WorkspaceFile{}, fmt.Errorf("source rejected: %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return db.WorkspaceFile{}, err
	}
	if int64(len(data)) != size || queryHash(data) != digest {
		return db.WorkspaceFile{}, ErrInvalid
	}
	if old, e := d.store.Queries.GetWorkspaceFileByPath(ctx, db.GetWorkspaceFileByPathParams{WorkspaceID: d.workspace, EnterpriseID: p.EnterpriseID, Path: path}); e == nil {
		if !bytes.Equal(d.contents[path], data) {
			return db.WorkspaceFile{}, ErrConflict
		}
		return old, nil
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return db.WorkspaceFile{}, e
	}
	d.contents[path] = data
	return d.store.Queries.CreateWorkspaceFile(ctx, db.CreateWorkspaceFileParams{ID: uuid.New(), EnterpriseID: p.EnterpriseID, WorkspaceID: d.workspace, ConversationID: p.ConversationID, Name: "query.json", Path: path, ByteSize: size, ContentHash: digest, MediaType: "application/json", SourceResultRefs: []string{ref}})
}

func testPublishedQueryJobs(t *testing.T, ctx context.Context, rt Runtime, actor Actor, dashboardID, host, source uuid.UUID, conn driver.Conn, logTable string) {
	endpoint := os.Getenv("ARGUS_QUERY_TEST_OBJECT_ENDPOINT")
	if endpoint == "" {
		t.Skip("query object store endpoint required")
	}
	access, secret := queryTestObjectCredentials()
	raw, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(access, secret, ""), Secure: false, Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	bucket := "query-" + uuid.NewString()
	if err = raw.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: "us-east-1"}); err != nil {
		t.Fatal(err)
	}
	objects, err := objectstore.Open(ctx, "http://"+endpoint, bucket, access, secret)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = objects.DeletePrefix(context.Background(), "enterprises/"+actor.EnterpriseID.String()+"/")
		_ = raw.RemoveBucket(context.Background(), bucket)
	})
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := rt.Store.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	model, conversation, workspace := uuid.New(), uuid.New(), uuid.New()
	exec(`INSERT INTO role_permissions(role_id,permission_id) SELECT role_id,'workspace.use' FROM role_bindings WHERE subject_id=$1 AND subject_type='user' ON CONFLICT DO NOTHING`, actor.SubjectID)
	exec(`INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,health_status) VALUES($1,$2,'Query','https://model.example.test','model','chat_completions',32768,1024,0,0,'healthy')`, model, actor.EnterpriseID)
	exec(`INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'Query',$4)`, conversation, actor.EnterpriseID, actor.SubjectID, model)
	exec(`INSERT INTO workspaces(id,enterprise_id,conversation_id,pvc_name,namespace,status,capacity_bytes,environment_version) VALUES($1,$2,$3,$4,'test-query','ready',2147483648,'test')`, workspace, actor.EnterpriseID, conversation, "query-"+workspace.String())
	large := strings.Repeat("observed query data ", 80000)
	at := time.Now().UTC().Add(-time.Minute)
	if err := conn.Exec(ctx, "INSERT INTO `"+logTable+"` (resource_id,timestamp,body,source_id,source_revision,source_type,event_id,expires_at) VALUES (?,?,?,?,1,'otlp',?,now64(3)+INTERVAL 1 HOUR)", host, at, large, source, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	backend := &countedQueryBackend{RuntimeBackend: rt.Backend}
	rt.Backend = backend
	wrapped := &queryTestObjects{QueryObjectStore: objects}
	jobs := QueryJobs{Runtime: rt, Objects: wrapped}
	delivery := &queryTestDelivery{store: rt.Store, workspace: workspace, sources: jobs, contents: map[string][]byte{}}
	jobs.Delivery = delivery
	input := QueryJobInput{DashboardID: dashboardID, Parameters: ExecutionInput{ResourceIDs: []uuid.UUID{host}}}
	start := func(key string) QueryJobView {
		t.Helper()
		v, e := jobs.Start(ctx, actor, conversation, input, key)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	claim := func(id uuid.UUID) taskruntime.Task {
		t.Helper()
		j, e := rt.Store.Queries.GetDashboardQueryJob(ctx, db.GetDashboardQueryJobParams{ID: id, EnterpriseID: actor.EnterpriseID})
		if e != nil {
			t.Fatal(e)
		}
		exec(`UPDATE runtime_tasks SET available_at=now() WHERE id=$1`, j.TaskID)
		task, e := rt.Store.Queries.ClaimRuntimeTask(ctx, db.ClaimRuntimeTaskParams{Queue: "dashboard_query", LeaseOwner: pgtype.Text{String: uuid.NewString(), Valid: true}, Column3: pgtype.Interval{Microseconds: 120_000_000, Valid: true}})
		if e != nil {
			t.Fatal(e)
		}
		if task.ID != j.TaskID {
			t.Fatal("wrong fixture task claimed")
		}
		task, e = rt.Store.Queries.StartRuntimeTask(ctx, db.StartRuntimeTaskParams{ID: task.ID, LeaseOwner: task.LeaseOwner, FenceToken: task.FenceToken})
		if e != nil {
			t.Fatal(e)
		}
		return taskruntime.Task{RuntimeTask: task}
	}
	finish := func(task taskruntime.Task, state string) {
		t.Helper()
		_, e := rt.Store.Queries.FinishRuntimeTask(ctx, db.FinishRuntimeTaskParams{ID: task.ID, LeaseOwner: task.LeaseOwner, FenceToken: task.FenceToken, Status: state})
		if e != nil {
			t.Fatal(e)
		}
	}
	requeue := func(task taskruntime.Task) {
		t.Helper()
		_, e := rt.Store.Queries.RequeueRuntimeTask(ctx, db.RequeueRuntimeTaskParams{ID: task.ID, LeaseOwner: task.LeaseOwner, FenceToken: task.FenceToken, AvailableAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}})
		if e != nil {
			t.Fatal(e)
		}
	}
	read := func(id uuid.UUID) QueryJobView {
		t.Helper()
		v, e := jobs.Get(ctx, actor, conversation, id)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}

	first := start("delivery-resume")
	if again := start("delivery-resume"); again.ID != first.ID {
		t.Fatal("duplicate request enqueued twice")
	}
	changed := input
	changed.Parameters.PanelIDs = []string{"missing"}
	if _, e := jobs.Start(ctx, actor, conversation, changed, "delivery-resume"); !errors.Is(e, ErrConflict) {
		t.Fatalf("request conflict lost: %v", e)
	}
	delivery.failAfter = 1
	task := claim(first.ID)
	before := backend.calls.Load()
	if e := jobs.Handle(ctx, task); e == nil {
		t.Fatal("injected disk quota failure disappeared")
	}
	view := read(first.ID)
	if view.Manifest == nil || view.Status != "delivering" || len(view.Manifest.Files) != 1 {
		t.Fatalf("snapshot not sealed before delivery: %+v", view)
	}
	if _, err := rt.Store.Pool.Exec(ctx, `UPDATE dashboard_query_jobs SET manifest='{}' WHERE id=$1`, first.ID); err == nil {
		t.Fatal("manifest is mutable")
	}
	file, err := rt.Store.Queries.GetDashboardQueryFile(ctx, db.GetDashboardQueryFileParams{ID: view.Manifest.Files[0].ID, EnterpriseID: actor.EnterpriseID})
	if err != nil {
		t.Fatal(err)
	}
	var chunks []queryChunk
	_ = json.Unmarshal(file.Chunks, &chunks)
	if len(chunks) < 2 {
		t.Fatal("fixture did not exercise real object fragments")
	}
	oldAttempt := view.Manifest.AttemptID
	requeue(task)
	delivery.failAfter = 0
	task = claim(first.ID)
	if e := jobs.Handle(ctx, task); e != nil {
		t.Fatal(e)
	}
	finish(task, "succeeded")
	view = read(first.ID)
	if view.Status != "complete" || view.Manifest.AttemptID != oldAttempt || backend.calls.Load() != before+1 {
		t.Fatalf("delivery repeated acquisition: %+v", view)
	}
	if view.Manifest.AnalysisStatus != "not_analyzed" || view.Manifest.Execution.RevisionID != first.RevisionID {
		t.Fatal("file delivery pretended analysis or lost revision")
	}
	for _, p := range view.Manifest.Execution.Panels {
		for _, target := range p.Targets {
			if target.Data != nil {
				t.Fatal("bulk result leaked through job status")
			}
		}
	}

	t.Run("fragment corruption", func(t *testing.T) {
		original, e := objects.ReadFile(ctx, chunks[0].Key, 0, 0)
		if e != nil {
			t.Fatal(e)
		}
		data, e := io.ReadAll(original)
		_ = original.Close()
		if e != nil {
			t.Fatal(e)
		}
		if e = objects.PutFile(ctx, chunks[0].Key, strings.NewReader("corrupt"), 7, "application/octet-stream"); e != nil {
			t.Fatal(e)
		}
		r, e := newQueryFileReader(ctx, objects, file, func() error { return nil })
		if e != nil {
			t.Fatal(e)
		}
		if _, e = io.ReadAll(r); !errors.Is(e, ErrInvalid) {
			t.Fatalf("corrupt fragment accepted: %v", e)
		}
		if e = objects.PutFile(ctx, chunks[0].Key, bytes.NewReader(data), int64(len(data)), "application/octet-stream"); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("interrupted acquisition gets a new attempt", func(t *testing.T) {
		v := start("acquisition-resume")
		task := claim(v.ID)
		wrapped.writes = 0
		wrapped.failAfter = len(chunks)
		if e := jobs.Handle(ctx, task); e == nil {
			t.Fatal("storage failure disappeared")
		}
		old, e := rt.Store.Queries.GetDashboardQueryJob(ctx, db.GetDashboardQueryJobParams{ID: v.ID, EnterpriseID: actor.EnterpriseID})
		if e != nil {
			t.Fatal(e)
		}
		oldFiles, e := rt.Store.Queries.ListDashboardQueryFiles(ctx, db.ListDashboardQueryFilesParams{AttemptID: old.AttemptID.UUID, EnterpriseID: actor.EnterpriseID})
		if e != nil || len(oldFiles) != 1 {
			t.Fatalf("unsealed first file missing: %v %v", oldFiles, e)
		}
		requeue(task)
		wrapped.failAfter = 0
		task = claim(v.ID)
		if e = jobs.Handle(ctx, task); e != nil {
			t.Fatal(e)
		}
		finish(task, "succeeded")
		current := read(v.ID)
		if current.Manifest.AttemptID == old.AttemptID.UUID {
			t.Fatal("two acquisition attempts merged")
		}
		principal := toolruntime.Principal{EnterpriseID: actor.EnterpriseID, UserID: actor.SubjectID, ConversationID: conversation, AuthorizationVersion: actor.AuthorizationVersion}
		if handled, e := jobs.AuthorizeSource(ctx, principal, queryFileView(oldFiles[0]).SourceRef); !handled || e == nil {
			t.Fatal("abandoned evidence became readable")
		}
	})
	t.Run("cancellation interrupts query and fences publication", func(t *testing.T) {
		v := start("cancel")
		task := claim(v.ID)
		backend.block = make(chan struct{})
		finished := make(chan error, 1)
		go func() { finished <- jobs.Handle(ctx, task) }()
		select {
		case <-backend.block:
		case <-time.After(5 * time.Second):
			t.Fatal("query never started")
		}
		if _, e := jobs.Cancel(ctx, actor, conversation, v.ID); e != nil {
			t.Fatal(e)
		}
		select {
		case e := <-finished:
			if e != nil {
				t.Fatal(e)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("cancel did not stop query")
		}
		backend.block = nil
		finish(task, "succeeded")
		if got := read(v.ID); got.Status != "cancelled" || got.Manifest != nil {
			t.Fatal("cancelled work published")
		}
	})
	t.Run("lost lease rejects stale worker", func(t *testing.T) {
		v := start("fence")
		old := claim(v.ID)
		exec(`UPDATE runtime_tasks SET lease_until=now()-interval '1 second' WHERE id=$1`, old.ID)
		fresh := claim(v.ID)
		n := backend.calls.Load()
		if e := jobs.Handle(ctx, old); e == nil {
			t.Fatal("stale worker accepted")
		}
		if backend.calls.Load() != n {
			t.Fatal("stale worker acquired data")
		}
		if e := jobs.Handle(ctx, fresh); e != nil {
			t.Fatal(e)
		}
		finish(fresh, "succeeded")
	})
	t.Run("exhausted delivery resumes without reacquiring", func(t *testing.T) {
		v := start("manual-resume")
		task := claim(v.ID)
		delivery.failAfter = delivery.calls + 1
		if e := jobs.Handle(ctx, task); e == nil {
			t.Fatal("delivery did not fail")
		}
		if e := jobs.HandleExhausted(ctx, task, io.ErrUnexpectedEOF); e != nil {
			t.Fatal(e)
		}
		finish(task, "failed")
		failed := read(v.ID)
		n := backend.calls.Load()
		delivery.failAfter = 0
		if _, e := jobs.Resume(ctx, actor, conversation, v.ID, failed.Version-1); !errors.Is(e, ErrConflict) {
			t.Fatal("stale resume accepted")
		}
		if _, e := jobs.Resume(ctx, actor, conversation, v.ID, failed.Version); e != nil {
			t.Fatal(e)
		}
		task = claim(v.ID)
		if e := jobs.Handle(ctx, task); e != nil {
			t.Fatal(e)
		}
		finish(task, "succeeded")
		if backend.calls.Load() != n || read(v.ID).Status != "complete" {
			t.Fatal("resumed materialized data was queried again")
		}
	})
	t.Run("source revalidation and conversation isolation", func(t *testing.T) {
		p := toolruntime.Principal{EnterpriseID: actor.EnterpriseID, UserID: actor.SubjectID, ConversationID: conversation, AuthorizationVersion: actor.AuthorizationVersion}
		ref := view.Manifest.Files[0].SourceRef
		if handled, e := jobs.AuthorizeSource(ctx, p, ref); !handled || e != nil {
			t.Fatal(e)
		}
		p.ConversationID = uuid.New()
		if _, e := jobs.AuthorizeSource(ctx, p, ref); e == nil {
			t.Fatal("cross-conversation source accepted")
		}
		p.ConversationID = conversation
		p.EnterpriseID = uuid.New()
		if _, e := jobs.AuthorizeSource(ctx, p, ref); e == nil {
			t.Fatal("cross-enterprise source accepted")
		}
		p.EnterpriseID = actor.EnterpriseID
		exec(`UPDATE data_authorization_grants SET status='disabled' WHERE enterprise_id=$1 AND resource_type='host' AND resource_id=$2`, actor.EnterpriseID, host)
		if _, e := jobs.AuthorizeSource(ctx, p, ref); !errors.Is(e, ErrDenied) {
			t.Fatalf("revoked source accepted: %v", e)
		}
		if _, e := jobs.Get(ctx, actor, conversation, first.ID); !errors.Is(e, ErrDenied) {
			t.Fatal("revoked job status disclosed")
		}
		exec(`UPDATE data_authorization_grants SET status='active' WHERE enterprise_id=$1 AND resource_type='host' AND resource_id=$2`, actor.EnterpriseID, host)
	})
	t.Run("latest revision at enqueue stays frozen during execution", func(t *testing.T) {
		old, revision, e := (Service{Store: rt.Store}).Get(ctx, actor, dashboardID)
		if e != nil {
			t.Fatal(e)
		}
		v := start("pinned-before-publish")
		spec, e := DecodeSpec(revision.Spec)
		if e != nil {
			t.Fatal(e)
		}
		spec.Panels[0].Targets[0].SourceDefinition = Definition{Builder: &Builder{Operation: "records", Filters: []Filter{{Field: "body", Operator: "=", Value: "new revision only"}}}}
		encoded, _ := json.Marshal(spec)
		updated, e := rt.Store.Queries.CreateDashboardRevision(ctx, db.CreateDashboardRevisionParams{ID: uuid.New(), EnterpriseID: actor.EnterpriseID, DashboardID: dashboardID, RevisionNumber: revision.RevisionNumber + 1, SchemaVersion: SchemaVersion, Name: revision.Name, Description: revision.Description, FolderID: revision.FolderID, Spec: encoded, SpecHash: queryHash(encoded), ValidationReport: revision.ValidationReport, SampleReport: revision.SampleReport, CreatedBy: actor.SubjectID})
		if e != nil {
			t.Fatal(e)
		}
		exec(`UPDATE dashboards SET active_revision_id=$2,version=version+1 WHERE id=$1`, dashboardID, updated.ID)
		defer exec(`UPDATE dashboards SET active_revision_id=$2,version=version+1 WHERE id=$1`, dashboardID, old.ActiveRevisionID.UUID)
		task := claim(v.ID)
		if e = jobs.Handle(ctx, task); e != nil {
			t.Fatal(e)
		}
		finish(task, "succeeded")
		frozen := read(v.ID)
		if frozen.Manifest.Execution.RevisionID != revision.ID || frozen.Manifest.Execution.Panels[0].Status != "success" {
			t.Fatal("enqueued query adopted a newer revision")
		}
		latest := start("after-publish")
		if latest.RevisionID != updated.ID {
			t.Fatal("new acquisition used an old revision")
		}
		task = claim(latest.ID)
		if e = jobs.Handle(ctx, task); e != nil {
			t.Fatal(e)
		}
		finish(task, "succeeded")
		if read(latest.ID).Manifest.Execution.Panels[0].Status != "no_data" {
			t.Fatal("new definition was not executed")
		}
	})
	t.Run("real processor dispatch and durable completion", func(t *testing.T) {
		v := start("processor")
		workerCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		stopped := make(chan error, 1)
		go func() {
			stopped <- (taskruntime.Processor{Store: rt.Store, Queue: "dashboard_query", Owner: "query-integration", Lease: 3 * time.Second, Poll: 20 * time.Millisecond, Handle: jobs, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}).Run(workerCtx)
		}()
		deadline := time.After(10 * time.Second)
		for {
			view := read(v.ID)
			var taskStatus string
			if e := rt.Store.Pool.QueryRow(ctx, `SELECT t.status FROM runtime_tasks t JOIN dashboard_query_jobs j ON j.task_id=t.id WHERE j.id=$1`, v.ID).Scan(&taskStatus); e != nil {
				t.Fatal(e)
			}
			if view.Status == "complete" && taskStatus == "succeeded" {
				break
			}
			select {
			case <-deadline:
				t.Fatal("worker did not complete query")
			case <-time.After(25 * time.Millisecond):
			}
		}
		cancel()
		if e := <-stopped; e != nil {
			t.Fatal(e)
		}
		got := read(v.ID)
		if len(got.Files) != 2 || got.Files[0].WorkspaceFileID == nil || got.Files[1].WorkspaceFileID == nil {
			t.Fatal("completed query did not return authoritative Workspace file references")
		}
	})
	t.Run("Run cancellation stops acquisition", func(t *testing.T) {
		runID := uuid.New()
		exec(`INSERT INTO runs(id,enterprise_id,conversation_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'en-US',1,'running')`, runID, actor.EnterpriseID, conversation, actor.SubjectID, model)
		withRun := input
		withRun.RunID = runID
		v, e := jobs.Start(ctx, actor, conversation, withRun, "run-cancel")
		if e != nil {
			t.Fatal(e)
		}
		task := claim(v.ID)
		exec(`UPDATE runs SET status='cancelled' WHERE id=$1`, runID)
		n := backend.calls.Load()
		if e = jobs.Handle(ctx, task); e != nil {
			t.Fatal(e)
		}
		finish(task, "succeeded")
		if backend.calls.Load() != n || read(v.ID).Status != "cancelled" {
			t.Fatal("cancelled Run still queried data")
		}
	})
	// Assert no duplicate Workspace records appeared after replay.
	var total, distinct int
	if e := rt.Store.Pool.QueryRow(ctx, `SELECT count(*),count(DISTINCT path) FROM workspace_files WHERE workspace_id=$1`, workspace).Scan(&total, &distinct); e != nil || total != distinct {
		t.Fatalf("duplicate file delivery: %d/%d %v", total, distinct, e)
	}
	t.Run("conversation deletion fences cleanup", func(t *testing.T) {
		active := start("delete-active")
		task := claim(active.ID)
		queued := start("delete-queued")
		domain := conversationservice.Service{Store: rt.Store, Idempotency: postgres.Idempotency{Key: bytes.Repeat([]byte{6}, 32)}}
		if _, e := domain.Delete(ctx, actor.EnterpriseID, actor.SubjectID, conversation, "delete-query-conversation"); e != nil {
			t.Fatal(e)
		}
		for _, id := range []uuid.UUID{active.ID, queued.ID} {
			job, e := rt.Store.Queries.GetDashboardQueryJob(ctx, db.GetDashboardQueryJobParams{ID: id, EnterpriseID: actor.EnterpriseID})
			if e != nil || job.Status != "cancelled" {
				t.Fatalf("deleted conversation left active query: %v %v", job, e)
			}
		}
		exec(`UPDATE workspaces SET status='deleted' WHERE id=$1`, workspace)
		pending, e := rt.Store.Queries.ListDeletedConversationsForCleanup(ctx, 100)
		if e != nil {
			t.Fatal(e)
		}
		for _, c := range pending {
			if c.ID == conversation {
				t.Fatal("object cleanup ran before active writer was fenced")
			}
		}
		if e := jobs.Handle(ctx, task); e != nil {
			t.Fatal(e)
		}
		finish(task, "succeeded")
		pending, e = rt.Store.Queries.ListDeletedConversationsForCleanup(ctx, 100)
		if e != nil {
			t.Fatal(e)
		}
		found := false
		for _, c := range pending {
			found = found || c.ID == conversation
		}
		if !found {
			t.Fatal("finished writer blocked cleanup")
		}
	})
}
