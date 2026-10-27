package kubernetesinventory

import (
	"encoding/json"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestServiceDigestKeepsExplicitAllocationsAndOtherIntent(t *testing.T) {
	service := func(ip string, port int64) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Service", "metadata": map[string]any{"name": "api"}, "spec": map[string]any{"type": "NodePort", "clusterIP": ip, "clusterIPs": []any{ip}, "ports": []any{map[string]any{"port": int64(80), "protocol": "TCP", "nodePort": port}}}}}
	}
	input := Mutation{Operation: "create", Query: Query{Type: "service"}, Object: json.RawMessage(`{"spec":{"type":"NodePort","ports":[{"port":80}]}}`)}
	first, second := service("10.0.0.1", 30001), service("10.0.0.2", 30002)
	if stableMutationDigest(first, input) != stableMutationDigest(second, input) {
		t.Fatal("automatic Service allocation made approved input unstable")
	}
	input.Object = json.RawMessage(`{"spec":{"type":"NodePort","clusterIP":"10.0.0.1","ports":[{"port":80,"nodePort":30001}]}}`)
	if stableMutationDigest(first, input) == stableMutationDigest(second, input) {
		t.Fatal("explicit Service allocation escaped approval")
	}
	input.Operation = "patch"
	input.Object = json.RawMessage(`{"spec":{"ports":[{"port":80}]}}`)
	second = service("10.0.0.1", 30002)
	if stableMutationDigest(first, input) != stableMutationDigest(second, input) {
		t.Fatal("patch with an automatically allocated port requires a full-kind manifest")
	}
	_ = unstructured.SetNestedField(second.Object, "changed", "metadata", "labels", "team")
	if stableMutationDigest(first, input) == stableMutationDigest(second, input) {
		t.Fatal("unrelated admission changes escaped approval")
	}
}

func TestPVCDigestAllowsControllerBindingButProtectsExplicitVolumeAndAnnotations(t *testing.T) {
	claim := func(volume string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": map[string]any{"name": "data"}, "spec": map[string]any{"volumeName": volume, "resources": map[string]any{"requests": map[string]any{"storage": "1Gi"}}}}}
	}
	input := Mutation{Operation: "create", Query: Query{Type: "pvc"}, Object: json.RawMessage(`{"spec":{"resources":{"requests":{"storage":"1Gi"}}}}`)}
	unbound, bound := claim(""), claim("allocated")
	bound.SetAnnotations(map[string]string{"pv.kubernetes.io/bind-completed": "yes"})
	if stableMutationDigest(unbound, input) != stableMutationDigest(bound, input) {
		t.Fatal("controller binding prevented observation of the approved PVC")
	}
	input.Object = json.RawMessage(`{"spec":{"volumeName":"selected","resources":{"requests":{"storage":"1Gi"}}}}`)
	if stableMutationDigest(unbound, input) == stableMutationDigest(bound, input) {
		t.Fatal("explicit volume choice was discarded")
	}
	if validatePVCRequestedMetadata(nil, bound) == nil {
		t.Fatal("user wrote controller binding metadata")
	}
	bound.SetAnnotations(map[string]string{"pv.kubernetes.io/bind-completed": ""})
	if validatePVCRequestedMetadata(nil, bound) == nil {
		t.Fatal("empty controller annotation bypassed ownership validation")
	}
	if validatePVCRequestedMetadata(bound, bound.DeepCopy()) != nil {
		t.Fatal("unchanged server metadata could not be preserved")
	}
}
