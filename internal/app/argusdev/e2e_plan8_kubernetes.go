package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (a *App) plan8KubernetesMutation(ctx context.Context, env *E2EEnvironment, cluster string, input kubernetesworkbench.MutationInput) (kubernetesworkbench.Operation, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return kubernetesworkbench.Operation{}, err
	}
	raw, _ := json.Marshal(input)
	body := map[string]any{}
	_ = json.Unmarshal(raw, &body)
	delete(body, "cluster_id")
	preview, err := client.JSON(ctx, "plan8-k8s-preview-"+input.Operation+"-"+input.Name, "enterprise", http.MethodPost, "/enterprise/kubernetes-clusters/"+cluster+"/inventory/actions/preview-mutation", 201, body, enterpriseHeaders(env, "plan8-k8s-"+uuid.NewString()))
	if err != nil {
		return kubernetesworkbench.Operation{}, err
	}
	ref, err := stringField(preview, "action_ref")
	if err != nil {
		return kubernetesworkbench.Operation{}, err
	}
	result, err := a.confirmPendingAction(ctx, env, "plan8-k8s-confirm-"+input.Operation+"-"+input.Name, ref)
	if err != nil {
		return kubernetesworkbench.Operation{}, err
	}
	id, err := stringField(result, "resource_ref", "resource_id")
	if err != nil {
		return kubernetesworkbench.Operation{}, err
	}
	deadline := time.Now().Add(3 * time.Minute)
	for {
		value, err := client.JSON(ctx, "plan8-k8s-operation-"+id, "enterprise", http.MethodGet, "/enterprise/kubernetes-clusters/"+cluster+"/operations/"+id, 200, nil, enterpriseHeaders(env, ""))
		if err != nil {
			return kubernetesworkbench.Operation{}, err
		}
		raw, _ := json.Marshal(value)
		var operation kubernetesworkbench.Operation
		if json.Unmarshal(raw, &operation) != nil {
			return operation, errors.New("invalid Kubernetes operation response")
		}
		observed, _ := json.MarshalIndent(redactJSON(operation), "", "  ")
		_ = writePrivate(filepath.Join(env.Options.Artifacts, "plan8-k8s-operation-"+id+".json"), observed)
		if operation.Status == "succeeded" {
			return operation, nil
		}
		if operation.Status == "failed" || operation.Status == "unknown" || operation.Status == "cancelled" {
			return operation, fmt.Errorf("owned Kubernetes operation %s: %s", operation.Status, operation.ErrorCode)
		}
		if time.Now().After(deadline) {
			return operation, errors.New("owned Kubernetes operation did not converge")
		}
		if err = waitContext(ctx, time.Second); err != nil {
			return operation, err
		}
	}
}

func (a *App) plan8KubernetesObject(ctx context.Context, env *E2EEnvironment, cluster string, ns uuid.UUID, kind, name, uid string) (kubernetesinventory.Object, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return kubernetesinventory.Object{}, err
	}
	query := url.Values{"type": {kind}, "name": {name}, "namespace_id": {ns.String()}, "uid": {uid}}
	value, err := client.JSON(ctx, "plan8-k8s-read-"+name, "enterprise", http.MethodGet, "/enterprise/kubernetes-clusters/"+cluster+"/inventory/object?"+query.Encode(), 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return kubernetesinventory.Object{}, err
	}
	raw, _ := json.Marshal(value)
	var out kubernetesinventory.Object
	err = json.Unmarshal(raw, &out)
	return out, err
}

