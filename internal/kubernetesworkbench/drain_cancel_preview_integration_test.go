package kubernetesworkbench

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	actionservice "github.com/kakj-go/Argus/internal/action"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func newDrainCancelFixture(t *testing.T) (*mutationFixture, DrainCancelInput) {
	t.Helper()
	f := newMutationFixture(t)
	f.exec("INSERT INTO role_permissions(role_id,permission_id) SELECT role_id,p FROM role_bindings CROSS JOIN unnest(ARRAY['kubernetes.node.manage','kubernetes.node.drain']) p WHERE enterprise_id=$1 AND subject_id=$2", f.actor.EnterpriseID, f.actor.SubjectID)
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'kubernetes_cluster',$4)", uuid.New(), f.actor.EnterpriseID, f.actor.SubjectID, f.cluster)
	id := uuid.New()
	plan, _ := json.Marshal(mutationPlan{SchemaVersion: kubernetesinventory.MutationSchemaVersion, Actor: f.actor, Scope: Scope{ClusterWide: true}, Input: MutationInput{ClusterID: f.cluster, Operation: "drain", Type: "node", Name: "owned-node"}})
	_, err := f.service.Store.Queries.CreateKubernetesOperation(t.Context(), db.CreateKubernetesOperationParams{ID: id, EnterpriseID: f.actor.EnterpriseID, ClusterID: f.cluster, ActorID: f.actor.SubjectID, ActorType: "user", AuthorizationVersion: 1, ClusterVersion: 1, Operation: "drain", TypeKey: "node", TargetName: "owned-node", TargetUid: "owned-node-uid", PrivatePlan: plan})
	if err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE kubernetes_operations SET status='queued' WHERE id=$1", id)
	return f, DrainCancelInput{ClusterID: f.cluster, ID: id, Version: f.operation(id).Version}
}

func TestPostgresDrainCancelPreviewConfirmAndVersionFence(t *testing.T) {
	for _, scenario := range []string{"success", "version_changed", "revoked_permission", "revoked_cluster"} {
		t.Run(scenario, func(t *testing.T) {
			f, in := newDrainCancelFixture(t)
			key := uuid.NewString()
			pending, err := f.service.PreviewDrainCancel(t.Context(), f.actor, in, key)
			if err != nil {
				t.Fatal(err)
			}
			if f.operation(in.ID).Status != "queued" {
				t.Fatal("preview cancelled the operation")
			}
			replay, err := f.service.PreviewDrainCancel(t.Context(), f.actor, in, key)
			if err != nil || replay.ID != pending.ID {
				t.Fatal("preview replay did not preserve the frozen action", err)
			}
			switch scenario {
			case "version_changed":
				f.exec("UPDATE kubernetes_operations SET version=version+1 WHERE id=$1", in.ID)
			case "revoked_permission":
				f.exec("DELETE FROM role_permissions WHERE permission_id='kubernetes.node.drain' AND role_id IN (SELECT role_id FROM role_bindings WHERE enterprise_id=$1)", f.actor.EnterpriseID)
			case "revoked_cluster":
				f.exec("DELETE FROM data_authorization_grants WHERE enterprise_id=$1 AND resource_type='kubernetes_cluster'", f.actor.EnterpriseID)
			}
			r := resource.Service{Store: f.service.Store, Actions: f.service.Actions, Extension: ActionExtension{Service: f.service}}
			workflow := actionservice.Service{Store: f.service.Store, Idempotency: f.service.Actions.Idempotency, Resources: r}
			confirmed, err := workflow.Confirm(t.Context(), f.actor.SubjectID.String(), uuid.NewString(), f.actor.EnterpriseID, f.actor.AuthorizationVersion, false, pending.ActionRef, uuid.NewString())
			if scenario != "success" {
				if err == nil || f.operation(in.ID).Status != "queued" {
					t.Fatal("stale or unauthorized cancel was confirmed")
				}
				return
			}
			if err != nil || confirmed.Execution == nil {
				t.Fatal("drain cancellation confirmation failed", err)
			}
			payload, _ := json.Marshal(actionservice.ExecutionTask{EnterpriseID: f.actor.EnterpriseID, ExecutionID: confirmed.Execution.ID})
			if err := (actionservice.Executor{Store: f.service.Store, Resources: r}).Handle(t.Context(), runtime.Task{RuntimeTask: db.RuntimeTask{Payload: payload}}); err != nil {
				t.Fatal(err)
			}
			if f.operation(in.ID).Status != "cancelled" || f.backend.writes != 0 {
				t.Fatal("cancel failed or issued a Kubernetes mutation")
			}
			var audits int
			if err := f.service.Store.Pool.QueryRow(t.Context(), "SELECT count(*) FROM audit_events WHERE enterprise_id=$1 AND action='kubernetes.drain.cancel'", f.actor.EnterpriseID).Scan(&audits); err != nil || audits != 1 {
				t.Fatal("cancel audit was not committed exactly once", err)
			}
		})
	}
}
