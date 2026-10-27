package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/logcollection"
	"github.com/kakj-go/Argus/internal/monitoring"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (a *App) plan8CollectorClusterPrerequisites(ctx context.Context, env *E2EEnvironment) error {
	if _, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, m7CollectorNamespace, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		return errors.New("Collector acceptance requires a fresh owned target namespace")
	}
	prepareErr := prepareM7CollectorNamespace(ctx, env)
	ns, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, m7CollectorNamespace, metav1.GetOptions{})
	if err != nil || ns.Labels["argus.io/release-id"] != env.ReleaseID {
		return errors.Join(prepareErr, errors.New("Collector namespace ownership not verified"))
	}
	env.State.Values["p8_collector_namespace_uid"] = string(ns.UID)
	proof, _ := json.MarshalIndent(map[string]any{"name": ns.Name, "uid": ns.UID, "release_id": env.ReleaseID}, "", "  ")
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan8-collector-namespace-owner.json"), proof); err != nil {
		return errors.Join(prepareErr, err)
	}
	if prepareErr != nil {
		return prepareErr
	}
	labels := map[string]string{"argus.io/release-id": env.ReleaseID, "app.kubernetes.io/part-of": "argus-e2e"}
	name := kubernetesNameForDev(env.ReleaseID + "-collector-runtime")
	subjects := []rbacv1.Subject{{Kind: "ServiceAccount", Name: "argus-e2e-kubernetes-connector-runtime", Namespace: env.SystemNS}}
	rules := []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"configmaps", "secrets", "services", "serviceaccounts", "persistentvolumeclaims"}, Verbs: []string{"get", "list", "watch", "create", "update", "patch", "delete"}},
		{APIGroups: []string{"apps"}, Resources: []string{"daemonsets", "deployments"}, Verbs: []string{"get", "list", "watch", "create", "update", "patch", "delete"}},
		{APIGroups: []string{"batch"}, Resources: []string{"jobs"}, Verbs: []string{"get", "list", "watch", "create", "delete"}},
		{APIGroups: []string{"rbac.authorization.k8s.io"}, Resources: []string{"roles", "rolebindings"}, Verbs: []string{"get", "list", "watch", "create", "update", "patch", "delete"}},
	}
	if _, err = env.Kube.Client.RbacV1().Roles(ns.Name).Create(ctx, &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns.Name, Labels: labels}, Rules: rules}, metav1.CreateOptions{}); err != nil {
		return err
	}
	if _, err = env.Kube.Client.RbacV1().RoleBindings(ns.Name).Create(ctx, &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns.Name, Labels: labels}, RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: name}, Subjects: subjects}, metav1.CreateOptions{}); err != nil {
		return err
	}
	// Namespace metadata is cluster scoped; only this exact fresh namespace can
	// be maintained. Other namespace/node workloads receive no write grant.
	metadataRules := []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"namespaces"}, ResourceNames: []string{ns.Name}, Verbs: []string{"get", "update", "patch"}},
		{APIGroups: []string{""}, Resources: []string{"pods", "services", "endpoints", "persistentvolumeclaims"}, Verbs: []string{"get", "list", "watch"}},
		{APIGroups: []string{"apps"}, Resources: []string{"deployments", "statefulsets", "daemonsets", "replicasets"}, Verbs: []string{"get", "list", "watch"}},
		{APIGroups: []string{"batch"}, Resources: []string{"jobs", "cronjobs"}, Verbs: []string{"get", "list", "watch"}},
		{APIGroups: []string{"autoscaling"}, Resources: []string{"horizontalpodautoscalers"}, Verbs: []string{"get", "list", "watch"}},
	}
	if _, err = env.Kube.Client.RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}, Rules: metadataRules}, metav1.CreateOptions{}); err != nil {
		return err
	}
	env.ManagedClusterRBAC = append(env.ManagedClusterRBAC, name)
	_, err = env.Kube.Client.RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}, RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: name}, Subjects: subjects}, metav1.CreateOptions{})
	return err
}

func (a *App) plan8WaitKubernetesCollector(ctx context.Context, env *E2EEnvironment, cluster string) (map[string]any, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return nil, err
	}
	bounded, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	for {
		value, err := client.JSON(bounded, "plan8-k8s-collector-status", "enterprise", http.MethodGet, "/enterprise/kubernetes-clusters/"+cluster+"/collector", 200, nil, enterpriseHeaders(env, ""))
		if err != nil {
			return nil, err
		}
		if value["status"] == "converged" {
			return value, nil
		}
		if value["status"] == "degraded" || value["status"] == "result_unknown" {
			return nil, errors.New("actual Kubernetes Collector failed its configured rollout")
		}
		if err = waitContext(bounded, time.Second); err != nil {
			return nil, err
		}
	}
}

// Called after the established Namespace/session stage, inside its same owned
// Namespace and normal enrolled Connector. It does not inject telemetry rows.
func plan8KubernetesLogConfig(namespaceID uuid.UUID, name, uid string) logcollection.Config {
	return logcollection.Config{Scope: "business", StartAt: "beginning", Encoding: "utf-8", Format: "text", MaxLogBytes: 256 << 10, Workload: &logcollection.Workload{NamespaceID: namespaceID, Type: "deployment", Name: name, UID: uid, Containers: []string{"http"}}}
}

