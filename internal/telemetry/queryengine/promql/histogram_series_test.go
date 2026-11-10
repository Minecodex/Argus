package promqlengine

import (
	"testing"
	"time"
)

func TestHistogramProjectionReusesSeriesWithoutFreezingBucketBoundaries(t *testing.T) {
	items := map[string]*series{}
	count, sum := uint64(2), 2.5
	attrs := map[string]string{"namespace": "a"}
	appendHistogramSeries(items, "id", "latency", attrs, time.Unix(10, 0), &count, &sum,
		[]uint64{1, 1, 0}, []float64{1, 2}, "latency_bucket", 20, 100)
	appendHistogramSeries(items, "id", "latency", attrs, time.Unix(20, 0), &count, &sum,
		[]uint64{1, 1, 0}, []float64{1, 3}, "latency_bucket", 20, 100)
	wanted := map[string]int{"1": 2, "2": 1, "3": 1, "+Inf": 2}
	if len(items) != len(wanted) {
		t.Fatal("bucket set changed", len(items))
	}
	for _, item := range items {
		if len(item.samples) != wanted[item.labels.Get("le")] || item.labels.Get("namespace") != "a" || item.labels.Get("__name__") != "latency_bucket" {
			t.Fatal("reused bucket lost observations or identity", item.labels, len(item.samples))
		}
	}
	if len(attrs) != 1 {
		t.Fatal("histogram expansion mutated shared source labels")
	}
}

func TestSummaryProjectionKeepsChangedQuantileSets(t *testing.T) {
	items := map[string]*series{}
	count, sum := uint64(2), 2.5
	attrs := map[string]string{"resource": "a"}
	appendSummarySeries(items, "id", "latency", attrs, time.Unix(10, 0), &count, &sum,
		[][2]float64{{.5, 1}, {.9, 2}}, "latency", 20, 100)
	appendSummarySeries(items, "id", "latency", attrs, time.Unix(20, 0), &count, &sum,
		[][2]float64{{.5, 1.2}, {.95, 2.2}}, "latency", 20, 100)
	wanted := map[string]int{"0.5": 2, "0.9": 1, "0.95": 1}
	if len(items) != len(wanted) {
		t.Fatal("quantile set changed", len(items))
	}
	for _, item := range items {
		if len(item.samples) != wanted[item.labels.Get("quantile")] || item.labels.Get("resource") != "a" {
			t.Fatal("reused quantile lost observations or identity", item.labels)
		}
	}
	if len(attrs) != 1 {
		t.Fatal("summary expansion mutated shared source labels")
	}
}
