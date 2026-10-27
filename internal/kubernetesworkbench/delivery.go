package kubernetesworkbench

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// ValidateDelivery rechecks the approved actor and scope immediately before a
// queued physical mutation enters a Connector's execution channel.
func ValidateDelivery(ctx context.Context, q *db.Queries, enterprise, cluster, id uuid.UUID, raw []byte) error {
	return validateDelivery(ctx, q, enterprise, cluster, id, raw, false)
}

func ValidateDrainDelivery(ctx context.Context, q *db.Queries, enterprise, cluster, id uuid.UUID, raw []byte) error {
	return validateDelivery(ctx, q, enterprise, cluster, id, raw, true)
}

func validateDelivery(ctx context.Context, q *db.Queries, enterprise, cluster, id uuid.UUID, raw []byte, advance bool) error {
	row, err := q.GetKubernetesOperation(ctx, db.GetKubernetesOperationParams{EnterpriseID: enterprise, ID: id})
	if err != nil {
		return err
	}
	if row.ClusterID != cluster || !advance && row.Status != "running" && row.Status != "result_unknown" || advance && row.Status != "observing" && row.Status != "result_unknown" {
		return resource.ErrActionInvalidated
	}
	var plan mutationPlan
	if json.Unmarshal(row.PrivatePlan, &plan) != nil || plan.SchemaVersion != kubernetesinventory.MutationSchemaVersion || plan.ID != id || plan.Input.ClusterID != cluster {
		return resource.ErrActionInvalidated
	}
	if advance && (plan.Mutation.Operation != "drain" || plan.Mutation.DrainPlan == nil) {
		return resource.ErrActionInvalidated
	}
	expected, _ := json.Marshal(plan.Mutation)
	expected, err = resource.CanonicalJSON(expected)
	if err != nil {
		return err
	}
	actual, err := resource.CanonicalJSON(raw)
	if err != nil || !bytes.Equal(actual, expected) {
		return resource.ErrActionInvalidated
	}
	actor := Actor{EnterpriseID: enterprise, SubjectID: row.ActorID, SubjectType: row.ActorType, AuthorizationVersion: row.AuthorizationVersion}
	if err = authorization.Authorize(ctx, q, actor, plan.Definition.ManagePermission); err != nil {
		return err
	}
	if plan.Mutation.Operation == "drain" {
		if !plan.Scope.ClusterWide {
			return authorization.ErrAccessDenied
		}
		if err = authorization.Authorize(ctx, q, actor, "kubernetes.node.drain"); err != nil {
			return err
		}
	}
	if err = checkMutationPermissions(ctx, q, actor, plan.Scope.ClusterWide, plan.Mutation.Policy); err != nil {
		return err
	}
	current, err := q.GetKubernetesCluster(ctx, db.GetKubernetesClusterParams{EnterpriseID: enterprise, ID: cluster})
	if err != nil {
		return err
	}
	if current.Status != "active" || current.ResourceVersion != row.ClusterVersion {
		return resource.ErrActionInvalidated
	}
	if plan.Scope.ClusterWide {
		return authorization.RequireResource(ctx, q, actor, "kubernetes_cluster", cluster)
	}
	if !row.NamespaceID.Valid {
		return authorization.ErrAccessDenied
	}
	if err = authorization.RequireResource(ctx, q, actor, "kubernetes_namespace", row.NamespaceID.UUID); err != nil {
		return err
	}
	namespace, err := q.GetKubernetesNamespaceResource(ctx, db.GetKubernetesNamespaceResourceParams{EnterpriseID: enterprise, ID: row.NamespaceID.UUID})
	if err != nil {
		return err
	}
	if namespace.ClusterID != cluster || namespace.Status != "active" || namespace.NamespaceUid != row.NamespaceUid || namespace.Name != plan.Mutation.Query.Namespace {
		return kubernetesinventory.ErrIdentityChanged
	}
	return nil
}
