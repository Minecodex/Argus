package argusdev

import "testing"

func TestCollectorFaultWindowRequiresActualSingleBlockedStart(t *testing.T) {
	for _, test := range []struct {
		status, phase string
		starts        int
		ready         bool
	}{
		{"acknowledged", "running", 0, false},
		{"running", "start-pre", 1, true},
		{"result_unknown", "start-pre", 1, true},
		{"result_unknown", "running", 1, false},
		{"succeeded", "start-pre", 1, false},
		{"failed", "start-pre", 1, false},
		{"running", "start-pre", 2, false},
	} {
		if plan8CollectorFaultReady(test.status, test.phase, test.starts) != test.ready {
			t.Fatal("fault readiness lost its physical process boundary", test)
		}
	}
}
