package toolgateway

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresNativeDescriptionAuthorityRejectsBatchRunAndVersionMixups(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable migrated PostgreSQL")
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
	e, d, u, model, convo, run, described, current, call, artifact, priorModel := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Description authority',$2,'UTC')", e, e.String())
	exec("INSERT INTO departments(id,enterprise_id,name,is_default) VALUES($1,$2,'Default',true)", d, e)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'User')", u, e, d, u.String())
	exec("INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,health_status) VALUES($1,$2,'Fixture','https://model.example.test','model','chat_completions',32768,1024,0,0,'healthy')", model, e)
	exec("INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'Description',$4)", convo, e, u, model)
	exec("INSERT INTO runs(id,conversation_id,enterprise_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'en-US',1,'running')", run, convo, e, u, model)
	exec("INSERT INTO run_steps(id,run_id,enterprise_id,sequence,step_type,status) VALUES($1,$3,$4,2,'tool_call','succeeded'),($2,$3,$4,4,'tool_call','running')", described, current, run, e)
	exec("INSERT INTO tool_calls(id,call_id,enterprise_id,run_id,step_id,model_call_id,tool_id,source,input,input_hash,status) VALUES($1,$2,$3,$4,$5,$6,'tool.describe','argus','{\"category\":\"alert\",\"name\":\"channel.list\"}',decode(repeat('00',32),'hex'),'succeeded')", call, call.String(), e, run, described, priorModel)
	exec("INSERT INTO artifacts(id,result_ref,enterprise_id,content_type,data_classification,content,content_hash,byte_size) VALUES($1,$2,$3,'application/json','internal',convert_to('{}','UTF8'),decode(repeat('00',32),'hex'),2)", artifact, artifact.String(), e)
	exec("INSERT INTO tool_results(id,tool_call_id,enterprise_id,artifact_id,projection,projection_hash,projection_bytes) VALUES($1,$2,$3,$4,'{\"summary\":{\"version\":\"current-version\"}}',decode(repeat('00',32),'hex'),42)", uuid.New(), call, e, artifact)
	queries := db.New(tx)
	base := db.HasEarlierNativeToolDescriptionParams{EnterpriseID: e, UserID: u, RunID: run, StepID: current, ModelCallID: uuid.New(), Category: "alert", Name: "channel.list", Version: "current-version"}
	check := func(params db.HasEarlierNativeToolDescriptionParams, want bool) {
		t.Helper()
		found, err := queries.HasEarlierNativeToolDescription(t.Context(), params)
		if err != nil || found != want {
			t.Fatal(found, want, err)
		}
	}
	check(base, true)
	for _, mutate := range []func(*db.HasEarlierNativeToolDescriptionParams){
		func(p *db.HasEarlierNativeToolDescriptionParams) { p.ModelCallID = priorModel },
		func(p *db.HasEarlierNativeToolDescriptionParams) { p.RunID = uuid.New() },
		func(p *db.HasEarlierNativeToolDescriptionParams) { p.UserID = uuid.New() },
		func(p *db.HasEarlierNativeToolDescriptionParams) { p.EnterpriseID = uuid.New() },
		func(p *db.HasEarlierNativeToolDescriptionParams) { p.Name = "policy.list" },
		func(p *db.HasEarlierNativeToolDescriptionParams) { p.Version = "old-version" },
	} {
		params := base
		mutate(&params)
		check(params, false)
	}
	exec("UPDATE tool_calls SET status='failed' WHERE id=$1", call)
	check(base, false)
	exec("UPDATE tool_calls SET status='succeeded' WHERE id=$1", call)
	exec("UPDATE run_steps SET sequence=5 WHERE id=$1", described)
	check(base, false)
}
