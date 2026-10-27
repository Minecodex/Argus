package alerting

import (
	"strings"
	"testing"
	"time"

	"github.com/kakj-go/Argus/internal/telemetry"
)

func TestCollectorGapNotificationKeepsTerminalAndRetryCountsDistinct(t *testing.T) {
	instance := Instance{Severity: "warning", State: InstanceState{PolicyName: "Collector delivery data gap", Metric: "collector.delivery.observed_failures", DeliveryGap: &telemetry.CollectorDeliveryReport{TerminalFailures: telemetry.DeliveryCounts{LogRecords: 1}, ReceiverRefusals: telemetry.DeliveryCounts{LogRecords: 7}, EnqueueFailures: telemetry.DeliveryCounts{LogRecords: 2}}}}
	message, err := renderNotification(instance, "first", ChannelSpec{}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	for _, fact := range []string{"terminal failures: logs=1", "receiver refusal attempts: logs=7", "enqueue failure attempts: logs=2", "exact targets and sample timestamps unknown"} {
		if !strings.Contains(message.Text, fact) {
			t.Fatal("notification lost the meaning of native failure counters", fact)
		}
	}
	if strings.Contains(message.Text, "threshold") || strings.Contains(message.Text, "logs=10") {
		t.Fatal("control failure became a fabricated metric threshold or lost-record sum")
	}
	custom, err := renderNotification(instance, "first", ChannelSpec{Template: "{{.policy}}: {{.value}}"}, time.Now().UTC())
	if err != nil || !strings.Contains(custom.Text, "terminal failures: logs=1") {
		t.Fatal("configured templates lost control evidence", err)
	}
}
