package kubernetesinventory

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// A dry-run Job receives a fresh API UID on every request. Its automatic
// selector follows that UID, so those generated values cannot bind approval.
// Explicit manual selectors and every unrelated label remain in the digest.
func normalizeGeneratedJobIdentity(value *unstructured.Unstructured) {
	if value.GetAPIVersion() != "batch/v1" || value.GetKind() != "Job" || value.GetUID() == "" {
		return
	}
	manual, _, _ := unstructured.NestedBool(value.Object, "spec", "manualSelector")
	if manual {
		return
	}
	for _, path := range [][]string{{"metadata", "labels"}, {"spec", "selector", "matchLabels"}, {"spec", "template", "metadata", "labels"}} {
		labels, found, err := unstructured.NestedStringMap(value.Object, path...)
		if err != nil || !found {
			continue
		}
		for _, key := range []string{"controller-uid", "batch.kubernetes.io/controller-uid"} {
			if labels[key] == string(value.GetUID()) {
				delete(labels, key)
			}
		}
		if len(labels) == 0 {
			unstructured.RemoveNestedField(value.Object, path...)
		} else {
			_ = unstructured.SetNestedStringMap(value.Object, labels, path...)
		}
	}
	selector, found, _ := unstructured.NestedMap(value.Object, "spec", "selector")
	if found && len(selector) == 0 {
		unstructured.RemoveNestedField(value.Object, "spec", "selector")
	}
}
