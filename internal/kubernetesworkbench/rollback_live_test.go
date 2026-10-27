package kubernetesworkbench

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
)

func runLiveRollbackWorkflows(t *testing.T, f *mutationFixture, namespace string) {
	t.Helper()
	backend := f.service.Backend.(liveOperationBackend)
	w := Worker{Service: f.service}
	for _, kind := range []string{"deployment", "statefulset", "daemonset"} {
		t.Run("rollback/"+kind, func(t *testing.T) {
			d, _ := kubernetesinventory.Lookup(kind)
			name := "history-" + kind
			labels := map[string]any{"app": name}
			spec := map[string]any{"selector": map[string]any{"matchLabels": labels}, "template": map[string]any{"metadata": map[string]any{"labels": labels}, "spec": map[string]any{"automountServiceAccountToken": false, "terminationGracePeriodSeconds": int64(1), "containers": []any{map[string]any{"name": "main", "image": "alpine:3.21", "imagePullPolicy": "Never", "command": []any{"sleep", "600"}, "env": []any{map[string]any{"name": "PUBLIC_VERSION", "value": "one"}}, "resources": map[string]any{"requests": map[string]any{"cpu": "5m", "memory": "8Mi"}, "limits": map[string]any{"cpu": "50m", "memory": "24Mi"}}}}}}}
			if kind != "daemonset" {
				spec["replicas"] = int64(1)
			}
			if kind == "statefulset" {
				spec["serviceName"] = name
			}
			manifest, _ := json.Marshal(map[string]any{"apiVersion": "apps/v1", "kind": d.Kind, "metadata": map[string]any{"name": name, "namespace": namespace}, "spec": spec})
			input := MutationInput{ClusterID: f.cluster, NamespaceID: f.namespace, Type: kind, Name: name, Operation: "create", Object: manifest}
			apply := func() {
				t.Helper()
				id, execution := f.queueInput(input)
				if err := w.execute(t.Context(), operationTask{f.actor.EnterpriseID, id}); err != nil {
					t.Fatal(err)
				}
				if err := wait.PollUntilContextTimeout(t.Context(), 300*time.Millisecond, 40*time.Second, true, func(ctx context.Context) (bool, error) {
					row := f.operation(id)
					if row.Status == "failed" {
						t.Fatalf("%s workflow failed: %s", input.Operation, row.ErrorCode)
					}
					if row.Status == "succeeded" {
						return true, nil
					}
					return false, w.observe(ctx, row)
				}); err != nil {
					t.Fatal(err)
				}
				f.reconcileExecution(execution)
			}
			apply()
			historyType := "controllerrevision"
			if kind == "deployment" {
				historyType = "replicaset"
			}
			hd, _ := kubernetesinventory.Lookup(historyType)
			historyAPI := backend.client.Dynamic.Resource(hd.GVR()).Namespace(namespace)
			history, err := historyAPI.List(t.Context(), metav1.ListOptions{LabelSelector: "app=" + name})
			if err != nil || len(history.Items) != 1 {
				t.Fatalf("initial history: %v", err)
			}
			historyName := history.Items[0].GetName()
			targetAPI := backend.client.Dynamic.Resource(d.GVR()).Namespace(namespace)
			current, err := targetAPI.Get(t.Context(), name, metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			input.Operation, input.UID, input.ExpectedVersion = "patch", string(current.GetUID()), current.GetResourceVersion()
			input.Object = []byte(`{"spec":{"template":{"spec":{"containers":[{"name":"main","image":"alpine:3.21","imagePullPolicy":"Never","command":["sleep","600"],"env":[{"name":"PUBLIC_VERSION","value":"two"}]}]}}}}`)
			apply()
			podDefinition, _ := kubernetesinventory.Lookup("pod")
			if err = wait.PollUntilContextTimeout(t.Context(), 200*time.Millisecond, 20*time.Second, true, func(ctx context.Context) (bool, error) {
				pods, e := backend.client.Dynamic.Resource(podDefinition.GVR()).Namespace(namespace).List(ctx, metav1.ListOptions{LabelSelector: "app=" + name})
				return e == nil && len(pods.Items) == 1 && pods.Items[0].GetDeletionTimestamp() == nil, e
			}); err != nil {
				t.Fatal(err)
			}
			old, err := historyAPI.Get(t.Context(), historyName, metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			revision, _, _ := unstructured.NestedInt64(old.Object, "revision")
			if kind == "deployment" {
				revision, err = strconv.ParseInt(old.GetAnnotations()["deployment.kubernetes.io/revision"], 10, 64)
				if err != nil {
					t.Fatal(err)
				}
			}
			current, err = targetAPI.Get(t.Context(), name, metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			input.Operation, input.ExpectedVersion, input.Object = "rollback", current.GetResourceVersion(), nil
			input.Rollback = &kubernetesinventory.RollbackReference{Name: old.GetName(), UID: string(old.GetUID()), ResourceVersion: old.GetResourceVersion(), Revision: revision}
			apply()
			current, err = targetAPI.Get(t.Context(), name, metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			containers, _, _ := unstructured.NestedSlice(current.Object, "spec", "template", "spec", "containers")
			env, _, _ := unstructured.NestedSlice(containers[0].(map[string]any), "env")
			if len(env) != 1 || env[0].(map[string]any)["value"] != "one" {
				t.Fatal("successful workflow did not restore actual historical template")
			}
			t.Log("PostgreSQL preview/confirm/worker/observation/execution restored actual controller history")
		})
	}
}
