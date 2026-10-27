package kubernetesinventory

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

func workloadPodPath(definition Definition) []string {
	switch definition.Key {
	case "pod":
		return []string{"spec"}
	case "deployment", "statefulset", "daemonset", "replicaset", "job":
		return []string{"spec", "template", "spec"}
	case "cronjob":
		return []string{"spec", "jobTemplate", "spec", "template", "spec"}
	default:
		return nil
	}
}

// New unprivileged workloads default to no API token. This is included in the
// server dry-run diff. Existing workloads are never rewritten during scaling.
func defaultWorkloadIdentity(definition Definition, value *unstructured.Unstructured, policy MutationPolicy) {
	path := workloadPodPath(definition)
	if policy.ServiceAccountUse || len(path) == 0 {
		return
	}
	field := append(append([]string{}, path...), "automountServiceAccountToken")
	if _, found, _ := unstructured.NestedFieldNoCopy(value.Object, field...); !found {
		_ = unstructured.SetNestedField(value.Object, false, field...)
	}
}

func validateServiceAccountIdentity(pod map[string]any, allowed bool) error {
	if pod == nil || allowed {
		return nil
	}
	if pod["automountServiceAccountToken"] != false {
		return ErrForbidden
	}
	for _, key := range []string{"serviceAccountName", "serviceAccount"} {
		if name, _ := pod[key].(string); name != "" && name != "default" {
			return ErrForbidden
		}
	}
	volumes, _ := pod["volumes"].([]any)
	for _, raw := range volumes {
		volume, _ := raw.(map[string]any)
		projection, _ := volume["projected"].(map[string]any)
		sources, _ := projection["sources"].([]any)
		for _, raw := range sources {
			source, _ := raw.(map[string]any)
			if _, exists := source["serviceAccountToken"]; exists {
				return ErrForbidden
			}
		}
	}
	return nil
}
