package monitoring

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"strings"
)

// Declared identities let file/workload log rules name an application service
// before its first SDK report. Discovery later reuses this stable identity.
func (s Service) prepareServiceDeclaration(ctx context.Context, q *db.Queries, a Actor, m Mutation, lock bool) (actionPlan, error) {
	p := actionPlan{SchemaVersion: "argus.monitoring_action/v1", Mutation: m, Actor: a, References: map[string]int64{}}
	if m.Operation != "create" || m.Service == nil || m.Application != nil || m.Binding != nil || m.Collection != nil || m.Endpoint != nil || m.IngestKey != nil {
		return p, ErrInvalid
	}
	in := m.Service
	if in.ApplicationID == uuid.Nil || !validName(in.Name, 200) || strings.HasPrefix(strings.ToLower(in.Name), "unknown_service") || in.Namespace != "" && !validName(in.Namespace, 200) {
		return p, ErrInvalid
	}
	if err := authorization.Authorize(ctx, q, a, "application.manage"); err != nil {
		return p, err
	}
	if err := authorization.RequireResource(ctx, q, a, "application", in.ApplicationID); err != nil {
		return p, err
	}
	app, err := q.GetMonitoringApplication(ctx, db.GetMonitoringApplicationParams{EnterpriseID: a.EnterpriseID, ID: in.ApplicationID})
	if err != nil {
		return p, lookupError(err)
	}
	if app.Status != "active" {
		return p, ErrConflict
	}
	p.References["application:"+app.ID.String()] = app.Version
	if lock {
		if err = q.LockMonitoringDiscoveryScope(ctx, db.LockMonitoringDiscoveryScopeParams{EnterpriseID: a.EnterpriseID, ApplicationID: app.ID}); err != nil {
			return p, err
		}
	}
	_, err = q.GetMonitoringServiceByName(ctx, db.GetMonitoringServiceByNameParams{EnterpriseID: a.EnterpriseID, ApplicationID: app.ID, ServiceNamespace: in.Namespace, ServiceName: in.Name})
	if err == nil {
		return p, ErrConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return p, err
	}
	count, err := q.CountMonitoringServices(ctx, db.CountMonitoringServicesParams{EnterpriseID: a.EnterpriseID, ApplicationID: app.ID})
	if err != nil {
		return p, err
	}
	if count >= 1000 {
		return p, ErrServiceLimit
	}
	return p, nil
}
