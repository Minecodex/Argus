package kubernetesinventory

import (
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

func TestPVCResizeRequiresExpansionCapabilityAndNeverShrinks(t *testing.T) {
	definition, _ := Lookup("pvc")
	claim := func(size, class string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{"spec": map[string]any{"storageClassName": class, "resources": map[string]any{"requests": map[string]any{"storage": size}}}}}
	}
	for _, tc := range []struct {
		name, from, to, class string
		expandable            bool
		want                  error
	}{
		{"shrink", "2Gi", "1Gi", "storage", true, ErrMutationInvalid},
		{"invalid", "1Gi", "nonsense", "storage", true, ErrMutationInvalid},
		{"same equivalent quantity", "1Gi", "1024Mi", "storage", false, nil},
		{"expansion disabled", "1Gi", "2Gi", "storage", false, ErrUnsupported},
		{"static volume has no capability proof", "1Gi", "2Gi", "", true, ErrUnsupported},
		{"declared expansion", "1Gi", "2Gi", "storage", true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			class := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "storage.k8s.io/v1", "kind": "StorageClass", "metadata": map[string]any{"name": "storage"}, "provisioner": "fixture.only", "allowVolumeExpansion": tc.expandable}}
			c := Client{Dynamic: fake.NewSimpleDynamicClient(runtime.NewScheme(), class)}
			if err := c.validatePVCResize(t.Context(), definition, claim(tc.from, tc.class), claim(tc.to, tc.class)); !errors.Is(err, tc.want) {
				t.Fatalf("resize: %v want %v", err, tc.want)
			}
		})
	}
}
