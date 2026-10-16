package argusdev

import (
	"encoding/json"
	"os"
	"testing"
)

// The opt-in record is produced by this task's E2E run. Require the entire
// recorded desired state before invoking restoration, so this acceptance only
// exercises the read-only, already-restored path on the real cluster.
func TestRecordedFormalWorkloadRestoreAgainstCluster(t *testing.T) {
	path := os.Getenv("ARGUS_E2E_RESTORE_RECORD_PATH")
	if path == "" {
		t.Skip("requires an owned E2E restoration record")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var paused []pausedWorkload
	if err = json.Unmarshal(data, &paused); err != nil || len(paused) == 0 {
		t.Fatal("invalid restoration record", err)
	}
	kube, err := NewE2EKube("docker-desktop", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	env := &E2EEnvironment{Kube: kube, Paused: paused}
	for _, item := range paused {
		restored, err := pausedWorkloadRestored(t.Context(), env, item)
		if err != nil || !restored {
			t.Fatalf("refusing live restoration until identity and original count match for %s/%s: %v", item.Namespace, item.Name, err)
		}
	}
	if err := restoreFormalWorkloads(t.Context(), env); err != nil {
		t.Fatal(err)
	}
	for _, item := range env.Paused {
		if item.Applied {
			t.Fatal("restoration did not close the pause record")
		}
	}
	t.Logf("%d real workloads already restored, ownership checked, no scale change required", len(paused))
}
