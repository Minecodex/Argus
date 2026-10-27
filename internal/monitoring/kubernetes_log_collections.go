package monitoring

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/logcollection"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// The physical Collector remains cluster-owned; a log assignment can be
// managed under its UID-bound Namespace plus its business application grant.
func requireLogCollection(ctx context.Context, q *db.Queries, a Actor, row db.MonitoringCollection) error {
	if row.IntegrationKey != "k8slog" {
		return authorization.RequireResource(ctx, q, a, row.TargetType, row.TargetID)
	}
	var config logcollection.Config
	if json.Unmarshal(row.Config, &config) != nil || config.Workload == nil {
		return ErrInvalid
	}
	w := config.Workload
	ns, err := q.GetKubernetesNamespaceResource(ctx, db.GetKubernetesNamespaceResourceParams{EnterpriseID: a.EnterpriseID, ID: w.NamespaceID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err != nil || ns.NamespaceUid != w.NamespaceUID {
		return ErrDenied
	}
	if ns.Status == "active" {
		if err = authorization.RequireResource(ctx, q, a, "kubernetes_namespace", ns.ID); err != nil {
			return err
		}
	} else if err = authorization.RequireResource(ctx, q, a, "kubernetes_cluster", ns.ClusterID); err != nil {
		return err
	}
	if row.TargetType != "kubernetes_cluster" {
		return authorization.RequireResource(ctx, q, a, row.TargetType, row.TargetID)
	}
	if row.TargetID != ns.ClusterID {
		return ErrInvalid
	}
	return nil
}

func (s Service) prepareKubernetesLogWorkload(ctx context.Context, q *db.Queries, p *actionPlan, collector db.CollectorInstance, config *logcollection.Config, lock bool) error {
	w := config.Workload
	if w == nil || w.NamespaceID == uuid.Nil {
		return ErrInvalid
	}
	ns, err := q.GetKubernetesNamespaceResource(ctx, db.GetKubernetesNamespaceResourceParams{EnterpriseID: p.Actor.EnterpriseID, ID: w.NamespaceID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err != nil || ns.ClusterID != collector.ResourceID {
		return ErrDenied
	}
	stopping := p.Mutation.Operation == "archive" || p.Mutation.Operation == "set_enabled" && !p.Mutation.Collection.Enabled
	if ns.Status == "active" {
		if err = authorization.RequireResource(ctx, q, p.Actor, "kubernetes_namespace", ns.ID); err != nil {
			return err
		}
	} else if !stopping {
		return ErrConflict
	} else if err = authorization.RequireResource(ctx, q, p.Actor, "kubernetes_cluster", ns.ClusterID); err != nil {
		return err
	}
	p.References["kubernetes_namespace:"+ns.ID.String()] = ns.Version
	if stopping {
		if w.NamespaceUID != ns.NamespaceUid || w.Namespace != ns.Name {
			return ErrInvalid
		}
		return nil
	}
	resolved, err := s.resolvedWorkload(ctx, p.Actor, collector.ResourceID, *w, lock)
	if err != nil {
		if errors.Is(err, kubernetesinventory.ErrIdentityChanged) {
			return ErrConflict
		}
		if errors.Is(err, kubernetesinventory.ErrInvalid) {
			return ErrInvalid
		}
		return err
	}
	if resolved.NamespaceID != ns.ID || resolved.NamespaceUID != ns.NamespaceUid || resolved.Namespace != ns.Name || resolved.Type != w.Type || resolved.Name != w.Name || resolved.UID != w.UID {
		return ErrConflict
	}
	config.Workload = &resolved
	return nil
}
