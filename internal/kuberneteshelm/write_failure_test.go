package kuberneteshelm

import (
	"errors"
	"testing"

	"github.com/kakj-go/Argus/internal/kubernetesinventory"
)

func TestRecordedGuardFailuresRetainKnownCausesAndUnknownPrecedence(t *testing.T) {
	dependency := WriteOutcome{Stage: "dependencies", State: "rejected", ErrorCode: kubernetesinventory.ErrCRDHasInstances.Error()}
	if !errors.Is(recordedWriteFailure([]WriteOutcome{dependency}), kubernetesinventory.ErrCRDHasInstances) {
		t.Fatal("Helm aggregation erased the independently observed dependency rejection")
	}
	unknown := WriteOutcome{State: "rejected", ErrorCode: ErrUnknown.Error()}
	if !errors.Is(recordedWriteFailure([]WriteOutcome{dependency, unknown}), ErrUnknown) {
		t.Fatal("a rejection hid an unknown write outcome")
	}
	for _, value := range []WriteOutcome{{State: "rejected", ErrorCode: "private-canary"}, {State: "applied", ErrorCode: ErrDenied.Error()}} {
		if recordedWriteFailure([]WriteOutcome{value}) != nil {
			t.Fatal("an untrusted or non-rejected value became a typed failure")
		}
	}
	if ErrorFromCode(ErrorCode(kubernetesinventory.ErrCRDHasInstances)) != kubernetesinventory.ErrCRDHasInstances {
		t.Fatal("the dependency cause did not survive Helm's versioned transport")
	}
}
