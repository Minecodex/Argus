package kubernetesinventory

import (
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

func TestCustomTypeRegistrationIsBoundToCRDUID(t *testing.T) {
	crd := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition", "metadata": map[string]any{"name": "widgets.plan8.test", "uid": "original-type", "resourceVersion": "10"}, "spec": map[string]any{"group": "plan8.test", "scope": "Namespaced", "names": map[string]any{"plural": "widgets", "kind": "Widget"}, "versions": []any{map[string]any{"name": "v1", "served": true, "storage": true}}}}}
	namespace := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": "owned", "uid": "namespace-uid", "resourceVersion": "10"}}}
	widget := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "plan8.test/v1", "kind": "Widget", "metadata": map[string]any{"name": "first", "namespace": "owned", "uid": "object-uid", "resourceVersion": "11"}, "spec": map[string]any{"replicas": int64(1)}}}
	gvr := schema.GroupVersionResource{Group: "plan8.test", Version: "v1", Resource: "widgets"}
	client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{crdResource: "CustomResourceDefinitionList", gvr: "WidgetList"}, crd, namespace, widget)
	reader := Client{Dynamic: client}
	catalog, err := reader.Types(t.Context())
	if err != nil || !catalog.Complete || len(catalog.Items) != len(Definitions())+1 {
		t.Fatalf("type discovery: %+v %v", catalog, err)
	}
	key := "custom/plan8.test/v1/widgets@original-type"
	page, err := reader.List(t.Context(), Query{Type: key, Namespace: "owned", NamespaceUID: "namespace-uid", Limit: 10})
	if err != nil || len(page.Items) != 1 || page.Items[0].Definition.Key != key {
		t.Fatalf("registered CRD read: %+v %v", page, err)
	}
	replacement := crd.DeepCopy()
	replacement.SetUID("replacement-type")
	if _, err = client.Resource(crdResource).Update(t.Context(), replacement, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err = reader.Definition(t.Context(), key); !errors.Is(err, ErrIdentityChanged) {
		t.Fatalf("recreated CRD inherited old key: %v", err)
	}
	if _, err = reader.Definition(t.Context(), "custom/plan8.test/v1/widgets"); err == nil {
		t.Fatal("unregistered GVR accepted without type identity")
	}
}
