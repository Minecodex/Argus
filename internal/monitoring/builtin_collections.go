package monitoring

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type Builtin struct {
	Definition  builtincollection.Definition `json:"definition"`
	ID          uuid.UUID                    `json:"id"`
	CollectorID *uuid.UUID                   `json:"collector_id,omitempty"`
	Collection  *Collection                  `json:"collection,omitempty"`
	Available   bool                         `json:"available"`
	Reason      string                       `json:"reason,omitempty"`
}

type BuiltinInput struct {
	Target          ResourceRef     `json:"target"`
	Key             string          `json:"key"`
	Enabled         bool            `json:"enabled"`
	ExpectedVersion int64           `json:"expected_version"`
	Config          json.RawMessage `json:"config,omitempty"`
}

func (s Service) PreviewBuiltin(ctx context.Context, a Actor, in BuiltinInput, key string) (db.PendingAction, error) {
	if s.Store == nil {
		return db.PendingAction{}, ErrUnavailable
	}
	if _, ok := builtincollection.Lookup(in.Key); !ok {
		return db.PendingAction{}, ErrUnsupported
	}
	if err := authorization.Authorize(ctx, s.Store.Queries, a, "telemetry.collector.manage"); err != nil {
		return db.PendingAction{}, err
	}
	if _, err := requireTarget(ctx, s.Store.Queries, a, in.Target); err != nil {
		return db.PendingAction{}, err
	}
	collector, err := s.Store.Queries.GetCollectorForResource(ctx, db.GetCollectorForResourceParams{EnterpriseID: a.EnterpriseID, ResourceType: in.Target.Kind, ResourceID: in.Target.ID})
	if err != nil {
		return db.PendingAction{}, ErrUnavailable
	}
	id := builtincollection.StableID(in.Target.Kind, in.Target.ID, in.Key)
	row, err := s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: a.EnterpriseID, ID: id})
	m := Mutation{Kind: "collection", Operation: "create", Collection: &CollectionInput{Integration: in.Key, Target: in.Target, CollectorID: collector.ID, Enabled: in.Enabled, Config: in.Config}}
	if err == nil {
		if in.ExpectedVersion != row.Version {
			return db.PendingAction{}, ErrConflict
		}
		m.ID, m.Operation, m.ExpectedVersion = id, "update", in.ExpectedVersion
		if len(in.Config) == 0 {
			m.Collection.Config = row.Config
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return db.PendingAction{}, err
	} else if in.ExpectedVersion != 0 {
		return db.PendingAction{}, ErrConflict
	}
	return s.Preview(ctx, a, m, key)
}

func (s Service) Builtins(ctx context.Context, a Actor, target ResourceRef) ([]Builtin, error) {
	if s.Store == nil {
		return nil, ErrUnavailable
	}
	q := s.Store.Queries
	if err := authorization.Authorize(ctx, q, a, "telemetry.collector.read"); err != nil {
		return nil, err
	}
	if target.Kind != "host" && target.Kind != "kubernetes_cluster" {
		return nil, ErrInvalid
	}
	if _, err := requireTarget(ctx, q, a, target); err != nil {
		return nil, err
	}
	collector, collectorErr := q.GetCollectorForResource(ctx, db.GetCollectorForResourceParams{EnterpriseID: a.EnterpriseID, ResourceType: target.Kind, ResourceID: target.ID})
	if collectorErr != nil && !errors.Is(collectorErr, pgx.ErrNoRows) {
		return nil, collectorErr
	}
	out := []Builtin{}
	for _, definition := range builtincollection.All() {
		if !slices.Contains(definition.ResourceTypes, target.Kind) {
			continue
		}
		item := Builtin{Definition: definition, ID: builtincollection.StableID(target.Kind, target.ID, definition.Key), Reason: "collector_not_installed"}
		if collectorErr == nil && collector.Status != "uninstalled" && collector.Status != "uninstalling" {
			item.CollectorID = &collector.ID
			distribution, err := q.GetMonitoringDistribution(ctx, collector.DistributionVersionID)
			if err != nil {
				return nil, err
			}
			item.Available = slices.Contains(definition.Platforms, collector.Platform) && distribution.CollectorVersion == monitoringcatalog.CollectorVersion && distribution.SupportStatus == "supported" && builtincollection.ComponentsAvailable(definition, distribution.Components)
			item.Reason = "component_unavailable"
			if item.Available {
				item.Reason = ""
			}
		}
		row, err := q.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: a.EnterpriseID, ID: item.ID})
		if err == nil {
			value := collectionDTO(row, s.now())
			item.Collection = &value
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		out = append(out, item)
	}
	if err := authorization.Authorize(ctx, q, a, "telemetry.collector.read"); err != nil {
		return nil, err
	}
	if _, err := requireTarget(ctx, q, a, target); err != nil {
		return nil, err
	}
	return out, nil
}

