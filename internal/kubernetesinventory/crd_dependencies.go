package kubernetesinventory

import (
	"context"
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var ErrCRDHasInstances = errors.New("KUBERNETES_CRD_HAS_INSTANCES")

// Every served version lists the same underlying set of CR instances. Prefer
// the storage version, and query across all namespaces. A forbidden, failed or
// incomplete list cannot establish that deleting the definition is safe.
func (c Client) CheckDeletionDependencies(ctx context.Context, definition Definition, crd *unstructured.Unstructured) error {
	if definition.Key != "crd" {
		return nil
	}
	if crd == nil || c.Dynamic == nil {
		return ErrMutationInvalid
	}
	group, _, _ := unstructured.NestedString(crd.Object, "spec", "group")
	plural, _, _ := unstructured.NestedString(crd.Object, "spec", "names", "plural")
	scope, _, _ := unstructured.NestedString(crd.Object, "spec", "scope")
	versions, _, err := unstructured.NestedSlice(crd.Object, "spec", "versions")
	if err != nil || group == "" || plural == "" || scope != "Namespaced" && scope != "Cluster" {
		return ErrMutationInvalid
	}
	selected := ""
	for _, raw := range versions {
		version, ok := raw.(map[string]any)
		if !ok {
			return ErrMutationInvalid
		}
		name, _ := version["name"].(string)
		if version["served"] == true && name != "" {
			if selected == "" || version["storage"] == true {
				selected = name
			}
		}
	}
	if selected == "" {
		return ErrUnavailable
	}
	instances, err := c.Dynamic.Resource(schema.GroupVersionResource{Group: group, Version: selected, Resource: plural}).List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return apiError(err)
	}
	if len(instances.Items) != 0 {
		return ErrCRDHasInstances
	}
	if instances.GetContinue() != "" || instances.GetRemainingItemCount() != nil && *instances.GetRemainingItemCount() != 0 {
		return ErrUnavailable
	}
	return nil
}
