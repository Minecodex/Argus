package telemetry

import (
	"testing"
	"time"

	"github.com/google/uuid"
	rpc "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	common "go.opentelemetry.io/proto/otlp/common/v1"
	metrics "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/proto"
)

func TestCollectionScrapeDiagnosticsDoNotFabricateFreshTargetData(t *testing.T) {
	id := uuid.New()
	at := time.Now().UTC().Truncate(time.Second)
	metric := func(name string, value float64, flags uint32) *metrics.Metric {
		return &metrics.Metric{Name: name, Data: &metrics.Metric_Gauge{Gauge: &metrics.Gauge{DataPoints: []*metrics.NumberDataPoint{{TimeUnixNano: uint64(at.UnixNano()), Value: &metrics.NumberDataPoint_AsDouble{AsDouble: value}, Flags: flags}}}}}
	}
	block := func(source string, values ...*metrics.Metric) *metrics.ResourceMetrics {
		return &metrics.ResourceMetrics{Resource: &resourcepb.Resource{Attributes: []*common.KeyValue{{Key: "argus.collection.id", Value: stringValue(id.String())}, {Key: "argus.collection.revision", Value: stringValue("1")}, {Key: "argus.assignment.epoch", Value: stringValue("2")}, {Key: "argus.source.type", Value: stringValue(source)}}}, ScopeMetrics: []*metrics.ScopeMetrics{{Metrics: values}}}
	}
	for _, tc := range []struct {
		name, health string
		blocks       []*metrics.ResourceMetrics
		hasData      bool
		count        int
	}{
		{"failed", "failed", []*metrics.ResourceMetrics{block("kubernetes_apiserver", metric("up", 0, 0), metric("scrape_samples_scraped", 0, 0))}, false, 1},
		{"scheduler unavailable", "failed", []*metrics.ResourceMetrics{block("kubernetes_scheduler", metric("up", 0, 0))}, false, 1},
		{"controller unavailable", "failed", []*metrics.ResourceMetrics{block("kubernetes_controller_manager", metric("up", 0, 0))}, false, 1},
		{"etcd unavailable", "failed", []*metrics.ResourceMetrics{block("kubernetes_etcd", metric("up", 0, 0))}, false, 1},
		{"self unavailable", "failed", []*metrics.ResourceMetrics{block("collector_self", metric("up", 0, 0))}, false, 1},
		{"healthy", "healthy", []*metrics.ResourceMetrics{block("kubernetes_apiserver", metric("up", 1, 0), metric("apiserver_request_total", 5, 0))}, true, 1},
		{"partial across targets", "degraded", []*metrics.ResourceMetrics{block("kubernetes_coredns", metric("up", 1, 0)), block("kubernetes_coredns", metric("up", 0, 0))}, false, 1},
		{"stale marker", "", []*metrics.ResourceMetrics{block("kubernetes_apiserver", metric("up", 0, 1))}, false, 0},
		{"empty resource", "", []*metrics.ResourceMetrics{block("kubernetes_apiserver")}, false, 0},
		{"empty other resource", "", []*metrics.ResourceMetrics{block("mysql")}, false, 0},
		{"ordinary metric named up", "healthy", []*metrics.ResourceMetrics{block("mysql", metric("up", 0, 0))}, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := proto.Marshal(&rpc.ExportMetricsServiceRequest{ResourceMetrics: tc.blocks})
			if err != nil {
				t.Fatal(err)
			}
			samples := acceptedCollectionSamples("metrics", raw)
			if len(samples) != tc.count {
				t.Fatalf("observations %d want %d", len(samples), tc.count)
			}
			if tc.count == 0 {
				return
			}
			sample := samples[0]
			if sample.Health != tc.health || sample.HasData != tc.hasData || sample.Epoch != 2 {
				t.Fatalf("observation: %+v", sample)
			}
			if tc.health == "failed" && sample.DataAt != nil {
				t.Fatal("diagnostic-only failure made a target sample")
			}
			if tc.name == "healthy" && (sample.DataAt == nil || !sample.DataAt.Equal(at)) {
				t.Fatal("actual sample timestamp lost")
			}
		})
	}
}
