package monitoring

import (
	"context"

	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Archival ends the current target lifecycle, independently of collection
// enablement. Sources and the stable identity remain available to history.
func (s Service) prepareTargetArchive(ctx context.Context, q *db.Queries, a Actor, m Mutation, lock bool) (actionPlan, error) {
	p := actionPlan{SchemaVersion: "argus.monitoring_action/v1", Mutation: m, Actor: a, References: map[string]int64{}}
	if err := authorization.Authorize(ctx, q, a, "monitoring.manage"); err != nil {
		return p, err
	}
	if m.Operation != "archive" || m.ExpectedVersion < 1 || m.Application != nil || m.Binding != nil || m.Collection != nil || m.Service != nil || m.Endpoint != nil || m.IngestKey != nil {
		return p, ErrInvalid
	}
	if err := authorization.RequireResource(ctx, q, a, "monitoring_target", m.ID); err != nil {
		return p, err
	}
	var row db.MonitoringTarget
	var err error
	if lock {
		row, err = q.LockMonitoringTarget(ctx, db.LockMonitoringTargetParams{EnterpriseID: a.EnterpriseID, ID: m.ID})
	} else {
		row, err = q.GetMonitoringTarget(ctx, db.GetMonitoringTargetParams{EnterpriseID: a.EnterpriseID, ID: m.ID})
	}
	if err != nil {
		return p, lookupError(err)
	}
	if row.Version != m.ExpectedVersion || row.Status != "active" {
		return p, ErrConflict
	}
	count, err := q.CountMonitoringTargetDependencies(ctx, db.CountMonitoringTargetDependenciesParams{EnterpriseID: a.EnterpriseID, TargetType: "monitoring_target", TargetID: m.ID})
	if err != nil {
		return p, err
	}
	if count != 0 {
		return p, ErrDependency
	}
	p.References["monitoring_target:"+m.ID.String()] = row.Version
	p.TargetName = row.Name
	return p, nil
}

func (s Service) commitTargetArchive(ctx context.Context, q *db.Queries, p actionPlan) (resource.ActionCommitResult, error) {
	// Recheck under the same transaction and lock that serializes new target
	// dependencies. No cascading delete, secret retirement or history rewrite.
	if _, err := s.prepareTargetArchive(ctx, q, p.Actor, p.Mutation, true); err != nil {
		return resource.ActionCommitResult{}, err
	}
	row, err := q.GetMonitoringTarget(ctx, db.GetMonitoringTargetParams{EnterpriseID: p.Actor.EnterpriseID, ID: p.Mutation.ID})
	if err != nil {
		return resource.ActionCommitResult{}, lookupError(err)
	}
	updated, err := q.UpdateMonitoringTarget(ctx, db.UpdateMonitoringTargetParams{EnterpriseID: p.Actor.EnterpriseID, ID: row.ID,
		Version: row.Version, Name: row.Name, IdentityConfig: row.IdentityConfig, Status: "archived"})
	if err != nil {
		return resource.ActionCommitResult{}, lookupError(err)
	}
	return resource.ActionCommitResult{ResourceType: "monitoring_target", ResourceID: updated.ID, ResourceVersion: updated.Version, Summary: "Monitoring target archived; historical identity retained"}, nil
}
