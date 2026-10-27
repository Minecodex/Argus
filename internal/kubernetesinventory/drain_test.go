package kubernetesinventory

import (
	"context"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
)

type drainFixture struct {
	client                              Client
	api                                 *fake.FakeDynamicClient
	input                               Mutation
	blocked                             bool
	dryEvictions, evictions, nodeWrites int
}

func drainObject(kind, name, namespace, uid string, spec map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": kind, "metadata": map[string]any{"name": name, "namespace": namespace, "uid": uid, "resourceVersion": "10"}, "spec": spec}}
}

func newDrainFixture(t *testing.T) *drainFixture {
	t.Helper()
	ns := drainObject("Namespace", "tenant", "", "namespace-uid", map[string]any{})
	node := drainObject("Node", "worker", "", "node-uid", map[string]any{"unschedulable": false})
	controller := drainObject("ReplicaSet", "web", "tenant", "controller-uid", map[string]any{})
	controller.SetAPIVersion("apps/v1")
	pod := drainObject("Pod", "web-one", "tenant", "pod-uid", map[string]any{"nodeName": "worker", "containers": []any{map[string]any{"name": "app", "image": "alpine:3.21"}}})
	control := true
	pod.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "web", UID: types.UID("controller-uid"), Controller: &control}})
	pod.SetLabels(map[string]string{"app": "web"})
	pdb := drainObject("PodDisruptionBudget", "protect", "tenant", "budget-uid", map[string]any{"minAvailable": int64(1), "selector": map[string]any{"matchLabels": map[string]any{"app": "web"}}})
	pdb.SetAPIVersion("policy/v1")
	pdb.Object["status"] = map[string]any{"disruptionsAllowed": int64(0)}
	listKinds := map[schema.GroupVersionResource]string{drainPods: "PodList", drainPDBs: "PodDisruptionBudgetList"}
	api := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, ns, node, controller, pod, pdb)
	f := &drainFixture{client: Client{Dynamic: api}, api: api, blocked: true, input: Mutation{Operation: "drain", ID: "unpredictable-marker", OperationRef: "op", Query: Query{Type: "node", Name: "worker", UID: "node-uid"}, ExpectedVersion: "10", ClusterWide: true}}
	api.PrependReactor("update", "nodes", func(action ktesting.Action) (bool, runtime.Object, error) {
		update := action.(ktesting.UpdateAction)
		if len(action.(ktesting.UpdateActionImpl).UpdateOptions.DryRun) > 0 {
			return true, update.GetObject().DeepCopyObject(), nil
		}
		f.nodeWrites++
		return false, nil, nil
	})
	api.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "eviction" {
			return false, nil, nil
		}
		request := action.(ktesting.CreateAction).GetObject().(*unstructured.Unstructured)
		uid, _, _ := unstructured.NestedString(request.Object, "deleteOptions", "preconditions", "uid")
		version, _, _ := unstructured.NestedString(request.Object, "deleteOptions", "preconditions", "resourceVersion")
		if uid != "pod-uid" || version != "10" {
			t.Fatalf("missing UID/version preconditions: %v", request.Object)
		}
		dry, _, _ := unstructured.NestedSlice(request.Object, "deleteOptions", "dryRun")
		if len(dry) > 0 {
			f.dryEvictions++
		} else {
			f.evictions++
		}
		if f.blocked {
			return true, nil, apierrors.NewTooManyRequests("PDB refuses eviction", 1)
		}
		if len(dry) == 0 {
			if err := api.Tracker().Delete(drainPods, "tenant", "web-one"); err != nil {
				t.Fatal(err)
			}
		}
		return true, request, nil
	})
	return f
}

