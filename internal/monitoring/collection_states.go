package monitoring

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// CollectionState deliberately excludes configuration, endpoints and credentials.
type CollectionState struct {
	ID                    uuid.UUID  `json:"id"`
	Name                  string     `json:"name"`
	Integration           string     `json:"integration"`
	ConfiguredCollectorID *uuid.UUID `json:"configured_collector_id,omitempty"`
	EffectiveCollectorID  *uuid.UUID `json:"effective_collector_id,omitempty"`
	PlacementState        string     `json:"placement_state"`
	DesiredEnabled        bool       `json:"desired_enabled"`
	DesiredRevision       int64      `json:"desired_revision"`
	EffectiveRevision     int64      `json:"effective_revision"`
	ApplyStatus           string     `json:"apply_status"`
	Health                string     `json:"health"`
	Freshness             string     `json:"freshness"`
	ObservedAt            *time.Time `json:"observed_at,omitempty"`
	LastSampleAt          *time.Time `json:"last_sample_at,omitempty"`
}

func (s Service) CollectionStates(ctx context.Context, a Actor, target ResourceRef, after uuid.UUID, limit int) ([]CollectionState, bool, error) {
	if limit < 1 || limit > 100 {
		return nil, false, ErrInvalid
	}
	if err := authorization.Authorize(ctx, s.Store.Queries, a, "monitoring.read"); err != nil {
		return nil, false, err
	}
	if err := authorization.RequireResource(ctx, s.Store.Queries, a, target.Kind, target.ID); err != nil {
		return nil, false, err
	}
	rows, err := s.Store.Queries.ListObservationCollectionStates(ctx, db.ListObservationCollectionStatesParams{EnterpriseID: a.EnterpriseID, TargetType: target.Kind, TargetID: target.ID, AfterID: after, PageLimit: int32(limit + 1)})
	if err != nil {
		return nil, false, err
	}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	out := []CollectionState{}
	for _, row := range rows {
		v := collectionDTO(row, s.now())
		state := CollectionState{ID: v.ID, Name: v.Name, Integration: v.Integration, EffectiveCollectorID: v.EffectiveCollectorID, PlacementState: "unassigned", DesiredEnabled: v.DesiredEnabled, DesiredRevision: v.DesiredRevision, EffectiveRevision: v.EffectiveRevision, ApplyStatus: v.ApplyStatus, Health: v.Health, Freshness: v.Freshness, ObservedAt: v.ObservedAt, LastSampleAt: v.LastSampleAt}
		if row.CollectorID.Valid {
			state.ConfiguredCollectorID = &v.CollectorID
		}
		if state.ConfiguredCollectorID != nil || state.EffectiveCollectorID != nil {
			state.PlacementState = "requires_authorized_resolution"
		}
		out = append(out, state)
	}
	if err := authorization.RequireResource(ctx, s.Store.Queries, a, target.Kind, target.ID); err != nil {
		return nil, false, err
	}
	return out, more, authorization.Authorize(ctx, s.Store.Queries, a, "monitoring.read")
}
