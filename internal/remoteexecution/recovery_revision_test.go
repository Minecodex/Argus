package remoteexecution

import (
	"github.com/google/uuid"
	"testing"
)

func TestRevisingRecoveryPlanIncreasesRevisionAndPreservesOrigin(t *testing.T) {
	f := newFixture(t)
	f.transport.unknown = 1
	original, err := f.service.Prepare(t.Context(), f.call(), "plan", f.input("original effect"))
	if err != nil {
		t.Fatal(err)
	}
	f.approve(original)
	if err = f.run(original); err != nil {
		t.Fatal(err)
	}
	input := f.input("inspect existing effect before recovery")
	input.RecoveryOfID = &original.ID
	recovery, err := f.service.Prepare(t.Context(), f.call(), "plan", input)
	if err != nil {
		t.Fatal(err)
	}
	if recovery.Revision != 2 {
		t.Fatalf("recovery revision=%d", recovery.Revision)
	}
	f.exec("UPDATE runs SET status='waiting_input' WHERE id=$1", f.r)
	if _, err = f.service.Feedback(t.Context(), f.call().Principal, recovery.ID, "retain original facts, revise verification", uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	nextInput := f.input("revised inspection")
	nextInput.RecoveryOfID = &original.ID
	nextInput.SupersedesID = &recovery.ID
	next, err := f.service.Prepare(t.Context(), f.call(), "plan", nextInput)
	if err != nil {
		t.Fatal(err)
	}
	if next.Revision != 3 || next.Spec.RecoveryOfID == nil || *next.Spec.RecoveryOfID != original.ID || next.Spec.SupersedesID == nil || *next.Spec.SupersedesID != recovery.ID {
		t.Fatalf("recovery lineage/revision regressed: %#v", next)
	}
	if f.transport.batches.Load() != 1 {
		t.Fatal("a recovery preview executed a command")
	}
}
