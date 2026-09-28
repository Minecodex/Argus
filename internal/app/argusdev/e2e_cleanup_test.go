package argusdev

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	kubetesting "k8s.io/client-go/testing"
)

func TestDeleteNamespaceRequiresOwnershipAndIdentity(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "owned", UID: "owned-uid", ResourceVersion: "7", Labels: map[string]string{"argus.io/release-id": "test-release"}}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "foreign", UID: "foreign-uid", Labels: map[string]string{"argus.io/release-id": "other-release"}}},
	)
	checked := false
	client.PrependReactor("delete", "namespaces", func(action kubetesting.Action) (bool, runtime.Object, error) {
		deletion := action.(kubetesting.DeleteAction)
		if deletion.GetName() != "owned" {
			t.Fatal("foreign namespace deletion attempted")
		}
		p := deletion.GetDeleteOptions().Preconditions
		if p == nil || p.UID == nil || *p.UID != "owned-uid" || p.ResourceVersion == nil || *p.ResourceVersion != "7" {
			t.Fatal("namespace deletion omitted identity preconditions")
		}
		checked = true
		return false, nil, nil
	})
	kube := &E2EKube{Client: client}
	if err := kube.DeleteNamespace(ctx, "foreign", "test-release"); err == nil {
		t.Fatal("foreign namespace accepted")
	}
	if err := kube.DeleteNamespace(ctx, "owned", "test-release"); err != nil {
		t.Fatal(err)
	}
	if !checked {
		t.Fatal("owned namespace was not deleted")
	}
	if err := kube.DeleteNamespace(ctx, "absent", "test-release"); err != nil {
		t.Fatal(err)
	}
}

func TestM7RegistersNamespaceBeforeInstallationAndRefusesExistingNamespace(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	env := &E2EEnvironment{ReleaseID: "test-release", SystemNS: "test-system", Kube: &E2EKube{Client: client}}
	if err := prepareM7CollectorNamespace(ctx, env); err != nil {
		t.Fatal(err)
	}
	if len(env.ManagedNamespaces) != 1 || env.ManagedNamespaces[0] != m7CollectorNamespace {
		t.Fatal("namespace missing from failure cleanup")
	}
	namespace, err := client.CoreV1().Namespaces().Get(ctx, m7CollectorNamespace, metav1.GetOptions{})
	if err != nil || namespace.Labels["app.kubernetes.io/part-of"] != "argus" || namespace.Labels["argus.io/release-id"] != "test-release" {
		t.Fatal("namespace ownership is incomplete")
	}
	other := &E2EEnvironment{ReleaseID: "other", SystemNS: "other-system", Kube: env.Kube}
	if err := prepareM7CollectorNamespace(ctx, other); err == nil || len(other.ManagedNamespaces) != 0 {
		t.Fatal("pre-existing namespace was adopted")
	}
}

func TestCleanupManagedCollectorRBACOnlyDeletesTemporaryNamespaceBindings(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	for _, item := range []struct {
		name      string
		namespace string
	}{
		{name: "argus-otelcol-owned", namespace: "argus-telemetry"},
		{name: "argus-otelcol-formal", namespace: "argus-formal-telemetry"},
	} {
		labels := map[string]string{"app.kubernetes.io/part-of": "argus"}
		if _, err := client.RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: item.name, Labels: labels}}, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
		if _, err := client.RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: item.name, Labels: labels},
			RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: item.name},
			Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "argus-otelcol", Namespace: item.namespace}},
		}, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	env := &E2EEnvironment{Kube: &E2EKube{Client: client}, ManagedNamespaces: []string{"argus-telemetry"}}
	if err := cleanupManagedCollectorRBAC(ctx, env); err != nil {
		t.Fatal(err)
	}
	if _, err := client.RbacV1().ClusterRoles().Get(ctx, "argus-otelcol-owned", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("temporary Collector ClusterRole was not deleted: %v", err)
	}
	if _, err := client.RbacV1().ClusterRoleBindings().Get(ctx, "argus-otelcol-owned", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("temporary Collector ClusterRoleBinding was not deleted: %v", err)
	}
	if _, err := client.RbacV1().ClusterRoles().Get(ctx, "argus-otelcol-formal", metav1.GetOptions{}); err != nil {
		t.Fatalf("unrelated Collector ClusterRole was deleted: %v", err)
	}
}

func TestDedicatedClusterConflictsReportsHelmOwner(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset(&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{
		Name: "strimzi-cluster-operator-namespaced",
		Annotations: map[string]string{
			"meta.helm.sh/release-name":      "argus-strimzi",
			"meta.helm.sh/release-namespace": "argus-observability",
		},
	}})
	kube := &E2EKube{Client: client}
	conflicts, err := kube.DedicatedClusterConflicts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("conflicts = %#v", conflicts)
	}
	want := "ClusterRole/strimzi-cluster-operator-namespaced owned by Helm release argus-strimzi in argus-observability"
	if conflicts[0] != want {
		t.Fatalf("conflict = %q, want %q", conflicts[0], want)
	}
}

func TestDedicatedClusterConflictsAllowsCleanCluster(t *testing.T) {
	kube := &E2EKube{Client: fake.NewSimpleClientset()}
	conflicts, err := kube.DedicatedClusterConflicts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %#v", conflicts)
	}
}

func TestDedicatedClusterAllowsOnlyValidatedSandboxOwner(t *testing.T) {
	role := func(name, owner string) *rbacv1.ClusterRole {
		return &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: map[string]string{"meta.helm.sh/release-name": owner, "meta.helm.sh/release-namespace": "external"}}}
	}
	kube := &E2EKube{Client: fake.NewSimpleClientset(role("opensandbox-manager-role", "verified"), role("opensandbox-server-role", "different"), role("strimzi-cluster-operator-namespaced", "verified"))}
	conflicts, err := kube.DedicatedClusterConflicts(context.Background(), "external/verified")
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 2 {
		t.Fatalf("shared dependency exception escaped its boundary: %v", conflicts)
	}
}
