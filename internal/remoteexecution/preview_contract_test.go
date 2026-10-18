package remoteexecution

import (
	"encoding/json"
	"testing"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestRemotePreviewPersistsArrayDiff(t *testing.T) {
	f := newFixture(t)
	v, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("pwd"))
	if err != nil {
		t.Fatal(err)
	}
	action, err := f.store.Queries.GetPendingAction(t.Context(), db.GetPendingActionParams{ActionRef: v.ActionRef, EnterpriseID: f.e})
	if err != nil {
		t.Fatal(err)
	}
	var diff []map[string]string
	if err := json.Unmarshal(action.Diff, &diff); err != nil || diff == nil {
		t.Fatalf("public pending-action diff must be an array, got %s: %v", action.Diff, err)
	}
	if len(v.Spec.Steps) != 1 || v.Spec.Steps[0].Script != "pwd" || f.transport.batches.Load() != 0 {
		t.Fatal("preview must preserve the complete command without dispatching")
	}
}
