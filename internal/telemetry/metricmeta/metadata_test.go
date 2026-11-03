package metricmeta

import (
	"fmt"
	"sync"
	"testing"
)

func TestClassificationAndBoundedConcurrentMetadataKeepConflictsVisible(t *testing.T) {
	for _, tc := range []struct {
		storage, temporality string
		monotonic            bool
		want                 string
	}{
		{"sum", "cumulative", true, "counter"},
		{"sum", "AGGREGATION_TEMPORALITY_CUMULATIVE", true, "counter"},
		{"sum", "delta", true, "gauge"},
		{"sum", "cumulative", false, "gauge"},
		{"gauge", "", false, "gauge"},
		{"histogram", "cumulative", false, "histogram"},
		{"", "", false, "unknown"},
	} {
		if got := Kind(tc.storage, tc.temporality, tc.monotonic); got != tc.want {
			t.Fatalf("%+v: %s", tc, got)
		}
	}
	tracker := &Tracker{}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			tracker.Observe(Descriptor{Name: "same_metric", StorageType: "sum", Temporality: "cumulative", Monotonic: true, Unit: "By", SourceKey: "source:1"})
			tracker.Observe(Descriptor{Name: "same_metric", StorageType: "gauge", Unit: "1", SourceKey: "source:2"})
		})
	}
	wg.Wait()
	items, complete := tracker.Snapshot()
	if !complete || len(items) != 2 || items[0].Type != "counter" || items[1].Type != "gauge" {
		t.Fatal("deduplication hid a type/unit/source conflict", items, complete)
	}
	items[0].Unit = "modified"
	if current, _ := tracker.Snapshot(); current[0].Unit != "By" {
		t.Fatal("snapshot mutated shared query metadata")
	}
	for i := 0; i < MaxDescriptors; i++ {
		tracker.Observe(Descriptor{Name: fmt.Sprintf("metric_%04d", i), StorageType: "gauge", Unit: "1"})
	}
	if items, complete := tracker.Snapshot(); complete || len(items) != MaxDescriptors {
		t.Fatal("over-limit metadata was silently complete or unbounded", len(items), complete)
	}
}
