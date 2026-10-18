package remoteexecution

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestIdleReclamationProtectsExecutionAndUniqueOutputs(t *testing.T) {
	for _, tc := range []struct {
		name                                   string
		active, collected, terminated, reclaim bool
	}{
		{"active execution", true, true, true, false},
		{"unique output", false, false, true, false},
		{"unknown termination", false, true, false, false},
		{"collected and idle", false, true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			v, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("pwd"))
			if err != nil {
				t.Fatal(err)
			}
			f.approve(v)
			if err = f.run(v); err != nil {
				t.Fatal(err)
			}
			step := v.Spec.Steps[0]
			f.exec("UPDATE remote_execution_generations SET last_active_at=now()-interval '2 hours' WHERE session_id=$1", step.SessionID)
			f.exec("UPDATE remote_execution_sessions SET last_active_at=now()-interval '2 hours' WHERE id=$1", step.SessionID)
			f.exec("UPDATE remote_execution_steps SET result=jsonb_set(jsonb_set(result,'{outputs_collected}',to_jsonb($2::boolean)),'{terminated}',to_jsonb($3::boolean)) WHERE request_id=$1", v.ID, tc.collected, tc.terminated)
			if tc.active {
				f.exec("UPDATE remote_execution_sessions SET active_request_id=$2 WHERE id=$1", step.SessionID, v.ID)
			}
			f.service.IdleTimeout = time.Minute
			if err = f.service.ReclaimIdle(t.Context()); err != nil {
				t.Fatal(err)
			}
			var state string
			if err = f.store.Pool.QueryRow(t.Context(), "SELECT status FROM remote_execution_generations WHERE session_id=$1 AND generation=$2", step.SessionID, step.Generation).Scan(&state); err != nil {
				t.Fatal(err)
			}
			if (state == "reclaimed") != tc.reclaim {
				t.Fatalf("reclaimed=%v, state=%s", tc.reclaim, state)
			}
			var count int
			if err = f.store.Pool.QueryRow(t.Context(), "SELECT count(*) FROM runtime_tasks WHERE enterprise_id=$1 AND payload->>'operation'='reclaim'", f.e).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if (count == 1) != tc.reclaim {
				t.Fatalf("unexpected cleanup work: %d", count)
			}
		})
	}
}

func TestIdleCandidateCannotReclaimAfterNewActivity(t *testing.T) {
	f := newFixture(t)
	v, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("pwd"))
	if err != nil {
		t.Fatal(err)
	}
	step := v.Spec.Steps[0]
	f.exec("UPDATE remote_execution_generations SET last_active_at=now()-interval '2 hours' WHERE session_id=$1", step.SessionID)
	rows, err := f.store.Queries.ListIdleRemoteGenerations(t.Context(), timeValue(time.Now().Add(-time.Minute)))
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.SessionID != step.SessionID {
			continue
		}
		f.exec("UPDATE remote_execution_sessions SET active_request_id=$2 WHERE id=$1", step.SessionID, v.ID)
		if err = f.store.InReadCommittedTx(t.Context(), func(q *db.Queries) error {
			n, e := reclaimGeneration(t.Context(), q, row)
			if e == nil && n != 0 {
				t.Error("stale idle candidate reclaimed an active session")
			}
			return e
		}); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Fatal("fixture did not create an idle candidate")
}

func TestFreshWorkerResumesPersistedCollectionWithoutRepeatingEffect(t *testing.T) {
	f := newFixture(t)
	f.transport.fileFailures.Store(1)
	v, err := f.service.Prepare(t.Context(), f.call(), "plan", f.input("first effect", "second effect"))
	if err != nil {
		t.Fatal(err)
	}
	f.approve(v)
	if err = f.run(v); err == nil {
		t.Fatal("expected first collection interruption")
	}
	if f.transport.batches.Load() != 1 {
		t.Fatal("later effect started before collection finished")
	}
	// A newly constructed service with a separate pool has no prior worker state.
	store, err := postgres.Open(context.Background(), f.store.Pool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	restarted := &Service{Store: store, Actions: f.service.Actions, Transport: f.transport}
	restarted.Actions.Store = store
	old := f.service
	f.service = restarted
	if err = f.run(v); err != nil {
		t.Fatal(err)
	}
	f.service = old
	if f.transport.batches.Load() != 2 {
		t.Fatalf("completed effect repeated: %d batches", f.transport.batches.Load())
	}
	got, err := restarted.Get(t.Context(), f.call().Principal, v.ID)
	if err != nil || got.Status != "succeeded" {
		t.Fatalf("persisted recovery: %s %v", got.Status, err)
	}
	for _, result := range got.Results {
		encoded, _ := json.Marshal(result)
		if !result.Terminated || !result.OutputsCollected {
			t.Fatalf("missing durable receipt: %s", encoded)
		}
	}
}
