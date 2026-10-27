package kubernetesworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kuberneteshelm"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type HelmInput struct {
	Source           *kuberneteshelm.ChartSource `json:"source,omitempty"`
	NamespaceID      uuid.UUID                   `json:"namespace_id"`
	Operation        string                      `json:"operation"`
	Name             string                      `json:"name"`
	ExpectedRevision int                         `json:"expected_revision"`
	RollbackRevision int                         `json:"rollback_revision,omitempty"`
	TimeoutSeconds   int                         `json:"timeout_seconds"`
	Reason           string                      `json:"reason"`
}
type helmBackend interface {
	HelmReleases(context.Context, db.KubernetesCluster, kuberneteshelm.Request) ([]kuberneteshelm.Release, error)
	PreviewHelm(context.Context, db.KubernetesCluster, kuberneteshelm.Request) (kuberneteshelm.Result, error)
	ExecuteHelm(context.Context, db.KubernetesCluster, kuberneteshelm.Request) (kuberneteshelm.Result, error)
	ObserveHelm(context.Context, db.KubernetesCluster, kuberneteshelm.Request) (kuberneteshelm.Result, error)
}
type helmPlan struct {
	Source        *kuberneteshelm.ChartProvenance `json:"source,omitempty"`
	SchemaVersion string                          `json:"schema_version"`
	ID            uuid.UUID                       `json:"id"`
	Actor         Actor                           `json:"actor"`
	ClusterID     uuid.UUID                       `json:"cluster_id"`
	Input         HelmInput                       `json:"input"`
	Scope         Scope                           `json:"scope"`
	Request       kuberneteshelm.Request          `json:"request"`
	Objects       []kuberneteshelm.Object         `json:"objects"`
	Route         StreamRoute                     `json:"route"`
}

