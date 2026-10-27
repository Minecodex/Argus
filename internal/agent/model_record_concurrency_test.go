package agent

import (
	"bytes"
	"context"
	"encoding/hex"
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func dispatchReceiptFixture(t *testing.T) (*recoveryFixture, db.ModelCall, []byte) {
	t.Helper()
	f := newRecoveryFixture(t)
	projection, err := AssembleContext(ContextInput{ContextWindow: 32768, MaxOutput: 1024, System: modelprovider.Message{Role: "system", Content: "Use current authorized facts."}, Checkpoint: map[string]any{"remote_execution": map[string]any{"concurrent_activity": map[string]any{"schema_version": "argus.remote_activity/v1", "status": "observed", "authorization_version": int64(1), "activities": []any{map[string]any{"command_summary": []string{"owned operation"}}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	step, err := f.store.Queries.CreateRunStep(t.Context(), db.CreateRunStepParams{ID: uuid.New(), RunID: f.r, EnterpriseID: f.e, Sequence: 1, StepType: "model_call", Status: "running"})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := hex.DecodeString(projection.Hash)
	if err != nil {
		t.Fatal(err)
	}
	zero := pgtype.Numeric{Int: big.NewInt(0), Valid: true}
	call, err := f.store.Queries.CreateModelCall(t.Context(), db.CreateModelCallParams{ID: uuid.New(), EnterpriseID: f.e, RunID: f.r, StepID: step.ID, ModelID: f.m, ModelRevision: 1, CallKind: "inference", ProjectionHash: digest, InputPriceSnapshot: zero, OutputPriceSnapshot: zero})
	if err != nil {
		t.Fatal(err)
	}
	if err = recordRemoteModelContext(t.Context(), f.store.Queries, call, f.run(), projection); err != nil {
		t.Fatal(err)
	}
	return f, call, digest
}

func TestModelDispatchReceiptSurvivesConcurrentRowObservation(t *testing.T) {
	f, call, _ := dispatchReceiptFixture(t)
	bounded, stop := context.WithTimeout(t.Context(), 15*time.Second)
	defer stop()
	ctx, cancel := withTaskLease(bounded, f.store, f.task("agent", "dispatch-concurrency"))
	defer cancel()
	other, err := f.store.Pool.BeginTx(bounded, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Rollback(context.Background())
	// An unchanged value still creates a new MVCC tuple. This forces a genuine
	// PostgreSQL concurrent-row conflict without changing call or lease identity.
	if _, err = other.Exec(bounded, "UPDATE model_calls SET latency_ms=latency_ms WHERE id=$1", call.ID); err != nil {
		t.Fatal(err)
	}
	dispatchHash := bytes.Repeat([]byte{0x42}, 32)
	done := make(chan error, 1)
	go func() {
		done <- persistModelDispatch(ctx, f.store, db.MarkModelDispatchedParams{ID: call.ID, EnterpriseID: f.e, ProjectionHash: dispatchHash, CapabilitySnapshot: []byte(`{}`)})
	}()
	for {
		var blocked bool
		if err = f.store.Pool.QueryRow(bounded, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1::integer=ANY(pg_blocking_pids(pid)))", int32(other.Conn().PgConn().PID())).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("dispatch did not wait on actual row ownership: %v", err)
		case <-bounded.Done():
			t.Fatal("dispatch did not establish the expected PostgreSQL lock wait")
		case <-time.After(5 * time.Millisecond):
		}
	}
	if err = other.Commit(bounded); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if err != nil {
			t.Fatalf("live undispatched call aborted by a concurrent row observation: %v", err)
		}
	case <-bounded.Done():
		t.Fatal("dispatch did not resume after the concurrent transaction")
	}
	var status string
	var dispatched bool
	var actual, observation []byte
	if err = f.store.Pool.QueryRow(bounded, "SELECT c.status,c.dispatched_at IS NOT NULL,c.projection_hash,o.projection_hash FROM model_calls c JOIN remote_execution_context_observations o ON o.model_call_id=c.id WHERE c.id=$1", call.ID).Scan(&status, &dispatched, &actual, &observation); err != nil {
		t.Fatal(err)
	}
	if status != "running" || !dispatched || !bytes.Equal(actual, dispatchHash) || !bytes.Equal(observation, dispatchHash) {
		t.Fatal("dispatch and private provenance were not committed together")
	}
	if err = assertTaskLease(ctx); err != nil {
		t.Fatal("valid task lost its lease", err)
	}
	if f.count("SELECT count(*) FROM model_calls WHERE run_id=$1", f.r) != 1 {
		t.Fatal("concurrency created another model call")
	}
}

func TestModelDispatchReceiptRejectsReplacedTaskLease(t *testing.T) {
	f, call, original := dispatchReceiptFixture(t)
	task := f.task("agent", "dispatch-replaced")
	ctx, cancel := withTaskLease(t.Context(), f.store, task)
	defer cancel()
	f.exec("UPDATE runtime_tasks SET fence_token=fence_token+1,lease_owner='replacement-worker' WHERE id=$1", task.ID)
	if err := persistModelDispatch(ctx, f.store, db.MarkModelDispatchedParams{ID: call.ID, EnterpriseID: f.e, ProjectionHash: bytes.Repeat([]byte{0x43}, 32), CapabilitySnapshot: []byte(`{}`)}); err == nil {
		t.Fatal("replaced worker could persist dispatch")
	}
	var status string
	var dispatched bool
	var actual, observation []byte
	if err := f.store.Pool.QueryRow(t.Context(), "SELECT c.status,c.dispatched_at IS NOT NULL,c.projection_hash,o.projection_hash FROM model_calls c JOIN remote_execution_context_observations o ON o.model_call_id=c.id WHERE c.id=$1", call.ID).Scan(&status, &dispatched, &actual, &observation); err != nil {
		t.Fatal(err)
	}
	if status != "reserved" || dispatched || !bytes.Equal(actual, original) || !bytes.Equal(observation, original) {
		t.Fatal("rejected worker changed the dispatch or private provenance")
	}
}

func waitModelRecordConflict(t *testing.T, f *recoveryFixture, table string, id uuid.UUID, apply func(context.Context) error) {
	t.Helper()
	ctx, stop := context.WithTimeout(t.Context(), 15*time.Second)
	defer stop()
	if table != "model_calls" && table != "run_steps" {
		t.Fatal("unexpected test table")
	}
	other, err := f.store.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Rollback(context.Background())
	column := "latency_ms"
	if table == "run_steps" {
		column = "updated_at"
	}
	if _, err = other.Exec(ctx, "UPDATE "+table+" SET "+column+"="+column+" WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- apply(ctx) }()
	for {
		var blocked bool
		if err = f.store.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1::integer=ANY(pg_blocking_pids(pid)))", int32(other.Conn().PgConn().PID())).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("operation did not wait on actual row: %v", err)
		case <-ctx.Done():
			t.Fatal("model record lock wait absent")
		case <-time.After(5 * time.Millisecond):
		}
	}
	if err = other.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if err != nil {
			t.Fatalf("model record lost to concurrent metadata observation: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("model record failed to resume")
	}
}

func TestModelOutcomeReceiptRetainsKnownUsageDuringConcurrentObservation(t *testing.T) {
	f, call, digest := dispatchReceiptFixture(t)
	task := f.task("agent", "outcome-concurrency")
	setup, cancel := withTaskLease(t.Context(), f.store, task)
	defer cancel()
	if err := persistModelDispatch(setup, f.store, db.MarkModelDispatchedParams{ID: call.ID, EnterpriseID: f.e, ProjectionHash: digest, CapabilitySnapshot: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	var usage modelprovider.TokenUsage
	usage.Observe(modelprovider.Event{InputKnown: true, OutputKnown: true, CachedInputKnown: true, Input: 123, Output: 17, CachedInput: 50})
	zero := pgtype.Numeric{Int: big.NewInt(0), Valid: true}
	waitModelRecordConflict(t, f, "model_calls", call.ID, func(ctx context.Context) error {
		guarded, stop := withTaskLease(ctx, f.store, task)
		defer stop()
		return (Loop{Store: f.store}).finishModelCall(guarded, call, db.AiModelRevision{InputPricePerMillion: zero, OutputPricePerMillion: zero}, usage, time.Second, "stop", "succeeded", "", zero)
	})
	var status, inputSource, outputSource, cacheSource string
	var input, output, cached int64
	if err := f.store.Pool.QueryRow(t.Context(), "SELECT status,input_tokens,output_tokens,cached_input_tokens,input_usage_source,output_usage_source,cached_input_usage_source FROM model_calls WHERE id=$1", call.ID).Scan(&status, &input, &output, &cached, &inputSource, &outputSource, &cacheSource); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" || input != 123 || output != 17 || cached != 50 || inputSource != "provider" || outputSource != "provider" || cacheSource != "provider" {
		t.Fatal("known response usage was lost or reclassified")
	}
}

func TestModelStepReceiptSurvivesConcurrentObservation(t *testing.T) {
	f, call, _ := dispatchReceiptFixture(t)
	task := f.task("agent", "step-concurrency")
	waitModelRecordConflict(t, f, "run_steps", call.StepID, func(ctx context.Context) error {
		guarded, stop := withTaskLease(ctx, f.store, task)
		defer stop()
		return persistStepStatus(guarded, f.store, db.FinishRunStepParams{ID: call.StepID, EnterpriseID: f.e, Status: "succeeded"})
	})
	if f.count("SELECT count(*) FROM run_steps WHERE id=$1 AND status='succeeded'", call.StepID) != 1 {
		t.Fatal("step outcome was lost")
	}
}

func TestModelOutcomeAndStepReceiptsRejectReplacedTaskLease(t *testing.T) {
	for _, phase := range []string{"outcome", "step"} {
		t.Run(phase, func(t *testing.T) {
			f, call, _ := dispatchReceiptFixture(t)
			task := f.task("agent", phase+"-replaced")
			f.exec("UPDATE runtime_tasks SET fence_token=fence_token+1,lease_owner='replacement-worker' WHERE id=$1", task.ID)
			ctx, cancel := withTaskLease(t.Context(), f.store, task)
			defer cancel()
			var err error
			if phase == "outcome" {
				zero := pgtype.Numeric{Int: big.NewInt(0), Valid: true}
				err = (Loop{Store: f.store}).finishModelCall(ctx, call, db.AiModelRevision{InputPricePerMillion: zero, OutputPricePerMillion: zero}, modelprovider.TokenUsage{}, 0, "stop", "succeeded", "", zero)
			} else {
				err = persistStepStatus(ctx, f.store, db.FinishRunStepParams{ID: call.StepID, EnterpriseID: f.e, Status: "succeeded"})
			}
			if err == nil {
				t.Fatal("replaced worker persisted model outcome")
			}
			if f.count("SELECT count(*) FROM model_calls WHERE id=$1 AND status='reserved' AND completed_at IS NULL", call.ID) != 1 || f.count("SELECT count(*) FROM run_steps WHERE id=$1 AND status='running'", call.StepID) != 1 {
				t.Fatal("replaced worker changed persisted state")
			}
		})
	}
}
