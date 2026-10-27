package monitoring

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type Collection struct {
	ID                       uuid.UUID       `json:"id"`
	Name                     string          `json:"name"`
	Integration              string          `json:"integration"`
	Target                   ResourceRef     `json:"target"`
	CollectorID              uuid.UUID       `json:"collector_id"`
	EffectiveCollectorID     *uuid.UUID      `json:"effective_collector_id,omitempty"`
	EffectiveAssignmentEpoch int64           `json:"effective_assignment_epoch"`
	ApplicationID            *uuid.UUID      `json:"application_id,omitempty"`
	ServiceID                *uuid.UUID      `json:"service_id,omitempty"`
	Config                   json.RawMessage `json:"config"`
	CredentialConfigured     bool            `json:"credential_configured"`
	DesiredEnabled           bool            `json:"desired_enabled"`
	DesiredRevision          int64           `json:"desired_revision"`
	EffectiveRevision        int64           `json:"effective_revision"`
	AssignmentEpoch          int64           `json:"assignment_epoch"`
	ApplyStatus              string          `json:"apply_status"`
	Health                   string          `json:"health"`
	Freshness                string          `json:"freshness"`
	LastAttemptAt            *time.Time      `json:"last_attempt_at,omitempty"`
	LastSampleAt             *time.Time      `json:"last_sample_at,omitempty"`
	ObservedAt               *time.Time      `json:"observed_at,omitempty"`
	ErrorCode                string          `json:"error_code"`
	Status                   string          `json:"status"`
	Version                  int64           `json:"version"`
}

type Target struct {
	ID       uuid.UUID       `json:"id"`
	Name     string          `json:"name"`
	Kind     string          `json:"kind"`
	Engine   string          `json:"engine"`
	Identity json.RawMessage `json:"identity"`
	Status   string          `json:"status"`
	Version  int64           `json:"version"`
}

func collectionDTO(row db.MonitoringCollection, now time.Time) Collection {
	value := Collection{ID: row.ID, Name: row.Name, Integration: row.IntegrationKey, Target: ResourceRef{Kind: row.TargetType, ID: row.TargetID}, CollectorID: row.CollectorID.UUID, Config: row.Config, CredentialConfigured: row.CredentialID.Valid, DesiredEnabled: row.DesiredEnabled, DesiredRevision: row.DesiredRevision, EffectiveRevision: row.EffectiveRevision, AssignmentEpoch: row.AssignmentEpoch, ApplyStatus: row.ApplyStatus, Health: row.Health, Freshness: "no_data", ErrorCode: row.ErrorCode, Status: row.Status, Version: row.Version}
	value.EffectiveAssignmentEpoch = row.EffectiveAssignmentEpoch
	if row.EffectiveCollectorID.Valid {
		value.EffectiveCollectorID = &row.EffectiveCollectorID.UUID
	}
	if row.ApplicationID.Valid {
		value.ApplicationID = &row.ApplicationID.UUID
	}
	if row.ServiceID.Valid {
		value.ServiceID = &row.ServiceID.UUID
	}
	if row.LastAttemptAt.Valid {
		value.LastAttemptAt = &row.LastAttemptAt.Time
	}
	if row.ObservedAt.Valid {
		value.ObservedAt = &row.ObservedAt.Time
	}
	if row.LastSampleAt.Valid {
		value.LastSampleAt = &row.LastSampleAt.Time
		var config monitoringcatalog.Config
		_ = json.Unmarshal(row.Config, &config)
		interval, err := time.ParseDuration(config.Interval)
		if err != nil {
			interval = 30 * time.Second
		}
		value.Freshness = "fresh"
		if now.Sub(row.LastSampleAt.Time) > max(3*interval, 2*time.Minute) {
			value.Freshness = "stale"
		}
	}
	return value
}

func (s Service) Integrations(ctx context.Context, a Actor) ([]monitoringcatalog.Entry, error) {
	if s.Store == nil {
		return nil, ErrUnavailable
	}
	if err := authorization.Authorize(ctx, s.Store.Queries, a, "monitoring.read"); err != nil {
		return nil, err
	}
	return monitoringcatalog.All(), nil
}

