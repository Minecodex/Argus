package kubernetesworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/kuberneteshelm"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func (e ActionExtension) commitHelm(ctx context.Context, q *db.Queries, action db.PendingAction, raw []byte) (resource.ActionCommitResult, error) {
	var plan helmPlan
	if json.Unmarshal(raw, &plan) != nil {
		return resource.ActionCommitResult{}, resource.ErrActionInvalidated
	}
	if _, err := q.AcquireKubernetesHelmLock(ctx, db.AcquireKubernetesHelmLockParams{EnterpriseID: action.EnterpriseID, ClusterID: plan.ClusterID, NamespaceUid: plan.Request.Namespace.UID, ReleaseName: plan.Request.Name, OperationID: plan.ID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			err = resource.ErrActionInvalidated
		}
		return resource.ActionCommitResult{}, err
	}
	n, err := q.QueueKubernetesOperation(ctx, db.QueueKubernetesOperationParams{EnterpriseID: action.EnterpriseID, ID: plan.ID})
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	if n != 1 {
		return resource.ActionCommitResult{}, resource.ErrActionInvalidated
	}
	payload, _ := json.Marshal(operationTask{EnterpriseID: action.EnterpriseID, OperationID: plan.ID})
	_, err = q.CreateRuntimeTask(ctx, db.CreateRuntimeTaskParams{ID: uuid.New(), EnterpriseID: uuid.NullUUID{UUID: action.EnterpriseID, Valid: true}, Queue: OperationQueue, RunID: action.RunID, Payload: payload, MaxAttempts: 1, AvailableAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}})
	return resource.ActionCommitResult{ResourceType: "kubernetes_operation", ResourceID: plan.ID, Summary: "Helm operation queued", KubernetesOperationID: uuid.NullUUID{UUID: plan.ID, Valid: true}}, err
}
func (w Worker) executeHelm(ctx context.Context, row db.KubernetesOperation) error {
	var plan helmPlan
	if json.Unmarshal(row.PrivatePlan, &plan) != nil {
		return w.finishHelm(ctx, row, "failed", nil, "KUBERNETES_HELM_PLAN_INVALID")
	}
	if err := w.Service.ValidateHelmDelivery(ctx, row.EnterpriseID, row.ClusterID, row.ID, plan.Request, true); err != nil {
		return w.finishHelm(ctx, row, "failed", nil, operationErrorCode(err))
	}
	backend, ok := w.Service.Backend.(helmBackend)
	if !ok {
		return w.finishHelm(ctx, row, "failed", nil, kuberneteshelm.ErrUnavailable.Error())
	}
	cluster, err := w.Service.Store.Queries.GetKubernetesCluster(ctx, db.GetKubernetesClusterParams{EnterpriseID: row.EnterpriseID, ID: row.ClusterID})
	if err != nil {
		return err
	}
	call, cancel := context.WithTimeout(ctx, 270*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-call.Done():
				return
			case <-ticker.C:
				if e := w.Service.Store.Queries.HeartbeatKubernetesOperation(call, db.HeartbeatKubernetesOperationParams{EnterpriseID: row.EnterpriseID, ID: row.ID}); e != nil {
					cancel()
					return
				}
			}
		}
	}()
	result, err := backend.ExecuteHelm(call, cluster, plan.Request)
	cancel()
	<-done
	if err != nil {
		if errors.Is(err, kuberneteshelm.ErrDenied) || isHelmConflict(err) || errors.Is(err, kuberneteshelm.ErrInvalid) {
			return w.finishHelm(ctx, row, "failed", nil, helmErrorCode(err))
		}
		_, e := w.Service.Store.Queries.MarkKubernetesOperationUnknown(ctx, db.MarkKubernetesOperationUnknownParams{EnterpriseID: row.EnterpriseID, ID: row.ID, ErrorCode: kuberneteshelm.ErrUnknown.Error()})
		return e
	}
	if result.ErrorCode != "" {
		return w.finishHelm(ctx, row, "failed", helmPublicResult(result), result.ErrorCode)
	}
	if !result.Accepted || result.DryRun {
		return w.finishHelm(ctx, row, "failed", nil, "KUBERNETES_HELM_PROTOCOL")
	}
	if _, err = w.Service.Store.Queries.ObserveKubernetesOperation(ctx, db.ObserveKubernetesOperationParams{EnterpriseID: row.EnterpriseID, ID: row.ID, Result: helmPublicResult(result)}); err != nil {
		return err
	}
	row.Status = "observing"
	row.Result = helmPublicResult(result)
	return w.observeHelm(ctx, row)
}
func (w Worker) observeHelm(ctx context.Context, row db.KubernetesOperation) error {
	var plan helmPlan
	if json.Unmarshal(row.PrivatePlan, &plan) != nil {
		return w.finishHelm(ctx, row, "failed", nil, "KUBERNETES_HELM_PLAN_INVALID")
	}
	if err := w.Service.ValidateHelmDelivery(ctx, row.EnterpriseID, row.ClusterID, row.ID, plan.Request, false); err != nil {
		_, e := w.Service.Store.Queries.MarkKubernetesOperationUnknown(ctx, db.MarkKubernetesOperationUnknownParams{EnterpriseID: row.EnterpriseID, ID: row.ID, ErrorCode: "KUBERNETES_HELM_AUTHORIZATION_CHANGED"})
		return e
	}
	backend, ok := w.Service.Backend.(helmBackend)
	if !ok {
		return nil
	}
	cluster, e := w.Service.Store.Queries.GetKubernetesCluster(ctx, db.GetKubernetesClusterParams{EnterpriseID: row.EnterpriseID, ID: row.ClusterID})
	if e != nil {
		return e
	}
	call, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result, e := backend.ObserveHelm(call, cluster, plan.Request)
	if e != nil {
		return nil
	}
	var previous kuberneteshelm.Result
	if json.Unmarshal(row.Result, &previous) == nil {
		if len(result.Writes) == 0 {
			result.Writes = previous.Writes
		}
		if len(result.Objects) == 0 {
			result.Objects = previous.Objects
		}
	}
	if result.ErrorCode != "" {
		return w.finishHelm(ctx, row, "failed", helmPublicResult(result), result.ErrorCode)
	}
	if result.Applied && result.Ready {
		return w.finishHelm(ctx, row, "succeeded", helmPublicResult(result), "")
	}
	if row.Status == "observing" && row.StartedAt.Valid && time.Since(row.StartedAt.Time) > 15*time.Minute {
		return w.finishHelm(ctx, row, "failed", helmPublicResult(result), "KUBERNETES_HELM_CONVERGENCE_TIMEOUT")
	}
	return nil
}
func helmErrorCode(err error) string {
	if isHelmConflict(err) {
		return kuberneteshelm.ErrConflict.Error()
	}
	if errors.Is(err, kuberneteshelm.ErrDenied) {
		return kuberneteshelm.ErrDenied.Error()
	}
	if errors.Is(err, kuberneteshelm.ErrInvalid) {
		return kuberneteshelm.ErrInvalid.Error()
	}
	return kuberneteshelm.ErrUnavailable.Error()
}

func (w Worker) finishHelm(ctx context.Context, row db.KubernetesOperation, status string, result []byte, code string) error {
	if len(result) == 0 {
		result = []byte(`{}`)
	}
	return w.Service.Store.InTx(ctx, func(q *db.Queries) error {
		count, e := q.FinishKubernetesOperation(ctx, db.FinishKubernetesOperationParams{EnterpriseID: row.EnterpriseID, ID: row.ID, Status: status, Result: result, ErrorCode: code})
		if e != nil || count == 0 {
			return e
		}
		result := "failure"
		if status == "succeeded" {
			result = "success"
		}
		_, e = audit.Append(ctx, q, audit.Entry{Domain: "enterprise", EnterpriseID: uuid.NullUUID{UUID: row.EnterpriseID, Valid: true}, ActorType: "system", ActorID: "kubernetes-operation-worker", Action: "kubernetes.helm.finish", ResourceType: "kubernetes_operation", ResourceID: row.ID.String(), Result: result, Details: map[string]any{"operation_id": row.ID.String(), "status": status, "summary": row.Operation + " " + row.TargetName}})
		return e
	})
}
