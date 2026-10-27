package kubernetesinventory

import (
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func requestedAllocation(value map[string]any, path ...string) bool {
	field, found, err := unstructured.NestedFieldNoCopy(value, path...)
	if err != nil || !found || field == nil {
		return false
	}
	switch typed := field.(type) {
	case string:
		return typed != ""
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case []any:
		return len(typed) > 0
	default:
		return true
	}
}

func servicePortIdentity(value map[string]any) string {
	if name, ok := value["name"].(string); ok && name != "" {
		return "name/" + name
	}
	protocol, _ := value["protocol"].(string)
	if protocol == "" {
		protocol = "TCP"
	}
	return fmt.Sprint(value["port"]) + "/" + protocol
}

// Only fields delegated to Kubernetes allocation are omitted. Explicit user
// choices remain part of approval; unrelated admission changes still conflict.
func stableMutationDigest(value *unstructured.Unstructured, input Mutation) string {
	copy := value.DeepCopy()
	var requested unstructured.Unstructured
	if len(input.Object) > 0 && json.Unmarshal(input.Object, &requested.Object) != nil {
		return stableObjectDigest(copy)
	}
	switch input.Query.Type {
	case "service":
		if input.Operation == "create" {
			for _, field := range []string{"clusterIP", "clusterIPs", "healthCheckNodePort"} {
				if !requestedAllocation(requested.Object, "spec", field) {
					unstructured.RemoveNestedField(copy.Object, "spec", field)
				}
			}
		}
		ports, found, _ := unstructured.NestedSlice(requested.Object, "spec", "ports")
		if found {
			delegated := map[string]bool{}
			for _, raw := range ports {
				port, ok := raw.(map[string]any)
				if ok {
					delegated[servicePortIdentity(port)] = !requestedAllocation(port, "nodePort")
				}
			}
			actual, found, _ := unstructured.NestedSlice(copy.Object, "spec", "ports")
			if found {
				for _, raw := range actual {
					port, ok := raw.(map[string]any)
					if ok && delegated[servicePortIdentity(port)] {
						delete(port, "nodePort")
					}
				}
				_ = unstructured.SetNestedSlice(copy.Object, actual, "spec", "ports")
			}
		}
	case "pvc":
		if !requestedAllocation(requested.Object, "spec", "volumeName") {
			unstructured.RemoveNestedField(copy.Object, "spec", "volumeName")
		}
		annotations := copy.GetAnnotations()
		for key := range annotations {
			if pvcControllerAnnotation(key) {
				delete(annotations, key)
			}
		}
		if len(annotations) == 0 {
			unstructured.RemoveNestedField(copy.Object, "metadata", "annotations")
		} else {
			copy.SetAnnotations(annotations)
		}
	}
	return stableObjectDigest(copy)
}

func pvcControllerAnnotation(key string) bool {
	return strings.HasPrefix(key, "pv.kubernetes.io/") || strings.HasPrefix(key, "volume.kubernetes.io/") || strings.HasPrefix(key, "volume.beta.kubernetes.io/")
}

func validatePVCRequestedMetadata(before, desired *unstructured.Unstructured) error {
	previous := map[string]string{}
	if before != nil {
		previous = before.GetAnnotations()
	}
	current := desired.GetAnnotations()
	for key, value := range current {
		prior, exists := previous[key]
		if pvcControllerAnnotation(key) && (!exists || prior != value) {
			return ErrMutationInvalid
		}
	}
	for key, value := range previous {
		next, exists := current[key]
		if pvcControllerAnnotation(key) && (!exists || next != value) {
			return ErrMutationInvalid
		}
	}
	return nil
}
