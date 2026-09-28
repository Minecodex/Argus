package agent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestTaskLeaseHeartbeatConflictDoesNotInterruptLiveModel(t *testing.T) {
	f := newRecoveryFixture(t)
	task := f.task("agent", "inference")
	ctx, cancel := withTaskLease(t.Context(), f.store, task)
	defer cancel()
	heartbeat, err := f.store.Pool.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		t.Fatal(err)
	}
	defer heartbeat.Rollback(context.Background())
	if _, err := heartbeat.Exec(t.Context(), "UPDATE runtime_tasks SET lease_until=now()+interval '5 minutes' WHERE id=$1", task.ID); err != nil {
		t.Fatal(err)
	}
	var entered atomic.Int32
	step := uuid.New()
	done := make(chan error, 1)
	go func() {
		done <- fencedTransaction(ctx, f.store, func(q *db.Queries) error {
			entered.Add(1)
			_, err := q.CreateRunStep(ctx, db.CreateRunStepParams{ID: step, RunID: f.r, EnterpriseID: f.e, Sequence: 1, StepType: "model_call", Status: "running"})
			return err
		})
	}()
	// Wait for a real PostgreSQL lock wait rather than relying on scheduling.
	deadline := time.Now().Add(5 * time.Second)
	for {
		var blocked bool
		err = f.store.Pool.QueryRow(t.Context(), "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1::integer=ANY(pg_blocking_pids(pid)))", int32(heartbeat.Conn().PgConn().PID())).Scan(&blocked)
		if err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fence acquisition never waited on heartbeat")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := heartbeat.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("heartbeat contention did not recover")
	}
	if ctx.Err() != nil || entered.Load() != 1 {
		t.Fatalf("live model cancelled or callback repeated: %v/%d", ctx.Err(), entered.Load())
	}
	if f.count("SELECT count(*) FROM run_steps WHERE id=$1", step) != 1 {
		t.Fatal("fenced event was lost")
	}
	if err := assertTaskLease(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestFenceDoesNotRetryApplicationCallback(t *testing.T) {
	f := newRecoveryFixture(t)
	ctx, cancel := withTaskLease(t.Context(), f.store, f.task("agent", "inference"))
	defer cancel()
	var calls int
	want := &pgconn.PgError{Code: "40001", Message: "application conflict"}
	err := fencedTransaction(ctx, f.store, func(*db.Queries) error { calls++; return want })
	if !errors.Is(err, want) || calls != 1 || ctx.Err() != nil {
		t.Fatalf("callback retried or lease falsely cancelled: calls=%d error=%v context=%v", calls, err, ctx.Err())
	}
}