func (a *App) plan8KubernetesCollection(ctx context.Context, env *E2EEnvironment, cluster string, namespace kubernetesworkbench.Namespace, deployment kubernetesinventory.Object) (failure error) {
	image := env.State.FixtureImages["otelcol"]
	if image == "" {
		return errors.New("accepted native Collector image missing")
	}
	if err := a.plan8CollectorClusterPrerequisites(ctx, env); err != nil {
		return err
	}
	distribution, _, _, err := a.verifyM7Catalog(ctx, env)
	if err != nil {
		return err
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	base := "/enterprise/kubernetes-clusters/" + cluster + "/collector"
	body := m7CollectorPreviewBody(distribution, []string{}, image)
	preview, err := client.JSON(ctx, "plan8-k8s-collector-install", "enterprise", http.MethodPost, base+"/actions/preview-install", 201, body, enterpriseHeaders(env, uuid.NewString()))
	if err != nil {
		return err
	}
	ref, err := stringField(preview, "action_ref")
	if err != nil {
		return err
	}
	if _, err = a.confirmPendingAction(ctx, env, "plan8-k8s-collector-install", ref); err != nil {
		return err
	}
	installed, err := a.plan8WaitKubernetesCollector(ctx, env, cluster)
	if err != nil {
		return err
	}
	clusterID, err := uuid.Parse(cluster)
	if err != nil {
		return err
	}
	definition := "kubernetes_cluster_metrics"
	items, err := client.JSONArray(ctx, "plan8-k8s-builtins", "enterprise", http.MethodGet, "/monitoring/builtins?target_type=kubernetes_cluster&target_id="+cluster, 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	version := int64(0)
	available := false
	for _, item := range items {
		raw, _ := json.Marshal(item)
		var builtin monitoring.Builtin
		if json.Unmarshal(raw, &builtin) != nil {
			return errors.New("invalid native component capability")
		}
		if builtin.Definition.Key == definition {
			available = builtin.Available
			if builtin.Collection != nil {
				version = builtin.Collection.Version
			}
		}
	}
	if !available {
		return errors.New("native cluster metric capability unavailable")
	}
	preview, err = client.JSON(ctx, "plan8-k8s-own-enable", "enterprise", http.MethodPost, "/monitoring/builtins/preview", 201, map[string]any{"target": monitoring.ResourceRef{Kind: "kubernetes_cluster", ID: clusterID}, "key": definition, "enabled": true, "expected_version": version, "config": map[string]string{"interval": "5s"}}, enterpriseHeaders(env, uuid.NewString()))
	if err != nil {
		return err
	}
	ref, err = stringField(preview, "action_ref")
	if err != nil {
		return err
	}
	if _, err = a.confirmPendingAction(ctx, env, "plan8-k8s-own-enable", ref); err != nil {
		return err
	}
	metricID := builtincollection.StableID("kubernetes_cluster", clusterID, definition)
	row, err := a.plan8ServiceCollection(ctx, env, metricID)
	if err != nil {
		return err
	}
	row, err = a.plan8ServiceEffective(ctx, env, metricID, row.CollectorID, true, nil)
	if err != nil {
		return err
	}
	metric, err := a.plan8KubernetesDeploymentMetric(ctx, env, cluster, metricID, namespace.Name, deployment.Name)
	if err != nil {
		return err
	}
	app, err := a.plan8ServiceCommit(ctx, env, "k8s-log-application", map[string]any{"kind": "application", "operation": "create", "application": map[string]any{"name": "Plan8 Kubernetes workload logs", "environment": "testing"}})
	if err != nil {
		return err
	}
	service, err := a.plan8ServiceCommit(ctx, env, "k8s-log-service", plan8DeclaredServiceBody(app))
	if err != nil {
		return err
	}
	config := plan8KubernetesLogConfig(namespace.ID, deployment.Name, deployment.UID)
	logID, err := a.plan8ServiceCommit(ctx, env, "k8s-workload-log", map[string]any{"kind": "collection", "operation": "create", "collection": map[string]any{"name": "Actual Deployment container logs", "integration": "k8slog", "collector_id": row.CollectorID, "application_id": app, "service_id": service, "enabled": true, "credential_update": "keep", "config": config}})
	if err != nil {
		return err
	}
	if _, err = a.plan8WaitKubernetesCollector(ctx, env, cluster); err != nil {
		return err
	}
	if err = a.plan8KubernetesStateRestart(ctx, env, row.CollectorID); err != nil {
		return err
	}
	pods, err := env.Kube.Client.CoreV1().Pods(namespace.Name).List(ctx, metav1.ListOptions{LabelSelector: "app=" + deployment.Name})
	if err != nil || len(pods.Items) != 2 {
		return errors.New("actual log workload no longer has two Pod identities")
	}
	marker := "/plan8-workload-log-" + uuid.NewString()
	if _, err = a.plan8KubernetesForwardPath(ctx, env, cluster, namespace.ID, pods.Items[0], marker); err != nil {
		return err
	}
	logRow, err := a.plan8ServiceEffective(ctx, env, logID, row.CollectorID, true, nil)
	if err != nil {
		return err
	}
	logs, err := a.plan8WaitScopedLog(ctx, env, app, strings.TrimPrefix(marker, "/"))
	if err != nil {
		return err
	}
	queueFailure := a.plan8KubernetesPendingQueueRecovery(ctx, env, cluster, namespace, pods.Items[0], app, row.CollectorID)
	logRow, err = a.plan8ServiceUpdate(ctx, env, "k8s-log-disable", logRow, row.CollectorID, false)
	if err != nil {
		return err
	}
	row, err = a.plan8ServiceUpdate(ctx, env, "k8s-own-disable", row, row.CollectorID, false)
	if err != nil {
		return err
	}
	current, err := client.JSON(ctx, "plan8-k8s-collector-before-uninstall", "enterprise", http.MethodGet, base, 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	currentVersion, err := numberField(current, "version")
	if err != nil {
		return err
	}
	uninstallBody := m7CollectorPreviewBody(distribution, []string{}, image)
	uninstallBody["expected_version"] = currentVersion
	preview, err = client.JSON(ctx, "plan8-k8s-collector-uninstall", "enterprise", http.MethodPost, base+"/actions/preview-uninstall", 201, uninstallBody, enterpriseHeaders(env, uuid.NewString()))
	if err != nil {
		return err
	}
	ref, err = stringField(preview, "action_ref")
	if err != nil {
		return err
	}
	if _, err = a.confirmPendingAction(ctx, env, "plan8-k8s-collector-uninstall", ref); err != nil {
		return err
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	for {
		current, err = client.JSON(bounded, "plan8-k8s-uninstalled", "enterprise", http.MethodGet, base, 200, nil, enterpriseHeaders(env, ""))
		if err != nil {
			return err
		}
		if current["status"] == "not_installed" || current["status"] == "uninstalled" {
			break
		}
		if err = waitContext(bounded, time.Second); err != nil {
			return err
		}
	}
	if _, err = env.Kube.Client.AppsV1().DaemonSets(m7CollectorNamespace).Get(ctx, "argus-otelcol-agent", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		return errors.New("Collector agent remained after uninstall")
	}
	if _, err = env.Kube.Client.AppsV1().Deployments(m7CollectorNamespace).Get(ctx, "argus-otelcol-gateway", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		return errors.New("Collector gateway remained after uninstall")
	}
	if _, err = a.plan8WaitScopedLog(ctx, env, app, strings.TrimPrefix(marker, "/")); err != nil {
		return err
	}
	proof, _ := json.MarshalIndent(map[string]any{"passed": true, "scope": "formal_enrolled_cluster_collector_actual_deployment_metrics_and_cri_workload_business_logs", "cluster_id": cluster, "namespace": namespace, "installed": redactJSON(installed), "uninstalled": redactJSON(current), "metric_collection": row, "workload_log_collection": logRow, "metric": redactJSON(metric), "logs": redactJSON(logs), "historical_log_after_uninstall": true, "synthetic_otlp_injection": false}, "", "  ")
	writeErr := writePrivate(filepath.Join(env.Options.Artifacts, "plan8-deployed-kubernetes-collection.json"), proof)
	if queueFailure != nil {
		return errors.Join(errPlan8KubernetesQueueRecovery, queueFailure, writeErr)
	}
	return writeErr
}

func (a *App) plan8KubernetesDeploymentMetric(ctx context.Context, env *E2EEnvironment, cluster string, collection uuid.UUID, namespace, deployment string) (map[string]any, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return nil, err
	}
	// Resource attributes use Writer's resource_ namespace, independently of
	// datapoint labels. Selection still passes through the authorized resolver.
	expression := "k8s_deployment_available{resource_k8s_namespace_name=" + strconv.Quote(namespace) + ",resource_k8s_deployment_name=" + strconv.Quote(deployment) + "}"
	deadline := time.Now().Add(45 * time.Second)
	for {
		result, err := client.JSON(ctx, "plan8-k8s-real-deployment-metric", "enterprise", http.MethodPost, "/enterprise/metrics/query", 200, map[string]any{"query": expression, "resource_ids": []string{cluster}, "time_range": telemetryTimeRange(5 * time.Minute), "budget": telemetryBudget(100)}, enterpriseHeaders(env, ""))
		if err != nil {
			return nil, err
		}
		proof, _ := json.MarshalIndent(redactJSON(result), "", "  ")
		if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan8-k8s-workload-metric-response.json"), proof); err != nil {
			return nil, err
		}
		values, err := plan8ExtraMetricValues(result, cluster, collection.String())
		if err != nil {
			return nil, err
		}
		if len(values) > 0 {
			for _, value := range values {
				if value != 2 {
					return nil, fmt.Errorf("actual available replica metric=%v want 2", value)
				}
			}
			return result, nil
		}
		if time.Now().After(deadline) {
			return nil, errors.New("actual workload metric missing; query response preserved")
		}
		if err = waitContext(ctx, time.Second); err != nil {
			return nil, err
		}
	}
}
