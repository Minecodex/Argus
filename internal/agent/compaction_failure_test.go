package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/conversation"
	modelservice "github.com/kakj-go/Argus/internal/model"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestCompactionFailurePublishesOneTerminalEvent(t *testing.T) {
	for _, mode := range []string{"no_boundary", "model_unavailable", "exhausted", "concurrent_cancel"} {
		t.Run(mode, func(t *testing.T) {
			f := newRecoveryFixture(t)
			f.exec("UPDATE runs SET status='waiting_system',stop_reason='context_compaction' WHERE id=$1", f.r)
			run := f.run()
			compactor := Compactor{Store: f.store, Models: modelservice.Service{Store: f.store}}
			if mode == "no_boundary" || mode == "model_unavailable" {
				f.exec("INSERT INTO ai_model_revisions(id,model_id,enterprise_id,revision,base_url,provider_model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,capabilities) SELECT gen_random_uuid(),id,enterprise_id,revision,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,capabilities FROM ai_models WHERE id=$1", f.m)
				f.exec(`UPDATE runs SET tool_snapshot='{"version":"argus.model_tool_set/v1","tools":[]}' WHERE id=$1`, f.r)
				scope, err := presentation.Scope(t.Context(), f.store, f.e, f.u)
				if err != nil {
					t.Fatal(err)
				}
				appendEvent := func(kind, content string) {
					_, err := conversation.AppendEvent(t.Context(), f.store.Queries, conversation.EventInput{EnterpriseID: f.e, ConversationID: f.c, RunID: uuid.NullUUID{UUID: f.r, Valid: true}, Type: kind, ActorType: "model", Payload: map[string]any{"content": content, "authorization_scope": scope}, Classification: "internal"})
					if err != nil {
						t.Fatal(err)
					}
				}
				appendEvent("user_message", strings.Repeat("x", 25000))
				if mode == "model_unavailable" {
					appendEvent("assistant_message", strings.Repeat("a", 10000))
					appendEvent("model_usage", "")
					appendEvent("user_message", "continue")
				}
				if err := compactor.Handle(t.Context(), f.task("compaction", "hard_limit")); err != nil {
					t.Fatal(err)
				}
			} else if mode == "exhausted" {
				task := f.task("compaction", "hard_limit")
				if err := compactor.HandleExhausted(t.Context(), task, errors.New("upstream failed")); err != nil {
					t.Fatal(err)
				}
			} else {
				var wg sync.WaitGroup
				failures := make(chan error, 8)
				for i := 0; i < 8; i++ {
					wg.Add(1)
					go func(i int) {
						defer wg.Done()
						if mode == "concurrent_cancel" && i%2 == 0 {
							failures <- f.store.InReadCommittedTx(t.Context(), func(q *db.Queries) error {
								current, err := q.GetRunForUpdate(t.Context(), db.GetRunForUpdateParams{ID: f.r, EnterpriseID: f.e})
								if err != nil {
									return err
								}
								_, err = conversation.CancelRunRecord(t.Context(), q, current, "user_cancelled", "user", f.u.String())
								return err
							})
						} else {
							failures <- compactor.fail(t.Context(), run, "CONTEXT_COMPACTION_FAILED")
						}
					}(i)
				}
				wg.Wait()
				close(failures)
				for err := range failures {
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			if !terminalRun(f.run().Status) {
				t.Fatal("Run did not terminate")
			}
			if n := f.count("SELECT count(*) FROM conversation_events WHERE run_id=$1 AND event_type='run_state_changed' AND payload->>'status' IN ('failed','cancelled')", f.r); n != 1 {
				t.Fatalf("terminal events=%d", n)
			}
			if err := compactor.fail(context.Background(), run, "CONTEXT_COMPACTION_FAILED"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCompletedRunStillAllowsConversationCompaction(t *testing.T) {
	f := newRecoveryFixture(t)
	f.exec("UPDATE runs SET status='succeeded' WHERE id=$1", f.r)
	for _, kind := range []string{"user_message", "assistant_message", "model_usage", "user_message"} {
		if _, err := conversation.AppendEvent(t.Context(), f.store.Queries, conversation.EventInput{EnterpriseID: f.e, ConversationID: f.c, RunID: uuid.NullUUID{UUID: f.r, Valid: true}, Type: kind, ActorType: "model", Payload: map[string]any{"content": "a complete historical turn"}, Classification: "internal"}); err != nil {
			t.Fatal(err)
		}
	}
	compactor := Compactor{Store: f.store, Models: modelservice.Service{Store: f.store}}
	for _, reason := range []string{"manual", "soft_limit", "hard_limit"} {
		task := f.task("compaction", reason)
		err := compactor.Handle(t.Context(), task)
		f.exec("UPDATE runtime_tasks SET status='succeeded',lease_owner=NULL,lease_until=NULL WHERE id=$1", task.ID)
		// The deliberately absent model must be reached for manual/soft work;
		// an old hard recovery is a no-op and cannot mutate the terminal Run.
		if (reason == "hard_limit") != (err == nil) {
			t.Fatalf("reason=%s err=%v", reason, err)
		}
	}
	if f.run().Status != "succeeded" {
		t.Fatal("background compaction changed the terminal Run")
	}
}
