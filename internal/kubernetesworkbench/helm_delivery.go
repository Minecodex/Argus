package kubernetesworkbench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kuberneteshelm"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func (s Service) validateHelmPlan(ctx context.Context, q *db.Queries, plan helmPlan) error {
	current, _, e := s.prepareHelm(ctx, plan.Actor, plan.ClusterID, plan.Input, plan.ID, true)
	if e != nil {
		return e
	}
	if current.Scope.ClusterVersion != plan.Scope.ClusterVersion || current.Request.Namespace != plan.Request.Namespace || current.Scope.ClusterWide != plan.Scope.ClusterWide || current.Route != plan.Route {
		return resource.ErrActionInvalidated
	}
	return authorizeHelmObjects(ctx, q, plan)
}
func (s Service) ValidateHelmDelivery(ctx context.Context, enterprise, cluster, id uuid.UUID, r kuberneteshelm.Request, write bool) error {
	row, e := s.Store.Queries.GetKubernetesOperation(ctx, db.GetKubernetesOperationParams{EnterpriseID: enterprise, ID: id})
	if e != nil || row.TypeKey != "helm_release" || row.ClusterID != cluster || (row.Status != "running" && (write || row.Status != "observing" && row.Status != "result_unknown")) {
		return resource.ErrActionInvalidated
	}
	var plan helmPlan
	if json.Unmarshal(row.PrivatePlan, &plan) != nil || plan.ID != id || plan.SchemaVersion != kuberneteshelm.SchemaVersion {
		return resource.ErrActionInvalidated
	}
	actual, _ := json.Marshal(r)
	expected, _ := json.Marshal(plan.Request)
	if !bytes.Equal(actual, expected) {
		return resource.ErrActionInvalidated
	}
	if e = s.validateHelmPlan(ctx, s.Store.Queries, plan); e != nil {
		return e
	}
	lock, e := s.Store.Queries.GetKubernetesHelmLock(ctx, db.GetKubernetesHelmLockParams{EnterpriseID: enterprise, ClusterID: cluster, NamespaceUid: row.NamespaceUid, ReleaseName: r.Name})
	if e != nil || lock.OperationID != id {
		return resource.ErrActionInvalidated
	}
	return nil
}
func (s Service) ValidateHelmWrite(ctx context.Context, enterprise, cluster, id uuid.UUID, r kuberneteshelm.Request, w kuberneteshelm.Write) error {
	if w.Verb != "POST" && w.Verb != "PUT" && w.Verb != "PATCH" && w.Verb != "DELETE" {
		return resource.ErrActionInvalidated
	}
	if e := s.ValidateHelmDelivery(ctx, enterprise, cluster, id, r, true); e != nil {
		return e
	}
	row, e := s.Store.Queries.GetKubernetesOperation(ctx, db.GetKubernetesOperationParams{EnterpriseID: enterprise, ID: id})
	if e != nil {
		return e
	}
	var plan helmPlan
	if json.Unmarshal(row.PrivatePlan, &plan) != nil {
		return resource.ErrActionInvalidated
	}
	if w.Group == "" && w.Version == "v1" && w.Resource == "secrets" && w.Namespace == r.Namespace.Name && w.Verb != "DELETE" {
		if w.Name == fmt.Sprintf("sh.helm.release.v1.%s.v%d", r.Name, r.ExpectedRevision) || w.Name == fmt.Sprintf("sh.helm.release.v1.%s.v%d", r.Name, r.ExpectedRevision+1) {
			return nil
		}
	}
	for _, o := range plan.Objects {
		d := o.Definition
		if d.Group == w.Group && d.Version == w.Version && d.Resource == w.Resource && o.Namespace == w.Namespace && o.Name == w.Name {
			if w.Verb == "DELETE" && (o.Action == "delete" || o.Action == "hook" || o.Action == "absent") {
				return nil
			}
			if w.Verb != "DELETE" && (o.Action == "apply" || o.Action == "hook") {
				return nil
			}
		}
	}
	return resource.ErrActionInvalidated
}
func (e ActionExtension) revalidateHelm(ctx context.Context, q *db.Queries, action db.PendingAction, raw []byte) ([]byte, error) {
	var plan helmPlan
	if json.Unmarshal(raw, &plan) != nil || plan.Actor.EnterpriseID != action.EnterpriseID || plan.Actor.SubjectID != action.CreatorSubjectID || plan.Actor.SubjectType != action.CreatorSubjectType || action.ActionType != "kubernetes.helm."+plan.Input.Operation {
		return nil, resource.ErrActionInvalidated
	}
	if err := e.Service.validateHelmPlan(ctx, q, plan); err != nil {
		return nil, err
	}
	return helmSnapshotHash(helmSnapshot(plan)), nil
}
func isHelmConflict(e error) bool {
	return errors.Is(e, kuberneteshelm.ErrConflict) || errors.Is(e, resource.ErrActionInvalidated)
}
