package kubernetesinventory

import (
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestCRDDeletionDependencyChecksFailClosedAndUseAllNamespaces(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "plan8.test", Version: "v2", Resource: "widgets"}
	definition, _ := Lookup("crd")
	for _, test := range []struct {
		name string
		list *unstructured.UnstructuredList
		err  error
		want error
	}{
		{"empty", &unstructured.UnstructuredList{}, nil, nil},
		{"instance", &unstructured.UnstructuredList{Items: []unstructured.Unstructured{{}}}, nil, ErrCRDHasInstances},
		{"incomplete", &unstructured.UnstructuredList{Object: map[string]any{"metadata": map[string]any{"continue": "next"}}}, nil, ErrUnavailable},
		{"forbidden", nil, apierrors.NewForbidden(schema.GroupResource{Group: gvr.Group, Resource: gvr.Resource}, "", errors.New("private detail")), ErrForbidden},
		{"unavailable", nil, errors.New("private-password-canary"), ErrUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			api := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{gvr: "WidgetList"})
			called := false
			api.PrependReactor("list", "widgets", func(action ktesting.Action) (bool, runtime.Object, error) {
				called = true
				if action.GetNamespace() != "" || action.GetResource() != gvr {
					t.Fatal("CRD dependency check narrowed namespaces or selected the wrong served version")
				}
				if test.err != nil {
					return true, nil, test.err
				}
				return true, test.list, nil
			})
			crd := &unstructured.Unstructured{Object: map[string]any{"spec": map[string]any{"group": gvr.Group, "scope": "Namespaced", "names": map[string]any{"plural": gvr.Resource}, "versions": []any{map[string]any{"name": "v1", "served": true}, map[string]any{"name": "v2", "served": true, "storage": true}}}}}
			err := (Client{Dynamic: api}).CheckDeletionDependencies(t.Context(), definition, crd)
			if !called || !errors.Is(err, test.want) {
				t.Fatalf("dependency result=%v want %v", err, test.want)
			}
			if test.want != nil && ErrorFromCode(ErrorCode(err)) != test.want {
				t.Fatal("safe dependency error did not survive the Connector round trip")
			}
		})
	}
}
