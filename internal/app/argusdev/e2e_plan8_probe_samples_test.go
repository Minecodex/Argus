package argusdev

import "testing"

func TestPlan8DedicatedSampleReadinessRequiresReachableICMP(t *testing.T) {
	for _, sample := range []struct {
		name        string
		integration string
		values      []float64
		ready       bool
	}{
		{"no samples", "icmp", nil, false},
		{"unreachable peer", "icmp", []float64{100}, false},
		{"partial packet loss", "icmp", []float64{50}, false},
		{"mixed healthy and failed", "icmp", []float64{0, 100}, false},
		{"actual healthy replies", "icmp", []float64{0}, true},
		{"zero is valid for another counter", "mysql", []float64{0}, true},
	} {
		t.Run(sample.name, func(t *testing.T) {
			if plan8DedicatedSamplesReady(sample.integration, sample.values) != sample.ready {
				t.Fatal("positive fixture readiness does not match actual observations")
			}
		})
	}
}
