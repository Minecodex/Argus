package argusidentity

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDeliveryLedgerClassifiesOnlyVerifiedPersistentShutdownAsPreserved(t *testing.T) {
	for _, tc := range []struct {
		name, exporter, errorType, permanent string
		persistent                           []string
		preserved                            bool
	}{
		{"durable shutdown", "otlp/argus", "Shutdown", "false", []string{"otlp/argus"}, true},
		{"memory queue shutdown", "otlp/gateway", "Shutdown", "false", []string{"otlp/argus"}, false},
		{"legacy unknown queue", "otlp/argus", "Shutdown", "false", nil, false},
		{"exhausted retries", "otlp/argus", "Unavailable", "false", []string{"otlp/argus"}, false},
		{"permanent refusal", "otlp/argus", "InvalidArgument", "true", []string{"otlp/argus"}, false},
		{"canceled request", "otlp/argus", "Canceled", "false", []string{"otlp/argus"}, false},
		{"inconsistent permanent shutdown", "otlp/argus", "Shutdown", "true", []string{"otlp/argus"}, false},
		{"missing permanence", "otlp/argus", "Shutdown", "", []string{"otlp/argus"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at := time.Now().UTC()
			path := filepath.Join(t.TempDir(), "ledger.json")
			state, err := loadDeliveryState(path, at)
			if err != nil {
				t.Fatal(err)
			}
			state.Report.TerminalFailures.MetricPoints = 17 // Preserve existing evidence.
			raw := deliveryFixture("one", "# TYPE otelcol_exporter_send_failed_metric_points counter\n"+
				"otelcol_exporter_send_failed_metric_points{exporter=\""+tc.exporter+"\",error_type=\""+tc.errorType+"\",error_permanent=\""+tc.permanent+"\"} 9\n")
			snapshot, err := readDeliverySnapshot(strings.NewReader(raw), tc.persistent...)
			if err != nil {
				t.Fatal(err)
			}
			if err = state.observe(snapshot, at); err != nil {
				t.Fatal(err)
			}
			wantTerminal, wantPreserved := int64(26), int64(0)
			if tc.preserved {
				wantTerminal, wantPreserved = 17, 9
			}
			if state.Report.TerminalFailures.MetricPoints != wantTerminal || state.PreservedShutdownFailures.MetricPoints != wantPreserved ||
				(tc.preserved && state.Report.LastFailureAt != nil) || (!tc.preserved && state.Report.LastFailureAt == nil) {
				t.Fatalf("incorrect durable shutdown classification: %+v", state)
			}
			if err = saveDeliveryState(path, state); err != nil {
				t.Fatal(err)
			}
			restored, err := loadDeliveryState(path, at)
			if err != nil {
				t.Fatal(err)
			}
			if err = restored.observe(snapshot, at.Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			if restored.Report.TerminalFailures != state.Report.TerminalFailures || restored.PreservedShutdownFailures != state.PreservedShutdownFailures {
				t.Fatal("restart recounted a native counter or erased evidence")
			}
		})
	}
}
