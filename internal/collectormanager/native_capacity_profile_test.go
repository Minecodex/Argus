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
// This is only the SDK load subset, never the complete A62 fault gate.
func TestNativeSDKCapacityProfileStoresMeasuredSeriesAndHonorsRevocation(t *testing.T) {
	if os.Getenv("ARGUS_PLAN8_CAPACITY_PROFILE") != "local-linux-amd64-v1" {
		t.Skip("explicit frozen capacity profile required")
	}
	f := newNativeSDKPipeline(t)
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
	var stored, series uint64
	deadline := time.Now().Add(30 * time.Second)
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
	receipt := map[string]any{"passed": true, "verification_level": "real_sdk_component_pipeline_controlled_registration_not_full_A62", "profile": "local-linux-amd64-v1", "actual_points": points, "stored_points": stored, "distinct_series": series, "initial_collector_rss_bytes": initialRSS, "peak_collector_rss_bytes": peakRSS, "query_p95_ms": p95, "query_count": 100, "revoke_rejection_ms": revokedMillis, "steady_duration_ms": loadDurationMillis, "duration_ms": time.Since(start).Milliseconds(), "remaining": []string{"32_dedicated_targets", "disk_queue_capacity", "network_tls_outage_and_recovery", "quota_pressure", "full_formal_enrollment_load"}}
	if path := os.Getenv("ARGUS_PLAN8_CAPACITY_EVIDENCE"); path != "" {
		encoded, _ := json.MarshalIndent(receipt, "", "  ")
		if err = os.WriteFile(path, encoded, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Log(fmt.Sprintf("actual capacity subset: %d points/%d series, RSS=%d p95=%dms; remaining fault phases are not passed", stored, series, peakRSS, p95))
}
