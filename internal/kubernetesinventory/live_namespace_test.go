package kubernetesinventory

import (
	"context"
	"os"
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

type liveInventoryNamespace struct {
	ctx       context.Context
	client    Client
	name, uid string
}

func newLiveInventoryNamespace(t *testing.T, prefix string) liveInventoryNamespace {
	t.Helper()
	contextName := os.Getenv("ARGUS_KUBERNETES_INVENTORY_CONTEXT")
	if contextName == "" {
		t.Skip("requires an explicitly selected disposable Kubernetes context")
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
	t.Cleanup(cancel)
	owner := "plan8-" + prefix + "-" + uuid.NewString()[:8]
	namespaces := api.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"})
	ns, err := namespaces.Create(ctx, &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": owner, "labels": map[string]any{"argus.owner": owner}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, c := context.WithTimeout(context.Background(), 65*time.Second)
		defer c()
		current, e := namespaces.Get(cleanup, owner, metav1.GetOptions{})
		if apierrors.IsNotFound(e) {
			return
		}
		if e != nil || current.GetUID() != ns.GetUID() || current.GetLabels()["argus.owner"] != owner {
			t.Errorf("inventory namespace ownership: %v", e)
			return
		}
		claimUIDs := map[string]bool{}
		claims, e := api.Resource(schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"}).Namespace(owner).List(cleanup, metav1.ListOptions{})
		if e != nil {
			t.Error(e)
			return
		}
		for _, claim := range claims.Items {
			claimUIDs[string(claim.GetUID())] = true
		}
		uid := current.GetUID()
		if e = namespaces.Delete(cleanup, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
			t.Error(e)
			return
		}
		e = wait.PollUntilContextTimeout(cleanup, 500*time.Millisecond, 60*time.Second, true, func(ctx context.Context) (bool, error) {
			_, e := namespaces.Get(ctx, owner, metav1.GetOptions{})
			if e != nil && !apierrors.IsNotFound(e) {
				return false, e
			}
			if !apierrors.IsNotFound(e) {
				return false, nil
			}
			if len(claimUIDs) == 0 {
				return true, nil
			}
			volumes, e := api.Resource(schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumes"}).List(ctx, metav1.ListOptions{})
			if e != nil {
				return false, e
			}
			for _, volume := range volumes.Items {
				claim, _, _ := unstructured.NestedString(volume.Object, "spec", "claimRef", "uid")
				if claimUIDs[claim] {
					return false, nil
				}
			}
			return true, nil
		})
		if e != nil {
			t.Errorf("owned namespace/volume cleanup: %v", e)
		}
	})
	return liveInventoryNamespace{ctx: ctx, client: Client{Dynamic: api}, name: owner, uid: string(ns.GetUID())}
}
