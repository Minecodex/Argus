package kubernetesinventory

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

func TestLiveMutationDryRunConflictScaleRestartAndDeletion(t *testing.T) {
	contextName := os.Getenv("ARGUS_KUBERNETES_INVENTORY_CONTEXT")
	if contextName == "" {
		t.Skip("requires isolated Kubernetes context and cached alpine:3.21")
	}
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: contextName}).ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	api, err := dynamic.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	owner := "plan8-mutation-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	nsResource := schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}
	deploymentResource := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	namespace, err := api.Resource(nsResource).Create(ctx, &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": owner, "labels": map[string]any{"argus.owner": owner}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, c := context.WithTimeout(context.Background(), 40*time.Second)
		defer c()
		current, e := api.Resource(nsResource).Get(cleanup, owner, metav1.GetOptions{})
		if apierrors.IsNotFound(e) {
			return
		}
		if e != nil || current.GetLabels()["argus.owner"] != owner {
			t.Errorf("namespace cleanup ownership: %v", e)
			return
		}
		uid := current.GetUID()
		if e = api.Resource(nsResource).Delete(cleanup, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
			t.Error(e)
			return
		}
		if e = wait.PollUntilContextTimeout(cleanup, 250*time.Millisecond, 35*time.Second, true, func(ctx context.Context) (bool, error) {
			_, e := api.Resource(nsResource).Get(ctx, owner, metav1.GetOptions{})
			if apierrors.IsNotFound(e) {
				return true, nil
			}
			return false, e
		}); e != nil {
			t.Error(e)
		}
	})
	labels := map[string]any{"app": "plan8", "argus.owner": owner}
	manifest := map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]any{"name": "web", "namespace": owner, "labels": labels}, "spec": map[string]any{"replicas": int64(1), "selector": map[string]any{"matchLabels": map[string]any{"app": "plan8"}}, "template": map[string]any{"metadata": map[string]any{"labels": labels}, "spec": map[string]any{"automountServiceAccountToken": false, "securityContext": map[string]any{"runAsUser": int64(65534)}, "containers": []any{map[string]any{"name": "app", "image": "alpine:3.21", "imagePullPolicy": "Never", "command": []any{"/bin/sh", "-c", "sleep 600"}, "resources": map[string]any{"requests": map[string]any{"cpu": "10m", "memory": "16Mi"}, "limits": map[string]any{"cpu": "100m", "memory": "32Mi"}}}}}}}}
	_ = unstructured.SetNestedField(manifest, int64(1), "spec", "template", "spec", "terminationGracePeriodSeconds")
	unstructured.RemoveNestedField(manifest, "spec", "template", "spec", "automountServiceAccountToken")
	raw, _ := json.Marshal(manifest)
	reader := Client{Dynamic: api}
	input := Mutation{ID: uuid.NewString(), Operation: "create", Query: Query{Type: "deployment", Namespace: owner, NamespaceUID: string(namespace.GetUID()), Name: "web"}, Object: raw}
	preview, err := reader.Mutate(ctx, input, true)
	if err != nil {
		t.Fatalf("server dry-run: %v", err)
	}
	if !preview.DryRun || preview.Accepted {
		t.Fatal("dry-run claimed execution")
	}
	var previewObject unstructured.Unstructured
	if preview.After == nil || previewObject.UnmarshalJSON(preview.After.Data) != nil {
		t.Fatal("missing effective workload preview")
	}
	if mount, found, _ := unstructured.NestedBool(previewObject.Object, "spec", "template", "spec", "automountServiceAccountToken"); !found || mount {
		t.Fatal("effective preview omitted the no-token default")
	}
	if _, err = api.Resource(deploymentResource).Namespace(owner).Get(ctx, "web", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatal("dry-run created a workload")
	}
	input.ExpectedDigest = "different-approved-object"
	if _, err = reader.Mutate(ctx, input, false); !errors.Is(err, ErrMutationConflict) {
		t.Fatalf("frozen dry-run diff was ignored: %v", err)
	}
	if _, err = api.Resource(deploymentResource).Namespace(owner).Get(ctx, "web", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatal("digest mismatch wrote an object")
	}
	input.ExpectedDigest = preview.Digest
	applied, err := reader.Mutate(ctx, input, false)
	if err != nil || !applied.Accepted || applied.After == nil {
		t.Fatalf("create: %+v %v", applied, err)
	}
	waitReady := func(replicas int64) *unstructured.Unstructured {
		t.Helper()
		var value *unstructured.Unstructured
		e := wait.PollUntilContextTimeout(ctx, 250*time.Millisecond, 45*time.Second, true, func(ctx context.Context) (bool, error) {
			var err error
			value, err = api.Resource(deploymentResource).Namespace(owner).Get(ctx, "web", metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			ready, _, _ := unstructured.NestedInt64(value.Object, "status", "readyReplicas")
			updated, _, _ := unstructured.NestedInt64(value.Object, "status", "updatedReplicas")
			observed, _, _ := unstructured.NestedInt64(value.Object, "status", "observedGeneration")
			total, _, _ := unstructured.NestedInt64(value.Object, "status", "replicas")
			return ready == replicas && updated == replicas && total == replicas && observed >= value.GetGeneration(), nil
		})
		if e != nil {
			t.Fatal(e)
		}
		return value
	}
	current := waitReady(1)
	// Real admission adds a projected API token only for the independently
	// authorized path. Neither dry-run case creates an actual Pod.
	podSpec := map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": "identity-probe", "namespace": owner}, "spec": map[string]any{"automountServiceAccountToken": true, "containers": []any{map[string]any{"name": "app", "image": "alpine:3.21", "command": []any{"sleep", "60"}}}}}
	podRaw, _ := json.Marshal(podSpec)
	podInput := Mutation{ID: uuid.NewString(), Operation: "create", Query: Query{Type: "pod", Namespace: owner, NamespaceUID: string(namespace.GetUID()), Name: "identity-probe"}, Object: podRaw, ClusterWide: true}
	if _, err = reader.Mutate(ctx, podInput, true); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cluster grant enabled a Pod API identity: %v", err)
	}
	podInput.Policy.ServiceAccountUse = true
	if _, err = reader.Mutate(ctx, podInput, true); err != nil {
		t.Fatalf("explicit Pod API identity dry-run: %v", err)
	}
	if _, err = api.Resource(schema.GroupVersionResource{Version: "v1", Resource: "pods"}).Namespace(owner).Get(ctx, "identity-probe", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatal("identity dry-run created a Pod")
	}
	two := int64(2)
	scale := Mutation{ID: uuid.NewString(), Operation: "scale", Query: Query{Type: "deployment", Namespace: owner, NamespaceUID: string(namespace.GetUID()), Name: "web", UID: string(current.GetUID())}, ExpectedVersion: current.GetResourceVersion(), Replicas: &two}
	if _, err = reader.Mutate(ctx, scale, true); err != nil {
		t.Fatal(err)
	}
	if _, err = reader.Mutate(ctx, scale, false); err != nil {
		t.Fatal(err)
	}
	if _, err = reader.Mutate(ctx, scale, false); !errors.Is(err, ErrMutationConflict) {
		t.Fatalf("stale editor overwrote newer revision: %v", err)
	}
	current = waitReady(2)
	restart := Mutation{ID: uuid.NewString(), Operation: "restart", Query: Query{Type: "deployment", Namespace: owner, NamespaceUID: string(namespace.GetUID()), Name: "web", UID: string(current.GetUID())}, ExpectedVersion: current.GetResourceVersion(), RestartAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if _, err = reader.Mutate(ctx, restart, false); err != nil {
		t.Fatal(err)
	}
	current = waitReady(2)
	remove := Mutation{ID: uuid.NewString(), Operation: "delete", Query: Query{Type: "deployment", Namespace: owner, NamespaceUID: string(namespace.GetUID()), Name: "web", UID: string(current.GetUID())}, ExpectedVersion: current.GetResourceVersion()}
	var deleted MutationResult
	for attempt := 0; attempt < 5; attempt++ {
		current, err = api.Resource(deploymentResource).Namespace(owner).Get(ctx, "web", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		remove.ID = uuid.NewString()
		remove.ExpectedVersion = current.GetResourceVersion()
		if _, err = reader.Mutate(ctx, remove, true); errors.Is(err, ErrMutationConflict) {
			continue
		} else if err != nil {
			t.Fatal(err)
		}
		deleted, err = reader.Mutate(ctx, remove, false)
		if !errors.Is(err, ErrMutationConflict) {
			break
		}
	}
	if err != nil || !deleted.DeleteRequested || deleted.Deleted {
		t.Fatalf("delete acceptance confused with completion: %+v %v", deleted, err)
	}
	if err = wait.PollUntilContextTimeout(ctx, 250*time.Millisecond, 20*time.Second, true, func(ctx context.Context) (bool, error) {
		result, e := reader.ReadMutationOutcome(ctx, remove, "")
		if errors.Is(e, ErrMutationUnknown) {
			return false, nil
		}
		return e == nil && result.Deleted, e
	}); err != nil {
		t.Fatal(err)
	}
	t.Log("dry-run had no side effects; real Deployment reached 1 then 2 ready replicas; stale revision rejected; restart observed; deletion confirmed separately")
}
