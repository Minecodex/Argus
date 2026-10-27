package kubernetesinventory

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

func TestJobDigestIgnoresOnlyAutomaticUIDSelectors(t *testing.T) {
	job := func(uid string, manual bool) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{"apiVersion": "batch/v1", "kind": "Job", "metadata": map[string]any{"name": "job", "uid": uid, "labels": map[string]any{"team": "orders"}}, "spec": map[string]any{"manualSelector": manual, "selector": map[string]any{"matchLabels": map[string]any{"batch.kubernetes.io/controller-uid": uid}}, "template": map[string]any{"metadata": map[string]any{"labels": map[string]any{"batch.kubernetes.io/controller-uid": uid, "batch.kubernetes.io/job-name": "job"}}, "spec": map[string]any{"restartPolicy": "Never"}}}}}
	}
	first, second := job("dry-run", false), job("actual", false)
	if stableObjectDigest(first) != stableObjectDigest(second) {
		t.Fatal("automatic Job UID made identical approved operations conflict")
	}
	if first.GetUID() != types.UID("dry-run") {
		t.Fatal("digest changed the original API object")
	}
	_ = unstructured.SetNestedField(second.Object, "payments", "metadata", "labels", "team")
	if stableObjectDigest(first) == stableObjectDigest(second) {
		t.Fatal("unrelated label change escaped the approved digest")
	}
	if stableObjectDigest(job("first", true)) == stableObjectDigest(job("second", true)) {
		t.Fatal("explicit manual selector was treated as generated")
	}
}
