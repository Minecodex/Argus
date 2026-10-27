package collectormanager_test

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	promengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
	prom "github.com/prometheus/prometheus/promql"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
)

func plan8FaultMetric(batch, points int) metricdata.ResourceMetrics {
	data := make([]metricdata.DataPoint[int64], points)
	at := time.Now().UTC()
	for i := range data {
		series := (batch*points + i) % 4096
		hash := sha256.Sum256([]byte(strconv.Itoa(series)))
		data[i] = metricdata.DataPoint[int64]{Attributes: attribute.NewSet(attribute.String("fault.series", strconv.Itoa(series)), attribute.String("fault.payload", fmt.Sprintf("%x", hash))), Time: at, Value: int64(batch*10000 + i)}
	}
	return metricdata.ResourceMetrics{Resource: resource.NewSchemaless(attribute.String("service.name", "plan8-storage-fault"), attribute.String("service.namespace", "acceptance"), attribute.String("service.instance.id", "one")), ScopeMetrics: []metricdata.ScopeMetrics{{Metrics: []metricdata.Metrics{{Name: "argus_plan8_storage_fault_gauge", Data: metricdata.Gauge[int64]{DataPoints: data}}}}}}
}

func plan8FaultExporter(t *testing.T, f *nativeSDKPipeline, trust *tls.Config) *otlpmetrichttp.Exporter {
	t.Helper()
	exporter, err := otlpmetrichttp.New(f.ctx, otlpmetrichttp.WithEndpoint(f.address), otlpmetrichttp.WithTLSClientConfig(trust), otlpmetrichttp.WithHeaders(map[string]string{"Authorization": "Bearer " + f.apps[0].token}), otlpmetrichttp.WithRetry(otlpmetrichttp.RetryConfig{Enabled: false}), otlpmetrichttp.WithTimeout(5*time.Second))
	if err != nil {
		t.Fatal("fault exporter initialization failed")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = exporter.Shutdown(ctx)
	})
	return exporter
}

