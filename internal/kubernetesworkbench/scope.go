package kubernetesworkbench

import (
	"context"
	"errors"
	"slices"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type Backend interface {
	ListInventory(context.Context, db.KubernetesCluster, kubernetesinventory.Query) (kubernetesinventory.Page, error)
	GetInventory(context.Context, db.KubernetesCluster, kubernetesinventory.Query) (kubernetesinventory.Object, error)
}
type Service struct {
	Store       *postgres.Store
	Backend     Backend
	CursorKey   []byte
	Actions     resource.PendingActionService
	Credentials secret.Service
}
type Actor = authorization.Actor
type Namespace struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	UID     string    `json:"uid"`
	Version int64     `json:"version"`
}
type Scope struct {
	ClusterID      uuid.UUID   `json:"cluster_id"`
	ClusterName    string      `json:"cluster_name"`
	ClusterVersion int64       `json:"cluster_version"`
	ClusterWide    bool        `json:"cluster_wide"`
	Namespaces     []Namespace `json:"namespaces"`
}
type Selection struct {
	Mode string      `json:"mode"`
	IDs  []uuid.UUID `json:"ids"`
}

func (s Service) scope(ctx context.Context, a Actor, clusterID uuid.UUID) (db.KubernetesCluster, Scope, error) {
	if s.Backend == nil {
		return db.KubernetesCluster{}, Scope{}, kubernetesinventory.ErrUnavailable
	}
	return s.resourceScope(ctx, a, clusterID)
}

// resourceScope is also used by Gateway write permits, where authorization
// reads authoritative storage without requiring an object transport backend.
func (s Service) resourceScope(ctx context.Context, a Actor, clusterID uuid.UUID) (db.KubernetesCluster, Scope, error) {
	out := Scope{ClusterID: clusterID, Namespaces: []Namespace{}}
	if s.Store == nil {
		return db.KubernetesCluster{}, out, kubernetesinventory.ErrUnavailable
	}
	q := s.Store.Queries
	if err := authorization.Authorize(ctx, q, a, "kubernetes.read"); err != nil {
		return db.KubernetesCluster{}, out, err
	}
	cluster, err := q.GetKubernetesCluster(ctx, db.GetKubernetesClusterParams{EnterpriseID: a.EnterpriseID, ID: clusterID})
	if errors.Is(err, pgx.ErrNoRows) {
		return cluster, out, authorization.ErrAccessDenied
	}
	if err != nil {
		return cluster, out, err
	}
	if cluster.Status != "active" {
		return cluster, out, authorization.ErrAccessDenied
	}
	out.ClusterVersion = cluster.ResourceVersion
	out.ClusterName = cluster.Name
	clusterIDs, err := authorization.AuthorizedResourceIDs(ctx, q, a, "kubernetes_cluster")
	if err != nil {
		return cluster, out, err
	}
	if slices.Contains(clusterIDs, clusterID) {
		out.ClusterWide = true
		return cluster, out, nil
	}
	ids, err := authorization.AuthorizedResourceIDs(ctx, q, a, "kubernetes_namespace")
	if err != nil {
		return cluster, out, err
	}
	rows, err := q.ListAuthorizedKubernetesNamespaceResources(ctx, db.ListAuthorizedKubernetesNamespaceResourcesParams{EnterpriseID: a.EnterpriseID, NamespaceIds: ids, ClusterIds: []uuid.UUID{}})
	if err != nil {
		return cluster, out, err
	}
	for _, row := range rows {
		if row.ClusterID == clusterID {
			out.Namespaces = append(out.Namespaces, Namespace{ID: row.ID, Name: row.Name, UID: row.NamespaceUid, Version: row.Version})
		}
	}
	if len(out.Namespaces) == 0 {
		return cluster, out, authorization.ErrAccessDenied
	}
	if len(out.Namespaces) > 1000 {
		return cluster, out, kubernetesinventory.ErrBudget
	}
	sort.Slice(out.Namespaces, func(i, j int) bool { return out.Namespaces[i].ID.String() < out.Namespaces[j].ID.String() })
	return cluster, out, nil
}
func (s Service) AccessScope(ctx context.Context, a Actor, cluster uuid.UUID) (Scope, error) {
	_, scope, err := s.scope(ctx, a, cluster)
	return scope, err
}

// DirectoryClusterIDs exposes parent navigation for a Namespace grant without
// granting the Cluster as a telemetry root or management target.
func (s Service) DirectoryClusterIDs(ctx context.Context, a Actor) ([]uuid.UUID, error) {
	if s.Store == nil {
		return nil, kubernetesinventory.ErrUnavailable
	}
	q := s.Store.Queries
	if err := authorization.Authorize(ctx, q, a, "kubernetes.read"); err != nil {
		return nil, err
	}
	ids, err := authorization.AuthorizedResourceIDs(ctx, q, a, "kubernetes_cluster")
	if err != nil {
		return nil, err
	}
	namespaces, err := authorization.AuthorizedResourceIDs(ctx, q, a, "kubernetes_namespace")
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAuthorizedKubernetesNamespaceResources(ctx, db.ListAuthorizedKubernetesNamespaceResourcesParams{EnterpriseID: a.EnterpriseID, ClusterIds: []uuid.UUID{}, NamespaceIds: namespaces})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if !slices.Contains(ids, row.ClusterID) {
			ids = append(ids, row.ClusterID)
		}
	}
	return ids, authorization.Authorize(ctx, q, a, "kubernetes.read")
}

func (s Service) selectedNamespaces(ctx context.Context, a Actor, scope Scope, selection Selection) ([]Namespace, bool, error) {
	if selection.Mode != "all_authorized" && selection.Mode != "explicit" || selection.Mode == "all_authorized" && len(selection.IDs) > 0 || len(selection.IDs) > 1000 {
		return nil, false, kubernetesinventory.ErrInvalid
	}
	if selection.Mode == "all_authorized" {
		return scope.Namespaces, scope.ClusterWide, nil
	}
	result := []Namespace{}
	seen := map[uuid.UUID]bool{}
	for _, id := range selection.IDs {
		if id == uuid.Nil || seen[id] {
			return nil, false, kubernetesinventory.ErrInvalid
		}
		seen[id] = true
		if scope.ClusterWide {
			row, err := s.Store.Queries.GetKubernetesNamespaceResource(ctx, db.GetKubernetesNamespaceResourceParams{EnterpriseID: a.EnterpriseID, ID: id})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil, false, authorization.ErrAccessDenied
				}
				return nil, false, err
			}
			if row.ClusterID != scope.ClusterID || row.Status != "active" {
				return nil, false, authorization.ErrAccessDenied
			}
			result = append(result, Namespace{ID: row.ID, Name: row.Name, UID: row.NamespaceUid, Version: row.Version})
		} else {
			index := slices.IndexFunc(scope.Namespaces, func(n Namespace) bool { return n.ID == id })
			if index < 0 {
				return nil, false, authorization.ErrAccessDenied
			}
			result = append(result, scope.Namespaces[index])
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID.String() < result[j].ID.String() })
	return result, false, nil
}
func scopeSignature(scope Scope, namespaces []Namespace) string {
	return pagination.HashFilter(struct {
		Scope      Scope
		Namespaces []Namespace
	}{scope, namespaces})
}
