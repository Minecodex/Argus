package kubernetesreader

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// Summaries expose observed health and capacity, never the full Pod spec,
// annotations, environment variables, mounted secrets or credentials.
func resourceSummary(kind string, object unstructured.Unstructured) map[string]any {
	result := map[string]any{"uid": string(object.GetUID()), "created_at": object.GetCreationTimestamp().Time}
	status := map[string]any{}
	copyField := func(name string, path ...string) {
		if value, exists, err := unstructured.NestedFieldCopy(object.Object, path...); err == nil && exists {
			status[name] = value
		}
	}
	switch kind {
	case "daemonset":
		for _, key := range []string{"desiredNumberScheduled", "currentNumberScheduled", "numberReady", "numberAvailable", "numberUnavailable", "numberMisscheduled", "updatedNumberScheduled", "observedGeneration"} {
			copyField(key, "status", key)
		}
	case "deployment", "statefulset":
		for _, key := range []string{"replicas", "readyReplicas", "availableReplicas", "updatedReplicas", "currentReplicas", "observedGeneration"} {
			copyField(key, "status", key)
		}
		if desired, exists, err := unstructured.NestedFieldCopy(object.Object, "spec", "replicas"); err == nil && exists {
			result["desired_replicas"] = desired
		}
	case "node":
		copyField("capacity", "status", "capacity")
		copyField("allocatable", "status", "allocatable")
		copyField("nodeInfo", "status", "nodeInfo")
		conditions, _, _ := unstructured.NestedSlice(object.Object, "status", "conditions")
		for _, condition := range conditions {
			value, _ := condition.(map[string]any)
			if value["type"] == "Ready" {
				status["ready"] = value["status"]
				break
			}
		}
	case "pod":
		declared, _, _ := unstructured.NestedSlice(object.Object, "spec", "containers")
		names := []string{}
		for _, item := range declared {
			container, _ := item.(map[string]any)
			if name, ok := container["name"].(string); ok && name != "" {
				names = append(names, name)
			}
		}
		result["container_names"] = names
		copyField("phase", "status", "phase")
		copyField("podIP", "status", "podIP")
		copyField("startTime", "status", "startTime")
		if node, exists, _ := unstructured.NestedString(object.Object, "spec", "nodeName"); exists {
			result["node"] = node
		}
		containers, exists, _ := unstructured.NestedSlice(object.Object, "status", "containerStatuses")
		if exists {
			ready, restarts := int64(0), int64(0)
			for _, container := range containers {
				value, _ := container.(map[string]any)
				if value["ready"] == true {
					ready++
				}
				if count, ok := value["restartCount"].(int64); ok {
					restarts += count
				}
			}
			status["readyContainers"], status["totalContainers"], status["restarts"] = ready, int64(len(containers)), restarts
		}
	case "namespace":
		copyField("phase", "status", "phase")
	case "service":
		for _, key := range []string{"type", "clusterIP", "clusterIPs", "ports"} {
			if value, exists, err := unstructured.NestedFieldCopy(object.Object, "spec", key); err == nil && exists {
				result[key] = value
			}
		}
	}
	if len(status) > 0 {
		result["status"] = status
	}
	return result
}
