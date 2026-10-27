package kubernetesinventory

import (
	"slices"
	"strings"

	"github.com/kakj-go/Argus/internal/telemetry/datapolicy"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// ValidateMutationResult applies the same policy to admission's effective
// object at the domain boundary, including Connector and future Helm paths.
func ValidateMutationResult(definition Definition, result MutationResult, clusterWide bool, policy MutationPolicy) error {
	if result.After == nil {
		return nil
	}
	value := &unstructured.Unstructured{}
	if value.UnmarshalJSON(result.After.Data) != nil {
		return ErrProtocol
	}
	return validateMutationPolicy(definition, value, clusterWide, policy)
}

func restoreRedactedFields(desired, current map[string]any) error {
	for key, value := range desired {
		previous, exists := current[key]
		if value == datapolicy.Marker {
			if !exists {
				return ErrMutationInvalid
			}
			desired[key] = runtime.DeepCopyJSONValue(previous)
			continue
		}
		switch next := value.(type) {
		case map[string]any:
			old, _ := previous.(map[string]any)
			if err := restoreRedactedFields(next, old); err != nil {
				return err
			}
		case []any:
			old, _ := previous.([]any)
			for index, item := range next {
				object, ok := item.(map[string]any)
				if !ok {
					if item == datapolicy.Marker {
						return ErrMutationInvalid
					}
					continue
				}
				var before map[string]any
				if name, ok := object["name"].(string); ok {
					for _, candidate := range old {
						mapping, _ := candidate.(map[string]any)
						if mapping["name"] == name {
							before = mapping
							break
						}
					}
				} else if index < len(old) {
					before, _ = old[index].(map[string]any)
				}
				if err := restoreRedactedFields(object, before); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Namespace permissions do not authorize host-level Pod settings or wildcard
// RBAC delegation. Kubernetes admission remains an additional authority.
func validateMutationPolicy(definition Definition, value *unstructured.Unstructured, clusterWide bool, policy MutationPolicy) error {
	if (policy.PrivilegedWorkloads || policy.RBACDelegation) && !clusterWide {
		return ErrForbidden
	}
	if definition.Key == "secret" {
		kind, _, _ := unstructured.NestedString(value.Object, "type")
		if kind == "kubernetes.io/service-account-token" && !policy.ServiceAccountUse {
			return ErrForbidden
		}
	}
	if definition.Category == "rbac" {
		// The platform connection may itself be cluster-admin. It cannot use
		// that identity to delegate API rights on behalf of a normal editor.
		if !clusterWide || !policy.RBACDelegation {
			return ErrForbidden
		}
		return nil
	}
	var pod map[string]any
	if path := workloadPodPath(definition); len(path) > 0 {
		pod, _, _ = unstructured.NestedMap(value.Object, path...)
	}
	if err := validateServiceAccountIdentity(pod, policy.ServiceAccountUse); err != nil {
		return err
	}
	if policy.PrivilegedWorkloads {
		return nil
	}
	podSecurity, _ := pod["securityContext"].(map[string]any)
	windows, _ := podSecurity["windowsOptions"].(map[string]any)
	if windows["hostProcess"] == true {
		return ErrForbidden
	}
	for _, key := range []string{"hostNetwork", "hostPID", "hostIPC"} {
		if pod[key] == true {
			return ErrForbidden
		}
	}
	volumes, _ := pod["volumes"].([]any)
	for _, raw := range volumes {
		volume, _ := raw.(map[string]any)
		if _, ok := volume["hostPath"]; ok {
			return ErrForbidden
		}
	}
	for _, field := range []string{"containers", "initContainers", "ephemeralContainers"} {
		containers, _ := pod[field].([]any)
		for _, raw := range containers {
			container, _ := raw.(map[string]any)
			security, _ := container["securityContext"].(map[string]any)
			if security["privileged"] == true {
				return ErrForbidden
			}
			windows, _ := security["windowsOptions"].(map[string]any)
			if windows["hostProcess"] == true || security["procMount"] == "Unmasked" {
				return ErrForbidden
			}
			capabilities, _ := security["capabilities"].(map[string]any)
			added, _ := capabilities["add"].([]any)
			for _, raw := range added {
				name, _ := raw.(string)
				if !slices.Contains([]string{"NET_BIND_SERVICE", "CHOWN", "SETUID", "SETGID"}, strings.ToUpper(name)) {
					return ErrForbidden
				}
			}
			ports, _ := container["ports"].([]any)
			for _, raw := range ports {
				port, _ := raw.(map[string]any)
				if value, ok := port["hostPort"]; ok && value != int64(0) && value != float64(0) {
					return ErrForbidden
				}
			}
			mounts, _ := container["volumeMounts"].([]any)
			for _, raw := range mounts {
				mount, _ := raw.(map[string]any)
				if mount["mountPropagation"] == "Bidirectional" {
					return ErrForbidden
				}
			}
		}
	}
	return nil
}