func (f *drainFixture) preview(t *testing.T) MutationResult {
	t.Helper()
	result, err := f.client.Mutate(t.Context(), f.input, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = validateDrainResult(f.input, result, true); err != nil {
		t.Fatal(err)
	}
	f.input.DrainPlan, f.input.ExpectedDigest = result.DrainPlan, result.Digest
	return result
}

func TestDrainDryRunPDBRecoveryAndReadOnlyObservation(t *testing.T) {
	f := newDrainFixture(t)
	preview := f.preview(t)
	if preview.Drain.Phase != "blocked" || preview.Drain.Pods[0].Budgets[0].Name != "protect" || f.nodeWrites != 0 || f.evictions != 0 || f.dryEvictions != 1 {
		t.Fatalf("preview side effects or missing impact: %+v", f)
	}
	result, err := f.client.Mutate(t.Context(), f.input, false)
	if err != nil || !result.Accepted || result.Drain.Phase != "blocked" || f.nodeWrites != 1 {
		t.Fatalf("blocked drain: %+v %v", result, err)
	}
	attempts := f.evictions
	if _, err = f.client.ReadMutationOutcome(t.Context(), f.input, ""); err != nil || f.evictions != attempts {
		t.Fatalf("observation wrote: %v", err)
	}
	f.blocked = false
	result, err = f.client.AdvanceDrain(t.Context(), f.input)
	if err != nil || result.Drain.Pods[0].State != "eviction_requested" {
		t.Fatalf("recovery: %+v %v", result, err)
	}
	result, err = f.client.AdvanceDrain(t.Context(), f.input)
	if err != nil || result.Drain.Phase != "complete" || result.Drain.Pods[0].State != "gone" {
		t.Fatalf("completion: %+v %v", result, err)
	}
	node, _ := f.api.Resource(drainNodes).Get(t.Context(), "worker", metav1.GetOptions{})
	cordoned, _, _ := unstructured.NestedBool(node.Object, "spec", "unschedulable")
	if !cordoned || f.nodeWrites != 1 {
		t.Fatal("must retain cordon without replaying node write")
	}
}

func TestDrainUnsafeStorageAndUnmanagedPodsNeverCordon(t *testing.T) {
	for _, kind := range []string{"emptyDir", "hostPath", "ephemeral", "csi", "unmanaged"} {
		t.Run(kind, func(t *testing.T) {
			f := newDrainFixture(t)
			pod, _ := f.api.Resource(drainPods).Namespace("tenant").Get(t.Context(), "web-one", metav1.GetOptions{})
			if kind == "unmanaged" {
				pod.SetOwnerReferences(nil)
			} else {
				_ = unstructured.SetNestedSlice(pod.Object, []any{map[string]any{"name": "data", kind: map[string]any{}}}, "spec", "volumes")
			}
			if _, err := f.api.Resource(drainPods).Namespace("tenant").Update(t.Context(), pod, metav1.UpdateOptions{}); err != nil {
				t.Fatal(err)
			}
			preview := f.preview(t)
			if preview.Drain.Phase != "blocked" {
				t.Fatal("unsafe data missing from preview")
			}
			_, err := f.client.Mutate(t.Context(), f.input, false)
			if !errors.Is(err, ErrDrainUnsafe) || f.nodeWrites != 0 || f.evictions != 0 {
				t.Fatalf("unsafe drain performed side effects: %v", err)
			}
		})
	}
}

func TestDrainSnapshotChangesRejectBeforeSideEffects(t *testing.T) {
	for _, change := range []string{"pod_uid", "pod_spec", "namespace_uid", "extra_pod", "marker", "namespace_before_eviction"} {
		t.Run(change, func(t *testing.T) {
			f := newDrainFixture(t)
			f.preview(t)
			if change == "marker" || change == "namespace_before_eviction" {
				if _, err := f.client.Mutate(t.Context(), f.input, false); err != nil {
					t.Fatal(err)
				}
			}
			switch change {
			case "pod_uid", "pod_spec", "extra_pod":
				pod, _ := f.api.Resource(drainPods).Namespace("tenant").Get(t.Context(), "web-one", metav1.GetOptions{})
				if change == "pod_uid" {
					pod.SetUID("replacement")
				} else if change == "pod_spec" {
					_ = unstructured.SetNestedField(pod.Object, "other", "spec", "serviceAccountName")
				} else {
					pod.SetName("new-pod")
					pod.SetUID("new-uid")
				}
				if change == "extra_pod" {
					_, _ = f.api.Resource(drainPods).Namespace("tenant").Create(t.Context(), pod, metav1.CreateOptions{})
				} else {
					_, _ = f.api.Resource(drainPods).Namespace("tenant").Update(t.Context(), pod, metav1.UpdateOptions{})
				}
			case "namespace_uid", "namespace_before_eviction":
				ns, _ := f.api.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}).Get(t.Context(), "tenant", metav1.GetOptions{})
				ns.SetUID("replacement-ns")
				_, _ = f.api.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}).Update(t.Context(), ns, metav1.UpdateOptions{})
			case "marker":
				node, _ := f.api.Resource(drainNodes).Get(t.Context(), "worker", metav1.GetOptions{})
				node.SetAnnotations(nil)
				_, _ = f.api.Resource(drainNodes).Update(t.Context(), node, metav1.UpdateOptions{})
			}
			attempts, writes := f.evictions, f.nodeWrites
			var err error
			if change == "marker" || change == "namespace_before_eviction" {
				_, err = f.client.AdvanceDrain(t.Context(), f.input)
			} else {
				_, err = f.client.Mutate(t.Context(), f.input, false)
			}
			if err == nil || f.evictions != attempts || f.nodeWrites != writes {
				t.Fatalf("changed snapshot accepted: %v", err)
			}
		})
	}
}

