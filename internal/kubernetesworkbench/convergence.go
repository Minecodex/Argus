package kubernetesworkbench

import (
	"encoding/json"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Mutation completion records observed state, rather than treating HTTP 2xx as
// workload readiness. Later external changes are reported as superseding it.
func mutationConverged(input kubernetesinventory.Mutation, result kubernetesinventory.MutationResult, accepted []byte) (bool, string) {
	if input.Operation == "drain" {
		if result.Drain == nil || result.After == nil {
			return false, ""
		}
		if result.After.UID != input.Query.UID {
			return false, kubernetesinventory.ErrIdentityChanged.Error()
		}
		return result.Drain.Phase == "complete", ""
	}
	if input.Operation == "delete" {
		return result.Deleted, ""
	}
	if result.After == nil {
		return false, ""
	}
	if input.Query.UID != "" && result.After.UID != input.Query.UID {
		return false, kubernetesinventory.ErrIdentityChanged.Error()
	}
	var previous struct {
		Generation int64  `json:"generation"`
		UID        string `json:"uid"`
	}
	_ = json.Unmarshal(accepted, &previous)
	if previous.UID != "" && previous.UID != result.After.UID {
		return false, kubernetesinventory.ErrIdentityChanged.Error()
	}
	if previous.Generation > 0 && result.After.Generation > previous.Generation {
		return false, "KUBERNETES_OPERATION_SUPERSEDED"
	}
	var value unstructured.Unstructured
	if value.UnmarshalJSON(result.After.Data) != nil {
		return false, kubernetesinventory.ErrProtocol.Error()
	}
	integer := func(path ...string) int64 { n, _, _ := unstructured.NestedInt64(value.Object, path...); return n }
	if input.Operation == "scale" && input.Replicas != nil && integer("spec", "replicas") != *input.Replicas {
		return false, "KUBERNETES_OPERATION_SUPERSEDED"
	}
	if input.Operation == "restart" {
		at, _, _ := unstructured.NestedString(value.Object, "spec", "template", "metadata", "annotations", "kubectl.kubernetes.io/restartedAt")
		if at != input.RestartAt {
			return false, "KUBERNETES_OPERATION_SUPERSEDED"
		}
	}
	if input.Operation == "cordon" || input.Operation == "uncordon" {
		value, _, _ := unstructured.NestedBool(value.Object, "spec", "unschedulable")
		return value == (input.Operation == "cordon"), ""
	}
	switch input.Query.Type {
	case "pdb":
		return integer("status", "observedGeneration") >= result.After.Generation, ""
	case "job":
		conditions, _, _ := unstructured.NestedSlice(value.Object, "status", "conditions")
		suspended, _, _ := unstructured.NestedBool(value.Object, "spec", "suspend")
		complete, suspensionObserved := false, false
		for _, item := range conditions {
			condition, ok := item.(map[string]any)
			if !ok || condition["status"] != "True" {
				continue
			}
			if condition["type"] == "Failed" || condition["type"] == "FailureTarget" {
				return false, "KUBERNETES_WORKLOAD_FAILED"
			}
			if condition["type"] == "Complete" {
				complete = true
			}
			if suspended && condition["type"] == "Suspended" && integer("status", "active") == 0 {
				suspensionObserved = true
			}
		}
		if complete || suspensionObserved {
			return true, ""
		}
		parallel, found, _ := unstructured.NestedInt64(value.Object, "spec", "parallelism")
		if !found {
			parallel = 1
		}
		if suspended || parallel == 0 {
			return !suspended && integer("status", "active") == 0, ""
		}
		if completions, found, _ := unstructured.NestedInt64(value.Object, "spec", "completions"); found {
			parallel = min(parallel, max(0, completions-integer("status", "succeeded")))
		}
		return parallel > 0 && integer("status", "ready") >= parallel, ""
	case "pvc":
		phase, _, _ := unstructured.NestedString(value.Object, "status", "phase")
		if phase == "Lost" {
			return false, "KUBERNETES_VOLUME_LOST"
		}
		if phase != "Bound" {
			return false, ""
		}
		requested, _, _ := unstructured.NestedString(value.Object, "spec", "resources", "requests", "storage")
		capacity, _, _ := unstructured.NestedString(value.Object, "status", "capacity", "storage")
		want, e1 := resource.ParseQuantity(requested)
		actual, e2 := resource.ParseQuantity(capacity)
		if e1 != nil || e2 != nil || want.Sign() <= 0 || actual.Cmp(want) < 0 {
			return false, ""
		}
		conditions, _, _ := unstructured.NestedSlice(value.Object, "status", "conditions")
		for _, item := range conditions {
			condition, ok := item.(map[string]any)
			if ok && condition["status"] == "True" && (condition["type"] == "Resizing" || condition["type"] == "FileSystemResizePending") {
				return false, ""
			}
		}
		return true, ""
	case "deployment", "statefulset", "replicaset":
		desired, found, _ := unstructured.NestedInt64(value.Object, "spec", "replicas")
		if !found {
			desired = 1
		}
		if integer("status", "observedGeneration") < result.After.Generation {
			return false, ""
		}
		ready := integer("status", "readyReplicas") == desired && integer("status", "replicas") == desired
		if input.Query.Type == "deployment" {
			ready = ready && integer("status", "updatedReplicas") == desired && integer("status", "availableReplicas") == desired
		}
		if input.Query.Type == "statefulset" {
			strategy, _, _ := unstructured.NestedString(value.Object, "spec", "updateStrategy", "type")
			partition, _, _ := unstructured.NestedInt64(value.Object, "spec", "updateStrategy", "rollingUpdate", "partition")
			if strategy != "OnDelete" && partition == 0 {
				ready = ready && integer("status", "updatedReplicas") == desired
			}
		}
		return ready, ""
	case "daemonset":
		desired := integer("status", "desiredNumberScheduled")
		return integer("status", "observedGeneration") >= result.After.Generation && integer("status", "updatedNumberScheduled") == desired && integer("status", "numberAvailable") == desired, ""
	case "pod":
		phase, _, _ := unstructured.NestedString(value.Object, "status", "phase")
		if phase == "Failed" {
			return false, "KUBERNETES_WORKLOAD_FAILED"
		}
		if phase == "Succeeded" {
			return true, ""
		}
		conditions, _, _ := unstructured.NestedSlice(value.Object, "status", "conditions")
		for _, item := range conditions {
			m, ok := item.(map[string]any)
			if ok && m["type"] == "Ready" && m["status"] == "True" {
				return true, ""
			}
		}
		return false, ""
	default:
		return true, ""
	}
}
