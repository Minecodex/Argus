package kubernetesinventory

import (
	"context"
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

func TestLiveCRDDeleteRefusesExistingInstances(t *testing.T) {
	name := os.Getenv("ARGUS_KUBERNETES_INVENTORY_CONTEXT")
	if name == "" {
		t.Skip("explicit isolated context required")
	}
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: name}).ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	api, err := dynamic.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	owner := "plan8-crd-deps-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	group := owner + ".test"
	crdName := "widgets." + group
	nsGVR := schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}
	gvr := schema.GroupVersionResource{Group: group, Version: "v1", Resource: "widgets"}
	metadata := func(n string) map[string]any {
		return map[string]any{"name": n, "labels": map[string]any{"argus.owner": owner}}
	}
	ns, err := api.Resource(nsGVR).Create(t.Context(), &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": metadata(owner)}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cleanupResource := func(resource schema.GroupVersionResource, n string) {
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		current, e := api.Resource(resource).Get(ctx, n, metav1.GetOptions{})
		if apierrors.IsNotFound(e) {
			return
		}
		if e != nil || current.GetLabels()["argus.owner"] != owner {
			t.Errorf("owned cleanup mismatch: %v", e)
			return
		}
		uid := current.GetUID()
		if e = api.Resource(resource).Delete(ctx, n, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
			t.Error(e)
			return
		}
		if e = wait.PollUntilContextTimeout(ctx, 200*time.Millisecond, 30*time.Second, true, func(ctx context.Context) (bool, error) {
			_, e := api.Resource(resource).Get(ctx, n, metav1.GetOptions{})
			return apierrors.IsNotFound(e), nil
		}); e != nil {
			t.Error(e)
		}
	}
	t.Cleanup(func() { cleanupResource(nsGVR, owner); cleanupResource(crdResource, crdName) })
	manifest := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition", "metadata": metadata(crdName), "spec": map[string]any{"group": group, "scope": "Namespaced", "names": map[string]any{"plural": "widgets", "singular": "widget", "kind": "Plan8DependencyWidget"}, "versions": []any{map[string]any{"name": "v1", "served": true, "storage": true, "schema": map[string]any{"openAPIV3Schema": map[string]any{"type": "object", "properties": map[string]any{"spec": map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "string"}}}}}}}}}}}
	crd, err := api.Resource(crdResource).Create(t.Context(), manifest, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err = wait.PollUntilContextTimeout(t.Context(), 100*time.Millisecond, 20*time.Second, true, func(ctx context.Context) (bool, error) {
		c, e := api.Resource(crdResource).Get(ctx, crdName, metav1.GetOptions{})
		if e != nil {
			return false, e
		}
		conditions, _, _ := unstructured.NestedSlice(c.Object, "status", "conditions")
		for _, raw := range conditions {
			condition, _ := raw.(map[string]any)
			if condition["type"] == "Established" && condition["status"] == "True" {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	instance := &unstructured.Unstructured{Object: map[string]any{"apiVersion": group + "/v1", "kind": "Plan8DependencyWidget", "metadata": metadata("keep-instance"), "spec": map[string]any{"value": "retained"}}}
	actual, err := api.Resource(gvr).Namespace(owner).Create(t.Context(), instance, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	crd, err = api.Resource(crdResource).Get(t.Context(), crdName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	reader := Client{Dynamic: api}
	input := Mutation{OperationRef: uuid.NewString(), ID: uuid.NewString(), Operation: "delete", ClusterWide: true, Query: Query{Type: "crd", Name: crdName, UID: string(crd.GetUID())}, ExpectedVersion: crd.GetResourceVersion()}
	_, err = reader.Mutate(t.Context(), input, true)
	if !errors.Is(err, ErrCRDHasInstances) {
		t.Errorf("CRD delete must explicitly reject existing instances before approval; actual error=%v", err)
	}
	retained, e := api.Resource(gvr).Namespace(owner).Get(t.Context(), actual.GetName(), metav1.GetOptions{})
	if e != nil || retained.GetUID() != actual.GetUID() {
		t.Fatal("dependency check changed the actual instance", e)
	}
	if e = api.Resource(gvr).Namespace(owner).Delete(t.Context(), actual.GetName(), metav1.DeleteOptions{}); e != nil {
		t.Fatal(e)
	}
	if err = wait.PollUntilContextTimeout(t.Context(), 100*time.Millisecond, 10*time.Second, true, func(ctx context.Context) (bool, error) {
		_, e := api.Resource(gvr).Namespace(owner).Get(ctx, actual.GetName(), metav1.GetOptions{})
		return apierrors.IsNotFound(e), nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = reader.Mutate(t.Context(), input, true); err != nil {
		t.Fatal("empty CRD deletion preview was rejected", err)
	}
	if _, err = api.Resource(gvr).Namespace(owner).Create(t.Context(), instance.DeepCopy(), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err = reader.Mutate(t.Context(), input, false); !errors.Is(err, ErrCRDHasInstances) {
		t.Fatal("an instance appearing after preview did not stop the actual write", err)
	}
	if _, err = api.Resource(crdResource).Get(t.Context(), crdName, metav1.GetOptions{}); err != nil {
		t.Fatal("denied CRD write removed its definition", err)
	}
	if e = api.Resource(gvr).Namespace(owner).Delete(t.Context(), actual.GetName(), metav1.DeleteOptions{}); e != nil {
		t.Fatal(e)
	}
	if err = wait.PollUntilContextTimeout(t.Context(), 100*time.Millisecond, 10*time.Second, true, func(ctx context.Context) (bool, error) {
		_, e := api.Resource(gvr).Namespace(owner).Get(ctx, actual.GetName(), metav1.GetOptions{})
		return apierrors.IsNotFound(e), nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = reader.Mutate(t.Context(), input, false); err != nil {
		t.Fatal("empty CRD could not be deleted through the guarded path", err)
	}
	if ns.GetUID() == "" {
		t.Fatal("namespace identity missing")
	}
}
