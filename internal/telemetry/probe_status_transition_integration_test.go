package telemetry

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	promqlengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
	"github.com/prometheus/prometheus/promql"
	"github.com/twmb/franz-go/pkg/kgo"
	collectmetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

func TestClickHouseProbeTemplatesSelectCurrentStatusAcrossDiagnosticLabels(t *testing.T) {
	conn := sourceTestClickHouse(t)
	tenant, target, source := uuid.New(), uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(t.Context(), tenant); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.DropTenant(context.Background(), tenant); err != nil {
			t.Error(err)
		}
	})
	now := time.Now().UTC().Truncate(time.Second)
	attrs := func(values map[string]string) []*commonpb.KeyValue {
		out := []*commonpb.KeyValue{}
		for k, v := range values {
			out = append(out, &commonpb.KeyValue{Key: k, Value: stringValue(v)})
		}
		return out
	}
	point := func(seconds int, value float64, labels map[string]string) *metricspb.NumberDataPoint {
		return &metricspb.NumberDataPoint{TimeUnixNano: uint64(now.Add(time.Duration(seconds) * time.Second).UnixNano()), Attributes: attrs(labels), Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: value}}
	}
	resource := func(domain string, metric string, points ...*metricspb.NumberDataPoint) *metricspb.ResourceMetrics {
		labels := map[string]string{"argus.source.id": source.String(), "argus.source.revision": "1", "argus.source.type": "http_check"}
		if domain != "" {
			labels["dns.domain"] = domain
		}
		return &metricspb.ResourceMetrics{Resource: &resourcepb.Resource{Attributes: attrs(labels)}, ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: []*metricspb.Metric{{Name: metric, Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: points}}}}}}}
	}
	payload := &collectmetrics.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{
		resource("", "httpcheck.status",
			point(-30, 1, map[string]string{"http.status_class": "2xx", "http.status_code": "200"}),
			point(-30, 0, map[string]string{"http.status_class": "5xx"}),
			point(-10, 0, map[string]string{"http.status_class": "2xx"}),
			point(-10, 1, map[string]string{"http.status_class": "5xx", "http.status_code": "503"}),
			point(-1, 1, map[string]string{"http.status_class": "2xx", "http.status_code": "200"}),
			point(-1, 0, map[string]string{"http.status_class": "5xx"})),
		resource("app.test", "dnscheck.status",
			point(-30, 1, map[string]string{"dns.rcode": "0", "dns.resolved.ip": "192.0.2.1"}),
			point(-10, 0, map[string]string{"dns.rcode": "3"}),
			point(-1, 1, map[string]string{"dns.rcode": "0", "dns.resolved.ip": "192.0.2.1"})),
		resource("other.test", "dnscheck.status", point(-1, 0, map[string]string{"dns.rcode": "3"})),
	}}
	writer := Writer{ClickHouse: conn, Router: router}
	if _, err := writer.writeMetrics(t.Context(), &kgo.Record{Topic: "owned-probe-transition", Offset: 1}, TrustedIdentity{EnterpriseID: tenant, ResourceID: target, CollectorID: uuid.New()}, payload, time.Hour); err != nil {
		t.Fatal(err)
	}
	var templates []struct {
		Key  string `json:"key"`
		Spec struct {
			Panels []struct {
				ID      string `json:"id"`
				Targets []struct {
					SourceDefinition struct {
						DSL struct {
							Expression string `json:"expression"`
						} `json:"dsl"`
					} `json:"source_definition"`
				} `json:"targets"`
			} `json:"panels"`
		} `json:"spec"`
	}
	raw, err := os.ReadFile("../../api/contracts/dashboard-templates.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &templates); err != nil {
		t.Fatal(err)
	}
	expr := func(key, panel string) string {
		for _, template := range templates {
			if template.Key == key {
				for _, p := range template.Spec.Panels {
					if p.ID == panel {
						return p.Targets[0].SourceDefinition.DSL.Expression
					}
				}
			}
		}
		t.Fatal("template missing")
		return ""
	}
	engine := promqlengine.NewEngine(conn, router, nil)
	query := func(expression string, end time.Time) promql.Vector {
		t.Helper()
		result, e := engine.Execute(t.Context(), promqlengine.Request{Expression: expression, Instant: true, Start: now.Add(-time.Minute), End: end, Scope: promqlengine.Scope{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{target}, SourceKeys: []string{source.String() + ":1"}}, MaxSeries: 100, MaxSamples: 1000, MaxScanBytes: 16 << 20, Timeout: 10 * time.Second})
		if e != nil {
			t.Fatal(e)
		}
		v, ok := result.Value.(promql.Vector)
		if !ok {
			t.Fatal("not a vector")
		}
		return v
	}
	for _, tc := range []struct {
		at             time.Time
		success, error float64
	}{{now.Add(-5 * time.Second), 0, 1}, {now, 1, 0}} {
		for panel, want := range map[string]float64{"success": tc.success, "server_error": tc.error} {
			v := query(expr("http.overview", panel), tc.at)
			if len(v) != 1 || v[0].F != want {
				t.Fatalf("HTTP %s got %v want %v", panel, v, want)
			}
		}
	}
	dns := query(expr("dns.overview", "status"), now)
	if len(dns) != 2 {
		t.Fatalf("DNS domains collapsed: %v", dns)
	}
	for _, sample := range dns {
		want := float64(0)
		if sample.Metric.Get("resource_dns_domain") == "app.test" {
			want = 1
		}
		if sample.F != want {
			t.Fatalf("DNS retained an obsolete status: %v", dns)
		}
	}
}
