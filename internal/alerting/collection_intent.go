package alerting

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
	"github.com/kakj-go/Argus/internal/observationscope"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Only an explicit stop for every metric collection of every selected resource
// suspends the entire policy. Missing configurations, failed collectors and a
// partially stopped multi-resource scope continue through normal evaluation.
func metricScopeExplicitlyStopped(ctx context.Context, q *db.Queries, a Actor, scope observationscope.Snapshot) (bool, error) {
	if len(scope.Resources) == 0 {
		return false, nil
	}
	for _, resource := range scope.Resources {
		// Middleware targets are collected by another resource's Collector.
		// Their own collection intent, not a Collector with the target's ID,
		// determines whether evaluation was explicitly paused.
		if resource.Kind == "monitoring_target" {
			target, err := q.GetMonitoringTarget(ctx, db.GetMonitoringTargetParams{EnterpriseID: a.EnterpriseID, ID: resource.ID})
			if errors.Is(err, pgx.ErrNoRows) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			entry, known := monitoringcatalog.Lookup(target.Engine)
			if target.Status != "active" || !known || !slices.Contains(entry.Signals, "metrics") {
				return false, nil
			}
			collection, err := q.GetMonitoringCollectionForTarget(ctx, db.GetMonitoringCollectionForTargetParams{
				EnterpriseID: a.EnterpriseID, TargetType: resource.Kind, TargetID: resource.ID, IntegrationKey: target.Engine,
			})
			if errors.Is(err, pgx.ErrNoRows) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			if collection.DesiredEnabled {
				return false, nil
			}
			continue
		}
		collector, err := q.GetCollectorForResource(ctx, db.GetCollectorForResourceParams{EnterpriseID: a.EnterpriseID, ResourceType: resource.Kind, ResourceID: resource.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		collections, err := q.ListCollectorMonitoringCollections(ctx, db.ListCollectorMonitoringCollectionsParams{EnterpriseID: a.EnterpriseID, CollectorID: nullID(collector.ID)})
		if err != nil {
			return false, err
		}
		found := false
		for _, collection := range collections {
			if collection.TargetType != resource.Kind || collection.TargetID != resource.ID {
				continue
			}
			_, fixed := builtincollection.Lookup(collection.IntegrationKey)
			entry, known := monitoringcatalog.Lookup(collection.IntegrationKey)
			if !fixed && (!known || !slices.Contains(entry.Signals, "metrics")) {
				continue
			}
			found = true
			if collection.DesiredEnabled {
				return false, nil
			}
		}
		if !found {
			return false, nil
		}
	}
	return true, nil
}
