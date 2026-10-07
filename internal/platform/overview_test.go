package platform

import (
	"testing"
	"time"
)

func TestOverviewPeriodUsesUTCMonthsAtYearBoundary(t *testing.T) {
	at := time.Date(2026, 1, 1, 2, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	from, to := overviewPeriod(at)
	if from.Format("2006-01-02") != "2025-01-01" || to.Format("2006-01-02") != "2026-01-01" {
		t.Fatalf("unexpected UTC monthly interval: %v..%v", from, to)
	}
}
