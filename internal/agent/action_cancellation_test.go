package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestCancelledRunAndEarlyPreviewCancellationNeverReturnToWaiting(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires a disposable migrated database")
	}
	store, err := postgres.Open(t.Context(), address)
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
	e, d, u, m, c := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Cancel race',$2,'UTC')", e, "cancel-"+e.String())
	exec("INSERT INTO departments(id,enterprise_id,name,is_default) VALUES($1,$2,'Default',true)", d, e)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Cancellation user')", u, e, d, "cancel-"+u.String())
	exec("INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,health_status) VALUES($1,$2,'P5','https://model.example.test','model','chat_completions',32768,1024,0,0,'healthy')", m, e)
	exec("INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'Cancel race',$4)", c, e, u, m)
	loop := Loop{Store: store}
	service := conversation.Service{Store: store, Idempotency: postgres.Idempotency{Key: bytes.Repeat([]byte{7}, 32)}}
	for _, earlyPreview := range []bool{false, true} {
		runID, actionID := uuid.New(), uuid.New()
		ref := "act_" + actionID.String()
		status := "awaiting_confirmation"
		if earlyPreview {
			status = "cancelled"
		}
		exec("INSERT INTO runs(id,conversation_id,enterprise_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'en-US',1,'running')", runID, c, e, u, m)
		exec("INSERT INTO pending_actions(id,action_ref,enterprise_id,creator_subject_id,authorization_version,action_type,title,summary,risk,preview,status,resource_type,impact_hash,expires_at,run_id) VALUES($1,$2,$3,$4,1,'host.update','Preview','Preview','write','{}',$5,'host',decode(repeat('00',32),'hex'),now()+interval '15 minutes',$6)", actionID, ref, e, u, status, runID)
		run, err := store.Queries.GetRun(t.Context(), db.GetRunParams{ID: runID, EnterpriseID: e})
		if err != nil {
			t.Fatal(err)
		}
		requestContext, stop := loop.toolContext(t.Context(), run)
		if !earlyPreview {
			if _, err := service.CancelRun(t.Context(), uuid.NewString(), e, runID, uuid.NewString()); err == nil {
				t.Fatal("another actor cancelled this Run")
			}
			key := uuid.NewString()
			for replay := 0; replay < 2; replay++ {
				if _, err := service.CancelRun(t.Context(), u.String(), e, runID, key); err != nil {
					t.Fatal(err)
				}
			}
		}
		for repeat := 0; repeat < 2; repeat++ {
			if _, _, err := loop.reconcileActions(t.Context(), run); err != nil {
				t.Fatal(err)
			}
		}
		select {
		case <-requestContext.Done():
		case <-time.After(2 * time.Second):
			t.Fatal("model/tool request did not observe durable cancellation")
		}
		stop()
		current, err := store.Queries.GetRun(t.Context(), db.GetRunParams{ID: runID, EnterpriseID: e})
		if err != nil || current.Status != "cancelled" {
			t.Fatalf("cancelled Run resumed waiting: %v", err)
		}
		var count int
		if err := store.Pool.QueryRow(t.Context(), "SELECT count(*) FROM conversation_events WHERE run_id=$1 AND event_type='run_state_changed' AND payload->>'status'='cancelled'", runID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("terminal event count=%d / %v", count, err)
		}
	}
	t.Run("persisted unknown result stops before another model call", func(t *testing.T) {
		run, step, call, task := uuid.New(), uuid.New(), uuid.New(), uuid.New()
		exec("INSERT INTO runs(id,conversation_id,enterprise_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'en-US',1,'running')", run, c, e, u, m)
		exec("INSERT INTO run_steps(id,run_id,enterprise_id,sequence,step_type,status) VALUES($1,$2,$3,1,'model_call','running')", step, run, e)
		exec("INSERT INTO tool_calls(id,call_id,enterprise_id,run_id,step_id,tool_id,source,input,input_hash,status,dispatched_at) VALUES($1,$2,$3,$4,$5,'bash','sandbox','{}',decode(repeat('00',32),'hex'),'result_unknown',now())", call, call.String(), e, run, step)
		payload, _ := json.Marshal(conversation.AgentTask{RunID: run, EnterpriseID: e})
		exec("INSERT INTO runtime_tasks(id,enterprise_id,queue,run_id,payload,status,lease_owner,lease_until,fence_token,attempt) VALUES($1,$2,'agent',$3,$4,'running','recovery',now()+interval '30 seconds',1,1)", task, e, run, payload)
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if _, err := store.Pool.Exec(cleanup, "UPDATE runtime_tasks SET status='succeeded',lease_owner=NULL,lease_until=NULL WHERE id=$1", task); err != nil {
				t.Error(err)
			}
		}()
		request := runtime.Task{RuntimeTask: db.RuntimeTask{ID: task, Payload: payload, LeaseOwner: pgtype.Text{String: "recovery", Valid: true}, FenceToken: 1}}
		if err := loop.Handle(t.Context(), request); err != nil {
			t.Fatal(err)
		}
		current, err := store.Queries.GetRun(t.Context(), db.GetRunParams{ID: run, EnterpriseID: e})
		if err != nil || current.Status != "failed" || current.StopReason.String != "result_unknown" {
			t.Fatalf("recovery continued after unknown result: %s/%s %v", current.Status, current.StopReason.String, err)
		}
		var calls int
		if err := store.Pool.QueryRow(t.Context(), "SELECT count(*) FROM model_calls WHERE run_id=$1", run).Scan(&calls); err != nil || calls != 0 {
			t.Fatalf("recovery dispatched another model call: %d %v", calls, err)
		}
	})
}
