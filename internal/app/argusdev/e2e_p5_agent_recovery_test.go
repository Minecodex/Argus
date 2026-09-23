package argusdev

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
)

func TestP5CompletedToolRecoveryFixturePreservesExistingEvents(t *testing.T) {
	url := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires a disposable migrated database")
	}
	store, err := postgres.Open(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(t.Context(), sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	e, d, u, m, c, r, sourceRun, sourceStep, original := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Fixture SQL',$2,'UTC')", e, "sql-"+e.String())
	exec("INSERT INTO departments(id,enterprise_id,name,is_default) VALUES($1,$2,'Default',true)", d, e)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Fixture')", u, e, d, "sql-"+u.String())
	exec("INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,health_status) VALUES($1,$2,'Fixture','https://model.example.test','model','chat_completions',32768,1024,0,0,'healthy')", m, e)
	exec("INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id,event_sequence) VALUES($1,$2,$3,'Fixture',$4,1)", c, e, u, m)
	for _, id := range []uuid.UUID{r, sourceRun} {
		exec("INSERT INTO runs(id,conversation_id,enterprise_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'en-US',1,'succeeded')", id, c, e, u, m)
	}
	exec("INSERT INTO run_steps(id,run_id,enterprise_id,sequence,step_type,status) VALUES($1,$2,$3,1,'model_call','succeeded')", sourceStep, sourceRun, e)
	exec("INSERT INTO tool_calls(id,call_id,enterprise_id,run_id,step_id,tool_id,source,input,input_hash,status,authorization_scope) VALUES(gen_random_uuid(),'source',$1,$2,$3,'tool.invoke','argus','{}',sha256('{}'::bytea),'succeeded','fixture-scope')", e, sourceRun, sourceStep)
	exec("INSERT INTO conversation_events(id,enterprise_id,conversation_id,run_id,sequence,event_type,actor_type,actor_id,payload,content_hash,data_classification) VALUES($1,$2,$3,$4,1,'user_message','user','fixture','{\"content\":\"keep\"}',sha256('{}'::bytea),'internal')", original, e, c, r)
	encoded, _ := json.Marshal(map[string]any{"summary": strings.Repeat("x", 25000)})
	exec(p5CompletedToolFixtureSQL(r.String(), e.String(), encoded))
	var preserved, events, results int
	err = store.Pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM conversation_events WHERE id=$1 AND payload->>'content'='keep'),(SELECT count(*) FROM conversation_events WHERE run_id=$2),(SELECT count(*) FROM tool_results t JOIN tool_calls c ON c.id=t.tool_call_id WHERE c.run_id=$2 AND t.projection_bytes>25000)", original, r).Scan(&preserved, &events, &results)
	if err != nil || preserved != 1 || events != 3 || results != 1 {
		t.Fatalf("fixture failed: preserved=%d events=%d results=%d err=%v", preserved, events, results, err)
	}
}
