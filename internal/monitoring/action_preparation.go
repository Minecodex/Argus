package monitoring

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/logcollection"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type workloadSnapshotKey struct{}

// This capability only exists in the worker's request context. It is never
// accepted from the preview, task payload, model or browser.
type workloadSnapshot struct {
	actor     Actor
	clusterID uuid.UUID
	requested [32]byte
	resolved  logcollection.Workload
	checkedAt time.Time
	actionID  uuid.UUID
	planHash  [32]byte
}

type workloadCapture struct {
	resolver LogWorkloadResolver
	now      func() time.Time
	snapshot *workloadSnapshot
}

func (c *workloadCapture) ResolveLogWorkload(ctx context.Context, a Actor, cluster uuid.UUID, w logcollection.Workload) (logcollection.Workload, error) {
	if c.resolver == nil {
		return w, ErrUnavailable
	}
	raw, _ := json.Marshal(w)
	resolved, err := c.resolver.ResolveLogWorkload(ctx, a, cluster, w)
	if err == nil {
		c.snapshot = &workloadSnapshot{actor: a, clusterID: cluster, requested: sha256.Sum256(raw), resolved: resolved, checkedAt: c.now()}
	}
	return resolved, err
}

// prepareExternalReads runs the existing authorization and identity validation
// without a database transaction. Only K8s workload reads need this boundary:
// Connector queries create their own commands/leases and wait for remote I/O.
func (s Service) prepareExternalReads(ctx context.Context, a Actor, m Mutation) (context.Context, error) {
	if m.Kind != "collection" || m.Collection == nil || m.Operation == "archive" || m.Operation == "set_enabled" && !m.Collection.Enabled {
		return ctx, nil
	}
	integration := m.Collection.Integration
	if m.Operation == "set_enabled" {
		row, err := s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: a.EnterpriseID, ID: m.ID})
		if err != nil {
			return ctx, lookupError(err)
		}
		integration = row.IntegrationKey
	}
	if integration != "k8slog" {
		return ctx, nil
	}
	capture := &workloadCapture{resolver: s.Workloads, now: s.now}
	preflight := s
	preflight.Workloads = capture
	if _, err := preflight.prepare(ctx, s.Store.Queries, a, m, false); err != nil {
		return ctx, err
	}
	if capture.snapshot == nil {
		return ctx, ErrUnavailable
	}
	return context.WithValue(ctx, workloadSnapshotKey{}, *capture.snapshot), nil
}

func (e ActionExtension) PrepareActionExecution(ctx context.Context, action db.PendingAction, raw json.RawMessage) (context.Context, error) {
	if !monitoringAction(action) {
		if next, ok := e.Next.(resource.ActionExecutionPreparer); ok {
			return next.PrepareActionExecution(ctx, action, raw)
		}
		return ctx, nil
	}
	var plan actionPlan
	if json.Unmarshal(raw, &plan) != nil || plan.SchemaVersion != "argus.monitoring_action/v1" {
		return ctx, ErrInvalid
	}
	a := actionActor(action)
	if plan.Actor != a {
		return ctx, ErrDenied
	}
	prepared, err := e.Service.prepareExternalReads(ctx, a, plan.Mutation)
	if err != nil {
		return ctx, err
	}
	if snapshot, ok := prepared.Value(workloadSnapshotKey{}).(workloadSnapshot); ok {
		snapshot.actionID, snapshot.planHash = action.ID, sha256.Sum256(raw)
		prepared = context.WithValue(prepared, workloadSnapshotKey{}, snapshot)
	}
	return prepared, nil
}

func actionActor(a db.PendingAction) Actor {
	return Actor{EnterpriseID: a.EnterpriseID, SubjectID: a.CreatorSubjectID, SubjectType: a.CreatorSubjectType, AuthorizationVersion: a.AuthorizationVersion, RunID: a.RunID.UUID}
}

func (s Service) resolvedWorkload(ctx context.Context, a Actor, cluster uuid.UUID, w logcollection.Workload, lock bool) (logcollection.Workload, error) {
	if !lock {
		if s.Workloads == nil {
			return w, ErrUnavailable
		}
		return s.Workloads.ResolveLogWorkload(ctx, a, cluster, w)
	}
	snapshot, ok := ctx.Value(workloadSnapshotKey{}).(workloadSnapshot)
	raw, _ := json.Marshal(w)
	age := s.now().Sub(snapshot.checkedAt)
	if !ok || snapshot.actor != a || snapshot.clusterID != cluster || snapshot.requested != sha256.Sum256(raw) || age < 0 || age > 30*time.Second {
		return w, resource.ErrActionUnavailable
	}
	return snapshot.resolved, nil
}
