package kubernetesworkbench

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
)

func TestPostgresExplicitKubernetesPrivilegesRemainScopeBoundAndExpire(t *testing.T) {
	f := newMutationFixture(t)
	ctx := t.Context()
	role, binding := uuid.New(), uuid.New()
	f.exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Explicit Kubernetes powers')", role, f.actor.EnterpriseID)
	f.exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'kubernetes.workload.privileged'),($1,'kubernetes.rbac.escalate'),($1,'kubernetes.service_account.use')", role)
	f.exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id,valid_until) VALUES($1,$2,'user',$3,$4,now()+interval '1 hour')", binding, f.actor.EnterpriseID, f.actor.SubjectID, role)
	namespace, err := mutationPermissions(ctx, f.service.Store.Queries, f.actor, false)
	if err != nil || namespace.PrivilegedWorkloads || namespace.RBACDelegation || !namespace.ServiceAccountUse {
		t.Fatalf("namespace capabilities: %+v %v", namespace, err)
	}
	cluster, err := mutationPermissions(ctx, f.service.Store.Queries, f.actor, true)
	if err != nil || !cluster.PrivilegedWorkloads || !cluster.RBACDelegation || !cluster.ServiceAccountUse {
		t.Fatalf("explicit cluster capabilities: %+v %v", cluster, err)
	}
	if err = checkMutationPermissions(ctx, f.service.Store.Queries, f.actor, false, cluster); !errors.Is(err, authorization.ErrAccessDenied) {
		t.Fatal("privileged policy bypassed namespace scope")
	}
	// The queued operation freezes the received capabilities. Natural expiry
	// does not bump the Actor version, so delivery must check each power again.
	id, _ := f.queue()
	var plan mutationPlan
	if json.Unmarshal(f.operation(id).PrivatePlan, &plan) != nil || plan.SchemaVersion != kubernetesinventory.MutationSchemaVersion || !plan.Mutation.Policy.ServiceAccountUse {
		t.Fatal("operation did not freeze explicit capability")
	}
	f.exec("UPDATE role_bindings SET valid_until=now()-interval '1 second' WHERE id=$1", binding)
	if err = checkMutationPermissions(ctx, f.service.Store.Queries, f.actor, false, namespace); !errors.Is(err, authorization.ErrAccessDenied) {
		t.Fatal("expired API identity permission survived")
	}
	if err = (Worker{Service: f.service}).execute(ctx, operationTask{f.actor.EnterpriseID, id}); err != nil {
		t.Fatal(err)
	}
	if row := f.operation(id); row.Status != "failed" || f.backend.writes != 0 {
		t.Fatalf("expired capability delivered a write: status=%s writes=%d", row.Status, f.backend.writes)
	}
}
