package agent

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/storage/postgres"
)

func TestRunEndpointsEnforceOwnerEnterpriseAndDeletion(t *testing.T) {
	f := newRecoveryFixture(t)
	other := uuid.New()
	f.exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Other')", other, f.e, f.d, "other-"+other.String())
	service := conversation.Service{Store: f.store, Idempotency: postgres.Idempotency{Key: bytes.Repeat([]byte{7}, 32)}}
	for _, actor := range []struct{ enterprise, user uuid.UUID }{{f.e, other}, {uuid.New(), f.u}} {
		if _, err := service.GetRun(t.Context(), actor.enterprise, actor.user, f.r); err == nil {
			t.Fatal("foreign Run read allowed")
		}
		if _, err := service.CancelRun(t.Context(), actor.user.String(), actor.enterprise, f.r, uuid.NewString()); err == nil {
			t.Fatal("foreign cancellation allowed")
		}
		if _, err := service.RequestCompaction(t.Context(), actor.user.String(), actor.enterprise, f.r, uuid.NewString()); err == nil {
			t.Fatal("foreign compaction allowed")
		}
	}
	if n := f.count("SELECT count(*) FROM runtime_tasks WHERE run_id=$1", f.r); n != 0 {
		t.Fatal("denied request created work")
	}
	if _, err := service.GetRun(t.Context(), f.e, f.u, f.r); err != nil {
		t.Fatal(err)
	}
	key := uuid.NewString()
	for i := 0; i < 2; i++ {
		if _, err := service.RequestCompaction(t.Context(), f.u.String(), f.e, f.r, key); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.count("SELECT count(*) FROM runtime_tasks WHERE run_id=$1", f.r); n != 1 {
		t.Fatal("owner compaction not idempotent")
	}
	if _, err := service.Delete(t.Context(), f.e, f.u, f.c, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetRun(t.Context(), f.e, f.u, f.r); err == nil {
		t.Fatal("deleted conversation Run visible")
	}
	if _, err := service.RequestCompaction(t.Context(), f.u.String(), f.e, f.r, key); err == nil {
		t.Fatal("idempotency replay bypassed deletion")
	}
	if _, err := service.CancelRun(t.Context(), f.u.String(), f.e, f.r, uuid.NewString()); err == nil {
		t.Fatal("deleted conversation Run mutation allowed")
	}
	if n := f.count("SELECT count(*) FROM runtime_tasks WHERE run_id=$1", f.r); n != 1 {
		t.Fatal("deleted request queued new work")
	}
}
