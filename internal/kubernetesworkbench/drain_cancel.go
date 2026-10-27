package kubernetesworkbench

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// CancelDrain stops future batches, without claiming to undo requests already
// dispatched to Kubernetes or silently restoring node scheduling.
func (s Service) CancelDrain(ctx context.Context, a Actor, clusterID, id uuid.UUID, version int64) (Operation, error) {
	if _, err := s.GetOperation(ctx, a, clusterID, id); err != nil {
		return Operation{}, err
	}
	var output Operation
	err := s.Store.InTx(ctx, func(q *db.Queries) error {
		var err error
		output, err = cancelDrainWithQueries(ctx, q, a, clusterID, id, version)
		return err
	})
	if err != nil {
		return Operation{}, err
	}
	return output, nil
}

func cancellableDrain(ctx context.Context, q *db.Queries, a Actor, clusterID, id uuid.UUID, version int64) (db.KubernetesOperation, error) {
	var empty db.KubernetesOperation
	for _, permission := range []string{"kubernetes.read", "kubernetes.node.manage", "kubernetes.node.drain"} {
		if err := authorization.Authorize(ctx, q, a, permission); err != nil {
			return empty, err
		}
	}
	if err := authorization.RequireResource(ctx, q, a, "kubernetes_cluster", clusterID); err != nil {
		return empty, err
	}
	row, err := q.LockKubernetesOperation(ctx, db.LockKubernetesOperationParams{EnterpriseID: a.EnterpriseID, ID: id})
	if err != nil {
		return empty, err
	}
	var plan mutationPlan
	if row.ClusterID != clusterID || row.Operation != "drain" || json.Unmarshal(row.PrivatePlan, &plan) != nil || plan.SchemaVersion != kubernetesinventory.MutationSchemaVersion || !plan.Scope.ClusterWide {
		return empty, kubernetesinventory.ErrMutationInvalid
	}
	if row.Status != "cancelled" && (row.Version != version || row.Status != "queued" && row.Status != "running" && row.Status != "observing" && row.Status != "result_unknown") {
		return empty, kubernetesinventory.ErrMutationConflict
	}
	return row, nil
}

func cancelDrainWithQueries(ctx context.Context, q *db.Queries, a Actor, clusterID, id uuid.UUID, version int64) (Operation, error) {
	row, err := cancellableDrain(ctx, q, a, clusterID, id, version)
	if err != nil || row.Status == "cancelled" {
		return operationDTO(row), err
	}
	row, err = q.CancelKubernetesDrain(ctx, db.CancelKubernetesDrainParams{EnterpriseID: a.EnterpriseID, ID: id, Version: version})
	if err != nil {
		return Operation{}, err
	}
	actor := a.SubjectType
	if actor == "user" {
		actor = "enterprise_user"
	}
	_, err = audit.Append(ctx, q, audit.Entry{Domain: "enterprise", EnterpriseID: uuid.NullUUID{UUID: a.EnterpriseID, Valid: true}, ActorType: actor, ActorID: a.SubjectID.String(), Action: "kubernetes.drain.cancel", ResourceType: "kubernetes_operation", ResourceID: id.String(), Result: "success", Details: map[string]any{"cluster_id": clusterID.String(), "version": row.Version, "in_flight_batch_may_finish": true, "node_stays_cordoned": true}})
	return operationDTO(row), err
}
