package config

import "testing"

func TestRemoteWorkerCapacityIsBoundedAndKeepsControlSlots(t *testing.T) {
	t.Setenv("ARGUS_REMOTE_EXECUTION_WORKERS", "")
	t.Setenv("ARGUS_REMOTE_CONTROL_WORKERS", "")
	value, err := LoadRuntimeWorkers()
	if err != nil || value.RemoteExecution != 8 || value.RemoteControl != 2 {
		t.Fatalf("unexpected defaults: %+v %v", value, err)
	}
	for _, key := range []string{"ARGUS_REMOTE_EXECUTION_WORKERS", "ARGUS_REMOTE_CONTROL_WORKERS"} {
		for _, invalid := range []string{"0", "-1", "65", "invalid"} {
			t.Setenv(key, invalid)
			if _, err := LoadRuntimeWorkers(); err == nil {
				t.Fatalf("unbounded/invalid worker count accepted: %s=%s", key, invalid)
			}
		}
		t.Setenv(key, "")
	}
}
