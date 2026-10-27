package kubernetesinventory

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
)

func TestLiveServiceAllocationAndPVCBindingPreserveApprovedIntent(t *testing.T) {
	f := newLiveInventoryNamespace(t, "allocation")
	for _, kind := range []string{"ClusterIP", "NodePort"} {
		name := "service"
		if kind == "NodePort" {
			name = "nodeport"
		}
		object := map[string]any{"apiVersion": "v1", "kind": "Service", "metadata": map[string]any{"name": name, "namespace": f.name}, "spec": map[string]any{"type": kind, "ports": []any{map[string]any{"port": int64(8080)}}}}
		raw, _ := json.Marshal(object)
		input := Mutation{ID: uuid.NewString(), Operation: "create", Query: Query{Type: "service", Namespace: f.name, NamespaceUID: f.uid, Name: name}, Object: raw}
		first, e := f.client.Mutate(f.ctx, input, true)
		if e != nil {
			t.Fatal(e)
		}
		second, e := f.client.Mutate(f.ctx, input, true)
		if e != nil || first.Digest == "" || first.Digest != second.Digest {
			t.Fatalf("unstable Service allocation approval: %v", e)
		}
		input.ExpectedDigest = first.Digest
		actual, e := f.client.Mutate(f.ctx, input, false)
		if e != nil || !actual.Accepted {
			t.Fatalf("actual Service creation: %v", e)
		}
		observed, e := f.client.ReadMutationOutcome(f.ctx, input, first.Digest)
		if e != nil || observed.After == nil {
			t.Fatalf("actual allocation no longer matches approved input: %v", e)
		}
	}
	className := os.Getenv("ARGUS_PVC_NONEXPANDING_STORAGE_CLASS")
	if className == "" {
		t.Fatal("explicit Delete-policy StorageClass required for PVC binding gate")
	}
	classDef, _ := Lookup("storageclass")
	class, e := f.client.Dynamic.Resource(classDef.GVR()).Get(f.ctx, className, metav1.GetOptions{})
	if e != nil {
		t.Fatal(e)
	}
	policy, _, _ := unstructured.NestedString(class.Object, "reclaimPolicy")
	if policy != "Delete" {
		t.Fatal("test PVC requires Delete reclaim policy")
	}
	object := map[string]any{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": map[string]any{"name": "data", "namespace": f.name}, "spec": map[string]any{"storageClassName": className, "accessModes": []any{"ReadWriteOnce"}, "resources": map[string]any{"requests": map[string]any{"storage": "1Mi"}}}}
	raw, _ := json.Marshal(object)
	input := Mutation{ID: uuid.NewString(), Operation: "create", Query: Query{Type: "pvc", Namespace: f.name, NamespaceUID: f.uid, Name: "data"}, Object: raw}
	preview, e := f.client.Mutate(f.ctx, input, true)
	if e != nil {
		t.Fatal(e)
	}
	input.ExpectedDigest = preview.Digest
	if actual, e := f.client.Mutate(f.ctx, input, false); e != nil || !actual.Accepted {
		t.Fatalf("actual approved PVC: %v", e)
	}
	prepareLivePVCBindingConsumer(t, f.ctx, f.client.Dynamic, class, f.name, "data")
	if e = wait.PollUntilContextTimeout(f.ctx, 300*time.Millisecond, 40*time.Second, true, func(ctx context.Context) (bool, error) {
		result, e := f.client.ReadMutationOutcome(ctx, input, preview.Digest)
		if e != nil {
			return false, e
		}
		var value unstructured.Unstructured
		if value.UnmarshalJSON(result.After.Data) != nil {
			return false, ErrProtocol
		}
		phase, _, _ := unstructured.NestedString(value.Object, "status", "phase")
		return phase == "Bound", nil
	}); e != nil {
		t.Fatal(e)
	}
	t.Log("actual ClusterIP and NodePort allocations plus controller-bound PVC remain observable under the approved digest")
}
