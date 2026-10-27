package kubernetesinventory

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
)

func TestLiveWorkloadRollbackUsesActualControllerHistory(t *testing.T) {
	f := newLiveInventoryNamespace(t, "rollback")
	for _, kind := range []string{"deployment", "statefulset", "daemonset"} {
		t.Run(kind, func(t *testing.T) {
			d, _ := Lookup(kind)
			name := kind + "-app"
			labels := map[string]any{"argus.owner": f.name, "app": name}
			spec := map[string]any{
				"selector": map[string]any{"matchLabels": labels},
				"template": map[string]any{"metadata": map[string]any{"labels": labels}, "spec": map[string]any{
					"automountServiceAccountToken": false, "terminationGracePeriodSeconds": int64(1),
					"containers": []any{map[string]any{"name": "main", "image": "alpine:3.21", "imagePullPolicy": "Never", "command": []any{"sleep", "600"}, "env": []any{map[string]any{"name": "PUBLIC_VERSION", "value": "one"}}, "resources": map[string]any{"requests": map[string]any{"cpu": "5m", "memory": "8Mi"}, "limits": map[string]any{"cpu": "50m", "memory": "24Mi"}}}},
				}},
			}
			if kind != "daemonset" {
				spec["replicas"] = int64(1)
			}
			if kind == "statefulset" {
				spec["serviceName"] = name
			}
			manifest, _ := json.Marshal(map[string]any{"apiVersion": "apps/v1", "kind": d.Kind, "metadata": map[string]any{"name": name, "namespace": f.name, "labels": labels}, "spec": spec})
			input := Mutation{ID: uuid.NewString(), Operation: "create", Query: Query{Type: kind, Name: name, Namespace: f.name, NamespaceUID: f.uid}, Object: manifest}
			if _, err := f.client.Mutate(f.ctx, input, false); err != nil {
				t.Fatal(err)
			}
			api := f.client.Dynamic.Resource(d.GVR()).Namespace(f.name)
			ready := func(version string) *unstructured.Unstructured {
				t.Helper()
				var current *unstructured.Unstructured
				err := wait.PollUntilContextTimeout(f.ctx, 300*time.Millisecond, 40*time.Second, true, func(ctx context.Context) (bool, error) {
					var err error
					current, err = api.Get(ctx, name, metav1.GetOptions{})
					if err != nil {
						return false, err
					}
					integer := func(path ...string) int64 { n, _, _ := unstructured.NestedInt64(current.Object, path...); return n }
					if integer("status", "observedGeneration") < current.GetGeneration() {
						return false, nil
					}
					if kind == "daemonset" {
						if integer("status", "desiredNumberScheduled") != 1 || integer("status", "updatedNumberScheduled") != 1 || integer("status", "numberAvailable") != 1 {
							return false, nil
						}
					} else if integer("status", "readyReplicas") != 1 || integer("status", "updatedReplicas") != 1 || integer("status", "replicas") != 1 {
						return false, nil
					}
					pods, e := f.client.List(ctx, Query{Type: "pod", Namespace: f.name, NamespaceUID: f.uid, Labels: "app=" + name, Limit: 100})
					if e != nil || len(pods.Items) != 1 {
						return false, e
					}
					var pod map[string]any
					_ = json.Unmarshal(pods.Items[0].Data, &pod)
					containers, _, _ := unstructured.NestedSlice(pod, "spec", "containers")
					if len(containers) != 1 {
						return false, nil
					}
					env, _, _ := unstructured.NestedSlice(containers[0].(map[string]any), "env")
					return len(env) == 1 && env[0].(map[string]any)["value"] == version, nil
				})
				if err != nil {
					t.Fatalf("%s actual ready pods at version %s: %v", kind, version, err)
				}
				return current
			}
			current := ready("one")
			historyDefinition, _ := Lookup(rollbackHistoryType(kind))
			historyAPI := f.client.Dynamic.Resource(historyDefinition.GVR()).Namespace(f.name)
			list, err := historyAPI.List(f.ctx, metav1.ListOptions{LabelSelector: "app=" + name})
			if err != nil || len(list.Items) != 1 {
				t.Fatalf("initial real history: %d %v", len(list.Items), err)
			}
			historyName := list.Items[0].GetName()
			patch := Mutation{ID: uuid.NewString(), Operation: "patch", Query: Query{Type: kind, Name: name, Namespace: f.name, NamespaceUID: f.uid, UID: string(current.GetUID())}, ExpectedVersion: current.GetResourceVersion(), Object: []byte(`{"spec":{"template":{"spec":{"containers":[{"name":"main","image":"alpine:3.21","imagePullPolicy":"Never","command":["sleep","600"],"env":[{"name":"PUBLIC_VERSION","value":"two"}]}]}}}}`)}
			if _, err = f.client.Mutate(f.ctx, patch, false); err != nil {
				t.Fatal(err)
			}
			current = ready("two")
			history, err := historyAPI.Get(f.ctx, historyName, metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			revision, _, _ := unstructured.NestedInt64(history.Object, "revision")
			if kind == "deployment" {
				revision, err = strconv.ParseInt(history.GetAnnotations()["deployment.kubernetes.io/revision"], 10, 64)
				if err != nil {
					t.Fatal(err)
				}
			}
			rollback := Mutation{ID: uuid.NewString(), Operation: "rollback", Query: patch.Query, ExpectedVersion: current.GetResourceVersion(), Rollback: &RollbackReference{Name: history.GetName(), UID: string(history.GetUID()), ResourceVersion: history.GetResourceVersion(), Revision: revision}}
			preview, err := f.client.Mutate(f.ctx, rollback, true)
			if err != nil || preview.After == nil || preview.Accepted {
				t.Fatalf("history rollback dry-run: %v", err)
			}
			ready("two") // Preview must not start a rollout.
			rollback.ExpectedDigest = preview.Digest
			rollback.Rollback.UID = "forged-history-uid"
			if _, err = f.client.Mutate(f.ctx, rollback, false); !errors.Is(err, ErrMutationConflict) {
				t.Fatalf("history identity mismatch accepted: %v", err)
			}
			rollback.Rollback.UID = string(history.GetUID())
			if result, e := f.client.Mutate(f.ctx, rollback, false); e != nil || !result.Accepted {
				t.Fatalf("actual rollback: %v", e)
			}
			ready("one")
			t.Log("actual controller history restored original Pod template and ready Pod; preview caused no rollout and forged history identity was rejected")
		})
	}
}