func TestNativeQueueFilesystemFullRejectsThenRecoversAcknowledgedPoints(t *testing.T) {
	if os.Getenv("ARGUS_PLAN8_CAPACITY_PROFILE") != "local-linux-amd64-v1" {
		t.Skip("explicit owned fault profile required")
	}
	var unavailable atomic.Bool
	f := newNativeSDKProtocolPipelineWithFaults(t, "otlp", nativeSDKFaultOptions{Duration: 5 * time.Minute, ExportFailure: &unavailable, QueueFilesystemBytes: 8 << 20})
	defer func() {
		if t.Failed() {
			logs, _ := f.d.command("logs", f.container)
			for _, secret := range []string{f.private, f.apps[0].token, f.apps[1].token} {
				if secret != "" {
					logs = strings.ReplaceAll(logs, secret, "[REDACTED]")
				}
			}
			if path := os.Getenv("ARGUS_PLAN8_DISK_FAULT_EVIDENCE"); path != "" {
				_ = os.WriteFile(path+".failure-native.log", []byte(logs), 0600)
			}
		}
	}()
	exporter := plan8FaultExporter(t, f, f.sdkTLS())
	unavailable.Store(true)
	preexisting := plan8FaultMetric(80, 1000)
	if err := exporter.Export(f.ctx, &preexisting); err != nil {
		t.Fatal("could not enqueue the acknowledged pre-fault batch")
	}
	// Fill only a dedicated 8 MiB in-container filesystem. Never fill a host,
	// shared Docker volume or a platform database disk.
	if _, err := f.d.command("exec", f.container, "sh", "-c", "dd if=/dev/zero of=/var/lib/argus-otelcol/identity/queue/plan8-owned-space-fault bs=65536 count=256 2>/dev/null; test $? -ne 0"); err != nil {
		t.Fatal("owned filesystem did not reach actual ENOSPC")
	}
	available, err := f.d.command("exec", f.container, "df", "-Pk", "/var/lib/argus-otelcol/identity/queue")
	if err != nil {
		t.Fatal("actual queue filesystem capacity unavailable")
	}
	lines := strings.Split(strings.TrimSpace(available), "\n")
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 4 || fields[3] != "0" {
		t.Fatal("owned queue filesystem was not actually full")
	}
	accepted := []int{80}
	rejected := false
	for batch := 0; batch < 60; batch++ {
		data := plan8FaultMetric(batch, 1000)
		if err = exporter.Export(f.ctx, &data); err != nil {
			rejected = true
			break
		}
		accepted = append(accepted, batch)
	}
	if !rejected {
		t.Fatal("full queue filesystem never rejected a bounded SDK request")
	}
	running, err := f.d.command("inspect", "--format", "{{.State.Running}}", f.container)
	if err != nil || strings.TrimSpace(running) != "true" {
		t.Fatal("disk-full handling crashed the native Collector")
	}
	if _, err = f.d.command("exec", f.container, "rm", "-f", "/var/lib/argus-otelcol/identity/queue/plan8-owned-space-fault"); err != nil {
		t.Fatal("owned filler cleanup failed")
	}
	unavailable.Store(false)
	tables, err := f.router.Tables(f.enterprise)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(120 * time.Second)
	acknowledged := uint64(len(accepted) * 1000)
	var stored uint64
	for {
		stored = 0
		for _, batch := range accepted {
			var count uint64
			err = f.conn.QueryRow(f.ctx, "SELECT count() FROM "+tables.MetricSamples+" FINAL WHERE series_id IN(SELECT series_id FROM "+tables.MetricSeries+" FINAL WHERE resource_id=? AND metric_name='argus_plan8_storage_fault_gauge') AND value>=? AND value<?", f.apps[0].app, int64(batch*10000), int64(batch*10000+1000)).Scan(&count)
			if err != nil {
				t.Fatal("actual acknowledged point recovery query failed")
			}
			stored += count
		}
		if stored == acknowledged {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("acknowledged points did not recover after freeing queue space", stored, acknowledged)
		}
		time.Sleep(200 * time.Millisecond)
	}
	recovery := plan8FaultMetric(999, 1000)
	if err = exporter.Export(f.ctx, &recovery); err != nil {
		t.Fatal("native receiver did not recover after disk space was freed")
	}
	var fresh uint64
	for {
		err = f.conn.QueryRow(f.ctx, "SELECT count() FROM "+tables.MetricSamples+" FINAL WHERE series_id IN(SELECT series_id FROM "+tables.MetricSeries+" FINAL WHERE resource_id=? AND metric_name='argus_plan8_storage_fault_gauge') AND value>=9990000 AND value<9991000", f.apps[0].app).Scan(&fresh)
		if err != nil {
			t.Fatal(err)
		}
		if fresh == 1000 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("recovered SDK request did not reach storage")
		}
		time.Sleep(200 * time.Millisecond)
	}
	diagnostics, _ := f.d.command("logs", f.container)
	if !strings.Contains(strings.ToLower(diagnostics), "no space left on device") {
		t.Fatal("native disk-full diagnostic missing")
	}
	for _, app := range f.apps {
		if strings.Contains(diagnostics, app.token) {
			t.Fatal("native disk diagnostic leaked an application key")
		}
	}
	proof := map[string]any{"passed": true, "scope": "actual_queue_filesystem_ENOSPC_native_rejection_recovery_controlled_registration_not_full_capacity_or_persistent_restart", "filesystem_bytes": 8 << 20, "acknowledged_points": acknowledged, "recovered_acknowledged_points": stored, "new_recovery_points": fresh, "unacknowledged_request_outcome": "may_have_arrived_no_zero_claim", "collector_survived": true}
	if path := os.Getenv("ARGUS_PLAN8_DISK_FAULT_EVIDENCE"); path != "" {
		raw, _ := json.MarshalIndent(proof, "", "  ")
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNativeSDKRejectsWrongTLSIdentityAndHonorsActualQuerySeriesQuota(t *testing.T) {
	f := newNativeSDKPipeline(t)
	data := plan8FaultMetric(1, 1000)
	for _, kind := range []string{"wrong_hostname", "untrusted_root"} {
		trust := f.sdkTLS().Clone()
		if kind == "wrong_hostname" {
			trust.ServerName = "forged.application.example.invalid"
		} else {
			trust.RootCAs = x509.NewCertPool()
		}
		exporter := plan8FaultExporter(t, f, trust)
		if exporter.Export(f.ctx, &data) == nil {
			t.Fatal("SDK accepted an invalid native TLS identity", kind)
		}
	}
	valid := plan8FaultExporter(t, f, f.sdkTLS())
	if valid.Export(f.ctx, &data) != nil {
		t.Fatal("correctly pinned SDK TLS did not recover")
	}
	tables, err := f.router.Tables(f.enterprise)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	var count uint64
	for {
		err = f.conn.QueryRow(f.ctx, "SELECT count() FROM "+tables.MetricSamples+" FINAL WHERE series_id IN(SELECT series_id FROM "+tables.MetricSeries+" FINAL WHERE resource_id=? AND metric_name='argus_plan8_storage_fault_gauge')", f.apps[0].app).Scan(&count)
		if err != nil {
			t.Fatal(err)
		}
		if count == 1000 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("TLS rejection produced data, or the valid batch is missing", count)
		}
		time.Sleep(200 * time.Millisecond)
	}
	engine := promengine.NewEngine(f.conn, f.router, nil)
	now := time.Now()
	request := promengine.Request{Expression: "argus_plan8_storage_fault_gauge", Instant: true, Start: now.Add(-5 * time.Minute), End: now, Scope: promengine.Scope{EnterpriseID: f.enterprise, ResourceIDs: []uuid.UUID{f.apps[0].app}, SourceKeys: []string{f.apps[0].source + ":1"}}, MaxSamples: 5000000, MaxSeries: 1, MaxScanBytes: 256 << 20, Timeout: 10 * time.Second}
	if _, err = engine.Execute(f.ctx, request); !errors.Is(err, promengine.ErrBudget) {
		t.Fatal("actual series quota did not return its bounded failure")
	}
	request.MaxSeries = 100000
	result, err := engine.Execute(f.ctx, request)
	if err != nil {
		t.Fatal("normal frozen series allowance failed")
	}
	values, ok := result.Value.(prom.Vector)
	if !ok || len(values) != 1000 {
		t.Fatal("normal query did not recover its actual series")
	}
	proof := map[string]any{"passed": true, "scope": "actual_sdk_native_tls_identity_rejection_and_actual_query_series_budget_controlled_registration_not_ingest_rate_quota", "stored_points": count, "invalid_tls_cases": 2, "limited_series": 1, "normal_frozen_series_limit": 100000, "actual_series": len(values)}
	if path := os.Getenv("ARGUS_PLAN8_TLS_QUOTA_EVIDENCE"); path != "" {
		raw, _ := json.MarshalIndent(proof, "", "  ")
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
