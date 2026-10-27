package kubernetesworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type Worker struct {
	Service Service
	Logger  *slog.Logger
}
type operationTask struct {
	EnterpriseID uuid.UUID `json:"enterprise_id"`
	OperationID  uuid.UUID `json:"operation_id"`
}

func (w Worker) Handle(ctx context.Context, task runtime.Task) error {
	var input operationTask
	if json.Unmarshal(task.Payload, &input) != nil || input.EnterpriseID == uuid.Nil || input.OperationID == uuid.Nil || !task.EnterpriseID.Valid || task.EnterpriseID.UUID != input.EnterpriseID {
		return runtime.Error{ErrorCode: "KUBERNETES_OPERATION_TASK_INVALID", Permanent: true}
	}
	return w.execute(ctx, input)
}

func (w Worker) execute(ctx context.Context, input operationTask) error {
	if w.Service.Store == nil {
		return kubernetesinventory.ErrUnavailable
	}
	q := w.Service.Store.Queries
	row, err := q.StartKubernetesOperation(ctx, db.StartKubernetesOperationParams{EnterpriseID: input.EnterpriseID, ID: input.OperationID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	} // Atomic ownership: never replay an already-started write.
	if err != nil {
		return err
	}
	if row.TypeKey == "helm_release" {
		return w.executeHelm(ctx, row)
	}
	backend, ok := w.Service.Backend.(mutationBackend)
	if !ok {
		return w.finish(ctx, row, "failed", nil, kubernetesinventory.ErrUnavailable.Error())
	}
	var plan mutationPlan
	if json.Unmarshal(row.PrivatePlan, &plan) != nil {
		return w.finish(ctx, row, "failed", nil, "KUBERNETES_OPERATION_PLAN_INVALID")
	}
	raw, _ := json.Marshal(plan.Mutation)
	if err = ValidateDelivery(ctx, q, row.EnterpriseID, row.ClusterID, row.ID, raw); err != nil {
		return w.finish(ctx, row, "failed", nil, operationErrorCode(err))
	}
	current, cluster, err := w.Service.prepareMutation(ctx, plan.Actor, plan.Input, plan.ID, plan.Mutation.ID)
	if err != nil {
		return w.finish(ctx, row, "failed", nil, operationErrorCode(err))
	}
	if current.Definition != plan.Definition || current.Mutation.Query.NamespaceUID != plan.Mutation.Query.NamespaceUID {
		return w.finish(ctx, row, "failed", nil, kubernetesinventory.ErrIdentityChanged.Error())
	}
	callCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-callCtx.Done():
				return
			case <-ticker.C:
				if e := q.HeartbeatKubernetesOperation(callCtx, db.HeartbeatKubernetesOperationParams{EnterpriseID: row.EnterpriseID, ID: row.ID}); e != nil {
					cancel()
					return
				}
			}
		}
	}()
	result, err := backend.MutateInventory(callCtx, cluster, plan.Mutation, false)
	cancel()
	<-heartbeatDone
	// Persist using the worker lifetime, not the completed physical-call context.
	if err != nil {
		if definiteMutationFailure(err) {
			return w.finish(ctx, row, "failed", nil, operationErrorCode(err))
		}
		_, e := q.MarkKubernetesOperationUnknown(ctx, db.MarkKubernetesOperationUnknownParams{EnterpriseID: row.EnterpriseID, ID: row.ID, ErrorCode: kubernetesinventory.ErrorCode(kubernetesinventory.ErrMutationUnknown)})
		return e
	}
	if !result.Accepted || result.DryRun {
		_, err = q.MarkKubernetesOperationUnknown(ctx, db.MarkKubernetesOperationUnknownParams{EnterpriseID: row.EnterpriseID, ID: row.ID, ErrorCode: kubernetesinventory.ErrProtocol.Error()})
		return err
	}
	encoded := publicMutationResult(result)
	count, err := q.ObserveKubernetesOperation(ctx, db.ObserveKubernetesOperationParams{EnterpriseID: row.EnterpriseID, ID: row.ID, Result: encoded})
	if err != nil || count != 1 {
		return err
	}
	row.Status, row.Result = "observing", encoded
	return w.observe(ctx, row)
}

func definiteMutationFailure(err error) bool {
	if errors.Is(err, authorization.ErrAccessDenied) || errors.Is(err, secret.ErrKubernetesPrivateInput) {
		return true
	}
	for _, known := range []error{kubernetesinventory.ErrMutationConflict, kubernetesinventory.ErrMutationInvalid, kubernetesinventory.ErrForbidden, kubernetesinventory.ErrIdentityChanged, kubernetesinventory.ErrUnsupported, kubernetesinventory.ErrInvalid, kubernetesinventory.ErrDrainUnsafe} {
		if errors.Is(err, known) {
			return true
		}
	}
	return false
}
func operationErrorCode(err error) string {
	if errors.Is(err, secret.ErrKubernetesPrivateInput) {
		return "KUBERNETES_PRIVATE_INPUT_UNAVAILABLE"
	}
	if errors.Is(err, authorization.ErrAccessDenied) {
		return "PERMISSION_DENIED"
	}
	if errors.Is(err, resource.ErrActionInvalidated) {
		return "ACTION_INVALIDATED"
	}
	return kubernetesinventory.ErrorCode(err)
}
func publicMutationResult(result kubernetesinventory.MutationResult) []byte {
	// Digests and operation markers are control-plane evidence, never user/Agent data.
	value := map[string]any{"accepted": result.Accepted, "deleted": result.Deleted, "delete_requested": result.DeleteRequested, "before": previewManifest(result.Before), "after": previewManifest(result.After)}
	if result.Drain != nil {
		value["drain"] = result.Drain
	}
	if result.After != nil {
		value["uid"], value["resource_version"], value["generation"] = result.After.UID, result.After.ResourceVersion, result.After.Generation
	}
	raw, _ := json.Marshal(value)
	return raw
}
func (w Worker) finish(ctx context.Context, row db.KubernetesOperation, status string, result []byte, code string) error {
	if len(result) == 0 {
		result = []byte(`{}`)
	}
	_, err := w.Service.Store.Queries.FinishKubernetesOperation(ctx, db.FinishKubernetesOperationParams{EnterpriseID: row.EnterpriseID, ID: row.ID, Status: status, Result: result, ErrorCode: code})
	return err
}

