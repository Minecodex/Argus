package agent

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestRunQueueSerializesTasksAndFencesAnExpiredWorker(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires a disposable migrated database")
	}
	ctx := t.Context()
	store, err := postgres.Open(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	enterprise, department, user, model, conversation, run := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'P5 fence',$2,'UTC')", enterprise, "p5-"+enterprise.String())
	exec("INSERT INTO departments(id,enterprise_id,name,is_default) VALUES($1,$2,'Default',true)", department, enterprise)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'P5 fence')", user, enterprise, department, "p5-"+user.String())
	exec("INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,health_status) VALUES($1,$2,'P5','https://model.example.test','model','chat_completions',32768,1024,0,0,'healthy')", model, enterprise)
	exec("INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'P5 fence',$4)", conversation, enterprise, user, model)
	exec("INSERT INTO runs(id,conversation_id,enterprise_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'en-US',1,'running')", run, conversation, enterprise, user, model)
	first, second := uuid.New(), uuid.New()
	defer func() {
		_, _ = store.Pool.Exec(context.Background(), "UPDATE runtime_tasks SET status='cancelled',lease_owner=NULL,lease_until=NULL WHERE id=ANY($1::uuid[])", []uuid.UUID{first, second})
	}()
	for index, id := range []uuid.UUID{first, second} {
		exec("INSERT INTO runtime_tasks(id,enterprise_id,queue,run_id,payload,max_attempts,available_at) VALUES($1,$2,'agent',$3,'{}',1,now()-interval '1 hour'+$4*interval '1 second')", id, enterprise, run, index)
	}
	claim := func(owner string) db.RuntimeTask {
		t.Helper()
		task, err := store.Queries.ClaimRuntimeTask(ctx, db.ClaimRuntimeTaskParams{Queue: "agent", LeaseOwner: pgtype.Text{String: owner, Valid: true}, Column3: pgtype.Interval{Microseconds: 60_000_000, Valid: true}})
		if err != nil {
			t.Fatal(err)
		}
		task, err = store.Queries.StartRuntimeTask(ctx, db.StartRuntimeTaskParams{ID: task.ID, LeaseOwner: task.LeaseOwner, FenceToken: task.FenceToken})
		if err != nil {
			t.Fatal(err)
		}
		return task
	}
	old := claim("first-worker")
	if old.ID != first {
		t.Fatal("incorrect task ordering")
	}
	oldCtx, cancel := withTaskLease(ctx, store, runtime.Task{RuntimeTask: old})
	defer cancel()
	if err := assertTaskLease(oldCtx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Queries.ClaimRuntimeTask(ctx, db.ClaimRuntimeTaskParams{Queue: "agent", LeaseOwner: pgtype.Text{String: "overlap", Valid: true}, Column3: pgtype.Interval{Microseconds: 60_000_000, Valid: true}}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("overlapping Run task claimed: %v", err)
	}
	exec("UPDATE runtime_tasks SET lease_until=now()-interval '1 second' WHERE id=$1", first)
	taken := claim("replacement-worker")
	if taken.ID != first || taken.Attempt <= taken.MaxAttempts || taken.FenceToken <= old.FenceToken {
		t.Fatal("exhausted expired task was not reclaimed for terminal reconciliation")
	}
	if err := assertTaskLease(oldCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("old worker retained authority: %v", err)
	}
	wrote := false
	if err := fencedTransaction(oldCtx, store, func(*db.Queries) error { wrote = true; return nil }); err == nil || wrote {
		t.Fatal("old worker appended facts after takeover")
	}
	if _, err := store.Queries.FinishRuntimeTask(ctx, db.FinishRuntimeTaskParams{ID: taken.ID, LeaseOwner: taken.LeaseOwner, FenceToken: taken.FenceToken, Status: "failed"}); err != nil {
		t.Fatal(err)
	}
	next := claim("next-worker")
	if next.ID != second {
		t.Fatal("next task remained blocked after terminal recovery")
	}
}