func (s Service) prepareBuiltinCollection(ctx context.Context, q *db.Queries, p actionPlan, current db.MonitoringCollection) (actionPlan, error) {
	m, a, in := p.Mutation, p.Actor, p.Mutation.Collection
	definition, ok := builtincollection.Lookup(in.Integration)
	if !ok || m.Operation == "archive" || in.CollectorID == uuid.Nil || in.Target.ID == uuid.Nil || !slices.Contains(definition.ResourceTypes, in.Target.Kind) || in.ApplicationID != uuid.Nil || in.ServiceID != uuid.Nil || len(in.Bindings) > 0 || len(in.TargetIdentity) > 0 || in.TargetName != "" || in.CredentialDraftID != uuid.Nil || in.CredentialUpdate != "" && in.CredentialUpdate != "keep" {
		return p, ErrInvalid
	}
	if m.ID != builtincollection.StableID(in.Target.Kind, in.Target.ID, in.Integration) {
		return p, ErrInvalid
	}
	for _, permission := range []string{"telemetry.collector.manage"} {
		if err := authorization.Authorize(ctx, q, a, permission); err != nil {
			return p, err
		}
	}
	version, err := requireTarget(ctx, q, a, in.Target)
	if err != nil {
		return p, err
	}
	collector, err := q.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: a.EnterpriseID, ID: in.CollectorID})
	if err != nil {
		return p, lookupError(err)
	}
	if collector.ResourceType != in.Target.Kind || collector.ResourceID != in.Target.ID || !slices.Contains(definition.Platforms, collector.Platform) || collector.Status == "uninstalled" || collector.Status == "uninstalling" {
		return p, ErrUnsupported
	}
	if m.Operation == "create" {
		if _, err = q.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: a.EnterpriseID, ID: m.ID}); err == nil {
			return p, ErrConflict
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return p, err
		}
	}
	if current.CollectorID.Valid && current.CollectorID.UUID != collector.ID {
		return p, ErrInvalid
	}
	distribution, err := q.GetMonitoringDistribution(ctx, collector.DistributionVersionID)
	if err != nil {
		return p, err
	}
	if distribution.CollectorVersion != monitoringcatalog.CollectorVersion || distribution.SupportStatus != "supported" || !builtincollection.ComponentsAvailable(definition, distribution.Components) {
		return p, ErrUnsupported
	}
	config, err := builtincollection.ParseFor(definition.Key, in.Config)
	if err != nil {
		return p, ErrInvalid
	}
	in.Config, _ = json.Marshal(config)
	in.Name = definition.Name
	in.CredentialUpdate = "keep"
	p.Mutation.Collection = in
	p.References[in.Target.Kind+":"+in.Target.ID.String()] = version
	p.References["collector:"+collector.ID.String()] = collector.Version
	p.References["distribution:"+distribution.ID.String()] = int64(distribution.CatalogRevision)
	if s.Runtime == nil {
		return p, ErrUnavailable
	}
	raw, err := s.Runtime.Plan(ctx, q, a.EnterpriseID, collector.ID)
	if err != nil {
		return p, err
	}
	digest := sha256.Sum256(raw)
	p.References["runtime:"+hex.EncodeToString(digest[:])] = 1
	p.RuntimePlans = []json.RawMessage{raw}
	return p, nil
}
