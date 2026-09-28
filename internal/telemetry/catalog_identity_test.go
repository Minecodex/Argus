package telemetry

import "testing"

func TestTraceIdentityCandidatesUseStoredColumns(t *testing.T) {
	for _, field := range []string{"source_id", "resource_id"} {
		column, args, err := catalogFieldSQL("traces", field)
		if err != nil || column != field || len(args) != 0 {
			t.Fatalf("identity is not a stored scoped column: %s %v", column, err)
		}
	}
	if _, _, err := catalogFieldSQL("traces", "source_id OR 1=1"); err == nil {
		t.Fatal("unregistered identity expression accepted")
	}
}