func (w Worker) observe(ctx context.Context, row db.KubernetesOperation) error {
	if row.TypeKey == "helm_release" {
		return w.observeHelm(ctx, row)
	}
	var plan mutationPlan
	if json.Unmarshal(row.PrivatePlan, &plan) != nil || plan.SchemaVersion != kubernetesinventory.MutationSchemaVersion {
		return w.finish(ctx, row, "failed", row.Result, kubernetesinventory.ErrProtocol.Error())
	}
	cluster, err := w.Service.Store.Queries.GetKubernetesCluster(ctx, db.GetKubernetesClusterParams{EnterpriseID: row.EnterpriseID, ID: row.ClusterID})
	if err != nil {
		return err
	}
	if cluster.ResourceVersion != row.ClusterVersion || cluster.Status != "active" {
		if plan.Mutation.Operation == "drain" {
			return w.finish(ctx, row, "failed", row.Result, "ACTION_INVALIDATED")
		}
		return nil
	}
	backend, ok := w.Service.Backend.(mutationBackend)
	if !ok {
		return kubernetesinventory.ErrUnavailable
	}
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	// The unguessable marker proves this operation reached the API. A dry-run
	// digest cannot be compared to server-generated fields such as Service IPs.
	var result kubernetesinventory.MutationResult
	if plan.Mutation.Operation == "drain" {
		if row.StartedAt.Valid && time.Since(row.StartedAt.Time) > 15*time.Minute {
			return w.finish(ctx, row, "failed", row.Result, "KUBERNETES_CONVERGENCE_TIMEOUT")
		}
		raw, _ := json.Marshal(plan.Mutation)
		if err = ValidateDrainDelivery(callCtx, w.Service.Store.Queries, row.EnterpriseID, row.ClusterID, row.ID, raw); err != nil {
			return w.finish(ctx, row, "failed", row.Result, operationErrorCode(err))
		}
		advance, ok := w.Service.Backend.(interface {
			AdvanceInventoryDrain(context.Context, db.KubernetesCluster, kubernetesinventory.Mutation) (kubernetesinventory.MutationResult, error)
		})
		if !ok {
			return w.finish(ctx, row, "failed", row.Result, kubernetesinventory.ErrProtocol.Error())
		}
		result, err = advance.AdvanceInventoryDrain(callCtx, cluster, plan.Mutation)
		if err != nil && definiteMutationFailure(err) {
			return w.finish(ctx, row, "failed", row.Result, operationErrorCode(err))
		}
	} else {
		result, err = backend.ObserveInventoryMutation(callCtx, cluster, plan.Mutation, "")
	}
	if err != nil {
		return nil
	} // Ambiguous outcomes remain inspectable and are never re-applied.
	ready, code := mutationConverged(plan.Mutation, result, row.Result)
	if code != "" {
		return w.finish(ctx, row, "failed", publicMutationResult(result), code)
	}
	if ready {
		result.Accepted = true
		return w.finish(ctx, row, "succeeded", publicMutationResult(result), "")
	}
	if plan.Mutation.Operation == "drain" {
		_, err = w.Service.Store.Queries.UpdateKubernetesDrainProgress(ctx, db.UpdateKubernetesDrainProgressParams{EnterpriseID: row.EnterpriseID, ID: row.ID, Result: publicMutationResult(result)})
		return err
	}
	if row.Status == "observing" && row.StartedAt.Valid && time.Since(row.StartedAt.Time) > 15*time.Minute {
		return w.finish(ctx, row, "failed", publicMutationResult(result), "KUBERNETES_CONVERGENCE_TIMEOUT")
	}
	return nil
}

func (w Worker) Reconcile(ctx context.Context) error {
	if err := w.Service.Store.Queries.DeleteRetiredKubernetesHelmInputs(ctx); err != nil {
		return err
	}
	if err := w.Service.Store.Queries.RetireKubernetesLogSessions(ctx); err != nil {
		return err
	}
	if err := w.Service.Store.Queries.DeleteRetiredKubernetesLogSessions(ctx); err != nil {
		return err
	}
	if err := w.Service.Store.Queries.DeleteRetiredKubernetesPrivateInputs(ctx); err != nil {
		return err
	}
	if err := w.Service.Store.Queries.RetireUnconfirmedKubernetesOperations(ctx); err != nil {
		return err
	}
	rows, err := w.Service.Store.Queries.ListKubernetesOperationsToRecover(ctx, 50)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if row.Status == "queued" {
			err = w.execute(ctx, operationTask{row.EnterpriseID, row.ID})
		} else {
			var claimed db.KubernetesOperation
			claimed, err = w.Service.Store.Queries.ClaimKubernetesObservation(ctx, db.ClaimKubernetesObservationParams{EnterpriseID: row.EnterpriseID, ID: row.ID, Version: row.Version})
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err == nil {
				err = w.observe(ctx, claimed)
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}
func (w Worker) RunReconciler(ctx context.Context) error {
	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if err := w.Reconcile(ctx); err != nil && ctx.Err() == nil {
			logger.Error("Kubernetes operation reconciliation failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
