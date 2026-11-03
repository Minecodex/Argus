package dashboard

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/telemetry"
)

// Native observation and dashboard queries share the same persistent Run ledger.
// Admission to this ledger requires conversation ownership; each caller retains
// its domain-specific resource/query authorization before execution.
func MeterObservationBackend(store *postgres.Store, a telemetry.Actor, base telemetry.EngineQueryBackend) telemetry.EngineQueryBackend {
	if !a.RunID.Valid || a.SubjectType != "user" || base == nil {
		return base
	}
	return &meteredBackend{base: observationBackend{base}, meter: runMeter{store: store, actor: Actor{EnterpriseID: a.EnterpriseID, SubjectID: a.SubjectID, SubjectType: a.SubjectType, AuthorizationVersion: a.AuthorizationVersion, RunID: a.RunID.UUID}, permission: "conversation.use"}}
}

type observationBackend struct{ telemetry.EngineQueryBackend }

func (b observationBackend) DiscoverData(ctx context.Context, r telemetry.DataCatalogRequest) (telemetry.DataCatalogResult, error) {
	if catalog, ok := b.EngineQueryBackend.(telemetry.DataCatalogBackend); ok {
		return catalog.DiscoverData(ctx, r)
	}
	return telemetry.DataCatalogResult{}, telemetry.ErrQueryBackend
}

func (s Service) ObservationRead(ctx context.Context, a Actor, read func() (any, error)) (any, error) {
	if a.RunID == uuid.Nil {
		return read()
	}
	meter := runMeter{store: s.Store, actor: a, permission: "conversation.use"}
	ticket, err := meter.reserve(ctx, budgetCost{Bytes: 1 << 20, Rows: 1000}, telemetry.DefaultTimeout, true)
	if err != nil {
		return nil, err
	}
	value, err := read()
	if err != nil {
		_ = meter.settle(ctx, ticket, nil)
		return nil, err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		_ = meter.settle(ctx, ticket, nil)
		return nil, err
	}
	observed := budgetCost{Bytes: int64(len(raw)), Rows: 1}
	if err = meter.settle(ctx, ticket, &observed); err != nil {
		return nil, err
	}
	return value, nil
}
