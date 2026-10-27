package monitoring

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type IngestEndpointInput struct {
	Name             string      `json:"name"`
	ApplicationID    uuid.UUID   `json:"application_id"`
	CollectorID      uuid.UUID   `json:"collector_id"`
	Protocol         string      `json:"protocol"`
	Signals          []string    `json:"signals"`
	ServiceAllowlist []uuid.UUID `json:"service_allowlist"`
	Enabled          bool        `json:"enabled"`
}
type IngestKeyInput struct {
	EndpointID uuid.UUID `json:"endpoint_id"`
	ExpiresAt  time.Time `json:"expires_at"`
}

func (s Service) prepareIngest(ctx context.Context, q *db.Queries, a Actor, m Mutation, lock bool) (actionPlan, error) {
	p := actionPlan{SchemaVersion: "argus.monitoring_action/v1", Mutation: m, Actor: a, References: map[string]int64{}}
	if m.Application != nil || m.Collection != nil || m.Binding != nil {
		return p, ErrInvalid
	}
	if err := authorization.Authorize(ctx, q, a, "application.manage"); err != nil {
		return p, err
	}
	if m.Kind == "ingest_key" {
		return s.prepareIngestKey(ctx, q, a, m, p)
	}
	if m.Endpoint == nil || m.IngestKey != nil {
		return p, ErrInvalid
	}
	in := *m.Endpoint
	if m.Operation != "create" {
		var row db.MonitoringIngestEndpoint
		var err error
		args := db.GetMonitoringIngestEndpointParams{EnterpriseID: a.EnterpriseID, ID: m.ID}
		if lock {
			row, err = q.LockMonitoringIngestEndpoint(ctx, db.LockMonitoringIngestEndpointParams(args))
		} else {
			row, err = q.GetMonitoringIngestEndpoint(ctx, args)
		}
		if err != nil {
			return p, lookupError(err)
		}
		if row.Version != m.ExpectedVersion {
			return p, ErrConflict
		}
		p.References["endpoint:"+row.ID.String()] = row.Version
		if m.Operation == "set_enabled" {
			in = IngestEndpointInput{Name: row.Name, ApplicationID: row.ApplicationID, CollectorID: row.CollectorID.UUID, Protocol: row.Protocol, Signals: row.Signals, ServiceAllowlist: row.ServiceAllowlist, Enabled: in.Enabled}
		} else if m.Operation != "update" {
			return p, ErrInvalid
		}
		if in.ApplicationID != row.ApplicationID || in.CollectorID != row.CollectorID.UUID || in.Protocol != row.Protocol || !slices.Equal(in.Signals, row.Signals) {
			return p, ErrInvalid
		}
	}
	if !validName(in.Name, 200) || in.ApplicationID == uuid.Nil || in.CollectorID == uuid.Nil || !slices.Contains([]string{"otlp", "jaeger", "skywalking"}, in.Protocol) || len(in.Signals) < 1 || len(in.Signals) > 3 || len(in.ServiceAllowlist) > 100 {
		return p, ErrInvalid
	}
	in.Signals = slices.Clone(in.Signals)
	if in.Enabled && !SDKProtocolEligible(in.Protocol) {
		return p, ErrUnsupported
	}
	slices.Sort(in.Signals)
	if len(slices.Compact(slices.Clone(in.Signals))) != len(in.Signals) {
		return p, ErrInvalid
	}
	for _, signal := range in.Signals {
		if !slices.Contains([]string{"metrics", "logs", "traces"}, signal) || in.Protocol != "otlp" && signal != "traces" {
			return p, ErrInvalid
		}
	}
	if err := s.collectionApplication(ctx, q, a, in.ApplicationID, uuid.Nil, p.References); err != nil {
		return p, err
	}
	for _, id := range in.ServiceAllowlist {
		if id == uuid.Nil {
			return p, ErrInvalid
		}
		if err := s.collectionApplication(ctx, q, a, in.ApplicationID, id, p.References); err != nil {
			return p, err
		}
	}
	if err := authorization.Authorize(ctx, q, a, "collector.use"); err != nil {
		return p, err
	}
	collector, err := q.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: a.EnterpriseID, ID: in.CollectorID})
	if err != nil {
		return p, lookupError(err)
	}
	if err = authorization.RequireResource(ctx, q, a, collector.ResourceType, collector.ResourceID); err != nil {
		return p, err
	}
	if collector.Status == "uninstalled" || collector.Status == "uninstalling" {
		return p, ErrUnsupported
	}
	distribution, err := q.GetMonitoringDistribution(ctx, collector.DistributionVersionID)
	if err != nil {
		return p, err
	}
	if !sdkDistributionEligible(in.Protocol, distribution.Version, distribution.CollectorVersion, distribution.ConfigSchemaVersion, distribution.SupportStatus, distribution.Components) {
		return p, ErrUnsupported
	}
	p.References["collector:"+collector.ID.String()] = collector.Version
	p.References["distribution:"+distribution.ID.String()] = int64(distribution.CatalogRevision)
	if s.Runtime == nil {
		return p, ErrUnavailable
	}
	raw, err := s.Runtime.Plan(ctx, q, a.EnterpriseID, in.CollectorID)
	if err != nil {
		return p, err
	}
	digest := sha256.Sum256(raw)
	p.References["runtime:"+hex.EncodeToString(digest[:])] = 1
	p.RuntimePlans = append(p.RuntimePlans, raw)
	p.Mutation.Endpoint = &in
	return p, nil
}

