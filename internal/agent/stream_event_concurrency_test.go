package agent

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestStreamEventsSurviveConcurrentQueryProgress(t *testing.T) {
	for _, kind := range []string{"delta", "assistant"} {
		t.Run(kind, func(t *testing.T) {
			f := newRecoveryFixture(t)
			task := f.task("agent", "inference")
			ctx, cancel := withTaskLease(t.Context(), f.store, task)
			defer cancel()
			step, err := f.store.Queries.CreateRunStep(t.Context(), db.CreateRunStepParams{ID: uuid.New(), RunID: f.r, EnterpriseID: f.e, Sequence: 1, StepType: "model_call", Status: "running"})
			if err != nil {
				t.Fatal(err)
			}
			writer, err := f.store.Pool.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Rollback(context.Background())
			if _, err = conversation.AppendEvent(t.Context(), db.New(writer), conversation.EventInput{EnterpriseID: f.e, ConversationID: f.c, RunID: uuid.NullUUID{UUID: f.r, Valid: true}, Type: "run_state_changed", ActorType: "system", Classification: "internal", Payload: map[string]any{"query_progress": "complete"}}); err != nil {
				t.Fatal(err)
			}
			loop := Loop{Store: f.store}
			run := f.run()
			done := make(chan error, 1)
			go func() {
				if kind == "delta" {
					done <- loop.persistDelta(ctx, run, step, "received once")
				} else {
					done <- loop.persistAssistant(ctx, run, step, "received once", "", modelprovider.TokenUsage{})
				}
			}()
			deadline := time.Now().Add(5 * time.Second)
			for {
				var blocked bool
				if err = f.store.Pool.QueryRow(t.Context(), "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1::integer=ANY(pg_blocking_pids(pid)))", int32(writer.Conn().PgConn().PID())).Scan(&blocked); err != nil {
					t.Fatal(err)
				}
				if blocked {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("model event did not wait on actual query progress transaction")
				}
				time.Sleep(5 * time.Millisecond)
			}
			if err = writer.Commit(t.Context()); err != nil {
				t.Fatal(err)
			}
			select {
			case err = <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("event write remained blocked")
			}
			want := 2
			if kind == "assistant" {
				want = 3
			}
			if ctx.Err() != nil || f.count("SELECT count(*) FROM conversation_events WHERE conversation_id=$1", f.c) != want || f.count("SELECT max(sequence) FROM conversation_events WHERE conversation_id=$1", f.c) != want {
				t.Fatal("event group lost, duplicated or model context cancelled")
			}
			if f.count("SELECT count(*) FROM conversation_events WHERE conversation_id=$1 AND (payload->>'delta'='received once' OR payload->>'content'='received once')", f.c) != 1 {
				t.Fatal("model content was not committed exactly once")
			}
			f.exec("UPDATE runtime_tasks SET lease_owner='replacement',fence_token=fence_token+1 WHERE id=$1", task.ID)
			if err = loop.persistDelta(ctx, run, step, "stale writer"); err == nil {
				t.Fatal("event transaction bypassed replaced task fence")
			}
			if f.count("SELECT count(*) FROM conversation_events WHERE conversation_id=$1", f.c) != want {
				t.Fatal("expired writer appended an event")
			}
		})
	}
}