func (s Service) prepareHelm(ctx context.Context, a Actor, clusterID uuid.UUID, in HelmInput, id uuid.UUID, write bool) (helmPlan, db.KubernetesCluster, error) {
	plan := helmPlan{SchemaVersion: kuberneteshelm.SchemaVersion, ID: id, Actor: a, ClusterID: clusterID, Input: in}
	cluster, scope, e := s.resourceScope(ctx, a, clusterID)
	if e != nil {
		return plan, cluster, e
	}
	if write {
		if e = authorization.Authorize(ctx, s.Store.Queries, a, "kubernetes.helm.manage"); e != nil {
			return plan, cluster, e
		}
	}
	namespaces, _, e := s.selectedNamespaces(ctx, a, scope, Selection{Mode: "explicit", IDs: []uuid.UUID{in.NamespaceID}})
	if e != nil || len(namespaces) != 1 {
		return plan, cluster, authorization.ErrAccessDenied
	}
	if in.TimeoutSeconds == 0 {
		in.TimeoutSeconds = 120
	}
	if in.TimeoutSeconds < 10 || in.TimeoutSeconds > 240 || in.ExpectedRevision < 0 || in.Name == "" {
		return plan, cluster, kuberneteshelm.ErrInvalid
	}
	request := kuberneteshelm.Request{SchemaVersion: kuberneteshelm.SchemaVersion, OperationID: id.String(), APIServer: cluster.ApiServer, Namespace: kuberneteshelm.Namespace{Name: namespaces[0].Name, UID: namespaces[0].UID}, Name: in.Name, Operation: in.Operation, ExpectedRevision: in.ExpectedRevision, RollbackRevision: in.RollbackRevision, TimeoutSeconds: in.TimeoutSeconds}
	request.Source = in.Source
	request.Access.ClusterWide = scope.ClusterWide
	request.Access.Policy, e = mutationPermissions(ctx, s.Store.Queries, a, scope.ClusterWide)
	if e != nil {
		return plan, cluster, e
	}
	for _, n := range scope.Namespaces {
		request.Access.Namespaces = append(request.Access.Namespaces, kuberneteshelm.Namespace{Name: n.Name, UID: n.UID})
	}
	if scope.ClusterWide {
		request.Access.Namespaces = []kuberneteshelm.Namespace{request.Namespace}
	}
	for _, d := range kubernetesinventory.Definitions() {
		if d.ManagePermission == "" || slices.Contains(request.Access.Permissions, d.ManagePermission) {
			continue
		}
		if e := authorization.Authorize(ctx, s.Store.Queries, a, d.ManagePermission); e == nil {
			request.Access.Permissions = append(request.Access.Permissions, d.ManagePermission)
		} else if !errors.Is(e, authorization.ErrAccessDenied) {
			return plan, cluster, e
		}
	}
	if e := authorization.Authorize(ctx, s.Store.Queries, a, "kubernetes.custom_resource.manage"); e == nil {
		if !slices.Contains(request.Access.Permissions, "kubernetes.custom_resource.manage") {
			request.Access.Permissions = append(request.Access.Permissions, "kubernetes.custom_resource.manage")
		}
	} else if !errors.Is(e, authorization.ErrAccessDenied) {
		return plan, cluster, e
	}
	sort.Strings(request.Access.Permissions)
	plan.Scope, plan.Request = scope, request
	plan.Input = in
	plan.Route, e = currentClusterRoute(ctx, s.Store.Queries, cluster, kuberneteshelm.SchemaVersion)
	return plan, cluster, e
}
func authorizeHelmObjects(ctx context.Context, q *db.Queries, plan helmPlan) error {
	if len(plan.Objects) > kuberneteshelm.MaxObjects {
		return kuberneteshelm.ErrBudget
	}
	if e := authorization.Authorize(ctx, q, plan.Actor, "kubernetes.helm.manage"); e != nil {
		return e
	}
	if e := checkMutationPermissions(ctx, q, plan.Actor, plan.Scope.ClusterWide, plan.Request.Access.Policy); e != nil {
		return e
	}
	for _, o := range plan.Objects {
		if o.Name == "" || o.Definition.ManagePermission == "" || o.Before != nil || o.After != nil {
			return kuberneteshelm.ErrInvalid
		}
		if d, ok := kubernetesinventory.Lookup(o.Definition.Key); ok {
			if d.Group != o.Definition.Group || d.Resource != o.Definition.Resource || d.Kind != o.Definition.Kind || d.Namespaced != o.Definition.Namespaced || d.ManagePermission != o.Definition.ManagePermission {
				return kuberneteshelm.ErrInvalid
			}
		} else if (!strings.HasPrefix(o.Definition.Key, "custom/") && !strings.HasPrefix(o.Definition.Key, "pending-crd/")) || o.Definition.ManagePermission != "kubernetes.custom_resource.manage" {
			return kuberneteshelm.ErrInvalid
		}
		if e := authorization.Authorize(ctx, q, plan.Actor, o.Definition.ManagePermission); e != nil {
			return e
		}
		if o.Definition.Namespaced {
			if o.Namespace == "" {
				return kuberneteshelm.ErrInvalid
			}
			if !plan.Scope.ClusterWide {
				allowed := false
				for _, ns := range plan.Scope.Namespaces {
					if ns.Name == o.Namespace && ns.UID == o.NamespaceUID {
						if e := authorization.RequireResource(ctx, q, plan.Actor, "kubernetes_namespace", ns.ID); e != nil {
							return e
						}
						allowed = true
						break
					}
				}
				if !allowed {
					return authorization.ErrAccessDenied
				}
			}
		} else if !plan.Scope.ClusterWide {
			return authorization.ErrAccessDenied
		}
	}
	return nil
}
func (s Service) HelmReleases(ctx context.Context, a Actor, cluster, namespace uuid.UUID, name string, history bool) ([]kuberneteshelm.Release, error) {
	mode := "list"
	if history {
		mode = "history"
	} else {
		name = "all"
	}
	plan, target, e := s.prepareHelm(ctx, a, cluster, HelmInput{NamespaceID: namespace, Name: name, Operation: mode, TimeoutSeconds: 15}, uuid.Nil, false)
	if e != nil {
		return nil, e
	}
	backend, ok := s.Backend.(helmBackend)
	if !ok {
		return nil, kuberneteshelm.ErrUnavailable
	}
	rows, e := backend.HelmReleases(ctx, target, plan.Request)
	if e != nil {
		return nil, e
	}
	if _, _, e = s.prepareHelm(ctx, a, cluster, plan.Input, uuid.Nil, false); e != nil {
		return nil, e
	}
	return rows, nil
}
func helmSnapshot(plan helmPlan) any {
	return struct {
		Cluster   uuid.UUID
		Version   int64
		Namespace uuid.UUID
		Request   kuberneteshelm.Request
		Route     StreamRoute
	}{plan.ClusterID, plan.Scope.ClusterVersion, plan.Input.NamespaceID, plan.Request, plan.Route}
}
func helmPublicResult(result kuberneteshelm.Result) []byte {
	result.SealedPlan = nil
	result.EncryptedChart = nil
	for i := range result.Objects {
		result.Objects[i].Before, result.Objects[i].After = nil, nil
	}
	b, _ := json.Marshal(result)
	return b
}