// This is an additional enrolled-Connector namespace gate. It does not claim
// Node/cluster-RBAC/CRD behavior on the shared Docker Desktop cluster.
func (a *App) plan8DeployedKubernetesNamespace(ctx context.Context, env *E2EEnvironment) error {
	production, err := os.ReadFile(filepath.Join(env.Options.Artifacts, "plan6-production-backend.json"))
	if err != nil {
		return err
	}
	var accepted struct {
		Image        string `json:"image"`
		Passed       bool   `json:"passed"`
		Tags         string `json:"go_build_tags"`
		NamespaceUID string `json:"namespace_uid"`
	}
	if json.Unmarshal(production, &accepted) != nil || !accepted.Passed || accepted.Tags != "" || !strings.Contains(accepted.Image, "@sha256:") {
		return errors.New("Kubernetes gate requires the exact accepted normal-production image")
	}
	currentNamespace, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, env.SystemNS, metav1.GetOptions{})
	if err != nil || string(currentNamespace.UID) != accepted.NamespaceUID || currentNamespace.Labels["argus.io/release-id"] != env.ReleaseID {
		return errors.New("Kubernetes production image destination ownership changed")
	}
	env.State.Values["p8_kubernetes_connector_image"] = accepted.Image
	namespace, err := a.plan7TargetNamespace(ctx, env)
	if err != nil {
		return err
	}
	cluster, err := a.createM3InCluster(ctx, env, namespace)
	if err != nil {
		return err
	}
	if err = a.startM3KubernetesConnector(ctx, env); err != nil {
		return err
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	value, err := client.JSON(ctx, "plan8-k8s-scope", "enterprise", http.MethodGet, "/enterprise/kubernetes-clusters/"+cluster+"/inventory/scope", 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(value)
	var scope kubernetesworkbench.Scope
	if json.Unmarshal(raw, &scope) != nil {
		return errors.New("invalid Kubernetes scope")
	}
	var selected kubernetesworkbench.Namespace
	for _, row := range scope.Namespaces {
		if row.Name == namespace {
			selected = row
		}
	}
	if scope.ClusterWide {
		query := url.Values{"type": {"namespace"}, "name": {namespace}, "namespace_mode": {"all_authorized"}, "limit": {"30"}}
		listed, listErr := client.JSON(ctx, "plan8-k8s-namespace-directory", "enterprise", http.MethodGet, "/enterprise/kubernetes-clusters/"+cluster+"/inventory?"+query.Encode(), 200, nil, enterpriseHeaders(env, ""))
		if listErr != nil {
			return listErr
		}
		encoded, _ := json.Marshal(listed)
		var directory kubernetesworkbench.Page
		if json.Unmarshal(encoded, &directory) != nil || !directory.Complete {
			return errors.New("owned Namespace directory was not complete")
		}
		proof, _ := json.MarshalIndent(directory, "", "  ")
		_ = writePrivate(filepath.Join(env.Options.Artifacts, "plan8-k8s-namespace-directory.json"), proof)
		for _, object := range directory.Items {
			if object.Name == namespace {
				selected = kubernetesworkbench.Namespace{ID: directory.NamespaceReferences[object.UID], Name: object.Name, UID: object.UID}
			}
		}
	}
	if selected.ID == uuid.Nil || selected.UID != env.State.Values["p7_target_namespace_uid"] {
		return errors.New("Kubernetes scope did not bind the owned Namespace UID")
	}
	configName := "plan8-native-settings"
	config, _ := json.Marshal(map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": configName, "namespace": namespace, "labels": map[string]string{"argus.io/release-id": env.ReleaseID}}, "data": map[string]string{"value": "approved"}})
	created, err := a.plan8KubernetesMutation(ctx, env, cluster, kubernetesworkbench.MutationInput{NamespaceID: selected.ID, Operation: "create", Type: "configmap", Name: configName, Object: config})
	if err != nil {
		return err
	}
	actual, err := env.Kube.Client.CoreV1().ConfigMaps(namespace).Get(ctx, configName, metav1.GetOptions{})
	if err != nil || actual.UID == "" || actual.Data["value"] != "approved" {
		return errors.New("approved ConfigMap did not reach the actual Kubernetes API")
	}
	observed, err := a.plan8KubernetesObject(ctx, env, cluster, selected.ID, "configmap", configName, string(actual.UID))
	if err != nil || observed.ResourceVersion != actual.ResourceVersion || observed.NamespaceUID != selected.UID {
		return errors.New("workbench read lost actual object/Namespace identity")
	}
	patch := json.RawMessage(`{"data":{"value":"approved-update"}}`)
	patched, err := a.plan8KubernetesMutation(ctx, env, cluster, kubernetesworkbench.MutationInput{NamespaceID: selected.ID, Operation: "patch", Type: "configmap", Name: configName, UID: string(actual.UID), ExpectedVersion: actual.ResourceVersion, Object: patch})
	if err != nil {
		return err
	}
	currentConfig, err := env.Kube.Client.CoreV1().ConfigMaps(namespace).Get(ctx, configName, metav1.GetOptions{})
	if err != nil || currentConfig.Data["value"] != "approved-update" {
		return errors.New("approved ConfigMap patch did not reach the actual object")
	}
	if _, err = client.JSON(ctx, "plan8-k8s-stale-version", "enterprise", http.MethodPost, "/enterprise/kubernetes-clusters/"+cluster+"/inventory/actions/preview-mutation", 409, map[string]any{"namespace_id": selected.ID, "operation": "patch", "type": "configmap", "name": configName, "uid": string(actual.UID), "expected_resource_version": actual.ResourceVersion, "object": map[string]any{"data": map[string]string{"value": "must-not-apply"}}}, enterpriseHeaders(env, "plan8-k8s-stale-"+uuid.NewString())); err != nil {
		return err
	}
	afterConflict, err := env.Kube.Client.CoreV1().ConfigMaps(namespace).Get(ctx, configName, metav1.GetOptions{})
	if err != nil || afterConflict.ResourceVersion != currentConfig.ResourceVersion || afterConflict.Data["value"] != "approved-update" {
		return errors.New("stale preview modified the actual object")
	}
	deploymentName := "plan8-native-http"
	httpServer := "from http.server import BaseHTTPRequestHandler, HTTPServer\nclass H(BaseHTTPRequestHandler):\n def do_GET(self):\n  self.send_response(200)\n  self.end_headers()\n  self.wfile.write(b'plan8-forward-actual-result')\nHTTPServer(('0.0.0.0',8080),H).serve_forever()\n"
	definition := map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]any{"name": deploymentName, "namespace": namespace, "labels": map[string]string{"argus.io/release-id": env.ReleaseID}}, "spec": map[string]any{"replicas": 1, "selector": map[string]any{"matchLabels": map[string]string{"app": deploymentName}}, "template": map[string]any{"metadata": map[string]any{"labels": map[string]string{"app": deploymentName, "argus.io/release-id": env.ReleaseID}}, "spec": map[string]any{"automountServiceAccountToken": false, "securityContext": map[string]any{"runAsNonRoot": true, "runAsUser": 65532, "runAsGroup": 65532, "seccompProfile": map[string]string{"type": "RuntimeDefault"}}, "containers": []any{map[string]any{"name": "http", "image": env.State.FixtureImages["remote-runtime"], "imagePullPolicy": "Never", "command": []string{"python3", "-u", "-c", httpServer}, "securityContext": map[string]any{"allowPrivilegeEscalation": false, "readOnlyRootFilesystem": true, "capabilities": map[string]any{"drop": []string{"ALL"}}}, "resources": map[string]any{"requests": map[string]string{"cpu": "10m", "memory": "32Mi"}, "limits": map[string]string{"cpu": "100m", "memory": "96Mi"}}}}}}}}
	deployment, _ := json.Marshal(definition)
	deployed, err := a.plan8KubernetesMutation(ctx, env, cluster, kubernetesworkbench.MutationInput{NamespaceID: selected.ID, Operation: "create", Type: "deployment", Name: deploymentName, Object: deployment})
	if err != nil {
		return err
	}
	live, err := env.Kube.Client.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil || live.Status.ReadyReplicas != 1 {
		return errors.New("approved Deployment has no actual ready replica")
	}
	count := int64(2)
	scaled, err := a.plan8KubernetesMutation(ctx, env, cluster, kubernetesworkbench.MutationInput{NamespaceID: selected.ID, Operation: "scale", Type: "deployment", Name: deploymentName, UID: string(live.UID), ExpectedVersion: live.ResourceVersion, Replicas: &count})
	if err != nil {
		return err
	}
	live, err = env.Kube.Client.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil || live.Status.ReadyReplicas != 2 {
		return errors.New("approved scale did not create two actual ready replicas")
	}
	pods, err := env.Kube.Client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: "app=" + deploymentName})
	if err != nil || len(pods.Items) != 2 {
		return errors.New("scaled Deployment did not expose two actual Pod identities")
	}
	if err = a.plan8KubernetesSessions(ctx, env, cluster, selected, pods.Items[0]); err != nil {
		return err
	}
	if err = a.plan8ModelKubernetesRecovery(ctx, env, cluster, selected, deploymentName); err != nil {
		return err
	}
	observedWorkload, readErr := a.plan8KubernetesObject(ctx, env, cluster, selected.ID, "deployment", live.Name, string(live.UID))
	if readErr != nil || observedWorkload.NamespaceUID != selected.UID {
		return errors.New("workload identity missing before collection acceptance")
	}
	if err = a.plan8KubernetesCollection(ctx, env, cluster, selected, observedWorkload); err != nil {
		return err
	}
	latest, err := env.Kube.Client.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil || latest.UID != live.UID {
		return errors.New("Deployment identity changed before deletion preview")
	}
	live = latest
	deleted, err := a.plan8KubernetesMutation(ctx, env, cluster, kubernetesworkbench.MutationInput{NamespaceID: selected.ID, Operation: "delete", Type: "deployment", Name: deploymentName, UID: string(live.UID), ExpectedVersion: live.ResourceVersion})
	if err != nil {
		return err
	}
	removedConfig, err := a.plan8KubernetesMutation(ctx, env, cluster, kubernetesworkbench.MutationInput{NamespaceID: selected.ID, Operation: "delete", Type: "configmap", Name: configName, UID: string(afterConflict.UID), ExpectedVersion: afterConflict.ResourceVersion})
	if err != nil {
		return err
	}
	if _, err = env.Kube.Client.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		return errors.New("approved Deployment deletion was not observed")
	}
	if _, err = env.Kube.Client.CoreV1().ConfigMaps(namespace).Get(ctx, configName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		return errors.New("approved ConfigMap deletion was not observed")
	}
	proof, _ := json.MarshalIndent(map[string]any{"passed": true, "verification_level": "enrolled_connector_gateway_http_approval_worker_actual_api", "cluster_id": cluster, "namespace": selected, "operations": []kubernetesworkbench.Operation{created, patched, deployed, scaled, deleted, removedConfig}, "cluster_scoped_writes": false}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan8-deployed-kubernetes-namespace.json"), proof)
}