func TestDrainRejectsForgedCompletionAndNonDrainContinuation(t *testing.T) {
	f := newDrainFixture(t)
	result := f.preview(t)
	result.Drain.Phase = "complete"
	if err := validateDrainResult(f.input, result, true); !errors.Is(err, ErrProtocol) {
		t.Fatalf("forged completion: %v", err)
	}
	f.input.Operation = "cordon"
	if _, err := f.client.AdvanceDrain(context.Background(), f.input); !errors.Is(err, ErrMutationInvalid) {
		t.Fatal(err)
	}
}

func TestDrainPVCBindingIsFrozenAndNodeLocalDataBlocksExecution(t *testing.T) {
	for _, local := range []bool{false, true} {
		t.Run(map[bool]string{false: "network_pv", true: "local_pv"}[local], func(t *testing.T) {
			f := newDrainFixture(t)
			pvc := drainObject("PersistentVolumeClaim", "data", "tenant", "claim-uid", map[string]any{"volumeName": "pv-one"})
			pv := drainObject("PersistentVolume", "pv-one", "", "pv-uid", map[string]any{"claimRef": map[string]any{"name": "data", "namespace": "tenant", "uid": "claim-uid"}, "nfs": map[string]any{"server": "storage.internal", "path": "/data"}})
			if local {
				unstructured.RemoveNestedField(pv.Object, "spec", "nfs")
				_ = unstructured.SetNestedMap(pv.Object, map[string]any{"path": "/node-data"}, "spec", "local")
			}
			if _, err := f.api.Resource(drainPVCs).Namespace("tenant").Create(t.Context(), pvc, metav1.CreateOptions{}); err != nil {
				t.Fatal(err)
			}
			if _, err := f.api.Resource(drainPVs).Create(t.Context(), pv, metav1.CreateOptions{}); err != nil {
				t.Fatal(err)
			}
			pod, _ := f.api.Resource(drainPods).Namespace("tenant").Get(t.Context(), "web-one", metav1.GetOptions{})
			_ = unstructured.SetNestedSlice(pod.Object, []any{map[string]any{"name": "data", "persistentVolumeClaim": map[string]any{"claimName": "data"}}}, "spec", "volumes")
			_, _ = f.api.Resource(drainPods).Namespace("tenant").Update(t.Context(), pod, metav1.UpdateOptions{})
			preview := f.preview(t)
			volume := preview.Drain.Pods[0].Volumes[0]
			if volume.Local != local || volume.ClaimUID != "claim-uid" || volume.VolumeUID != "pv-uid" {
				t.Fatalf("storage identity missing: %+v", volume)
			}
			if local {
				if _, err := f.client.Mutate(t.Context(), f.input, false); !errors.Is(err, ErrDrainUnsafe) || f.nodeWrites != 0 {
					t.Fatal("local PV did not block before cordon")
				}
			} else {
				pv.SetUID("replacement-pv")
				_, _ = f.api.Resource(drainPVs).Update(t.Context(), pv, metav1.UpdateOptions{})
				if _, err := f.client.Mutate(t.Context(), f.input, false); !errors.Is(err, ErrMutationConflict) || f.nodeWrites != 0 || f.evictions != 0 {
					t.Fatalf("rebound storage accepted: %v", err)
				}
			}
		})
	}
}

func TestDrainStaticAndDaemonSetPodsAreExplicitlyRetained(t *testing.T) {
	for _, kind := range []string{"mirror", "daemonset"} {
		t.Run(kind, func(t *testing.T) {
			f := newDrainFixture(t)
			pod, _ := f.api.Resource(drainPods).Namespace("tenant").Get(t.Context(), "web-one", metav1.GetOptions{})
			if kind == "mirror" {
				pod.SetAnnotations(map[string]string{"kubernetes.io/config.mirror": "hash"})
			} else {
				ds := drainObject("DaemonSet", "agent", "tenant", "daemon-uid", map[string]any{})
				ds.SetAPIVersion("apps/v1")
				_, _ = f.api.Resource(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}).Namespace("tenant").Create(t.Context(), ds, metav1.CreateOptions{})
				control := true
				pod.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "DaemonSet", Name: "agent", UID: "daemon-uid", Controller: &control}})
			}
			_, _ = f.api.Resource(drainPods).Namespace("tenant").Update(t.Context(), pod, metav1.UpdateOptions{})
			preview := f.preview(t)
			if preview.Drain.Phase != "complete" || preview.Drain.Pods[0].State != "skipped" || preview.Drain.Pods[0].Reason != kind || f.evictions != 0 || f.dryEvictions != 0 {
				t.Fatal("retained Pod was omitted or evicted")
			}
		})
	}
}