func (s Service) prepareIngestKey(ctx context.Context, q *db.Queries, a Actor, m Mutation, p actionPlan) (actionPlan, error) {
	if m.IngestKey == nil || m.Endpoint != nil || len(s.Actions.Key) < 32 {
		return p, ErrInvalid
	}
	if m.Operation == "create" && a.SubjectType != "user" {
		return p, ErrDenied
	}
	in := *m.IngestKey
	if m.Operation == "revoke" {
		key, err := q.GetMonitoringIngestKey(ctx, db.GetMonitoringIngestKeyParams{EnterpriseID: a.EnterpriseID, ID: m.ID})
		if err != nil {
			return p, lookupError(err)
		}
		if key.Version != m.ExpectedVersion || key.RevokedAt.Valid {
			return p, ErrConflict
		}
		in = IngestKeyInput{EndpointID: key.EndpointID, ExpiresAt: key.ExpiresAt.Time}
		p.References["ingest_key:"+key.ID.String()] = key.Version
	} else if m.Operation != "create" || in.ExpiresAt.Before(s.now().Add(time.Minute)) || in.ExpiresAt.After(s.now().Add(366*24*time.Hour)) {
		return p, ErrInvalid
	}
	endpoint, err := q.LockMonitoringIngestEndpoint(ctx, db.LockMonitoringIngestEndpointParams{EnterpriseID: a.EnterpriseID, ID: in.EndpointID})
	if err != nil {
		return p, lookupError(err)
	}
	if err = s.collectionApplication(ctx, q, a, endpoint.ApplicationID, uuid.Nil, p.References); err != nil {
		return p, err
	}
	if m.Operation == "create" {
		if !endpoint.Enabled {
			return p, ErrInvalid
		}
		collector, err := q.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: a.EnterpriseID, ID: endpoint.CollectorID.UUID})
		if err != nil {
			return p, lookupError(err)
		}
		distribution, err := q.GetMonitoringDistribution(ctx, collector.DistributionVersionID)
		if err != nil {
			return p, err
		}
		if !sdkDistributionEligible(endpoint.Protocol, distribution.Version, distribution.CollectorVersion, distribution.ConfigSchemaVersion, distribution.SupportStatus, distribution.Components) {
			return p, ErrUnsupported
		}
		p.References["collector:"+collector.ID.String()] = collector.Version
		p.References["distribution:"+distribution.ID.String()] = int64(distribution.CatalogRevision)
		count, err := q.CountMonitoringEndpointKeys(ctx, db.CountMonitoringEndpointKeysParams{EnterpriseID: a.EnterpriseID, EndpointID: endpoint.ID})
		if err != nil {
			return p, err
		}
		if count >= 4 {
			return p, ErrDependency
		}
	}
	p.References["endpoint:"+endpoint.ID.String()] = endpoint.Version
	p.Mutation.IngestKey = &in
	return p, nil
}

func (s Service) commitIngest(ctx context.Context, q *db.Queries, p actionPlan) (resource.ActionCommitResult, error) {
	m, a := p.Mutation, p.Actor
	result := resource.ActionCommitResult{ResourceType: m.Kind, ResourceID: m.ID, Summary: "Application ingestion configuration saved"}
	if m.Kind == "endpoint" {
		in := m.Endpoint
		if in == nil {
			return result, ErrInvalid
		}
		var row db.MonitoringIngestEndpoint
		var err error
		if m.Operation == "create" {
			allowlist := append([]uuid.UUID{}, in.ServiceAllowlist...)
			row, err = q.CreateMonitoringIngestEndpoint(ctx, db.CreateMonitoringIngestEndpointParams{ID: m.ID, EnterpriseID: a.EnterpriseID, ApplicationID: in.ApplicationID, Name: in.Name, Protocol: in.Protocol, Signals: in.Signals, CollectorID: nullID(in.CollectorID), ServiceAllowlist: allowlist, CreatedBy: a.SubjectID, Enabled: in.Enabled})
		} else {
			row, err = q.UpdateMonitoringIngestEndpoint(ctx, db.UpdateMonitoringIngestEndpointParams{EnterpriseID: a.EnterpriseID, ID: m.ID, Version: m.ExpectedVersion, Enabled: in.Enabled, Name: in.Name, ServiceAllowlist: append([]uuid.UUID{}, in.ServiceAllowlist...)})
		}
		result.ResourceVersion = row.Version
		return result, err
	}
	if m.Operation == "revoke" {
		count, err := q.RevokeMonitoringIngestKey(ctx, db.RevokeMonitoringIngestKeyParams{EnterpriseID: a.EnterpriseID, ID: m.ID, Version: m.ExpectedVersion})
		if err != nil {
			return result, err
		}
		if count != 1 {
			return result, ErrConflict
		}
		result.ResourceVersion = m.ExpectedVersion + 1
		return result, nil
	}
	in := m.IngestKey
	if in == nil {
		return result, ErrInvalid
	}
	endpoint, err := q.GetMonitoringIngestEndpoint(ctx, db.GetMonitoringIngestEndpointParams{EnterpriseID: a.EnterpriseID, ID: in.EndpointID})
	if err != nil {
		return result, err
	}
	value := s.deriveIngestKey(a.EnterpriseID, m.ID)
	digest := sha256.Sum256([]byte(value))
	row, err := q.CreateMonitoringIngestKey(ctx, db.CreateMonitoringIngestKeyParams{ID: m.ID, EnterpriseID: a.EnterpriseID, EndpointID: endpoint.ID, ApplicationID: endpoint.ApplicationID, KeyPrefix: "argus_app." + m.ID.String(), KeyDigest: digest[:], ExpiresAt: pgtype.Timestamptz{Time: in.ExpiresAt, Valid: true}, CreatedBy: a.SubjectID})
	result.ResourceVersion = row.Version
	return result, err
}
