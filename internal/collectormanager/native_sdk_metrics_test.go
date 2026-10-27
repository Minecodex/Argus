package collectormanager_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	promengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
	prom "github.com/prometheus/prometheus/promql"
)

func verifySDKMetricSum(t *testing.T, f *nativeSDKPipeline, app sdkPipelineApplication, want float64) {
	t.Helper()
	engine := promengine.NewEngine(f.conn, f.router, nil)
	deadline := time.Now().Add(20 * time.Second)
	for {
		now := time.Now().UTC()
		result, err := engine.Execute(f.ctx, promengine.Request{Expression: "sum(plan8_sdk_request_count)", Instant: true, Start: now.Add(-time.Minute), End: now.Add(time.Second), Scope: promengine.Scope{EnterpriseID: f.enterprise, ResourceIDs: []uuid.UUID{app.app}, SourceKeys: []string{app.source + ":1"}}, MaxSamples: 10000, MaxSeries: 1000, MaxScanBytes: 64 << 20, Timeout: 5 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		vector, ok := result.Value.(prom.Vector)
		if ok && len(vector) == 1 && vector[0].F == want {
			return
		}
		if time.Now().After(deadline) {
			tables, _ := f.router.Tables(f.enterprise)
			var count uint64
			var names, roots, sources []string
			_ = f.conn.QueryRow(f.ctx, "SELECT count(),groupUniqArray(metric_name),groupUniqArray(toString(resource_id)),groupUniqArray(source_key) FROM "+tables.MetricSeries).Scan(&count, &names, &roots, &sources)
			t.Logf("metric series count=%d names=%v roots=%v sources=%v expected=%s/%s", count, names, roots, sources, app.app, app.source)
			logs, _ := f.d.command("logs", "--tail", "25", f.container)
			for _, secret := range []string{f.private, f.apps[0].token, f.apps[1].token} {
				if secret != "" {
					logs = strings.ReplaceAll(logs, secret, "[REDACTED]")
				}
			}
			t.Logf("Collector metric diagnostics: %s", logs)
			t.Fatalf("actual SDK metric sum did not reach %v: %v", want, result.Value)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
