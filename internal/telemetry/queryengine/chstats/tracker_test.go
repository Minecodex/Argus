package chstats

import (
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

func TestTrackerAccumulatesProgress(t *testing.T) {
	tracker := &Tracker{}
	tracker.record(&clickhouse.Progress{Rows: 3, Bytes: 5})
	tracker.record(&clickhouse.Progress{Rows: 7, Bytes: 11})
	if tracker.Rows() != 10 || tracker.Bytes() != 16 {
		t.Fatalf("unexpected progress rows=%d bytes=%d", tracker.Rows(), tracker.Bytes())
	}
}

func TestObservedEventTimeIsIndependentOfScanProgress(t *testing.T) {
	tracker := &Tracker{}
	tracker.record(&clickhouse.Progress{Rows: 1, Bytes: 20})
	if tracker.LatestEvent() != nil {
		t.Fatal("scan progress fabricated an event time")
	}
	tracker.ObserveEvent(time.Unix(20, 5))
	tracker.ObserveEvent(time.Unix(10, 0))
	tracker.ObserveEvent(time.Time{})
	if got := tracker.LatestEvent(); got == nil || !got.Equal(time.Unix(20, 5)) {
		t.Fatalf("lost original event time: %v", got)
	}
}
