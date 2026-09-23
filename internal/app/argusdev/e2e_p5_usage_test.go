package argusdev

import (
	"encoding/json"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
)

func TestP5UsageGateRequiresReportedSettledSuccessfulCalls(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires a disposable migrated database")
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(t.Context(), query, args...); err != nil {
			t.Fatal(err)
		}
	}
	e, d, u, m, c, r, step, call := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Usage',$2,'UTC')", e, "usage-"+e.String())
	exec("INSERT INTO departments(id,enterprise_id,name,is_default) VALUES($1,$2,'Default',true)", d, e)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Usage')", u, e, d, "usage-"+u.String())
	exec("INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,health_status) VALUES($1,$2,'Usage','https://model.example.test','model','chat_completions',32768,1024,0,0,'healthy')", m, e)
	exec("INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'Usage',$4)", c, e, u, m)
	exec("INSERT INTO runs(id,conversation_id,enterprise_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'en-US',1,'succeeded')", r, c, e, u, m)
	exec("INSERT INTO run_steps(id,run_id,enterprise_id,sequence,step_type,status) VALUES($1,$2,$3,1,'context_compaction','succeeded')", step, r, e)
	exec("INSERT INTO model_calls(id,enterprise_id,run_id,step_id,model_id,model_revision,call_kind,projection_hash,input_price_snapshot,output_price_snapshot,status,input_tokens,output_tokens) VALUES($1,$2,$3,$4,$5,1,'compaction',sha256('{}'::bytea),0,0,'succeeded',49,10)", call, e, r, step, m)
	exec("INSERT INTO model_quota_reservations(id,enterprise_id,model_call_id,model_id,department_id,user_id,month,reserved_amount,settled_amount,status,expires_at) VALUES($1,$2,$3,$4,$5,$6,current_date,0,0,'settled',now()+interval '1 hour')", uuid.New(), e, call, m, d, u)
	for _, test := range []struct {
		name, input, output, status, settled, source string
		complete                                     bool
		reportedInput, reportedOutput                int64
	}{
		{"reported", "provider", "provider", "succeeded", "settled", "provider", true, 49, 10},
		{"estimated", "estimated", "estimated", "succeeded", "settled", "estimated", false, 0, 0},
		{"partial", "provider", "estimated", "succeeded", "settled", "estimated", false, 49, 0},
		{"missing", "missing", "missing", "succeeded", "settled", "missing", false, 0, 0},
		{"failed", "provider", "provider", "failed", "settled", "provider", false, 49, 10},
		{"unsettled", "provider", "provider", "succeeded", "active", "provider", false, 49, 10},
	} {
		t.Run(test.name, func(t *testing.T) {
			exec("UPDATE model_calls SET input_usage_source=$2,output_usage_source=$3,status=$4 WHERE id=$1", call, test.input, test.output, test.status)
			exec("UPDATE model_quota_reservations SET status=$2,usage_source=$3 WHERE model_call_id=$1", call, test.settled, test.source)
			var data []byte
			if err := store.Pool.QueryRow(t.Context(), p5BenchmarkSampleSQL(r.String())).Scan(&data); err != nil {
				t.Fatal(err)
			}
			var value p5BenchmarkSample
			if err := json.Unmarshal(data, &value); err != nil {
				t.Fatal(err)
			}
			if value.UsageComplete != test.complete || value.InputTokens != test.reportedInput || value.OutputTokens != test.reportedOutput {
				t.Fatalf("wrong usage: %s", data)
			}
		})
	}
	t.Run("invalid observations", func(t *testing.T) {
		exec("UPDATE model_calls SET input_tokens=50,output_tokens=8,cached_input_tokens=80,input_usage_source='invalid',output_usage_source='invalid',cached_input_usage_source='invalid',status='failed',completed_at=now() WHERE id=$1", call)
		exec("UPDATE model_quota_reservations SET status='settled',usage_source='invalid' WHERE model_call_id=$1", call)
		for _, query := range []string{
			"UPDATE model_calls SET input_usage_source='provider',output_usage_source='provider',cached_input_usage_source='provider' WHERE id=$1",
			"UPDATE model_calls SET input_usage_source='provider' WHERE id=$1",
			"UPDATE model_calls SET status='succeeded' WHERE id=$1",
			"UPDATE model_calls SET input_tokens=-1 WHERE id=$1",
		} {
			if _, err := store.Pool.Exec(t.Context(), query, call); err == nil {
				t.Fatal("invalid model call accepted by database")
			}
		}
		var data []byte
		if err := store.Pool.QueryRow(t.Context(), p5BenchmarkSampleSQL(r.String())).Scan(&data); err != nil {
			t.Fatal(err)
		}
		var sample p5BenchmarkSample
		if err := json.Unmarshal(data, &sample); err != nil {
			t.Fatal(err)
		}
		if sample.UsageComplete || sample.CachedUsageComplete || sample.InputTokens != 0 || sample.OutputTokens != 0 || sample.CachedInputTokens != 0 {
			t.Fatalf("invalid benchmark=%s", data)
		}
		rows, err := store.Queries.ListModelUsage(t.Context(), db.ListModelUsageParams{EnterpriseID: e, CompletedAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true}, CompletedAt_2: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true}})
		if err != nil || len(rows) != 1 {
			t.Fatalf("public usage: %v %v", rows, err)
		}
		got := rows[0]
		if got.UsageComplete || got.CachedUsageComplete || got.InputTokens != 0 || got.OutputTokens != 0 || got.CachedInputTokens != 0 || got.EstimatedInputTokens != 0 || got.EstimatedOutputTokens != 0 {
			t.Fatalf("invalid public totals=%+v", got)
		}
	})

}
