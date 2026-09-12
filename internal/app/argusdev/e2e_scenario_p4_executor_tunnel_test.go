package argusdev

import "testing"

func TestValidateP4ExecutorTunnelRetryFacts(t *testing.T) {
	t.Parallel()

	valid := p4ExecutorTunnelRetryFacts{
		PreviousOperationID:  "11111111-1111-1111-1111-111111111111",
		RetryOf:              "11111111-1111-1111-1111-111111111111",
		PreviousConnectorID:  "22222222-2222-2222-2222-222222222222",
		CurrentConnectorID:   "33333333-3333-3333-3333-333333333333",
		PreviousTunnelID:     "44444444-4444-4444-4444-444444444444",
		CurrentTunnelID:      "55555555-5555-5555-5555-555555555555",
		PreviousStatus:       "removed",
		PreviousDropReason:   "host_onboarding_retry",
		PreviousLeaseOwner:   "",
		ActivePreviousLeases: 0,
		CurrentStatus:        "established",
		CurrentConnector:     "online",
	}
	if err := validateP4ExecutorTunnelRetryFacts(valid); err != nil {
		t.Fatalf("valid retry facts rejected: %v", err)
	}

	mutations := []struct {
		name   string
		mutate func(*p4ExecutorTunnelRetryFacts)
	}{
		{"missing retry relation", func(f *p4ExecutorTunnelRetryFacts) { f.RetryOf = "" }},
		{"reused connector", func(f *p4ExecutorTunnelRetryFacts) { f.CurrentConnectorID = f.PreviousConnectorID }},
		{"reused tunnel", func(f *p4ExecutorTunnelRetryFacts) { f.CurrentTunnelID = f.PreviousTunnelID }},
		{"previous tunnel still desired", func(f *p4ExecutorTunnelRetryFacts) { f.PreviousStatus = "established" }},
		{"wrong retirement reason", func(f *p4ExecutorTunnelRetryFacts) { f.PreviousDropReason = "network_error" }},
		{"stale owner retained", func(f *p4ExecutorTunnelRetryFacts) { f.PreviousLeaseOwner = "executor-a" }},
		{"active old lease retained", func(f *p4ExecutorTunnelRetryFacts) { f.ActivePreviousLeases = 1 }},
		{"retry tunnel unavailable", func(f *p4ExecutorTunnelRetryFacts) { f.CurrentStatus = "down" }},
		{"retry connector offline", func(f *p4ExecutorTunnelRetryFacts) { f.CurrentConnector = "offline" }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			facts := valid
			mutation.mutate(&facts)
			if err := validateP4ExecutorTunnelRetryFacts(facts); err == nil {
				t.Fatal("invalid retry facts were accepted")
			}
		})
	}
}
