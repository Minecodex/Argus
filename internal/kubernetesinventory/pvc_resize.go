package kubernetesinventory

import (
	"context"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// The Kubernetes API permits some pending-claim request edits. The workbench
// promises expansion only, so its policy is deliberately stricter than that.
func (c Client) validatePVCResize(ctx context.Context, definition Definition, before, desired *unstructured.Unstructured) error {
	if definition.Key != "pvc" || before == nil {
		return nil
	}
	oldText, oldFound, oldErr := unstructured.NestedString(before.Object, "spec", "resources", "requests", "storage")
	newText, newFound, newErr := unstructured.NestedString(desired.Object, "spec", "resources", "requests", "storage")
	if oldErr != nil || newErr != nil || !oldFound || !newFound {
		return ErrMutationInvalid
	}
	oldSize, e1 := resource.ParseQuantity(oldText)
	newSize, e2 := resource.ParseQuantity(newText)
	if e1 != nil || e2 != nil || oldSize.Sign() <= 0 || newSize.Sign() <= 0 || newSize.Cmp(oldSize) < 0 {
		return ErrMutationInvalid
	}
	if newSize.Cmp(oldSize) == 0 {
		return nil
	}
	className, found, e := unstructured.NestedString(before.Object, "spec", "storageClassName")
	if e != nil || !found || className == "" {
		return ErrUnsupported
	}
	definition, _ = Lookup("storageclass")
	class, e := c.Dynamic.Resource(definition.GVR()).Get(ctx, className, metav1.GetOptions{})
	if e != nil {
		return apiError(e)
	}
	allowed, _, e := unstructured.NestedBool(class.Object, "allowVolumeExpansion")
	if e != nil || !allowed {
		return ErrUnsupported
	}
	return nil
}
