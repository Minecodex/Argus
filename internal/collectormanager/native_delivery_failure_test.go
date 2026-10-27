package collectormanager_test

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/alerting"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/testsupport/postgrestest"
)

func TestNativeFiveMinuteExportFailurePersistsGapAcrossRestartAndControlOutage(t *testing.T) {
	if os.Getenv("ARGUS_MONITORING_NATIVE_PIPELINE") != "1" {
		t.Skip("requires owned native pipeline and full five-minute fault window")
	}
	migrations := os.Getenv("ARGUS_PLAN8_DELIVERY_MIGRATIONS")
	if migrations == "" {
		migrations = filepath.Join("..", "..", "migrations", "postgresql")
	}
	isolated := postgrestest.New(t, "ARGUS_MONITORING_TEST_DATABASE_URL", "native_delivery", migrations)
	t.Setenv("ARGUS_MONITORING_TEST_DATABASE_URL", isolated.Pool.Config().ConnString())
	var exportFailure, reportFailure atomic.Bool
	f := newNativeSDKProtocolPipelineWithFaults(t, "otlp", nativeSDKFaultOptions{Duration: 10 * time.Minute, Migrations: migrations, ExportFailure: &exportFailure, ReportFailure: &reportFailure})
	service := newActualSDKService(t, f, f.apps[0], "delivery-outage")
	before := "plan8-delivery-before-" + uuid.NewString()
	service.log(f.ctx, before, "")
	if err := service.logExport.failure(); err != nil {
		t.Fatal("real SDK baseline failed", err)
	}
	f.waitRows(0, 1, 0, 0)
	var beforeLedger struct {
		Report telemetry.CollectorDeliveryReport `json:"report"`
	}
	ledgerPath := filepath.Join(f.d.directory, "delivery-state.json")
	waitNativeDelivery(t, f.ctx, 30*time.Second, func() bool {
		raw, err := os.ReadFile(ledgerPath)
		return err == nil && json.Unmarshal(raw, &beforeLedger) == nil && beforeLedger.Report.Sequence > 0
	})
	t.Log("native baseline and durable control ledger ready")
	reportFailure.Store(true)
	exportFailure.Store(true)
	started := time.Now()
	lost := "plan8-delivery-lost-" + uuid.NewString()
	service.log(f.ctx, lost, "")
	if err := service.logExport.failure(); err != nil {
		t.Fatal("real SDK log was not accepted into the native queue", err)
	}
	// Keep the real failure active for a full five minutes. Configuration is
	// never shortened to make this check faster; the native backoff can stop
	// before its upper bound when the next delay would exceed the window.
	if err := nativeDeliveryWait(f.ctx, 301*time.Second); err != nil {
		t.Fatal(err)
	}
	var failedLedger struct {
		Report telemetry.CollectorDeliveryReport `json:"report"`
	}
	waitNativeDelivery(t, f.ctx, 35*time.Second, func() bool {
		raw, err := os.ReadFile(ledgerPath)
		return err == nil && json.Unmarshal(raw, &failedLedger) == nil && failedLedger.Report.TerminalFailures.LogRecords == 1
	})
	if failedLedger.Report.RetryLimitSeconds != 300 || failedLedger.Report.ReceiverRefusals.LogRecords != 0 || failedLedger.Report.TerminalFailures.Spans != 0 || failedLedger.Report.TerminalFailures.MetricPoints != 0 {
		t.Fatal("terminal loss was confused with retried refusal counts", failedLedger.Report)
	}
	t.Log("full five-minute outage observed; native ledger recorded one terminal log failure")
	var acknowledged int64
	if err := f.store.Pool.QueryRow(f.ctx, "SELECT count(*) FROM collector_delivery_reports WHERE enterprise_id=$1 AND (report->'terminal_failures'->>'log_records')::bigint>0", f.enterprise).Scan(&acknowledged); err != nil || acknowledged != 0 {
		t.Fatal("control outage was bypassed", acknowledged, err)
	}
	// The native ledger and persistent queue use the real mounted directory.
	// Restart does not reconstruct a report from expected values in this test.
	f.d.must("restart", f.container)
	exportFailure.Store(false)
	reportFailure.Store(false)
	waitNativeDelivery(t, f.ctx, 45*time.Second, func() bool {
		rows, err := f.store.Queries.ListCollectorDeliveryReports(f.ctx, db.ListCollectorDeliveryReportsParams{EnterpriseID: f.enterprise, CollectorID: f.collector})
		if err != nil || len(rows) != 1 {
			return false
		}
		var report telemetry.CollectorDeliveryReport
		return json.Unmarshal(rows[0].Report, &report) == nil && report.InstanceID == failedLedger.Report.InstanceID && report.TerminalFailures.LogRecords == 1
	})
	f.address = f.d.port(f.container, "24318")
	waitNativeDelivery(t, f.ctx, 20*time.Second, func() bool {
		connection, err := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", f.address, f.sdkTLS())
		if err != nil {
			return false
		}
		connection.Close()
		return true
	})
	recoveredService := newActualSDKService(t, f, f.apps[0], "delivery-outage")
	recovered := "plan8-delivery-after-" + uuid.NewString()
	recoveredService.log(f.ctx, recovered, "")
	if err := recoveredService.logExport.failure(); err != nil {
		t.Fatal("native pipeline did not recover", err)
	}
	f.waitRows(0, 2, 0, 0)
	tables, _ := f.router.Tables(f.enterprise)
	var lostRows uint64
	if err := f.conn.QueryRow(f.ctx, "SELECT count(*) FROM "+tables.Logs+" WHERE resource_id=? AND body=?", f.apps[0].app, lost).Scan(&lostRows); err != nil || lostRows != 0 {
		t.Fatal("expired batch was silently replayed", lostRows, err)
	}
	alerts := alerting.Service{Store: f.store}
	if err := alerts.EnqueueCollectorDeliveryAlerts(f.ctx); err != nil {
		t.Fatal(err)
	}
	workerCtx, cancel := context.WithCancel(f.ctx)
	done := make(chan error, 1)
	go func() {
		done <- (runtime.Processor{Store: f.store, Queue: telemetry.CollectorDeliveryQueue, Owner: "native-delivery", Handle: alerting.DeliveryGapWorker{Service: alerts}}).Run(workerCtx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(10 * time.Second):
			t.Error("delivery alert worker did not stop")
		}
	})
	var alertID uuid.UUID
	waitNativeDelivery(t, f.ctx, 30*time.Second, func() bool {
		return f.store.Pool.QueryRow(f.ctx, "SELECT a.id FROM alert_instances a JOIN collector_delivery_policies p ON p.policy_id=a.policy_id WHERE p.collector_id=$1 AND a.lifecycle='firing' AND (a.state->'delivery_gap'->'terminal_failures'->>'log_records')::bigint=1", f.collector).Scan(&alertID) == nil
	})
	binary, err := os.ReadFile(os.Getenv("ARGUS_MONITORING_COLLECTOR_BINARY"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(binary)
	proof := map[string]any{"passed": true, "scope": "native_sdk_collector_mtls_kafka_writer_clickhouse_and_actual_pg_alert_worker_controlled_registration_not_formal_enrollment", "collector_id": f.collector, "collector_binary_sha256": hex.EncodeToString(digest[:]), "fault_duration_ms": time.Since(started).Milliseconds(), "max_elapsed_time": "5m", "expired_log_records_observed": 1, "lost_marker_persisted_rows": lostRows, "recovered_total_log_rows": 2, "report_persisted_before_restart": failedLedger.Report, "same_reporter_identity_after_restart": true, "control_outage_verified": true, "alert_instance_id": alertID, "synthetic_telemetry_injection": false}
	if destination := os.Getenv("ARGUS_PLAN8_DELIVERY_EVIDENCE"); destination != "" {
		raw, _ := json.MarshalIndent(proof, "", "  ")
		if err = os.WriteFile(destination, append(raw, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func waitNativeDelivery(t *testing.T, ctx context.Context, budget time.Duration, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(budget)
	for {
		if check() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("native delivery observation did not reach the required state")
		}
		if err := nativeDeliveryWait(ctx, 300*time.Millisecond); err != nil {
			t.Fatal(err)
		}
	}
}
func nativeDeliveryWait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
