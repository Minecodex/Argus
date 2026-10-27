package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type IngestObservation struct {
	KeyID                                                            uuid.UUID
	EndpointID                                                       uuid.UUID
	ApplicationID                                                    uuid.UUID
	Protocol                                                         string
	ServiceNamespace, ServiceName, InstanceKey, Runtime, Environment string
}
type ApplicationIdentity struct {
	ApplicationID uuid.UUID `json:"application_id"`
	ServiceID     uuid.UUID `json:"service_id"`
	InstanceID    uuid.UUID `json:"instance_id"`
	EndpointID    uuid.UUID `json:"endpoint_id"`
	KeyID         uuid.UUID `json:"key_id"`
	Environment   string    `json:"environment"`
}

// RegisterIngestResources is called only after the transport has authenticated
// the Collector and resolved each registered receiver source. It resolves Key
// references again, so receiver-side caches cannot extend a revoked Key.
// The entire batch registers atomically; no SDK-provided application ID is used.
func (s Service) RegisterIngestResources(ctx context.Context, enterprise, collector uuid.UUID, signal string, observations []IngestObservation) ([]ApplicationIdentity, error) {
	if s.Store == nil {
		return nil, ErrUnavailable
	}
	if enterprise == uuid.Nil || collector == uuid.Nil || len(observations) == 0 || len(observations) > 256 {
		return nil, ErrIngestRejected
	}
	for _, v := range observations {
		if !SDKProtocolEligible(v.Protocol) {
			return nil, ErrIngestRejected
		}
		if !validName(v.ServiceName, 200) || strings.HasPrefix(strings.ToLower(v.ServiceName), "unknown_service") || v.ServiceNamespace != "" && !validName(v.ServiceNamespace, 200) || v.InstanceKey != "" && !validName(v.InstanceKey, 512) || v.Runtime != "" && !validName(v.Runtime, 100) {
			return nil, ErrIngestRejected
		}
	}
	result := make([]ApplicationIdentity, len(observations))
	err := s.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		keys := map[uuid.UUID]db.ResolveMonitoringIngestKeyRow{}
		apps := map[uuid.UUID]bool{}
		for _, observation := range observations {
			row, exists := keys[observation.KeyID]
			if !exists {
				var err error
				row, err = q.ResolveMonitoringIngestKey(ctx, observation.KeyID)
				if err != nil {
					if errors.Is(err, pgx.ErrNoRows) {
						return ErrIngestRejected
					}
					return err
				}
				keys[observation.KeyID] = row
			}
			if !sdkDistributionEligible(row.Protocol, row.DistributionVersion, row.CollectorVersion, row.ConfigSchemaVersion, row.SupportStatus, row.Components) {
				return ErrIngestRejected
			}
			if row.EnterpriseID != enterprise || row.EndpointID != observation.EndpointID || row.ApplicationID != observation.ApplicationID || !row.CollectorID.Valid || row.CollectorID.UUID != collector || row.Protocol != observation.Protocol || !slices.Contains(row.Signals, signal) || observation.Environment != "" && observation.Environment != row.Environment {
				return ErrIngestRejected
			}
			apps[row.ApplicationID] = true
		}
		ordered := []uuid.UUID{}
		for id := range apps {
			ordered = append(ordered, id)
		}
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].String() < ordered[j].String() })
		for _, id := range ordered {
			if err := q.LockMonitoringDiscoveryScope(ctx, db.LockMonitoringDiscoveryScopeParams{EnterpriseID: enterprise, ApplicationID: id}); err != nil {
				return err
			}
		}
		// Re-read after waiting for discovery locks, including revocation and
		// application state. New requests never inherit another request's grant.
		for id, prior := range keys {
			current, err := q.ResolveMonitoringIngestKey(ctx, id)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return ErrIngestRejected
				}
				return err
			}
			if current.EndpointVersion != prior.EndpointVersion || !sdkDistributionEligible(current.Protocol, current.DistributionVersion, current.CollectorVersion, current.ConfigSchemaVersion, current.SupportStatus, current.Components) {
				return ErrIngestRejected
			}
			keys[id] = current
		}
		for index, value := range observations {
			key := keys[value.KeyID]
			service, err := q.GetMonitoringServiceByName(ctx, db.GetMonitoringServiceByNameParams{EnterpriseID: enterprise, ApplicationID: key.ApplicationID, ServiceNamespace: value.ServiceNamespace, ServiceName: value.ServiceName})
			if errors.Is(err, pgx.ErrNoRows) {
				if len(key.ServiceAllowlist) > 0 {
					return ErrIngestRejected
				}
				count, e := q.CountMonitoringServices(ctx, db.CountMonitoringServicesParams{EnterpriseID: enterprise, ApplicationID: key.ApplicationID})
				if e != nil {
					return e
				}
				if count >= 1000 {
					return ErrIngestRejected
				}
			} else if err != nil {
				return err
			} else if service.Status != "active" || len(key.ServiceAllowlist) > 0 && !slices.Contains(key.ServiceAllowlist, service.ID) {
				return ErrIngestRejected
			}
			encoded, _ := json.Marshal([]string{value.ServiceNamespace, value.ServiceName})
			service, err = q.DiscoverMonitoringService(ctx, db.DiscoverMonitoringServiceParams{ID: uuid.NewSHA1(key.ApplicationID, encoded), EnterpriseID: enterprise, ApplicationID: key.ApplicationID, ServiceNamespace: value.ServiceNamespace, ServiceName: value.ServiceName, Runtime: value.Runtime})
			if err != nil {
				return lookupError(err)
			}
			identity := ApplicationIdentity{ApplicationID: key.ApplicationID, ServiceID: service.ID, EndpointID: key.EndpointID, KeyID: key.ID, Environment: key.Environment}
			if value.InstanceKey != "" {
				_, err = q.GetMonitoringInstanceByKey(ctx, db.GetMonitoringInstanceByKeyParams{EnterpriseID: enterprise, ServiceID: service.ID, InstanceKey: value.InstanceKey})
				if errors.Is(err, pgx.ErrNoRows) {
					count, e := q.CountMonitoringServiceInstances(ctx, db.CountMonitoringServiceInstancesParams{EnterpriseID: enterprise, ServiceID: service.ID})
					if e != nil {
						return e
					}
					if count >= 1000 {
						return ErrIngestRejected
					}
				} else if err != nil {
					return err
				}
				instance, err := q.ObserveMonitoringInstance(ctx, db.ObserveMonitoringInstanceParams{ID: uuid.NewSHA1(service.ID, []byte(value.InstanceKey)), EnterpriseID: enterprise, ServiceID: service.ID, InstanceKey: value.InstanceKey, Placement: json.RawMessage(`{}`), LastSeenAt: pgtype.Timestamptz{Time: s.now(), Valid: true}})
				if err != nil {
					return err
				}
				identity.InstanceID = instance.ID
			}
			result[index] = identity
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
