package remoteexecution

import (
	"testing"

	"github.com/google/uuid"
)

func TestReadProjectsChangedContextAsUnconfirmableWithoutRewritingHistory(t *testing.T) {
	for _, kind := range []string{"generation", "revision", "active", "lost"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			v, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("pwd"))
			if err != nil {
				t.Fatal(err)
			}
			session := v.Spec.Steps[0].SessionID
			switch kind {
			case "generation":
				f.exec("UPDATE remote_execution_sessions SET generation=$2 WHERE id=$1", session, uuid.New())
			case "revision":
				f.exec("UPDATE remote_execution_sessions SET context_revision=context_revision+1 WHERE id=$1", session)
			case "active":
				f.exec("UPDATE remote_execution_sessions SET active_request_id=$2 WHERE id=$1", session, v.ID)
			case "lost":
				f.exec("UPDATE remote_execution_sessions SET status='lost' WHERE id=$1", session)
			}
			got, err := f.service.Get(t.Context(), f.call().Principal, v.ID)
			if err != nil || got.Status != "expired" || got.ErrorCode != "REMOTE_CONTEXT_CHANGED" {
				t.Fatalf("stale preview appears confirmable: %s/%s %v", got.Status, got.ErrorCode, err)
			}
			var status, hash string
			if err = f.store.Pool.QueryRow(t.Context(), "SELECT status,content_hash FROM remote_execution_requests WHERE id=$1", v.ID).Scan(&status, &hash); err != nil {
				t.Fatal(err)
			}
			if status != "awaiting_confirmation" || hash != v.ContentHash || f.transport.batches.Load() != 0 {
				t.Fatal("read changed immutable history or executed a command")
			}
		})
	}
}
