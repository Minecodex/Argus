package dashboard

import (
	"context"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"testing"
)

func TestExecutionBudgetChargesActualWorkAcrossUnequalPanels(t *testing.T) {
	b := newExecutionBudget(6)
	// A first heavy process query and a later I/O query both fit the same 256 MiB run.
	for _, mib := range []int64{72, 8, 8, 36, 108, 18} {
		allocation, err := b.next(context.Background())
		if err != nil || allocation.MaxScanBytes < mib<<20 {
			t.Fatalf("affordable panel rejected: %d MiB, allowance %d, err %v", mib, allocation.MaxScanBytes, err)
		}
		b.record(queryengine.QueryMeta{ScannedBytes: mib << 20, ReturnedRows: 1, LoadedSamples: 1}, nil)
	}
	if b.scan != telemetry.DefaultMaxScanBytes-(250<<20) {
		t.Fatal("run usage was not charged cumulatively")
	}
	a, err := b.next(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b.failed(a)
	if _, err = b.next(context.Background()); err == nil {
		t.Fatal("failed work allowance was reused")
	}
}
