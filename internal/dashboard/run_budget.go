package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"time"
)

type budgetCost struct {
	Scan    int64 `json:"scan_bytes"`
	Bytes   int64 `json:"result_bytes"`
	Rows    int64 `json:"rows"`
	Samples int64 `json:"samples"`
}
type RunBudgetView struct {
	Remaining      budgetCost `json:"remaining"`
	CallsRemaining int64      `json:"calls_remaining"`
	Limit          budgetCost `json:"limit"`
	CallLimit      int64      `json:"call_limit"`
}

const runCallLimit = int64(256)

func runBudgetLimit() budgetCost {
	return budgetCost{Scan: telemetry.DefaultMaxScanBytes, Bytes: 8 << 20, Rows: telemetry.DefaultMaxRows, Samples: telemetry.DefaultMaxSamples}
}

type runMeter struct {
	store *postgres.Store
	actor Actor
}
type budgetTicket struct {
	id   uuid.UUID
	cost budgetCost
}

var errBudgetBusy = errors.New("dashboard budget reserved by in-flight requests")

func (meter runMeter) authorize(ctx context.Context, q *db.Queries) error {
	if meter.actor.RunID == uuid.Nil || meter.actor.SubjectType != "user" {
		return ErrDenied
	}
	if err := authorize(ctx, q, meter.actor, "telemetry.dashboard.read"); err != nil {
		return err
	}
	run, err := q.GetRun(ctx, db.GetRunParams{ID: meter.actor.RunID, EnterpriseID: meter.actor.EnterpriseID})
	if err != nil || run.ActorUserID != meter.actor.SubjectID || run.Status == "cancelled" || run.Status == "timed_out" {
		return ErrDenied
	}
	return nil
}
func (service Service) RunBudget(ctx context.Context, actor Actor) (RunBudgetView, error) {
	meter := runMeter{service.Store, actor}
	limit := runBudgetLimit()
	view := RunBudgetView{Remaining: limit, Limit: limit, CallsRemaining: runCallLimit, CallLimit: runCallLimit}
	if err := meter.authorize(ctx, service.Store.Queries); err != nil {
		return view, err
	}
	row, err := service.Store.Queries.GetDashboardRunBudget(ctx, db.GetDashboardRunBudgetParams{RunID: actor.RunID, EnterpriseID: actor.EnterpriseID, OwnerUserID: actor.SubjectID})
	if errors.Is(err, pgx.ErrNoRows) {
		return view, nil
	}
	if err != nil {
		return view, err
	}
	view.Remaining = remainingBudget(row)
	view.CallsRemaining = row.CallsRemaining
	return view, nil
}
func remainingBudget(row db.DashboardRunBudget) budgetCost {
	return budgetCost{row.ScanRemaining, row.BytesRemaining, row.RowsRemaining, row.SamplesRemaining}
}
func (meter runMeter) save(ctx context.Context, q *db.Queries, c budgetCost, calls int64) error {
	return q.SetDashboardRunBudget(ctx, db.SetDashboardRunBudgetParams{RunID: meter.actor.RunID, EnterpriseID: meter.actor.EnterpriseID, OwnerUserID: meter.actor.SubjectID, ScanRemaining: c.Scan, BytesRemaining: c.Bytes, RowsRemaining: c.Rows, SamplesRemaining: c.Samples, CallsRemaining: calls})
}
func (meter runMeter) reserve(ctx context.Context, wanted budgetCost, ttl time.Duration, exact bool) (budgetTicket, error) {
	ticket := budgetTicket{}
	if wanted.Scan < 0 || wanted.Bytes < 0 || wanted.Rows < 0 || wanted.Samples < 0 {
		return ticket, ErrInvalid
	}
	if ttl <= 0 {
		ttl = telemetry.DefaultTimeout
	}
	waitCtx, cancel := context.WithTimeout(ctx, ttl)
	defer cancel()
	for {
		err := meter.store.InReadCommittedTx(waitCtx, func(q *db.Queries) error {
			if err := meter.authorize(waitCtx, q); err != nil {
				return err
			}
			limit := runBudgetLimit()
			if err := q.EnsureDashboardRunBudget(waitCtx, db.EnsureDashboardRunBudgetParams{RunID: meter.actor.RunID, EnterpriseID: meter.actor.EnterpriseID, OwnerUserID: meter.actor.SubjectID, ScanRemaining: limit.Scan, BytesRemaining: limit.Bytes, RowsRemaining: limit.Rows, SamplesRemaining: limit.Samples, CallsRemaining: runCallLimit}); err != nil {
				return err
			}
			row, err := q.LockDashboardRunBudget(waitCtx, db.LockDashboardRunBudgetParams{RunID: meter.actor.RunID, EnterpriseID: meter.actor.EnterpriseID, OwnerUserID: meter.actor.SubjectID})
			if err != nil {
				return err
			}
			if err = q.ExpireDashboardBudgetReservations(waitCtx, db.ExpireDashboardBudgetReservationsParams{RunID: meter.actor.RunID, EnterpriseID: meter.actor.EnterpriseID}); err != nil {
				return err
			}
			available := remainingBudget(row)
			ticket.cost = budgetCost{min(wanted.Scan, available.Scan), min(wanted.Bytes, available.Bytes), min(wanted.Rows, available.Rows), min(wanted.Samples, available.Samples)}
			if row.CallsRemaining <= 0 {
				return telemetry.ErrQueryBudget
			}
			insufficient := wanted.Scan > 0 && ticket.cost.Scan == 0 || wanted.Bytes > 0 && ticket.cost.Bytes == 0 || wanted.Rows > 0 && ticket.cost.Rows == 0 || wanted.Samples > 0 && ticket.cost.Samples == 0
			if exact && ticket.cost != wanted {
				insufficient = true
			}
			if insufficient {
				pending, e := q.HasPendingDashboardBudgetReservations(waitCtx, db.HasPendingDashboardBudgetReservationsParams{RunID: meter.actor.RunID, EnterpriseID: meter.actor.EnterpriseID})
				if e != nil {
					return e
				}
				if pending {
					return errBudgetBusy
				}
				return telemetry.ErrQueryBudget
			}
			ticket.id = uuid.New()
			raw, _ := json.Marshal(ticket.cost)
			if _, err = q.CreateDashboardBudgetReservation(waitCtx, db.CreateDashboardBudgetReservationParams{ID: ticket.id, RunID: meter.actor.RunID, EnterpriseID: meter.actor.EnterpriseID, Allocation: raw, ExpiresAt: pgtype.Timestamptz{Time: time.Now().UTC().Add(ttl + 5*time.Second), Valid: true}}); err != nil {
				return err
			}
			return meter.save(waitCtx, q, budgetCost{available.Scan - ticket.cost.Scan, available.Bytes - ticket.cost.Bytes, available.Rows - ticket.cost.Rows, available.Samples - ticket.cost.Samples}, row.CallsRemaining-1)
		})
		if !errors.Is(err, errBudgetBusy) {
			return ticket, err
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			return budgetTicket{}, telemetry.ErrQueryBudget
		case <-timer.C:
		}
	}
}
func exceeds(cost, allocation budgetCost) bool {
	return cost.Scan < 0 || cost.Bytes < 0 || cost.Rows < 0 || cost.Samples < 0 || cost.Scan > allocation.Scan || cost.Bytes > allocation.Bytes || cost.Rows > allocation.Rows || cost.Samples > allocation.Samples
}
func (meter runMeter) settle(ctx context.Context, ticket budgetTicket, observed *budgetCost) error {
	// Unknown completion, expired reservations and lost responses never refund
	// credits. Settlement is fenced by the immutable ticket and pending state.
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	over := observed != nil && exceeds(*observed, ticket.cost)
	err := meter.store.InReadCommittedTx(cleanup, func(q *db.Queries) error {
		ledger, err := q.LockDashboardRunBudget(cleanup, db.LockDashboardRunBudgetParams{RunID: meter.actor.RunID, EnterpriseID: meter.actor.EnterpriseID, OwnerUserID: meter.actor.SubjectID})
		if err != nil {
			return err
		}
		row, err := q.LockDashboardBudgetReservation(cleanup, db.LockDashboardBudgetReservationParams{ID: ticket.id, RunID: meter.actor.RunID, EnterpriseID: meter.actor.EnterpriseID})
		if err != nil {
			return err
		}
		if row.Status != "pending" {
			return nil
		}
		var allocation budgetCost
		if json.Unmarshal(row.Allocation, &allocation) != nil || allocation != ticket.cost {
			return ErrInvalid
		}
		status := "charged_unknown"
		if observed != nil && time.Now().Before(row.ExpiresAt.Time) {
			status = "settled"
			left := remainingBudget(ledger)
			calls := ledger.CallsRemaining
			if over {
				left = budgetCost{}
				calls = 0
			} else {
				left.Scan += allocation.Scan - observed.Scan
				left.Bytes += allocation.Bytes - observed.Bytes
				left.Rows += allocation.Rows - observed.Rows
				left.Samples += allocation.Samples - observed.Samples
			}
			if err = meter.save(cleanup, q, left, calls); err != nil {
				return err
			}
		}
		data, _ := json.Marshal(observed)
		return q.SetDashboardBudgetReservation(cleanup, db.SetDashboardBudgetReservationParams{ID: ticket.id, RunID: meter.actor.RunID, EnterpriseID: meter.actor.EnterpriseID, Status: status, Observed: data})
	})
	if err == nil && over {
		return telemetry.ErrQueryBudget
	}
	return err
}
func (runtime Runtime) metered(actor Actor) Runtime {
	if actor.RunID == uuid.Nil || runtime.Backend == nil {
		return runtime
	}
	if prior, ok := runtime.Backend.(*meteredBackend); ok {
		if prior.meter.actor == actor && prior.meter.store == runtime.Store {
			return runtime
		}
		runtime.Backend = prior.base
	}
	runtime.Backend = &meteredBackend{base: runtime.Backend, meter: runMeter{runtime.Store, actor}}
	return runtime
}

