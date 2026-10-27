package telemetry

import (
	"context"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func (server *IngestServer) observeApplicationSamples(ctx context.Context, identity TrustedIdentity, signal string, payload []byte) error {
	if server.Applications.Store == nil {
		return nil
	}
	endpoints, keys := map[uuid.UUID]bool{}, map[uuid.UUID]bool{}
	services := map[uuid.UUID]uuid.UUID{}
	for _, resource := range acceptedTelemetryResources(signal, payload) {
		attrs := attributesMap(resource.GetAttributes())
		application, appErr := uuid.Parse(attrs["argus.application.id"])
		service, serviceErr := uuid.Parse(attrs["argus.service.id"])
		if appErr == nil && serviceErr == nil && application != uuid.Nil && service != uuid.Nil {
			services[service] = application
		}
		endpoint, e1 := uuid.Parse(attrs["argus.application.endpoint.id"])
		key, e2 := uuid.Parse(attrs["argus.application.key.id"])
		if e1 != nil || e2 != nil || endpoint == uuid.Nil || key == uuid.Nil {
			continue
		}
		endpoints[endpoint] = true
		keys[key] = true
	}
	q := server.Applications.Store.Queries
	for service, application := range services {
		if err := q.ObserveMonitoringApplicationService(ctx, db.ObserveMonitoringApplicationServiceParams{EnterpriseID: identity.EnterpriseID, ID: service, ApplicationID: application}); err != nil {
			return err
		}
	}
	for id := range endpoints {
		if err := q.ObserveApplicationIngest(ctx, db.ObserveApplicationIngestParams{EnterpriseID: identity.EnterpriseID, ID: id}); err != nil {
			return err
		}
	}
	for id := range keys {
		if err := q.TouchMonitoringIngestKey(ctx, db.TouchMonitoringIngestKeyParams{EnterpriseID: identity.EnterpriseID, ID: id}); err != nil {
			return err
		}
	}
	return nil
}
