package dashboard

import (
	"context"
	"errors"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type countedBudgetBackend struct{ calls atomic.Int32 }

func (b *countedBudgetBackend) ExecuteEngineQuery(_ context.Context, r queryengine.Request) (queryengine.Result, error) {
	b.calls.Add(1)
	return queryengine.Result{Language: r.Language, ResultType: "table", Data: []map[string]any{{"value": 1}}, Meta: queryengine.QueryMeta{ScannedBytes: 4, ReturnedRows: 1}}, nil
}
func (b *countedBudgetBackend) DiscoverData(_ context.Context, r telemetry.DataCatalogRequest) (telemetry.DataCatalogResult, error) {
	b.calls.Add(1)
	return telemetry.DataCatalogResult{Values: []string{"a"}, Complete: true, Membership: map[string]bool{}, Meta: queryengine.QueryMeta{ScannedBytes: 5, ReturnedRows: 1}}, nil
}
func setTestBudget(t *testing.T, f analysisFixture, cost budgetCost, calls int64) runMeter {
	t.Helper()
	q := f.store.Queries
	if err := q.EnsureDashboardRunBudget(t.Context(), db.EnsureDashboardRunBudgetParams{RunID: f.actor.RunID, EnterpriseID: f.actor.EnterpriseID, OwnerUserID: f.actor.SubjectID, ScanRemaining: cost.Scan, BytesRemaining: cost.Bytes, RowsRemaining: cost.Rows, SamplesRemaining: cost.Samples, CallsRemaining: calls}); err != nil {
		t.Fatal(err)
	}
	return runMeter{f.store, f.actor}
}
func TestPostgresRunBudgetSharesCatalogAndQueryAcrossBackendRecreation(t *testing.T) {
	f := newAnalysisFixture(t)
	f.begin(t, "检查")
	meter := setTestBudget(t, f, budgetCost{1000, 8192, 3, 100}, 20)
	backend := &countedBudgetBackend{}
	runtime := Runtime{Store: f.store, Backend: backend}
	bounded := runtime.metered(f.actor)
	request := queryengine.Request{Language: queryengine.LanguageKQL, Scope: queryengine.Scope{EnterpriseID: f.actor.EnterpriseID}, Budget: queryengine.Budget{MaxRows: 100, MaxScanBytes: 1000, MaxResultBytes: 4096, Timeout: time.Second}}
	if _, err := bounded.Backend.ExecuteEngineQuery(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	if _, err := bounded.Backend.DiscoverData(t.Context(), telemetry.DataCatalogRequest{EnterpriseID: f.actor.EnterpriseID, Kind: "values", Limit: 100, Budget: request.Budget}); err != nil {
		t.Fatal(err)
	}
	// A different Runtime instance uses the same durable Run ledger.
	if _, err := runtime.metered(f.actor).Backend.ExecuteEngineQuery(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.metered(f.actor).Backend.ExecuteEngineQuery(t.Context(), request); !errors.Is(err, telemetry.ErrQueryBudget) {
		t.Fatalf("page/restart reset cumulative budget: %v", err)
	}
	if backend.calls.Load() != 3 {
		t.Fatal("exhausted request reached telemetry backend")
	}
	view, err := f.service.RunBudget(t.Context(), f.actor)
	if err != nil || view.Remaining.Rows != 0 || view.Remaining.Scan != 987 || view.CallsRemaining != 17 {
		t.Fatalf("bad durable accounting: %+v %v", view, err)
	}
	if _, err = meter.reserve(t.Context(), budgetCost{Rows: 1}, time.Second, true); !errors.Is(err, telemetry.ErrQueryBudget) {
		t.Fatal("file proof escaped same row budget")
	}
	f.begin(t, "再次检查")
	if _, err := runtime.metered(f.actor).Backend.ExecuteEngineQuery(t.Context(), request); err != nil {
		t.Fatalf("new user Run did not get its own budget: %v", err)
	}
	if _, err = f.store.Pool.Exec(t.Context(), "UPDATE runs SET status='cancelled' WHERE id=$1", f.actor.RunID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.metered(f.actor).Backend.ExecuteEngineQuery(t.Context(), request); !errors.Is(err, ErrDenied) {
		t.Fatalf("cancelled Run queried: %v", err)
	}
}
func TestPostgresRunBudgetConcurrentReservationsNeverOverspend(t *testing.T) {
	f := newAnalysisFixture(t)
	f.begin(t, "检查")
	meter := setTestBudget(t, f, budgetCost{100, 100, 5, 100}, 20)
	var group sync.WaitGroup
	var success atomic.Int32
	for i := 0; i < 12; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			cost := budgetCost{Rows: 1}
			ticket, err := meter.reserve(t.Context(), cost, 2*time.Second, true)
			if err != nil {
				if !errors.Is(err, telemetry.ErrQueryBudget) {
					t.Errorf("reserve: %v", err)
				}
				return
			}
			success.Add(1)
			if err = meter.settle(t.Context(), ticket, &cost); err != nil {
				t.Errorf("settle: %v", err)
			}
		}()
	}
	group.Wait()
	view, err := f.service.RunBudget(t.Context(), f.actor)
	if err != nil || success.Load() != 5 || view.Remaining.Rows != 0 || view.CallsRemaining != 15 {
		t.Fatalf("overspent concurrent budget: %d %+v %v", success.Load(), view, err)
	}
}
func TestPostgresRunBudgetUnknownCompletionAndRepeatedSettlement(t *testing.T) {
	f := newAnalysisFixture(t)
	f.begin(t, "检查")
	meter := setTestBudget(t, f, budgetCost{100, 100, 10, 100}, 20)
	ticket, err := meter.reserve(t.Context(), budgetCost{Scan: 10, Bytes: 10, Rows: 4, Samples: 4}, time.Second, true)
	if err != nil {
		t.Fatal(err)
	}
	spent := budgetCost{Scan: 3, Bytes: 3, Rows: 1, Samples: 1}
	if err = meter.settle(t.Context(), ticket, &spent); err != nil {
		t.Fatal(err)
	}
	if err = meter.settle(t.Context(), ticket, &budgetCost{}); err != nil {
		t.Fatal(err)
	}
	view, _ := f.service.RunBudget(t.Context(), f.actor)
	if view.Remaining.Rows != 9 || view.Remaining.Scan != 97 {
		t.Fatal("second settlement refunded twice")
	}
	unknown, err := meter.reserve(t.Context(), budgetCost{Rows: 3}, time.Second, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = meter.settle(t.Context(), unknown, nil); err != nil {
		t.Fatal(err)
	}
	if err = meter.settle(t.Context(), unknown, &budgetCost{}); err != nil {
		t.Fatal(err)
	}
	view, _ = f.service.RunBudget(t.Context(), f.actor)
	if view.Remaining.Rows != 6 {
		t.Fatal("unknown work regained its reservation")
	}
	late, err := meter.reserve(t.Context(), budgetCost{Rows: 2}, time.Second, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Pool.Exec(t.Context(), "UPDATE dashboard_budget_reservations SET expires_at=now()-interval '1 second' WHERE id=$1", late.id); err != nil {
		t.Fatal(err)
	}
	if err = meter.settle(t.Context(), late, &budgetCost{}); err != nil {
		t.Fatal(err)
	}
	view, _ = f.service.RunBudget(t.Context(), f.actor)
	if view.Remaining.Rows != 4 {
		t.Fatal("expired reservation refunded")
	}
	pending, err := meter.reserve(t.Context(), budgetCost{Rows: 1}, time.Second, true)
	if err != nil {
		t.Fatal(err)
	}
	over, err := meter.reserve(t.Context(), budgetCost{Rows: 1}, time.Second, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = meter.settle(t.Context(), over, &budgetCost{Rows: 2}); !errors.Is(err, telemetry.ErrQueryBudget) {
		t.Fatal("over-budget backend result accepted")
	}
	if err = meter.settle(t.Context(), pending, &budgetCost{}); err != nil {
		t.Fatal(err)
	}
	if _, err = meter.reserve(t.Context(), budgetCost{Rows: 1}, time.Second, true); !errors.Is(err, telemetry.ErrQueryBudget) {
		t.Fatal("late refund reopened an exceeded Run")
	}
}
