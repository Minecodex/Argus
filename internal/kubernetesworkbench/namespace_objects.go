package kubernetesworkbench

import (
	"context"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Native receivers expose immutable workload/Pod UIDs but generally only the
// Namespace name. The existing inventory adapter verifies each object's live
// Namespace generation before returning it. Persist only that identity, never
// the object body, labels, configuration or Secret values.
func (s Service) readNamespaceObjectIdentities(ctx context.Context, cluster db.KubernetesCluster) ([]kubernetesinventory.Object, error) {
	objects := []kubernetesinventory.Object{}
	for _, kind := range []string{"pod", "deployment", "replicaset", "daemonset", "statefulset", "job", "cronjob", "pvc"} {
		cursor, version := "", ""
		seen := map[string]bool{}
		for {
			page, err := s.Backend.ListInventory(ctx, cluster, kubernetesinventory.Query{Type: kind, AllNamespaces: true, Limit: 100, Continue: cursor})
			if err != nil {
				return nil, err
			}
			if len(objects)+len(page.Items) > 10000 || len(seen) >= 100 || len(page.Items) > 100 {
				return nil, kubernetesinventory.ErrBudget
			}
			if version != "" && page.ResourceVersion != version {
				return nil, kubernetesinventory.ErrIdentityChanged
			}
			version = page.ResourceVersion
			for _, item := range page.Items {
				if item.Definition.Key != kind || item.Namespace == "" || item.NamespaceUID == "" || item.UID == "" || len(item.UID) > 128 || len(item.NamespaceUID) > 128 {
					return nil, kubernetesinventory.ErrInvalid
				}
				objects = append(objects, item)
			}
			if page.Continue == "" {
				if !page.Complete {
					return nil, kubernetesinventory.ErrInvalid
				}
				break
			}
			if seen[page.Continue] {
				return nil, kubernetesinventory.ErrInvalid
			}
			cursor, seen[page.Continue] = page.Continue, true
		}
	}
	return objects, nil
}

func applyNamespaceObjectIdentities(ctx context.Context, q *db.Queries, enterprise, cluster uuid.UUID, objects []kubernetesinventory.Object) error {

	for _, object := range objects {
		if parsed, err := uuid.Parse(object.NamespaceUID); err != nil || parsed == uuid.Nil || parsed.String() != object.NamespaceUID {
			return kubernetesinventory.ErrInvalid
		}
		if err := q.ObserveKubernetesNamespaceObjectIdentity(ctx, db.ObserveKubernetesNamespaceObjectIdentityParams{EnterpriseID: enterprise, ClusterID: cluster, ObjectUid: object.UID, ObjectType: object.Definition.Key, NamespaceName: object.Namespace, NamespaceUid: object.NamespaceUID}); err != nil {
			return err
		}
	}
	return nil
}
