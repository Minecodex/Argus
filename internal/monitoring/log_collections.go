package monitoring

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/logcollection"
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"slices"
	"strings"
)

var ErrLogOverlap = errors.New("MONITORING_LOG_RULE_OVERLAP")

func (s Service) prepareLogCollection(ctx context.Context, q *db.Queries, p actionPlan, current db.MonitoringCollection, lock bool) (actionPlan, error) {
	m, a, in := p.Mutation, p.Actor, p.Mutation.Collection
	if in == nil || !validName(in.Name, 200) || in.CollectorID == uuid.Nil || len(in.Bindings) > 0 || in.CredentialDraftID != uuid.Nil || in.CredentialUpdate != "" && in.CredentialUpdate != "keep" || len(in.TargetIdentity) > 0 || in.TargetName != "" {
		return p, ErrInvalid
	}
	for _, permission := range []string{"collector.use", "telemetry.collector.manage"} {
		if err := authorization.Authorize(ctx, q, a, permission); err != nil {
			return p, err
		}
	}
	collector, err := q.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: a.EnterpriseID, ID: in.CollectorID})
	if err != nil {
		return p, lookupError(err)
	}
	isKubernetes := in.Integration == "k8slog"
	if isKubernetes && collector.ResourceType != "kubernetes_cluster" || !isKubernetes && collector.ResourceType != "host" {
		return p, ErrUnsupported
	}
	if collector.Status == "uninstalled" || collector.Status == "uninstalling" {
		return p, ErrUnavailable
	}
	if !isKubernetes {
		if err = authorization.RequireResource(ctx, q, a, collector.ResourceType, collector.ResourceID); err != nil {
			return p, err
		}
	}
	config, err := logcollection.Normalize(in.Integration, collector.Platform, in.Config)
	if err != nil {
		return p, ErrInvalid
	}
	if isKubernetes {
		if err = s.prepareKubernetesLogWorkload(ctx, q, &p, collector, &config, lock); err != nil {
			return p, err
		}
	}
	expected := ResourceRef{Kind: collector.ResourceType, ID: collector.ResourceID}
	if config.Scope == "business" {
		if in.ApplicationID == uuid.Nil || in.ServiceID == uuid.Nil {
			return p, ErrInvalid
		}
		if err = s.collectionApplication(ctx, q, a, in.ApplicationID, in.ServiceID, p.References); err != nil {
			return p, err
		}
		expected = ResourceRef{Kind: "application", ID: in.ApplicationID}
	} else if config.Scope == "resource" {
		if in.Target.Kind != "monitoring_target" || in.Target.ID == uuid.Nil {
			return p, ErrInvalid
		}
		version, e := requireTarget(ctx, q, a, in.Target)
		if e != nil {
			return p, e
		}
		p.References["monitoring_target:"+in.Target.ID.String()] = version
		expected = in.Target
		if in.ApplicationID != uuid.Nil {
			if e := s.collectionApplication(ctx, q, a, in.ApplicationID, in.ServiceID, p.References); e != nil {
				return p, e
			}
		} else if in.ServiceID != uuid.Nil {
			return p, ErrInvalid
		}
	} else if in.ApplicationID != uuid.Nil || in.ServiceID != uuid.Nil {
		return p, ErrInvalid
	}
	if in.Target.ID != uuid.Nil && in.Target != expected {
		return p, ErrInvalid
	}
	in.Target = expected
	if m.Operation != "create" && current.TargetType != in.Target.Kind || m.Operation != "create" && current.TargetID != in.Target.ID {
		return p, ErrInvalid
	}
	if current.CollectorID.Valid && current.CollectorID.UUID != collector.ID {
		prior, e := q.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: a.EnterpriseID, ID: current.CollectorID.UUID})
		if e != nil {
			return p, e
		}
		if prior.ResourceType != collector.ResourceType || prior.ResourceID != collector.ResourceID {
			return p, ErrInvalid
		}
	}
	entry, _ := monitoringcatalog.Lookup(in.Integration)
	distribution, err := q.GetMonitoringDistribution(ctx, collector.DistributionVersionID)
	if err != nil {
		return p, err
	}
	if distribution.CollectorVersion != monitoringcatalog.CollectorVersion || distribution.SupportStatus != "supported" || !slices.Contains(distribution.Components, entry.Component) || !slices.Contains(distribution.Components, "file_storage") {
		return p, ErrUnsupported
	}
	if isKubernetes && (!slices.Contains(distribution.Components, "k8sattributes") || !slices.Contains(distribution.Components, "argus_log_identity")) {
		return p, ErrUnsupported
	}
	p.References["collector:"+collector.ID.String()] = collector.Version
	p.References["distribution:"+distribution.ID.String()] = int64(distribution.CatalogRevision)
	if in.Enabled {
		profiles, e := q.ListCollectorLegacyLogProfiles(ctx, db.ListCollectorLegacyLogProfilesParams{EnterpriseID: a.EnterpriseID, ID: collector.ID})
		if e != nil {
			return p, e
		}
		overlap, e := logcollection.ConflictsWithLegacy(ctx, config, in.Integration, collector.Platform, profiles)
		if e != nil || overlap {
			return p, ErrLogOverlap
		}
		if lock {
			if e := q.LockLogCollectionClaims(ctx, db.LockLogCollectionClaimsParams{EnterpriseID: a.EnterpriseID, ResourceType: collector.ResourceType, ResourceID: collector.ResourceID}); e != nil {
				return p, e
			}
		}
		rows, e := q.ListResourceLogCollections(ctx, db.ListResourceLogCollectionsParams{EnterpriseID: a.EnterpriseID, ResourceType: collector.ResourceType, ResourceID: collector.ResourceID})
		if e != nil {
			return p, e
		}
		for _, row := range rows {
			if row.ID == m.ID || !row.DesiredEnabled || row.IntegrationKey != in.Integration {
				continue
			}
			other, e := logcollection.Normalize(row.IntegrationKey, collector.Platform, row.Config)
			if e != nil {
				return p, ErrInvalid
			}
			overlap := false
			switch in.Integration {
			case "filelog":
				overlap, e = logcollection.FileOverlapForPlatform(ctx, config, other, collector.Platform == "windows_amd64")
			case "journald":
				overlap = len(config.Units) == 0 || len(other.Units) == 0
				for _, unit := range config.Units {
					overlap = overlap || slices.Contains(other.Units, unit)
				}
			case "eventlog":
				overlap = strings.EqualFold(config.Channel, other.Channel)
			case "k8slog":
				if config.Workload == nil || other.Workload == nil {
					return p, ErrInvalid
				}
				overlap = logcollection.WorkloadOverlap(*config.Workload, *other.Workload)
			}
			if e != nil || overlap {
				return p, ErrLogOverlap
			}
		}
	}
	in.Config, _ = json.Marshal(config)
	in.CredentialUpdate = "keep"
	p.Mutation.Collection = in
	if s.Runtime == nil {
		return p, ErrUnavailable
	}
	collectors := []uuid.UUID{collector.ID}
	if current.CollectorID.Valid && current.CollectorID.UUID != collector.ID {
		collectors = append(collectors, current.CollectorID.UUID)
	}
	for _, id := range collectors {
		raw, e := s.Runtime.Plan(ctx, q, a.EnterpriseID, id)
		if e != nil {
			return p, e
		}
		digest := sha256.Sum256(raw)
		p.References["runtime:"+hex.EncodeToString(digest[:])] = 1
		if id == in.CollectorID {
			p.RuntimePlans = append(p.RuntimePlans, raw)
		}
	}
	return p, nil
}
