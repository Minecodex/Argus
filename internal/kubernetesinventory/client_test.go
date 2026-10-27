package kubernetesinventory

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	kubetest "k8s.io/client-go/testing"
)

func TestProjectionMasksSecretsAndEnvironmentBeforeTransport(t *testing.T) {
	definition, _ := Lookup("secret")
	value := unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": "app", "uid": "id", "resourceVersion": "3", "namespace": "ns", "annotations": map[string]any{"kubectl.kubernetes.io/last-applied-configuration": "private-canary"}}, "data": map[string]any{"credentials": "private-base64-canary"}, "stringData": map[string]any{"ordinaryName": "private-string-canary"}}}
	object, err := Project(definition, value, "ns-uid", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"private-canary", "private-base64-canary", "private-string-canary"} {
		if strings.Contains(string(object.Data), private) {
			t.Fatal("secret escaped projection")
		}
	}
	if !object.Redacted || object.NamespaceUID != "ns-uid" {
		t.Fatal("projection provenance missing")
	}
	definition, _ = Lookup("pod")
	value.SetKind("Pod")
	delete(value.Object, "data")
	delete(value.Object, "stringData")
	value.Object["spec"] = map[string]any{"containers": []any{map[string]any{"name": "app", "image": "fixture", "env": []any{map[string]any{"name": "DB_PASSWORD", "value": "private-env-canary"}, map[string]any{"name": "TZ", "value": "UTC"}}}}}
	object, err = Project(definition, value, "ns-uid", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(object.Data), "private-env-canary") || !strings.Contains(string(object.Data), "UTC") {
		t.Fatal("environment masking failed")
	}
	var original map[string]any
	raw, _ := json.Marshal(value.Object)
	_ = json.Unmarshal(raw, &original)
	if !strings.Contains(string(raw), "private-env-canary") {
		t.Fatal("projection mutated source object")
	}
}

func TestPageRejectsNamespaceReplacementAndNeverSkipsOversizedObjects(t *testing.T) {
	namespace := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": "owned", "uid": "old-uid", "resourceVersion": "1"}}}
	client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{{Version: "v1", Resource: "configmaps"}: "ConfigMapList"}, namespace)
	client.PrependReactor("list", "configmaps", func(kubetest.Action) (bool, runtime.Object, error) {
		list := &unstructured.UnstructuredList{Object: map[string]any{"metadata": map[string]any{"resourceVersion": "8", "continue": "next-server-page"}}}
		for _, name := range []string{"a", "b"} {
			list.Items = append(list.Items, unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": name, "namespace": "owned", "uid": name, "resourceVersion": "8"}, "data": map[string]any{"value": strings.Repeat("x", 600)}}})
		}
		return true, list, nil
	})
	reader := Client{Dynamic: client, MaxBytes: 1000}
	query := Query{Type: "configmap", Namespace: "owned", NamespaceUID: "old-uid", Limit: 2}
	page, err := reader.List(t.Context(), query)
	if !errors.Is(err, ErrBudget) || len(page.Items) != 0 || page.Continue != "" {
		t.Fatalf("oversize skipped objects: %+v %v", page, err)
	}
	reader.MaxBytes = 4096
	reads := 0
	client.PrependReactor("get", "namespaces", func(kubetest.Action) (bool, runtime.Object, error) {
		reads++
		copy := namespace.DeepCopy()
		if reads > 1 {
			copy.SetUID("replacement-uid")
		}
		return true, copy, nil
	})
	page, err = reader.List(t.Context(), query)
	if !errors.Is(err, ErrIdentityChanged) || len(page.Items) != 0 {
		t.Fatalf("namespace changed mid-query: %+v %v", page, err)
	}
}
