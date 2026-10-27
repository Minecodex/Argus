package kubernetesworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Real API + PostgreSQL workflow. Registration and the API transport adapter
// are controlled fixtures; this is not an enrolled Connector/browser claim.
func TestLiveRBACApprovalScopeDelegationAndNativeAPIRejection(t *testing.T) {
	contextName := os.Getenv("ARGUS_KUBERNETES_INVENTORY_CONTEXT")
	if contextName == "" {
		t.Skip("explicit isolated Kubernetes context required")
	}
	f := newMutationFixture(t)
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: contextName}).ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	api, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	owner := "plan8-rbac-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	ns, err := api.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: owner, Labels: map[string]string{"argus.owner": owner}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		for _, resource := range []string{"clusterrolebindings", "clusterroles"} {
			objects := client.Resource(schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: resource})
			current, e := objects.Get(ctx, owner, metav1.GetOptions{})
			if apierrors.IsNotFound(e) {
				continue
			}
			if e != nil || current.GetLabels()["argus.owner"] != owner {
				t.Errorf("owned RBAC cleanup mismatch: %v", e)
				continue
			}
			uid := current.GetUID()
			if e = objects.Delete(ctx, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
				t.Error(e)
			}
		}
		current, e := api.CoreV1().Namespaces().Get(ctx, owner, metav1.GetOptions{})
		if apierrors.IsNotFound(e) {
			return
		}
		if e != nil || current.UID != ns.UID || current.Labels["argus.owner"] != owner {
			t.Errorf("owned namespace cleanup mismatch: %v", e)
			return
		}
		uid := ns.UID
		if e = api.CoreV1().Namespaces().Delete(ctx, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
			t.Error(e)
			return
		}
		for {
			_, e = api.CoreV1().Namespaces().Get(ctx, owner, metav1.GetOptions{})
			if apierrors.IsNotFound(e) {
				return
			}
			if ctx.Err() != nil {
				t.Error("owned RBAC namespace still exists")
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
	})
	f.exec("UPDATE kubernetes_namespace_resources SET name=$1,namespace_uid=$2 WHERE id=$3", owner, string(ns.UID), f.namespace)
	f.service.Backend = liveOperationBackend{client: kubernetesinventory.Client{Dynamic: client}, secrets: f.service.Credentials}
	f.exec("INSERT INTO role_permissions(role_id,permission_id) SELECT b.role_id,'kubernetes.rbac.manage' FROM role_bindings b WHERE b.enterprise_id=$1", f.actor.EnterpriseID)
	roleObject := func(name string, rules []any) json.RawMessage {
		raw, _ := json.Marshal(map[string]any{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role", "metadata": map[string]any{"name": name, "namespace": owner, "labels": map[string]any{"argus.owner": owner}}, "rules": rules})
		return raw
	}
	readonlyRules := []any{map[string]any{"apiGroups": []any{""}, "resources": []any{"configmaps"}, "resourceNames": []any{"owned-settings"}, "verbs": []any{"get"}}}
	denied := MutationInput{ClusterID: f.cluster, NamespaceID: f.namespace, Type: "role", Operation: "create", Name: "namespace-cannot-delegate", Object: roleObject("namespace-cannot-delegate", readonlyRules)}
	if _, err = f.service.PreviewMutation(t.Context(), f.actor, denied, uuid.NewString()); err == nil {
		t.Fatal("namespace-only editor delegated API rights")
	}
	f.exec("INSERT INTO role_permissions(role_id,permission_id) SELECT b.role_id,'kubernetes.rbac.escalate' FROM role_bindings b WHERE b.enterprise_id=$1", f.actor.EnterpriseID)
	if _, err = f.service.PreviewMutation(t.Context(), f.actor, denied, uuid.NewString()); err == nil {
		t.Fatal("escalation permission escaped its namespace data scope")
	}
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'kubernetes_cluster',$4)", uuid.New(), f.actor.EnterpriseID, f.actor.SubjectID, f.cluster)
	f.exec("DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM role_bindings WHERE enterprise_id=$1) AND permission_id='kubernetes.rbac.escalate'", f.actor.EnterpriseID)
	if _, err = f.service.PreviewMutation(t.Context(), f.actor, denied, uuid.NewString()); err == nil {
		t.Fatal("ordinary cluster editor implicitly gained delegation")
	}
	f.exec("INSERT INTO role_permissions(role_id,permission_id) SELECT b.role_id,'kubernetes.rbac.escalate' FROM role_bindings b WHERE b.enterprise_id=$1", f.actor.EnterpriseID)
	no := false
	if _, err = api.CoreV1().ServiceAccounts(owner).Create(t.Context(), &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "recipient"}, AutomountServiceAccountToken: &no}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	worker := Worker{Service: f.service}
	type target struct {
		key, kind, resource string
		namespaced          bool
		extra               map[string]any
	}
	targets := []target{
		{"role", "Role", "roles", true, map[string]any{"rules": readonlyRules}},
		{"clusterrole", "ClusterRole", "clusterroles", false, map[string]any{"rules": []any{map[string]any{"apiGroups": []any{""}, "resources": []any{"namespaces"}, "resourceNames": []any{owner}, "verbs": []any{"get"}}}}},
		{"rolebinding", "RoleBinding", "rolebindings", true, map[string]any{"roleRef": map[string]any{"apiGroup": "rbac.authorization.k8s.io", "kind": "Role", "name": owner}, "subjects": []any{map[string]any{"kind": "ServiceAccount", "name": "recipient", "namespace": owner}}}},
		{"clusterrolebinding", "ClusterRoleBinding", "clusterrolebindings", false, map[string]any{"roleRef": map[string]any{"apiGroup": "rbac.authorization.k8s.io", "kind": "ClusterRole", "name": owner}, "subjects": []any{map[string]any{"kind": "ServiceAccount", "name": "recipient", "namespace": owner}}}},
	}
	objectAPI := func(target target) dynamic.ResourceInterface {
		r := client.Resource(schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: target.resource})
		if target.namespaced {
			return r.Namespace(owner)
		}
		return r
	}
	for _, target := range targets {
		metadata := map[string]any{"name": owner, "labels": map[string]any{"argus.owner": owner, "argus.phase": "initial"}}
		if target.namespaced {
			metadata["namespace"] = owner
		}
		value := map[string]any{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": target.kind, "metadata": metadata}
		for k, v := range target.extra {
			value[k] = v
		}
		raw, _ := json.Marshal(value)
		input := MutationInput{ClusterID: f.cluster, Type: target.key, Operation: "create", Name: owner, Object: raw}
		if target.namespaced {
			input.NamespaceID = f.namespace
		}
		id, execution := f.queueInput(input)
		if _, e := objectAPI(target).Get(t.Context(), owner, metav1.GetOptions{}); !apierrors.IsNotFound(e) {
			t.Fatal("RBAC preview wrote before confirmation", target.key, e)
		}
		if err = worker.execute(t.Context(), operationTask{f.actor.EnterpriseID, id}); err != nil {
			t.Fatal(err)
		}
		if row := f.operation(id); row.Status != "succeeded" {
			t.Fatalf("actual %s create failed: %s %s", target.key, row.Status, row.ErrorCode)
		}
		f.reconcileExecution(execution)
		actual, e := objectAPI(target).Get(t.Context(), owner, metav1.GetOptions{})
		if e != nil {
			t.Fatal(e)
		}
		input.Operation = "patch"
		input.UID = string(actual.GetUID())
		input.ExpectedVersion = actual.GetResourceVersion()
		input.Object = json.RawMessage(`{"metadata":{"labels":{"argus.phase":"updated"}}}`)
		id, execution = f.queueInput(input)
		if err = worker.execute(t.Context(), operationTask{f.actor.EnterpriseID, id}); err != nil {
			t.Fatal(err)
		}
		if row := f.operation(id); row.Status != "succeeded" {
			t.Fatalf("actual %s patch failed: %s %s", target.key, row.Status, row.ErrorCode)
		}
		f.reconcileExecution(execution)
		actual, e = objectAPI(target).Get(t.Context(), owner, metav1.GetOptions{})
		if e != nil || actual.GetLabels()["argus.phase"] != "updated" {
			t.Fatal("approved RBAC patch missing", target.key, e)
		}
	}
	// Kubernetes API authorization is an independent boundary, even when the
	// Argus actor explicitly has cluster-wide delegation.
	if _, err = api.RbacV1().Roles(owner).Create(t.Context(), &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "limited-executor"}, Rules: []rbacv1.PolicyRule{{APIGroups: []string{"rbac.authorization.k8s.io"}, Resources: []string{"roles", "rolebindings"}, Verbs: []string{"create", "get", "list", "watch"}}}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err = api.RbacV1().RoleBindings(owner).Create(t.Context(), &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "limited-executor"}, RoleRef: rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: "limited-executor"}, Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: "recipient", Namespace: owner}}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	seconds := int64(600)
	token, err := api.CoreV1().ServiceAccounts(owner).CreateToken(t.Context(), "recipient", &authv1.TokenRequest{Spec: authv1.TokenRequestSpec{ExpirationSeconds: &seconds}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	limited := rest.AnonymousClientConfig(config)
	limited.BearerToken = token.Status.Token
	restricted, err := dynamic.NewForConfig(limited)
	if err != nil {
		t.Fatal(err)
	}
	f.service.Backend = liveOperationBackend{client: kubernetesinventory.Client{Dynamic: restricted}, secrets: f.service.Credentials}
	escalation := denied
	escalation.Name = "must-not-escalate"
	escalation.Object = roleObject(escalation.Name, []any{map[string]any{"apiGroups": []any{""}, "resources": []any{"secrets"}, "verbs": []any{"get"}}})
	if _, err = f.service.PreviewMutation(t.Context(), f.actor, escalation, uuid.NewString()); !errors.Is(err, kubernetesinventory.ErrForbidden) {
		t.Fatal("native API did not explicitly reject escalation", err)
	}
	binding, _ := json.Marshal(map[string]any{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "RoleBinding", "metadata": map[string]any{"name": "must-not-bind", "namespace": owner}, "roleRef": map[string]any{"apiGroup": "rbac.authorization.k8s.io", "kind": "ClusterRole", "name": "cluster-admin"}, "subjects": []any{map[string]any{"kind": "ServiceAccount", "name": "recipient", "namespace": owner}}})
	if _, err = f.service.PreviewMutation(t.Context(), f.actor, MutationInput{ClusterID: f.cluster, NamespaceID: f.namespace, Type: "rolebinding", Operation: "create", Name: "must-not-bind", Object: binding}, uuid.NewString()); !errors.Is(err, kubernetesinventory.ErrForbidden) {
		t.Fatal("native API did not explicitly reject cluster-admin binding", err)
	}
	for _, name := range []string{"must-not-escalate", "must-not-bind"} {
		resource := "roles"
		if name == "must-not-bind" {
			resource = "rolebindings"
		}
		if _, e := client.Resource(schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: resource}).Namespace(owner).Get(t.Context(), name, metav1.GetOptions{}); !apierrors.IsNotFound(e) {
			t.Fatal("denied RBAC request left an actual object", name, e)
		}
	}
	f.service.Backend = liveOperationBackend{client: kubernetesinventory.Client{Dynamic: client}, secrets: f.service.Credentials}
	for i := len(targets) - 1; i >= 0; i-- {
		target := targets[i]
		actual, e := objectAPI(target).Get(t.Context(), owner, metav1.GetOptions{})
		if e != nil {
			t.Fatal(e)
		}
		input := MutationInput{ClusterID: f.cluster, Type: target.key, Operation: "delete", Name: owner, UID: string(actual.GetUID()), ExpectedVersion: actual.GetResourceVersion()}
		if target.namespaced {
			input.NamespaceID = f.namespace
		}
		id, execution := f.queueInput(input)
		if err = worker.execute(t.Context(), operationTask{f.actor.EnterpriseID, id}); err != nil {
			t.Fatal(err)
		}
		if row := f.operation(id); row.Status != "succeeded" {
			t.Fatalf("actual %s delete failed: %s %s", target.key, row.Status, row.ErrorCode)
		}
		f.reconcileExecution(execution)
		if _, e = objectAPI(target).Get(t.Context(), owner, metav1.GetOptions{}); !apierrors.IsNotFound(e) {
			t.Fatal("approved RBAC deletion missing", target.key, e)
		}
	}
	t.Log("real Role/Binding and ClusterRole/Binding approval/create/patch/delete, namespace-only and implicit delegation denial, and independent API escalation/bind rejection passed; transport and enrollment remain controlled")
}
