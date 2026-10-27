package collectormanager_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
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
	metricapi "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
)

// The PKI/registration is the existing controlled component fixture. The load,
// native Collector, SASL Kafka, Writer, stored counts and queries are real.
// This fault subset does not prove full formal A62 or disk-full/quota handling.
func TestNativeSDKQueueCapacityRecoversAcceptedPointsAfterOutageAndCrash(t *testing.T) {
	if os.Getenv("ARGUS_PLAN8_CAPACITY_PROFILE") != "local-linux-amd64-v1" {
		t.Skip("explicit frozen capacity profile required")
	}
	var exportFailure atomic.Bool
	f := newNativeSDKProtocolPipelineWithFaults(t, "otlp", nativeSDKFaultOptions{Duration: 6 * time.Minute, ExportFailure: &exportFailure})
	app := f.apps[0]
	exporter, err := otlpmetrichttp.New(f.ctx, otlpmetrichttp.WithEndpoint(f.address), otlpmetrichttp.WithTLSClientConfig(f.sdkTLS()), otlpmetrichttp.WithHeaders(map[string]string{"Authorization": "Bearer " + app.token}), otlpmetrichttp.WithRetry(otlpmetrichttp.RetryConfig{Enabled: false}), otlpmetrichttp.WithTimeout(5*time.Second))
	if err != nil {
		t.Fatal("capacity exporter initialization failed")
	}
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader), sdkmetric.WithResource(resource.NewSchemaless(attribute.String("service.name", "plan8-capacity"), attribute.String("service.namespace", "acceptance"), attribute.String("service.instance.id", "load-generator"))))
	defer provider.Shutdown(f.ctx)
	defer exporter.Shutdown(f.ctx)
	var tick atomic.Int64
	_, err = provider.Meter("argus.plan8.isolated-capacity").Int64ObservableGauge("argus_plan8_capacity_gauge", metricapi.WithInt64Callback(func(_ context.Context, observer metricapi.Int64Observer) error {
		for i := 0; i < 1000; i++ {
			id := (int(tick.Load())*1000 + i) % 4096
			observer.Observe(int64(id+1), metricapi.WithAttributes(attribute.String("capacity.series", strconv.Itoa(id))))
		}
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	rss := func() uint64 {
		t.Helper()
		raw, err := f.d.command("exec", f.container, "cat", "/proc/1/status")
		if err != nil {
			t.Fatal("could not observe actual Collector RSS")
		}
		for _, line := range strings.Split(raw, "\n") {
			if strings.HasPrefix(line, "VmRSS:") {
				fields := strings.Fields(line)
				value, err := strconv.ParseUint(fields[1], 10, 64)
				if err != nil {
					t.Fatal(err)
				}
				return value * 1024
			}
		}
		t.Fatal("Collector RSS not observable")
		return 0
	}
	initialRSS := rss()
	peakRSS := initialRSS
	points := 0
	queueDiskPeak := uint64(0)
	exportFailure.Store(true)
	start := time.Now()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for i := 0; i < 60; i++ {
		<-ticker.C
		tick.Store(int64(i))
		var data metricdata.ResourceMetrics
		if err = reader.Collect(f.ctx, &data); err != nil {
			t.Fatal("actual SDK collection failed")
		}
		count := 0
		for _, scope := range data.ScopeMetrics {
			for _, metric := range scope.Metrics {
				if values, ok := metric.Data.(metricdata.Gauge[int64]); ok {
					count += len(values.DataPoints)
				}
			}
		}
		if count != 1000 {
			t.Fatal("SDK producer did not emit its frozen point count", count)
		}
		if err = exporter.Export(f.ctx, &data); err != nil {
			t.Fatal("actual capacity export failed")
		}
		points += count
		if measured := rss(); measured > peakRSS {
			peakRSS = measured
		}
		queueDisk, e := f.d.command("exec", f.container, "du", "-sk", "/var/lib/argus-otelcol/identity/queue")
		if e != nil {
			t.Fatal("actual persistent queue disk usage unavailable")
		}
		fields := strings.Fields(queueDisk)
		if len(fields) < 1 {
			t.Fatal("persistent queue disk output invalid")
		}
		kb, e := strconv.ParseUint(fields[0], 10, 64)
		if e != nil {
			t.Fatal("persistent queue disk output invalid")
		}
		if kb*1024 > queueDiskPeak {
			queueDiskPeak = kb * 1024
		}
		if queueDiskPeak > 128<<20 {
			t.Fatal("queue exceeded frozen 128 MiB disk threshold", queueDiskPeak)
		}
	}
	loadDurationMillis := time.Since(start).Milliseconds()
	if loadDurationMillis < 55000 || loadDurationMillis > 70000 {
		t.Fatal("producer did not maintain the frozen steady-load duration", loadDurationMillis)
	}
	if peakRSS > 384<<20 {
		t.Fatal("Collector exceeded frozen RSS threshold", peakRSS)
	}
	tables, err := f.router.Tables(f.enterprise)
	if err != nil {
		t.Fatal(err)
	}
	var before uint64
	if err = f.conn.QueryRow(f.ctx, "SELECT count() FROM "+tables.MetricSamples+" FINAL WHERE series_id IN(SELECT series_id FROM "+tables.MetricSeries+" FINAL WHERE resource_id=? AND metric_name='argus_plan8_capacity_gauge')", app.app).Scan(&before); err != nil || before != 0 {
		t.Fatal("outage did not isolate actual export", before, err)
	}
	if _, err = f.d.command("kill", "--signal=KILL", f.container); err != nil {
		t.Fatal("owned Collector crash injection failed")
	}
	if _, err = f.d.command("start", f.container); err != nil {
		t.Fatal("owned Collector restart failed")
	}
	exportFailure.Store(false)
	recoveryStart := time.Now()
	var stored, series uint64
	deadline := time.Now().Add(120 * time.Second)
	for {
		err = f.conn.QueryRow(f.ctx, "SELECT count(),uniqExact(series_id) FROM "+tables.MetricSamples+" FINAL WHERE series_id IN(SELECT series_id FROM "+tables.MetricSeries+" FINAL WHERE resource_id=? AND metric_name='argus_plan8_capacity_gauge')", app.app).Scan(&stored, &series)
		if err != nil {
			t.Fatal(err)
		}
		if stored == uint64(points) && series == 4096 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("actual durable point/series counts differ", stored, series, points)
		}
		time.Sleep(200 * time.Millisecond)
	}
	recoveredMillis := time.Since(recoveryStart).Milliseconds()
	engine := promengine.NewEngine(f.conn, f.router, nil)
	latencies := []int64{}
	for i := 0; i < 100; i++ {
		now := time.Now()
		result, err := engine.Execute(f.ctx, promengine.Request{Expression: "count(argus_plan8_capacity_gauge)", Instant: true, Start: now.Add(-5 * time.Minute), End: now, Scope: promengine.Scope{EnterpriseID: f.enterprise, ResourceIDs: []uuid.UUID{app.app}, SourceKeys: []string{app.source + ":1"}}, MaxSamples: 5000000, MaxSeries: 100000, MaxScanBytes: 256 << 20, Timeout: 10 * time.Second})
		latencies = append(latencies, time.Since(now).Milliseconds())
		if err != nil {
			message := err.Error()
			for _, secret := range []string{f.private, f.apps[0].token, f.apps[1].token} {
				if secret != "" {
					message = strings.ReplaceAll(message, secret, "[REDACTED]")
				}
			}
			t.Fatal("actual bounded capacity query failed", message)
		}
		values, ok := result.Value.(prom.Vector)
		if !ok || len(values) != 1 || values[0].F != 4096 {
			t.Fatal("actual capacity query lost scoped series")
		}
	}
	slices.Sort(latencies)
	p95 := latencies[94]
	if p95 > 5000 {
		t.Fatal("query exceeded frozen p95", p95)
	}
	f.exec("UPDATE monitoring_ingest_keys SET revoked_at=now(),version=version+1 WHERE id=$1", app.key)
	revokedAt := time.Now()
	var data metricdata.ResourceMetrics
	if reader.Collect(f.ctx, &data) != nil {
		t.Fatal("revocation probe SDK collection failed")
	}
	err = exporter.Export(f.ctx, &data)
	revokedMillis := time.Since(revokedAt).Milliseconds()
	if err == nil || revokedMillis > 5000 {
		t.Fatal("revocation bypassed frozen rejection window", revokedMillis)
	}
	receipt := map[string]any{"passed": true, "verification_level": "actual_sdk_native_persistent_queue_crash_recovery_controlled_registration_not_formal_A62", "profile": "local-linux-amd64-v1", "actual_points": points, "outage_stored_points": before, "persistent_queue_disk_peak_bytes": queueDiskPeak, "collector_crash": "SIGKILL", "recovered_within_ms": recoveredMillis, "stored_points": stored, "distinct_series": series, "initial_collector_rss_bytes": initialRSS, "peak_collector_rss_bytes": peakRSS, "query_p95_ms": p95, "query_count": 100, "revoke_rejection_ms": revokedMillis, "steady_duration_ms": loadDurationMillis, "duration_ms": time.Since(start).Milliseconds(), "remaining": []string{"formal_enrollment_and_kubernetes_export_batch_survival", "disk_full_behavior", "network_tls_certificate_fault", "quota_pressure", "full_formal_enrollment_load"}}
	if path := os.Getenv("ARGUS_PLAN8_QUEUE_CAPACITY_EVIDENCE"); path != "" {
		encoded, _ := json.MarshalIndent(receipt, "", "  ")
		if err = os.WriteFile(path, encoded, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Log(fmt.Sprintf("actual capacity subset: %d points/%d series, RSS=%d p95=%dms; remaining fault phases are not passed", stored, series, peakRSS, p95))
}
