package remoteexecution

import "testing"

func TestPreviewFreezesKnownLocationVersionAndPreviousBatch(t *testing.T) {
	f := newFixture(t)
	first, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("inspect"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Spec.Steps[0].KnownDirectory != "" || first.Spec.Steps[0].InterpreterVersion != "" || first.Spec.Steps[0].PreviousRequestID != nil {
		t.Fatal("preview invented an unobserved context")
	}
	f.approve(first)
	if err = f.run(first); err != nil {
		t.Fatal(err)
	}
	secondCall := f.call()
	second, err := f.service.Prepare(t.Context(), secondCall, "plan", f.input("next", "last"))
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range second.Spec.Steps {
		if step.KnownDirectory != "/fixture" || step.InterpreterVersion != "fixture-bash" || step.PreviousRequestID == nil || *step.PreviousRequestID != first.ID || step.PreviousSequence != 1 {
			t.Fatalf("missing recorded context: %#v", step)
		}
	}
	repeated, err := f.service.Prepare(t.Context(), secondCall, "plan", f.input("next", "last"))
	if err != nil || repeated.ID != second.ID || repeated.ContentHash != second.ContentHash {
		t.Fatalf("recorded context broke preview idempotency: %v", err)
	}
	f.exec("UPDATE remote_execution_sessions SET known_directory='/changed' WHERE id=$1", first.Spec.Steps[0].SessionID)
	stored, err := f.service.Get(t.Context(), f.call().Principal, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Spec.Steps[0].KnownDirectory != "/fixture" || stored.ContentHash != second.ContentHash || stored.Status != "expired" {
		t.Fatal("preview was rewritten or accepted changed context")
	}
}
