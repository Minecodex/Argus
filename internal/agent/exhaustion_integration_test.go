package agent

import (
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/runtime"
	"testing"
)

func TestExhaustedModelAttemptTerminatesRunWithAlreadyClosedStep(t *testing.T) {
	for _, state := range []string{"running", "failed", "succeeded", "cancelled"} {
		t.Run(state, func(t *testing.T) {
			f := newRecoveryFixture(t)
			step := uuid.New()
			f.exec("INSERT INTO run_steps(id,run_id,enterprise_id,sequence,step_type,status) VALUES($1,$2,$3,1,'model_call',$4)", step, f.r, f.e, state)
			f.exec("UPDATE runs SET current_step_id=$2 WHERE id=$1", f.r, step)
			loop := Loop{Store: f.store}
			task := f.task("agent", "user_message")
			cause := runtime.Error{ErrorCode: "MODEL_COMPATIBILITY_FAILED"}
			for i := 0; i < 2; i++ {
				if err := loop.HandleExhausted(t.Context(), task, cause); err != nil {
					t.Fatal(err)
				}
			}
			run := f.run()
			if run.Status != "failed" || run.StopReason.String != "task_exhausted" {
				t.Fatalf("exhausted run not terminal: %s %s", run.Status, run.StopReason.String)
			}
			expected := state
			if state == "running" {
				expected = "failed"
			}
			if f.count("SELECT count(*) FROM run_steps WHERE id=$1 AND status=$2", step, expected) != 1 {
				t.Fatal("terminal step fact rewritten")
			}
			if f.count("SELECT count(*) FROM conversation_events WHERE run_id=$1 AND event_type='run_state_changed' AND payload->>'status'='failed'", f.r) != 1 {
				t.Fatal("terminal event duplicated or omitted")
			}
		})
	}
}
