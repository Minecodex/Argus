package kubernetesinventory

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// Delayed binding needs a real scheduled consumer. Tests must not expect an
// unreferenced WaitForFirstConsumer claim to become Bound by itself.
func prepareLivePVCBindingConsumer(t *testing.T, ctx context.Context, api dynamic.Interface, class *unstructured.Unstructured, namespace, claim string) {
	t.Helper()
	mode, _, err := unstructured.NestedString(class.Object, "volumeBindingMode")
	if err != nil {
		t.Fatal(err)
	}
	if mode != "WaitForFirstConsumer" {
		return
	}
	pod, _ := Lookup("pod")
	_, err = api.Resource(pod.GVR()).Namespace(namespace).Create(ctx, &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]any{"name": "pvc-binding-consumer", "namespace": namespace, "labels": map[string]any{"argus.owner": namespace}},
		"spec": map[string]any{
			"automountServiceAccountToken": false, "restartPolicy": "Never", "terminationGracePeriodSeconds": int64(0),
			"securityContext": map[string]any{"runAsNonRoot": true, "runAsUser": int64(10001), "runAsGroup": int64(10001), "seccompProfile": map[string]any{"type": "RuntimeDefault"}},
			"volumes":         []any{map[string]any{"name": "data", "persistentVolumeClaim": map[string]any{"claimName": claim, "readOnly": true}}},
			"containers": []any{map[string]any{
				"name": "consumer", "image": "alpine:3.21", "imagePullPolicy": "Never", "command": []any{"sleep", "600"},
				"securityContext": map[string]any{"allowPrivilegeEscalation": false, "readOnlyRootFilesystem": true, "capabilities": map[string]any{"drop": []any{"ALL"}}},
				"volumeMounts":    []any{map[string]any{"name": "data", "mountPath": "/data", "readOnly": true}},
				"resources":       map[string]any{"requests": map[string]any{"cpu": "5m", "memory": "8Mi"}, "limits": map[string]any{"cpu": "50m", "memory": "24Mi"}},
			}},
		},
	}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal("could not create the owned delayed-binding consumer", err)
	}
}
