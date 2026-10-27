package kubernetesworkbench

import (
	"context"
	"crypto/sha256"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

const drainCancelAction = "kubernetes.object.cancel_drain"

type DrainCancelInput struct {
	ClusterID uuid.UUID `json:"cluster_id"`
	ID        uuid.UUID `json:"id"`
	Version   int64     `json:"expected_version"`
}

type drainCancelPlan struct {
	Schema string           `json:"schema_version"`
	Actor  Actor            `json:"actor"`
	Input  DrainCancelInput `json:"input"`
}

// Cancellation changes control-plane intent only. It needs confirmation, but
// cannot reverse an already issued eviction or implicitly uncordon a Node.
func (s Service) PreviewDrainCancel(ctx context.Context, a Actor, in DrainCancelInput, key string) (db.PendingAction, error) {
	if in.ClusterID == uuid.Nil || in.ID == uuid.Nil || in.Version < 1 || key == "" || len(key) > 200 || s.Store == nil {
		return db.PendingAction{}, kubernetesinventory.ErrMutationInvalid
	}
	if _, err := s.GetOperation(ctx, a, in.ClusterID, in.ID); err != nil {
		return db.PendingAction{}, err
	}
	plan := drainCancelPlan{Schema: "argus.kubernetes_drain_cancel/v1", Actor: a, Input: in}
	return postgres.ExecuteIdempotent(ctx, s.Store, s.Actions.Idempotency, "enterprise", a.SubjectType+":"+a.SubjectID.String(), "kubernetes.drain.cancel.preview", key, in, 201, func(q *db.Queries) (db.PendingAction, error) {
		row, err := cancellableDrain(ctx, q, a, in.ClusterID, in.ID, in.Version)
		if err != nil {
			return db.PendingAction{}, err
		}
		if row.Status == "cancelled" {
			return db.PendingAction{}, kubernetesinventory.ErrMutationConflict
		}
		return s.Actions.PrepareWithQueries(ctx, q, resource.ActionSubject{ID: a.SubjectID.String(), Type: a.SubjectType, AuthorizationVersion: a.AuthorizationVersion, ConfirmationRequired: true}, a.EnterpriseID, resource.PrepareActionInput{
			ActionType: drainCancelAction, Title: "Cancel drain " + row.TargetName, Summary: "Stop future drain batches; already dispatched evictions may finish and the Node remains cordoned.",
			Risk: "write", ResourceType: "kubernetes_cluster", ResourceID: uuid.NullUUID{UUID: in.ClusterID, Valid: true}, AuthorizationVersion: a.AuthorizationVersion,
			RunID:   uuid.NullUUID{UUID: a.RunID, Valid: a.RunID != uuid.Nil},
			Preview: map[string]any{"cluster_id": in.ClusterID, "operation_id": in.ID, "expected_version": in.Version, "name": row.TargetName, "in_flight_batch_may_finish": true, "node_stays_cordoned": true},
			Diff:    []DiffLine{{Kind: "change", Text: "drain desired state -> cancelled"}}, ImmutablePlan: plan, ResourceScopeSnapshot: in, CommitHandler: "kubernetes.drain.cancel.commit",
		})
	})
}

func (e ActionExtension) validateDrainCancel(ctx context.Context, q *db.Queries, action db.PendingAction, raw []byte) (drainCancelPlan, error) {
	var plan drainCancelPlan
	if json.Unmarshal(raw, &plan) != nil || plan.Schema != "argus.kubernetes_drain_cancel/v1" || plan.Actor.EnterpriseID != action.EnterpriseID || plan.Actor.SubjectID != action.CreatorSubjectID || plan.Actor.SubjectType != action.CreatorSubjectType || plan.Input.ClusterID != action.ResourceID.UUID || !action.ResourceID.Valid {
		return plan, resource.ErrActionInvalidated
	}
	row, err := cancellableDrain(ctx, q, plan.Actor, plan.Input.ClusterID, plan.Input.ID, plan.Input.Version)
	if err != nil {
		return plan, err
	}
	if row.Status == "cancelled" {
		return plan, resource.ErrActionInvalidated
	}
	return plan, nil
}

func (e ActionExtension) revalidateDrainCancel(ctx context.Context, q *db.Queries, action db.PendingAction, raw []byte) ([]byte, error) {
	plan, err := e.validateDrainCancel(ctx, q, action, raw)
	if err != nil {
		return nil, err
	}
	value, _ := json.Marshal(plan.Input)
	hash := sha256.Sum256(value)
	return hash[:], nil
}

func (e ActionExtension) commitDrainCancel(ctx context.Context, q *db.Queries, action db.PendingAction, raw []byte) (resource.ActionCommitResult, error) {
	plan, err := e.validateDrainCancel(ctx, q, action, raw)
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	_, err = cancelDrainWithQueries(ctx, q, plan.Actor, plan.Input.ClusterID, plan.Input.ID, plan.Input.Version)
	return resource.ActionCommitResult{ResourceType: "kubernetes_operation", ResourceID: plan.Input.ID, Summary: "Drain cancelled; already dispatched evictions may finish and the Node remains cordoned"}, err
}
