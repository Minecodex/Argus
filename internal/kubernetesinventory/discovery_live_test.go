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

func TestLiveCustomTypeDiscoveryAndRecreation(t *testing.T) {
	contextName := os.Getenv("ARGUS_KUBERNETES_INVENTORY_CONTEXT")
	if contextName == "" {
		t.Skip("requires explicit isolated Kubernetes context")
	}
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: contextName}).ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	api, err := dynamic.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	owner := "plan8-crd-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	group := owner + ".test"
	name := "widgets." + group
	namespaceResource := schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}
	metadata := func(name string) map[string]any {
		return map[string]any{"name": name, "labels": map[string]any{"argus.owner": owner}}
	}
	namespace, err := api.Resource(namespaceResource).Create(ctx, &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": metadata(owner)}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cleanupResource := func(resource schema.GroupVersionResource, name string) {
		cleanup, c := context.WithTimeout(context.Background(), 30*time.Second)
		defer c()
		value, e := api.Resource(resource).Get(cleanup, name, metav1.GetOptions{})
		if apierrors.IsNotFound(e) {
			return
		}
		if e != nil {
			t.Error(e)
			return
		}
		if value.GetLabels()["argus.owner"] != owner {
			t.Error("cleanup ownership changed")
			return
		}
		uid := value.GetUID()
		if e = api.Resource(resource).Delete(cleanup, name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
			t.Error(e)
			return
		}
		if e = wait.PollUntilContextTimeout(cleanup, 250*time.Millisecond, 25*time.Second, true, func(ctx context.Context) (bool, error) {
			_, e := api.Resource(resource).Get(ctx, name, metav1.GetOptions{})
			if apierrors.IsNotFound(e) {
				return true, nil
			}
			return false, e
		}); e != nil {
			t.Error(e)
		}
	}
	t.Cleanup(func() { cleanupResource(namespaceResource, owner); cleanupResource(crdResource, name) })
	manifest := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition", "metadata": metadata(name), "spec": map[string]any{"group": group, "scope": "Namespaced", "names": map[string]any{"plural": "widgets", "singular": "widget", "kind": "Plan8Widget"}, "versions": []any{map[string]any{"name": "v1", "served": true, "storage": true, "schema": map[string]any{"openAPIV3Schema": map[string]any{"type": "object", "properties": map[string]any{"spec": map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "string"}}}}}}}}}}}
	create := func() *unstructured.Unstructured {
		t.Helper()
		value, e := api.Resource(crdResource).Create(ctx, manifest.DeepCopy(), metav1.CreateOptions{})
		if e != nil {
			t.Fatal(e)
		}
		if e = wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 15*time.Second, true, func(ctx context.Context) (bool, error) {
			current, e := api.Resource(crdResource).Get(ctx, name, metav1.GetOptions{})
			if e != nil {
				return false, e
			}
			conditions, _, _ := unstructured.NestedSlice(current.Object, "status", "conditions")
			for _, raw := range conditions {
				condition, _ := raw.(map[string]any)
				if condition["type"] == "Established" && condition["status"] == "True" {
					return true, nil
				}
			}
			return false, nil
		}); e != nil {
			t.Fatal(e)
		}
		return value
	}
	crd := create()
	key := "custom/" + group + "/v1/widgets@" + string(crd.GetUID())
	reader := Client{Dynamic: api}
	catalog, err := reader.Types(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, definition := range catalog.Items {
		if definition.Key == key {
			found = true
		}
	}
	if !found {
		t.Fatal("owned CRD missing from live type discovery")
	}
	gvr := schema.GroupVersionResource{Group: group, Version: "v1", Resource: "widgets"}
	objectMetadata := metadata("first")
	objectMetadata["namespace"] = owner
	object, _ := json.Marshal(map[string]any{"apiVersion": group + "/v1", "kind": "Plan8Widget", "metadata": objectMetadata, "spec": map[string]any{"value": "observed"}})
	input := Mutation{ID: uuid.NewString(), OperationRef: uuid.NewString(), Operation: "create", Query: Query{Type: key, Namespace: owner, NamespaceUID: string(namespace.GetUID()), Name: "first"}, Object: object}
	preview, err := reader.Mutate(ctx, input, true)
	if err != nil {
		t.Fatal("custom resource creation preview", err)
	}
	if _, err = api.Resource(gvr).Namespace(owner).Get(ctx, "first", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatal("custom resource preview wrote an object", err)
	}
	input.ExpectedDigest = preview.Digest
	if _, err = reader.Mutate(ctx, input, false); err != nil {
		t.Fatal("custom resource actual creation", err)
	}
	actual, err := api.Resource(gvr).Namespace(owner).Get(ctx, "first", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	input.ID, input.Operation, input.Query.UID, input.ExpectedVersion, input.ExpectedDigest = uuid.NewString(), "patch", string(actual.GetUID()), actual.GetResourceVersion(), ""
	input.Object = json.RawMessage(`{"spec":{"value":42}}`)
	if _, err = reader.Mutate(ctx, input, true); !errors.Is(err, ErrMutationInvalid) {
		t.Fatal("the actual CRD Schema did not reject an incompatible field", err)
	}
	input.Object = json.RawMessage(`{"spec":{"value":"updated"}}`)
	preview, err = reader.Mutate(ctx, input, true)
	if err != nil {
		t.Fatal("custom resource update preview", err)
	}
	input.ExpectedDigest = preview.Digest
	if _, err = reader.Mutate(ctx, input, false); err != nil {
		t.Fatal("custom resource actual update", err)
	}
	page, err := reader.List(ctx, Query{Type: key, Namespace: owner, NamespaceUID: string(namespace.GetUID()), Limit: 10})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("live custom resource: %+v %v", page, err)
	}
	actual, err = api.Resource(gvr).Namespace(owner).Get(ctx, "first", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if value, _, _ := unstructured.NestedString(actual.Object, "spec", "value"); value != "updated" {
		t.Fatal("the approved custom resource field did not reach the actual object")
	}
	deletionStarted := time.Now()
	input.ID, input.Operation, input.Query.UID, input.ExpectedVersion, input.Object, input.ExpectedDigest = uuid.NewString(), "delete", string(actual.GetUID()), actual.GetResourceVersion(), nil, ""
	if _, err = reader.Mutate(ctx, input, false); err != nil {
		t.Fatal("custom resource actual deletion", err)
	}
	// New CRD kinds join the upstream GC graph on its discovery sync cycle
	// (30 seconds in v1.37). Foreground deletion must use this workflow's
	// existing 90-second deadline, without a shorter discovery-race window.
	if err = wait.PollUntilContextCancel(ctx, 250*time.Millisecond, true, func(ctx context.Context) (bool, error) {
		_, e := api.Resource(gvr).Namespace(owner).Get(ctx, "first", metav1.GetOptions{})
		if apierrors.IsNotFound(e) {
			return true, nil
		}
		return false, e
	}); err != nil {
		t.Fatal("custom resource actual deletion did not complete", err)
	}
	t.Logf("actual foreground deletion observed after %s within the original workflow deadline", time.Since(deletionStarted))
	cleanupResource(crdResource, name)
	replacement := create()
	if replacement.GetUID() == crd.GetUID() {
		t.Fatal("CRD UID did not change")
	}
	if _, err = reader.Definition(ctx, key); !errors.Is(err, ErrIdentityChanged) {
		t.Fatalf("old custom type rebound: %v", err)
	}
	t.Log("actual registered custom type discovery, dry-run/create/Schema rejection/update/delete and read passed; replacing CRD rejected the old type identity")
}