type meteredBackend struct {
	base  RuntimeBackend
	meter runMeter
}

func budgetRequestCost(b queryengine.Budget) budgetCost {
	return budgetCost{b.MaxScanBytes, b.MaxResultBytes, int64(b.MaxRows), int64(b.MaxSamples)}
}
func restrictBudget(original queryengine.Budget, c budgetCost) queryengine.Budget {
	original.MaxScanBytes, original.MaxResultBytes, original.MaxRows, original.MaxSamples = c.Scan, c.Bytes, int(c.Rows), int(c.Samples)
	return original
}
func observedBudget(meta queryengine.QueryMeta, value any) (budgetCost, error) {
	raw, err := json.Marshal(value)
	return budgetCost{meta.ScannedBytes, int64(len(raw)), meta.ReturnedRows, meta.LoadedSamples}, err
}
func (b *meteredBackend) ExecuteEngineQuery(ctx context.Context, r queryengine.Request) (queryengine.Result, error) {
	if r.Budget.MaxScanBytes <= 0 || r.Budget.MaxResultBytes <= 0 || r.Budget.MaxRows <= 0 || r.Budget.MaxSamples < 0 {
		return queryengine.Result{}, telemetry.ErrQueryBudget
	}
	if r.Scope.EnterpriseID != b.meter.actor.EnterpriseID {
		return queryengine.Result{}, ErrDenied
	}
	if r.Language == queryengine.LanguagePromQL && r.Budget.MaxSamples <= 0 {
		r.Budget.MaxSamples = telemetry.DefaultMaxSamples
	}
	ticket, err := b.meter.reserve(ctx, budgetRequestCost(r.Budget), r.Budget.Timeout, false)
	if err != nil {
		return queryengine.Result{}, err
	}
	r.Budget = restrictBudget(r.Budget, ticket.cost)
	result, err := b.base.ExecuteEngineQuery(ctx, r)
	if err != nil {
		_ = b.meter.settle(ctx, ticket, nil)
		return result, err
	}
	observed, e := observedBudget(result.Meta, result)
	if e != nil {
		_ = b.meter.settle(ctx, ticket, nil)
		return queryengine.Result{}, e
	}
	if err = b.meter.settle(ctx, ticket, &observed); err != nil {
		return queryengine.Result{}, err
	}
	return result, nil
}
func (b *meteredBackend) DiscoverData(ctx context.Context, r telemetry.DataCatalogRequest) (telemetry.DataCatalogResult, error) {
	if r.Budget.MaxScanBytes <= 0 || r.Budget.MaxResultBytes <= 0 || r.Budget.MaxRows <= 0 || r.Budget.MaxSamples < 0 {
		return telemetry.DataCatalogResult{}, telemetry.ErrQueryBudget
	}
	if r.EnterpriseID != b.meter.actor.EnterpriseID {
		return telemetry.DataCatalogResult{}, ErrDenied
	}
	ticket, err := b.meter.reserve(ctx, budgetRequestCost(r.Budget), r.Budget.Timeout, false)
	if err != nil {
		return telemetry.DataCatalogResult{}, err
	}
	r.Budget = restrictBudget(r.Budget, ticket.cost)
	if r.Budget.MaxRows <= len(r.SelectedValues) {
		_ = b.meter.settle(ctx, ticket, &budgetCost{})
		return telemetry.DataCatalogResult{}, telemetry.ErrQueryBudget
	}
	r.Limit = min(r.Limit, r.Budget.MaxRows-len(r.SelectedValues))
	result, err := b.base.DiscoverData(ctx, r)
	if err != nil {
		_ = b.meter.settle(ctx, ticket, nil)
		return result, err
	}
	observed, e := observedBudget(result.Meta, result)
	observed.Rows += int64(len(result.Membership))
	if e != nil {
		_ = b.meter.settle(ctx, ticket, nil)
		return telemetry.DataCatalogResult{}, e
	}
	if err = b.meter.settle(ctx, ticket, &observed); err != nil {
		return telemetry.DataCatalogResult{}, err
	}
	return result, nil
}
func chargeRunEvidence(ctx context.Context, store *postgres.Store, actor Actor, bytes, rows int64) error {
	if actor.RunID == uuid.Nil {
		return nil
	}
	meter := runMeter{store, actor}
	cost := budgetCost{Bytes: bytes, Rows: rows}
	ticket, err := meter.reserve(ctx, cost, telemetry.DefaultTimeout, true)
	if err != nil {
		return err
	}
	// A row proof verifies the complete immutable file. Reserve its known size
	// before opening any shard and charge it even if reading later fails.
	return meter.settle(ctx, ticket, &cost)
}

func (service Service) ChargeCatalog(ctx context.Context, actor Actor, value any, rows int64) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return chargeRunEvidence(ctx, service.Store, actor, int64(len(raw)), rows)
}
