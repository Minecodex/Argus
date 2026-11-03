package toolgateway

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresNativeSearchAuthorityRejectsUnlistedStaleAndForeignContexts(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires owned isolated migrated PostgreSQL")
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	tx, err := store.Pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(t.Context(), sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	e, d, u, m, c, r, s, now, call, artifact, searchModel := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Search authority',$2,'UTC')", e, e.String())
	exec("INSERT INTO departments(id,enterprise_id,name,is_default) VALUES($1,$2,'Default',true)", d, e)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'User')", u, e, d, u.String())
	exec("INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,health_status) VALUES($1,$2,'Fixture','https://model.example.test','model','chat_completions',32768,1024,0,0,'healthy')", m, e)
	exec("INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'Search',$4)", c, e, u, m)
	exec("INSERT INTO runs(id,conversation_id,enterprise_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'en-US',1,'running')", r, c, e, u, m)
	exec("INSERT INTO run_steps(id,run_id,enterprise_id,sequence,step_type,status) VALUES($1,$3,$4,1,'tool_call','succeeded'),($2,$3,$4,2,'tool_call','running')", s, now, r, e)
	exec("INSERT INTO tool_calls(id,call_id,enterprise_id,run_id,step_id,model_call_id,tool_id,source,input,input_hash,status) VALUES($1,$2,$3,$4,$5,$6,'tool.search','argus','{\"category\":\"alert\"}',decode(repeat('00',32),'hex'),'succeeded')", call, call.String(), e, r, s, searchModel)
	exec("INSERT INTO artifacts(id,result_ref,enterprise_id,content_type,data_classification,content,content_hash,byte_size) VALUES($1,$2,$3,'application/json','internal',convert_to('{}','UTF8'),decode(repeat('00',32),'hex'),2)", artifact, artifact.String(), e)
	exec("INSERT INTO tool_results(id,tool_call_id,enterprise_id,artifact_id,projection,projection_hash,projection_bytes) VALUES($1,$2,$3,$4,'{\"summary\":{\"category\":\"alert\",\"complete\":true,\"items\":[{\"name\":\"channel.list\",\"version\":\"current-version\"}]}}',decode(repeat('00',32),'hex'),120)", uuid.New(), call, e, artifact)
	q := db.New(tx)
	base := db.HasEarlierNativeToolSearchParams{StepID: now, EnterpriseID: e, UserID: u, RunID: r, ModelCallID: uuid.New(), Category: "alert", Name: "channel.list", Version: "current-version"}
	check := func(p db.HasEarlierNativeToolSearchParams, want bool) {
		t.Helper()
		found, err := q.HasEarlierNativeToolSearch(t.Context(), p)
		if err != nil || found != want {
			t.Fatal(found, want, err)
		}
	}
	check(base, true)
	for _, mutate := range []func(*db.HasEarlierNativeToolSearchParams){
		func(p *db.HasEarlierNativeToolSearchParams) { p.ModelCallID = searchModel },
		func(p *db.HasEarlierNativeToolSearchParams) { p.StepID = s },
		func(p *db.HasEarlierNativeToolSearchParams) { p.StepID = uuid.New() },
		func(p *db.HasEarlierNativeToolSearchParams) { p.RunID = uuid.New() },
		func(p *db.HasEarlierNativeToolSearchParams) { p.UserID = uuid.New() },
		func(p *db.HasEarlierNativeToolSearchParams) { p.EnterpriseID = uuid.New() },
		func(p *db.HasEarlierNativeToolSearchParams) { p.Category = "metric" },
		func(p *db.HasEarlierNativeToolSearchParams) { p.Name = "policy.list" },
		func(p *db.HasEarlierNativeToolSearchParams) { p.Version = "old-version" },
	} {
		p := base
		mutate(&p)
		check(p, false)
	}
	exec("UPDATE tool_calls SET status='failed' WHERE id=$1", call)
	check(base, false)
	exec("UPDATE tool_calls SET status='succeeded' WHERE id=$1", call)
	exec("UPDATE tool_results SET projection=jsonb_set(projection,'{summary,complete}','false') WHERE tool_call_id=$1", call)
	check(base, false)
	exec("UPDATE tool_results SET projection=jsonb_set(projection,'{summary,complete}','true') WHERE tool_call_id=$1", call)
	exec("UPDATE run_steps SET sequence=3 WHERE id=$1", s)
	check(base, false)
}
