package kubernetesworkbench

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

// The external CSI installation is an explicit test prerequisite. Only this
// test's Namespace/Class/PVC/Pod are created here; Cluster registration is a
// controlled fixture, rather than a claim of deployed Connector enrollment.
func TestLiveCSIExpansionApprovalWaitsForActualCapacity(t *testing.T) {
	contextName := os.Getenv("ARGUS_KUBERNETES_INVENTORY_CONTEXT")
	if contextName == "" || os.Getenv("ARGUS_CSI_EXPANSION_DRIVER") == "" {
		t.Skip("requires explicit owned CSI acceptance environment")
	}
	f := newMutationFixture(t)
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: contextName}).ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	api, err := dynamic.NewForConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	owner := "plan8-csi-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	resourceFor := func(version, plural string) schema.GroupVersionResource {
		return schema.GroupVersionResource{Version: version, Resource: plural}
	}
	nsAPI := api.Resource(resourceFor("v1", "namespaces"))
	ns, err := nsAPI.Create(ctx, &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": owner, "labels": map[string]any{"argus.owner": owner}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	classAPI := api.Resource(schema.GroupVersionResource{Group: "storage.k8s.io", Version: "v1", Resource: "storageclasses"})
	var classUID string
	var volume string
	t.Cleanup(func() {
		cleanup, c := context.WithTimeout(context.Background(), 90*time.Second)
		defer c()
		current, e := nsAPI.Get(cleanup, owner, metav1.GetOptions{})
		if e == nil && current.GetUID() == ns.GetUID() && current.GetLabels()["argus.owner"] == owner {
			uid := ns.GetUID()
			if e = nsAPI.Delete(cleanup, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
				t.Error(e)
			}
		} else if !apierrors.IsNotFound(e) {
			t.Error("Namespace cleanup ownership", e)
			return
		}
		if e := wait.PollUntilContextTimeout(cleanup, 200*time.Millisecond, 70*time.Second, true, func(ctx context.Context) (bool, error) {
			_, e := nsAPI.Get(ctx, owner, metav1.GetOptions{})
			return apierrors.IsNotFound(e), nil
		}); e != nil {
			t.Error(e)
			return
		}
		if volume != "" {
			if e := wait.PollUntilContextTimeout(cleanup, 200*time.Millisecond, 8*time.Second, true, func(ctx context.Context) (bool, error) {
				_, e := api.Resource(resourceFor("v1", "persistentvolumes")).Get(ctx, volume, metav1.GetOptions{})
				return apierrors.IsNotFound(e), nil
			}); e != nil {
				t.Error("owned CSI volume was not reclaimed", e)
			}
		}
		if classUID != "" {
			current, e := classAPI.Get(cleanup, owner, metav1.GetOptions{})
			if e == nil && string(current.GetUID()) == classUID && current.GetLabels()["argus.owner"] == owner {
				uid := current.GetUID()
				if e = classAPI.Delete(cleanup, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
					t.Error(e)
				}
			} else if !apierrors.IsNotFound(e) {
				t.Error("StorageClass cleanup ownership", e)
			}
		}
	})
	class, err := classAPI.Create(ctx, &unstructured.Unstructured{Object: map[string]any{"apiVersion": "storage.k8s.io/v1", "kind": "StorageClass", "metadata": map[string]any{"name": owner, "labels": map[string]any{"argus.owner": owner}}, "provisioner": os.Getenv("ARGUS_CSI_EXPANSION_DRIVER"), "allowVolumeExpansion": true, "reclaimPolicy": "Delete", "volumeBindingMode": "Immediate"}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	classUID = string(class.GetUID())
	claims := api.Resource(resourceFor("v1", "persistentvolumeclaims")).Namespace(owner)
	_, err = claims.Create(ctx, &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": map[string]any{"name": "data", "namespace": owner}, "spec": map[string]any{"storageClassName": owner, "accessModes": []any{"ReadWriteOnce"}, "resources": map[string]any{"requests": map[string]any{"storage": "32Mi"}}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	proof := uuid.NewString()
	pods := api.Resource(resourceFor("v1", "pods")).Namespace(owner)
	container := map[string]any{
		"name": "writer", "image": "busybox:1.36",
		"command":      []any{"sh", "-c", "printf '%s' '" + proof + "' > /data/proof; sleep 3600"},
		"volumeMounts": []any{map[string]any{"name": "data", "mountPath": "/data"}},
		"resources":    map[string]any{"requests": map[string]any{"cpu": "10m", "memory": "8Mi"}, "limits": map[string]any{"memory": "32Mi"}},
	}
	podSpec := map[string]any{
		"terminationGracePeriodSeconds": int64(1),
		"containers":                    []any{container},
		"volumes":                       []any{map[string]any{"name": "data", "persistentVolumeClaim": map[string]any{"claimName": "data"}}},
	}
	podObject := map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": "writer", "namespace": owner, "labels": map[string]any{"argus.owner": owner}}, "spec": podSpec}
	_, err = pods.Create(ctx, &unstructured.Unstructured{Object: podObject}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := wait.PollUntilContextTimeout(ctx, time.Second, 90*time.Second, true, func(ctx context.Context) (bool, error) {
		pod, e := pods.Get(ctx, "writer", metav1.GetOptions{})
		if e != nil {
			return false, e
		}
		phase, _, _ := unstructured.NestedString(pod.Object, "status", "phase")
		return phase == "Running", nil
	}); err != nil {
		t.Fatal("owned CSI workload did not become ready", err)
	}
	before, err := claims.Get(ctx, "data", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	volume, _, _ = unstructured.NestedString(before.Object, "spec", "volumeName")
	f.exec("UPDATE kubernetes_namespace_resources SET name=$1,namespace_uid=$2 WHERE id=$3", owner, string(ns.GetUID()), f.namespace)
	f.exec("INSERT INTO role_permissions(role_id,permission_id) SELECT role_id,'kubernetes.storage.manage' FROM role_bindings WHERE enterprise_id=$1", f.actor.EnterpriseID)
	f.service.Backend = liveOperationBackend{client: kubernetesinventory.Client{Dynamic: api}}
	id, _ := f.queueInput(MutationInput{ClusterID: f.cluster, NamespaceID: f.namespace, Type: "pvc", Operation: "patch", Name: "data", UID: string(before.GetUID()), ExpectedVersion: before.GetResourceVersion(), Object: []byte(`{"spec":{"resources":{"requests":{"storage":"64Mi"}}}}`)})
	worker := Worker{Service: f.service}
	if err := worker.execute(ctx, operationTask{f.actor.EnterpriseID, id}); err != nil {
		t.Fatal(err)
	}
	if err := wait.PollUntilContextTimeout(ctx, 300*time.Millisecond, 90*time.Second, true, func(ctx context.Context) (bool, error) {
		row := f.operation(id)
		if row.Status == "succeeded" {
			return true, nil
		}
		if row.Status == "failed" {
			t.Fatalf("CSI expansion failed: %s", row.ErrorCode)
		}
		if err := worker.observe(ctx, row); err != nil {
			return false, err
		}
		return f.operation(id).Status == "succeeded", nil
	}); err != nil {
		t.Fatal(err)
	}
	after, err := claims.Get(ctx, "data", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	capacity, _, _ := unstructured.NestedString(after.Object, "status", "capacity", "storage")
	size, e := resource.ParseQuantity(capacity)
	want := resource.MustParse("64Mi")
	if e != nil || size.Cmp(want) < 0 {
		t.Fatal("operation succeeded before actual PVC capacity", capacity, e)
	}
	out, err := exec.CommandContext(ctx, "kubectl", "--context", contextName, "-n", owner, "exec", "writer", "--", "cat", "/data/proof").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != proof {
		t.Fatal("mounted data changed during CSI expansion", err)
	}
	t.Logf("actual CSI %s expanded mounted PVC from 32Mi to %s via preview/confirm/worker; random file persisted", os.Getenv("ARGUS_CSI_EXPANSION_DRIVER"), capacity)
}
