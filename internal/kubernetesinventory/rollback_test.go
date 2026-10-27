package kubernetesinventory

import (
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic/fake"
)

func rollbackFixture(kind string) (*unstructured.Unstructured, *unstructured.Unstructured, Mutation) {
	d, _ := Lookup(kind)
	before := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1", "kind": d.Kind,
		"metadata": map[string]any{"name": "app", "namespace": "owned", "uid": "workload-uid", "resourceVersion": "10"},
		"spec":     map[string]any{"replicas": int64(3), "template": map[string]any{"metadata": map[string]any{"labels": map[string]any{"app": "app"}, "annotations": map[string]any{"later-only": "must-disappear"}}, "spec": map[string]any{"automountServiceAccountToken": false, "containers": []any{map[string]any{"name": "app", "image": "version:2"}}}}},
	}}
	template := map[string]any{"metadata": map[string]any{"labels": map[string]any{"app": "app", "pod-template-hash": "old-hash"}}, "spec": map[string]any{"automountServiceAccountToken": false, "containers": []any{map[string]any{"name": "app", "image": "version:1"}}}}
	h := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "apps/v1", "kind": "ControllerRevision", "metadata": map[string]any{"name": "app-history", "namespace": "owned", "uid": "history-uid", "resourceVersion": "5"}}}
	controller := true
	h.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "apps/v1", Kind: d.Kind, Name: "app", UID: before.GetUID(), Controller: &controller}})
	if kind == "deployment" {
		h.SetKind("ReplicaSet")
		h.SetAnnotations(map[string]string{"deployment.kubernetes.io/revision": "1"})
		h.Object["spec"] = map[string]any{"template": template}
	} else {
		template["$patch"] = "replace"
		h.Object["revision"] = int64(1)
		h.Object["data"] = map[string]any{"spec": map[string]any{"template": template}}
	}
	in := Mutation{ID: "operation", Operation: "rollback", Query: Query{Type: kind, Name: "app", Namespace: "owned", NamespaceUID: "namespace-uid", UID: string(before.GetUID())}, ExpectedVersion: "10", Rollback: &RollbackReference{Name: h.GetName(), UID: string(h.GetUID()), ResourceVersion: "5", Revision: 1}}
	return before, h, in
}

func TestRollbackRestoresOnlyFrozenOwnedPodTemplate(t *testing.T) {
	for _, kind := range []string{"deployment", "statefulset", "daemonset"} {
		t.Run(kind, func(t *testing.T) {
			before, history, in := rollbackFixture(kind)
			api := fake.NewSimpleDynamicClient(runtime.NewScheme(), history)
			c := Client{Dynamic: api}
			d, _ := Lookup(kind)
			desired, err := c.rollbackObject(t.Context(), in, d, before)
			if err != nil {
				t.Fatal(err)
			}
			desired, err = finalizeMutationObject(in, d, before, desired)
			if err != nil {
				t.Fatal(err)
			}
			containers, _, _ := unstructured.NestedSlice(desired.Object, "spec", "template", "spec", "containers")
			if containers[0].(map[string]any)["image"] != "version:1" || desired.GetResourceVersion() != "10" || desired.GetUID() != before.GetUID() {
				t.Fatal("rollback lost target identity or did not restore historical image")
			}
			if replicas, _, _ := unstructured.NestedInt64(desired.Object, "spec", "replicas"); replicas != 3 {
				t.Fatal("rollback changed replica count")
			}
			if _, found, _ := unstructured.NestedString(desired.Object, "spec", "template", "metadata", "annotations", "later-only"); found {
				t.Fatal("rollback merged the old template instead of replacing it")
			}
			if _, found, _ := unstructured.NestedFieldNoCopy(desired.Object, "spec", "template", "$patch"); found {
				t.Fatal("strategic patch directive escaped into the Pod template")
			}
			if kind == "deployment" {
				if _, found, _ := unstructured.NestedString(desired.Object, "spec", "template", "metadata", "labels", "pod-template-hash"); found {
					t.Fatal("ReplicaSet generated label became part of Deployment template")
				}
			}
			if err = validateMutationPolicy(d, desired, false, in.Policy); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRollbackRejectsHistoryReplacementForeignOwnerAndInjectedPatch(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*unstructured.Unstructured, *unstructured.Unstructured, *Mutation)
		want error
	}{
		{"history UID", func(_, h *unstructured.Unstructured, _ *Mutation) { h.SetUID(types.UID("replacement")) }, ErrMutationConflict},
		{"history version", func(_, h *unstructured.Unstructured, _ *Mutation) { h.SetResourceVersion("6") }, ErrMutationConflict},
		{"revision", func(_, _ *unstructured.Unstructured, in *Mutation) { in.Rollback.Revision = 2 }, ErrMutationConflict},
		{"owner UID", func(_, h *unstructured.Unstructured, _ *Mutation) {
			owners := h.GetOwnerReferences()
			owners[0].UID = "foreign"
			h.SetOwnerReferences(owners)
		}, ErrIdentityChanged},
		{"noncontroller owner", func(_, h *unstructured.Unstructured, _ *Mutation) {
			owners := h.GetOwnerReferences()
			owners[0].Controller = nil
			h.SetOwnerReferences(owners)
		}, ErrIdentityChanged},
		{"paused", func(b, _ *unstructured.Unstructured, _ *Mutation) {
			_ = unstructured.SetNestedField(b.Object, true, "spec", "paused")
		}, ErrMutationInvalid},
		{"caller template", func(_, _ *unstructured.Unstructured, in *Mutation) { in.Object = []byte(`{"spec":{"template":{}}}`) }, ErrMutationInvalid},
		{"missing reference", func(_, _ *unstructured.Unstructured, in *Mutation) { in.Rollback = nil }, ErrMutationInvalid},
		{"patch other spec", func(_, h *unstructured.Unstructured, _ *Mutation) {
			_ = unstructured.SetNestedField(h.Object, int64(500), "data", "spec", "replicas")
		}, ErrMutationInvalid},
		{"patch metadata", func(_, h *unstructured.Unstructured, _ *Mutation) {
			_ = unstructured.SetNestedMap(h.Object, map[string]any{"annotations": map[string]any{"injected": "value"}}, "data", "metadata")
		}, ErrMutationInvalid},
		{"patch directive", func(_, h *unstructured.Unstructured, _ *Mutation) {
			_ = unstructured.SetNestedField(h.Object, "merge", "data", "spec", "template", "$patch")
		}, ErrMutationInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			before, history, in := rollbackFixture("statefulset")
			test.edit(before, history, &in)
			c := Client{Dynamic: fake.NewSimpleDynamicClient(runtime.NewScheme(), history)}
			d, _ := Lookup("statefulset")
			if _, err := c.rollbackObject(t.Context(), in, d, before); !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
}

func TestRollbackRechecksHistoricalWorkloadPrivileges(t *testing.T) {
	before, history, in := rollbackFixture("deployment")
	_ = unstructured.SetNestedField(history.Object, true, "spec", "template", "spec", "hostPID")
	c := Client{Dynamic: fake.NewSimpleDynamicClient(runtime.NewScheme(), history)}
	d, _ := Lookup("deployment")
	desired, err := c.rollbackObject(t.Context(), in, d, before)
	if err != nil {
		t.Fatal(err)
	}
	if err = validateMutationPolicy(d, desired, false, MutationPolicy{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("history bypassed workload privileges: %v", err)
	}
}