func (s Service) Collection(ctx context.Context, a Actor, id uuid.UUID) (Collection, error) {
	if s.Store == nil {
		return Collection{}, ErrUnavailable
	}
	q := s.Store.Queries
	if err := authorization.Authorize(ctx, q, a, "monitoring.read"); err != nil {
		return Collection{}, err
	}
	row, err := q.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: a.EnterpriseID, ID: id})
	if err != nil {
		return Collection{}, lookupError(err)
	}
	if row.IntegrationKey == "k8slog" && row.TargetType == "kubernetes_cluster" {
		err = requireLogCollection(ctx, q, a, row)
	} else {
		err = authorization.RequireResource(ctx, q, a, row.TargetType, row.TargetID)
	}
	if err != nil {
		return Collection{}, err
	}
	return collectionDTO(row, s.now()), authorization.Authorize(ctx, q, a, "monitoring.read")
}

func (s Service) Collections(ctx context.Context, a Actor, in ListInput) (Page[Collection], error) {
	out := Page[Collection]{Items: []Collection{}}
	if s.Store == nil {
		return out, ErrUnavailable
	}
	q := s.Store.Queries
	if err := authorization.Authorize(ctx, q, a, "monitoring.read"); err != nil {
		return out, err
	}
	if !slices.Contains([]string{"", "pending", "applying", "effective", "failed", "unknown"}, in.ApplyStatus) ||
		!slices.Contains([]string{"", "unknown", "healthy", "degraded", "failed", "stopped"}, in.Health) ||
		!slices.Contains([]string{"", "no_data", "fresh", "stale"}, in.Freshness) {
		return out, ErrInvalid
	}
	ids := map[string][]uuid.UUID{}
	for _, kind := range []string{"host", "kubernetes_cluster", "monitoring_target", "application", "kubernetes_namespace"} {
		granted, err := authorization.AuthorizedResourceIDs(ctx, q, a, kind)
		if err != nil {
			return out, err
		}
		ids[kind] = granted
	}
	in.Search = strings.TrimSpace(in.Search)
	limit, after, binding, err := s.page(a, "collections", in)
	if err != nil {
		return out, err
	}
	observedAt := s.now()
	enabled := pgtype.Bool{}
	if in.DesiredEnabled != nil {
		enabled = pgtype.Bool{Bool: *in.DesiredEnabled, Valid: true}
	}
	rows, err := q.ListMonitoringCollections(ctx, db.ListMonitoringCollectionsParams{
		EnterpriseID: a.EnterpriseID, HostIds: ids["host"], ClusterIds: ids["kubernetes_cluster"],
		TargetIds: ids["monitoring_target"], ApplicationIds: ids["application"], NamespaceIds: ids["kubernetes_namespace"],
		AfterID: after, IntegrationKey: in.Integration, Search: in.Search, PageLimit: int32(limit + 1),
		DesiredEnabled: enabled, ApplyStatus: in.ApplyStatus, Health: in.Health, Freshness: in.Freshness,
		ObservedAt: pgtype.Timestamptz{Time: observedAt, Valid: true},
	})
	if err != nil {
		return out, err
	}
	if len(rows) > limit {
		out.NextCursor, err = s.next(binding, rows[limit-1].ID)
		if err != nil {
			return out, err
		}
		rows = rows[:limit]
	}
	for _, row := range rows {
		out.Items = append(out.Items, collectionDTO(row, observedAt))
	}
	return out, authorization.Authorize(ctx, q, a, "monitoring.read")
}

func (s Service) Targets(ctx context.Context, a Actor, in ListInput) (Page[Target], error) {
	out := Page[Target]{Items: []Target{}}
	if s.Store == nil {
		return out, ErrUnavailable
	}
	q := s.Store.Queries
	if err := authorization.Authorize(ctx, q, a, "monitoring.read"); err != nil {
		return out, err
	}
	ids, err := authorization.AuthorizedResourceIDs(ctx, q, a, "monitoring_target")
	if err != nil {
		return out, err
	}
	in.Search = strings.TrimSpace(in.Search)
	limit, after, binding, err := s.page(a, "monitoring_targets", in)
	if err != nil {
		return out, err
	}
	rows, err := q.ListMonitoringTargets(ctx, db.ListMonitoringTargetsParams{EnterpriseID: a.EnterpriseID, AuthorizedIds: ids, AfterID: after, Engine: in.Integration, Search: in.Search, PageLimit: int32(limit + 1)})
	if err != nil {
		return out, err
	}
	if len(rows) > limit {
		out.NextCursor, err = s.next(binding, rows[limit-1].ID)
		if err != nil {
			return out, err
		}
		rows = rows[:limit]
	}
	for _, row := range rows {
		out.Items = append(out.Items, Target{ID: row.ID, Name: row.Name, Kind: row.Kind, Engine: row.Engine, Identity: row.IdentityConfig, Status: row.Status, Version: row.Version})
	}
	return out, authorization.Authorize(ctx, q, a, "monitoring.read")
}
