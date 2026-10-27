package kubernetesinventory

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

func TestLivePVCUnsupportedExpansionAndShrinkHaveNoWrite(t *testing.T) {
	contextName, className := os.Getenv("ARGUS_KUBERNETES_INVENTORY_CONTEXT"), os.Getenv("ARGUS_PVC_NONEXPANDING_STORAGE_CLASS")
	if contextName == "" || className == "" {
		t.Skip("requires selected disposable Kubernetes context and actual nonexpanding Delete-policy StorageClass")
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
	classDef, _ := Lookup("storageclass")
	class, err := api.Resource(classDef.GVR()).Get(ctx, className, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	allowed, _, _ := unstructured.NestedBool(class.Object, "allowVolumeExpansion")
	reclaim, _, _ := unstructured.NestedString(class.Object, "reclaimPolicy")
	if allowed || reclaim != "Delete" {
		t.Fatal("selected class must declare no expansion and Delete reclaim; no test data was created")
	}
	owner := "plan8-pvc-" + uuid.NewString()[:8]
	namespaces := api.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"})
	namespace, err := namespaces.Create(ctx, &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": owner, "labels": map[string]any{"argus.owner": owner}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var claimUID types.UID
	t.Cleanup(func() {
		cleanup, c := context.WithTimeout(context.Background(), 65*time.Second)
		defer c()
		current, e := namespaces.Get(cleanup, owner, metav1.GetOptions{})
		if apierrors.IsNotFound(e) {
			return
		}
		if e != nil || current.GetUID() != namespace.GetUID() || current.GetLabels()["argus.owner"] != owner {
			t.Errorf("PVC namespace cleanup ownership: %v", e)
			return
		}
		uid := current.GetUID()
		if e = namespaces.Delete(cleanup, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
			t.Error(e)
			return
		}
		e = wait.PollUntilContextTimeout(cleanup, 500*time.Millisecond, 60*time.Second, true, func(ctx context.Context) (bool, error) {
			_, nsErr := namespaces.Get(ctx, owner, metav1.GetOptions{})
			if nsErr != nil && !apierrors.IsNotFound(nsErr) {
				return false, nsErr
			}
			volumes, e := api.Resource(schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumes"}).List(ctx, metav1.ListOptions{})
			if e != nil {
				return false, e
			}
			for _, volume := range volumes.Items {
				uid, _, _ := unstructured.NestedString(volume.Object, "spec", "claimRef", "uid")
				if claimUID != "" && uid == string(claimUID) {
					return false, nil
				}
			}
			return apierrors.IsNotFound(nsErr), nil
		})
		if e != nil {
			t.Errorf("owned PVC/PV cleanup: %v", e)
		}
	})
	pvcDef, _ := Lookup("pvc")
	claims := api.Resource(pvcDef.GVR()).Namespace(owner)
	claim, err := claims.Create(ctx, &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": map[string]any{"name": "data", "namespace": owner, "labels": map[string]any{"argus.owner": owner}}, "spec": map[string]any{"accessModes": []any{"ReadWriteOnce"}, "storageClassName": className, "resources": map[string]any{"requests": map[string]any{"storage": "1Mi"}}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	claimUID = claim.GetUID()
	prepareLivePVCBindingConsumer(t, ctx, api, class, owner, "data")
	if err = wait.PollUntilContextTimeout(ctx, 300*time.Millisecond, 35*time.Second, true, func(ctx context.Context) (bool, error) {
		var e error
		claim, e = claims.Get(ctx, "data", metav1.GetOptions{})
		if e != nil {
			return false, e
		}
		phase, _, _ := unstructured.NestedString(claim.Object, "status", "phase")
		return phase == "Bound", nil
	}); err != nil {
		t.Fatal(err)
	}
	client := Client{Dynamic: api}
	for _, tc := range []struct {
		size string
		want error
	}{{"2Mi", ErrUnsupported}, {"512Ki", ErrMutationInvalid}} {
		claim, err = claims.Get(ctx, "data", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		mutation := Mutation{ID: uuid.NewString(), Operation: "patch", Query: Query{Type: "pvc", Namespace: owner, NamespaceUID: string(namespace.GetUID()), Name: "data", UID: string(claimUID)}, ExpectedVersion: claim.GetResourceVersion(), Object: json.RawMessage(`{"spec":{"resources":{"requests":{"storage":"` + tc.size + `"}}}}`)}
		if _, err = client.Mutate(ctx, mutation, true); !errors.Is(err, tc.want) {
			t.Fatalf("storage policy for %s: %v", tc.size, err)
		}
		actual, e := claims.Get(ctx, "data", metav1.GetOptions{})
		if e != nil {
			t.Fatal(e)
		}
		requested, _, _ := unstructured.NestedString(actual.Object, "spec", "resources", "requests", "storage")
		if requested != "1Mi" || actual.GetUID() != claimUID {
			t.Fatal("rejected PVC operation changed the actual request or identity")
		}
	}
	t.Log("actual bound PVC: unsupported class expansion and shrink rejected without write; owned claim/PV cleanup verified")
}
