package agent

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type recoveryFixture struct {
	t                *testing.T
	store            *postgres.Store
	e, d, u, m, c, r uuid.UUID
}

func newRecoveryFixture(t *testing.T) *recoveryFixture {
	t.Helper()
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires a disposable migrated database")
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	f := &recoveryFixture{t: t, store: store, e: uuid.New(), d: uuid.New(), u: uuid.New(), m: uuid.New(), c: uuid.New(), r: uuid.New()}
	t.Cleanup(func() {
		_, err := store.Pool.Exec(context.Background(), "UPDATE runtime_tasks SET status='succeeded',lease_owner=NULL,lease_until=NULL WHERE enterprise_id=$1", f.e)
		if err != nil {
			t.Error(err)
		}
		store.Close()
	})
	f.exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Recovery',$2,'UTC')", f.e, "recovery-"+f.e.String())
	if err := audit.InitializeChain(t.Context(), store.Queries, "enterprise", uuid.NullUUID{UUID: f.e, Valid: true}); err != nil {
		t.Fatal(err)
	}
	f.exec("INSERT INTO departments(id,enterprise_id,name,is_default) VALUES($1,$2,'Default',true)", f.d, f.e)
	f.exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Recovery')", f.u, f.e, f.d, "recovery-"+f.u.String())
	f.exec("INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,health_status) VALUES($1,$2,'Recovery','https://model.example.test','model','chat_completions',32768,1024,0,0,'healthy')", f.m, f.e)
	f.exec("INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'Recovery',$4)", f.c, f.e, f.u, f.m)
	f.exec("INSERT INTO runs(id,conversation_id,enterprise_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'zh-CN',1,'running')", f.r, f.c, f.e, f.u, f.m)
	return f
}
func (f *recoveryFixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.store.Pool.Exec(f.t.Context(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}
func (f *recoveryFixture) run() db.Run {
	f.t.Helper()
	r, err := f.store.Queries.GetRun(f.t.Context(), db.GetRunParams{ID: f.r, EnterpriseID: f.e})
	if err != nil {
		f.t.Fatal(err)
	}
	return r
}
func (f *recoveryFixture) task(queue, reason string) runtime.Task {
	f.t.Helper()
	id := uuid.New()
	payload, _ := json.Marshal(conversation.AgentTask{RunID: f.r, EnterpriseID: f.e, Reason: reason})
	f.exec("INSERT INTO runtime_tasks(id,enterprise_id,queue,run_id,payload,status,lease_owner,lease_until,fence_token,attempt) VALUES($1,$2,$3,$4,$5,'running','recovery-test',now()+interval '5 minutes',1,1)", id, f.e, queue, f.r, payload)
	return runtime.Task{RuntimeTask: db.RuntimeTask{ID: id, RunID: uuid.NullUUID{UUID: f.r, Valid: true}, Payload: payload, LeaseOwner: pgtype.Text{String: "recovery-test", Valid: true}, FenceToken: 1}}
}
func (f *recoveryFixture) count(sql string, args ...any) int {
	f.t.Helper()
	var n int
	if err := f.store.Pool.QueryRow(f.t.Context(), sql, args...).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}
