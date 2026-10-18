package remoteexecution

import (
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"testing"
	"time"
)

func TestDirectQueuePreservesGenerationAffinityAndNeverReplaysLostWork(t *testing.T) {
	f := newFixture(t)
	view, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("fixture"))
	if err != nil {
		t.Fatal(err)
	}
	step := view.Spec.Steps[0]
	ownerA, ownerB := uuid.NewString(), uuid.NewString()
	for _, owner := range []string{ownerA, ownerB} {
		if err = f.store.Queries.RegisterRemoteDirectWorker(t.Context(), owner); err != nil {
			t.Fatal(err)
		}
	}
	create := func() db.RemoteExecutionDirectCommand {
		t.Helper()
		id := uuid.New()
		c, e := f.store.Queries.CreateRemoteDirectCommand(t.Context(), db.CreateRemoteDirectCommandParams{ID: id, CommandID: "direct-test-" + id.String(), EnterpriseID: f.e, IdentityID: step.Identity.ID, SessionID: step.SessionID, Generation: step.Generation, CommandType: "remote_execution_batch", Payload: []byte(`{}`), PayloadHash: make([]byte, 32), ExpiresAt: timeValue(time.Now().Add(time.Minute))})
		if e != nil {
			t.Fatal(e)
		}
		return c
	}
	first := create()
	claimed, err := f.store.Queries.ClaimRemoteDirectCommands(t.Context(), db.ClaimRemoteDirectCommandsParams{OwnerInstance: ownerA, RowLimit: 1})
	if err != nil || len(claimed) != 1 || claimed[0].ID != first.ID {
		t.Fatalf("claim: %#v %v", claimed, err)
	}
	if n, e := f.store.Queries.BindRemoteDirectOwner(t.Context(), db.BindRemoteDirectOwnerParams{SessionID: step.SessionID, Generation: step.Generation, DirectOwner: ownerA}); e != nil || n != 1 {
		t.Fatal("owner not bound", e)
	}
	second := create()
	recoveryID := uuid.New()
	_, err = f.store.Queries.CreateRemoteDirectCommand(t.Context(), db.CreateRemoteDirectCommandParams{ID: recoveryID, CommandID: "direct-recovery-" + recoveryID.String(), EnterpriseID: f.e, IdentityID: step.Identity.ID, SessionID: step.SessionID, Generation: step.Generation, CommandType: "remote_execution_control", Payload: []byte(`{"operation":"recover_files"}`), PayloadHash: make([]byte, 32), ExpiresAt: timeValue(time.Now().Add(time.Minute))})
	if err != nil {
		t.Fatal(err)
	}
	if other, e := f.store.Queries.ClaimRemoteDirectCommands(t.Context(), db.ClaimRemoteDirectCommandsParams{OwnerInstance: ownerB, RowLimit: 10}); e != nil || len(other) != 0 {
		t.Fatal("different worker acquired the existing interpreter", e)
	}
	if err = f.store.Queries.StopRemoteDirectWorker(t.Context(), ownerA); err != nil {
		t.Fatal(err)
	}
	if err = f.store.Queries.RecoverLostRemoteDirectCommands(t.Context()); err != nil {
		t.Fatal(err)
	}
	a, err := f.store.Queries.GetRemoteDirectCommand(t.Context(), db.GetRemoteDirectCommandParams{ID: first.ID, EnterpriseID: f.e})
	if err != nil || a.Status != "result_unknown" || a.DispatchRejected {
		t.Fatal("in-flight work was marked safe to replay", err)
	}
	b, err := f.store.Queries.GetRemoteDirectCommand(t.Context(), db.GetRemoteDirectCommandParams{ID: second.ID, EnterpriseID: f.e})
	if err != nil || b.Status != "failed" || !b.DispatchRejected {
		t.Fatal("unsent work was not distinguished", err)
	}
	if other, e := f.store.Queries.ClaimRemoteDirectCommands(t.Context(), db.ClaimRemoteDirectCommandsParams{OwnerInstance: ownerB, RowLimit: 10}); e != nil || len(other) != 1 || other[0].ID != recoveryID {
		t.Fatal("recovery must claim only file collection, never lost commands", e)
	}
	if n, e := f.store.Queries.BindRemoteDirectOwner(t.Context(), db.BindRemoteDirectOwnerParams{SessionID: step.SessionID, Generation: step.Generation, DirectOwner: ownerB}); e != nil || n != 0 {
		t.Fatal("ordinary dispatch replaced a lost interpreter owner", e)
	}
	if n, e := f.store.Queries.BindRemoteDirectRecoveryOwner(t.Context(), db.BindRemoteDirectRecoveryOwnerParams{SessionID: step.SessionID, Generation: step.Generation, DirectOwner: ownerB}); e != nil || n != 1 {
		t.Fatal("file-only recovery could not take over the lost owner", e)
	}
	if n, e := f.store.Queries.BindRemoteDirectRecoveryOwner(t.Context(), db.BindRemoteDirectRecoveryOwnerParams{SessionID: step.SessionID, Generation: step.Generation, DirectOwner: ownerA}); e != nil || n != 0 {
		t.Fatal("live file recovery owner was displaced", e)
	}
	if n, e := f.store.Queries.CompleteRemoteDirectCommand(t.Context(), db.CompleteRemoteDirectCommandParams{ID: first.ID, EnterpriseID: f.e, OwnerInstance: ownerA, Status: "succeeded", Result: []byte(`{"receipt":"late"}`)}); e != nil || n != 1 {
		t.Fatal("original worker could not persist its late receipt", e)
	}
}
