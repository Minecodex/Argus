package argusidentity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func deliveryFixture(process, counts string) string {
	return "# TYPE target_info gauge\ntarget_info{service_instance_id=\"" + process + "\"} 1\n" + counts
}
func terminalLogMetric(value string) string {
	return "# TYPE otelcol_exporter_send_failed_log_records counter\notelcol_exporter_send_failed_log_records{exporter=\"otlp/argus\"} " + value + "\n"
}

func TestDeliveryLedgerSurvivesRestartAndSeparatesRetriedRefusals(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.json")
	at := time.Now().UTC()
	state, err := loadDeliveryState(path, at)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := readDeliverySnapshot(strings.NewReader(deliveryFixture("process-1", terminalLogMetric("3")+"# TYPE otelcol_receiver_refused_log_records counter\notelcol_receiver_refused_log_records{receiver=\"filelog/private-name\"} 12\n")))
	if err != nil {
		t.Fatal(err)
	}
	if err = state.observe(snapshot, at); err != nil {
		t.Fatal(err)
	}
	if state.Report.TerminalFailures.LogRecords != 3 || state.Report.ReceiverRefusals.LogRecords != 12 {
		t.Fatal("retried attempts became confirmed drops", state.Report)
	}
	if err = saveDeliveryState(path, state); err != nil {
		t.Fatal(err)
	}
	restored, err := loadDeliveryState(path, at)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Report.InstanceID != state.Report.InstanceID {
		t.Fatal("restart changed report identity")
	}
	if err = restored.observe(snapshot, at.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if restored.Report.TerminalFailures.LogRecords != 3 || restored.Report.ReceiverRefusals.LogRecords != 12 {
		t.Fatal("same cumulative counters duplicated")
	}
	next, err := readDeliverySnapshot(strings.NewReader(deliveryFixture("process-2", terminalLogMetric("2"))))
	if err != nil {
		t.Fatal(err)
	}
	if err = restored.observe(next, at.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if restored.Report.TerminalFailures.LogRecords != 5 || restored.Report.ReceiverRefusals.LogRecords != 12 {
		t.Fatal("new process erased or replayed historical gaps", restored.Report)
	}
	raw, _ := json.Marshal(restored.Report)
	if strings.Contains(string(raw), "private-name") || strings.Contains(string(raw), "process-2") {
		t.Fatal("component/process labels escaped into report")
	}
}

func TestDeliveryLedgerRejectsCorruptHistoryInsteadOfResettingCounts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.json")
	if err := os.WriteFile(path, []byte(`{"report":{"schema_version":"old"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadDeliveryState(path, time.Now()); err == nil {
		t.Fatal("corrupt loss evidence was silently reset")
	}
}

func TestDeliveryMetricParsingRequiresIdentityAndFiniteCounters(t *testing.T) {
	for _, value := range []string{terminalLogMetric("1"), deliveryFixture("p", terminalLogMetric("NaN")), deliveryFixture("p", terminalLogMetric("-1")), deliveryFixture("p", terminalLogMetric("1.5")), deliveryFixture("p", terminalLogMetric("9223372036854775808"))} {
		if _, err := readDeliverySnapshot(strings.NewReader(value)); err == nil {
			t.Fatal("invalid native evidence accepted")
		}
	}
}
