package remoteexecution

import (
	"errors"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"testing"
)

func TestLostInterpreterReturnsOwnedRecoveryReferenceWithoutDispatch(t *testing.T) {
	f := newFixture(t)
	f.transport.unknown = 1
	original, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("effect"))
	if err != nil {
		t.Fatal(err)
	}
	f.approve(original)
	if err = f.run(original); err != nil {
		t.Fatal(err)
	}
	_, err = f.service.Prepare(t.Context(), f.call(), "connect", f.input("new effect"))
	var guidance toolruntime.Error
	if !errors.As(err, &guidance) || guidance.Kind != "REMOTE_CONTEXT_LOST" || guidance.Details["recovery_of_id"] != original.ID.String() || guidance.Details["requires_confirmation"] != true {
		t.Fatalf("missing safe recovery guidance: %#v %v", guidance, err)
	}
	if f.transport.batches.Load() != 1 {
		t.Fatal("guidance dispatched a new command")
	}
}
