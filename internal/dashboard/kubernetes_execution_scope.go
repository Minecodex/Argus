package dashboard

import (
	"context"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"slices"
)

func (r Runtime) normalizeKubernetesScope(ctx context.Context, a Actor, in ExecutionInput) (ExecutionInput, error) {
	scope := in.KubernetesScope
	if scope == nil {
		return in, nil
	}
	if scope.ClusterID == uuid.Nil || len(scope.NamespaceIDs) > 1000 || len(scope.FocusUID) > 128 || in.ResourceMode != "explicit" {
		return in, ErrInvalid
	}
	for _, id := range in.ResourceIDs {
		if id != scope.ClusterID && !slices.Contains(scope.NamespaceIDs, id) {
			return in, ErrDenied
		}
	}
	if len(scope.NamespaceIDs) > 0 {
		rows, err := r.Store.Queries.ListAuthorizedKubernetesNamespaceResources(ctx, db.ListAuthorizedKubernetesNamespaceResourcesParams{EnterpriseID: a.EnterpriseID, NamespaceIds: scope.NamespaceIDs, ClusterIds: []uuid.UUID{}})
		if err != nil {
			return in, err
		}
		if len(rows) != len(scope.NamespaceIDs) {
			return in, ErrDenied
		}
		for _, row := range rows {
			if row.ClusterID != scope.ClusterID {
				return in, ErrDenied
			}
		}
		in.ResourceIDs = slices.Clone(scope.NamespaceIDs)
		in.ResourceTypes = []string{"kubernetes_namespace"}
	} else {
		in.ResourceIDs = []uuid.UUID{scope.ClusterID}
		in.ResourceTypes = []string{"kubernetes_cluster"}
	}
	return in, nil
}
