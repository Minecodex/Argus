package argusdev

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The process-based M3 fixture does not run connector-install.sh. Prepare its
// Collector namespace and permissions explicitly, with the same bounded
// roles as the verified installer and ownership recorded before any writes.
func prepareM7CollectorNamespace(ctx context.Context, env *E2EEnvironment) error {
	if env.ReleaseID == "" || env.SystemNS == "" {
		return fmt.Errorf("M7 fixture release and system namespace are required")
	}
	labels := map[string]string{"app.kubernetes.io/part-of": "argus", "argus.io/release-id": env.ReleaseID}
	if _, err := env.Kube.Client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: m7CollectorNamespace, Labels: labels,
	}}, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("M7 requires its own fresh Collector namespace: %w", err)
	}
	env.ManagedNamespaces = append(env.ManagedNamespaces, m7CollectorNamespace)
	managerName := "argus-e2e-collector-manager"
	if _, err := env.Kube.Client.RbacV1().Roles(m7CollectorNamespace).Create(ctx, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: managerName, Namespace: m7CollectorNamespace, Labels: labels},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{rbacv1.GroupName}, Resources: []string{"roles", "rolebindings"}, ResourceNames: []string{"argus-otelcol-identity"}, Verbs: []string{"get", "update", "patch", "delete"}},
			{APIGroups: []string{rbacv1.GroupName}, Resources: []string{"roles", "rolebindings"}, Verbs: []string{"create"}},
		},
	}, metav1.CreateOptions{}); err != nil {
		return err
	}
	if _, err := env.Kube.Client.RbacV1().RoleBindings(m7CollectorNamespace).Create(ctx, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: managerName, Namespace: m7CollectorNamespace, Labels: labels},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: managerName},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "argus-e2e-kubernetes-connector-runtime", Namespace: env.SystemNS}},
	}, metav1.CreateOptions{}); err != nil {
		return err
	}
	readerName := kubernetesNameForDev(env.ReleaseID + "-collector-reader")
	if _, err := env.Kube.Client.RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: readerName, Labels: labels},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"nodes", "nodes/proxy", "nodes/stats", "namespaces", "pods", "services", "endpoints", "replicationcontrollers", "resourcequotas"}, Verbs: []string{"get", "list", "watch"}},
			{APIGroups: []string{"apps"}, Resources: []string{"deployments", "statefulsets", "daemonsets", "replicasets"}, Verbs: []string{"get", "list", "watch"}},
			{APIGroups: []string{"batch"}, Resources: []string{"jobs", "cronjobs"}, Verbs: []string{"get", "list", "watch"}},
			{APIGroups: []string{"autoscaling"}, Resources: []string{"horizontalpodautoscalers"}, Verbs: []string{"get", "list", "watch"}},
		},
	}, metav1.CreateOptions{}); err != nil {
		return err
	}
	env.ManagedClusterRBAC = append(env.ManagedClusterRBAC, readerName)
	_, err := env.Kube.Client.RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: readerName, Labels: labels},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: readerName},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "argus-otelcol", Namespace: m7CollectorNamespace}},
	}, metav1.CreateOptions{})
	return err
}
