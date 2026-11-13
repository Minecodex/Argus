package kubernetesinventory

import (
	"encoding/json"
	"fmt"
)

// Workload availability and rollout completion are separate observations. Ready
// old pods must not make an incomplete new revision appear healthy.
func summarizeRollout(object Object, result *Summary) {
	kind := object.Definition.Key
	if kind != "deployment" && kind != "statefulset" && kind != "daemonset" && kind != "replicaset" {
		return
	}
	var data struct {
		Spec struct {
			Replicas       *int64 `json:"replicas"`
			Paused         bool   `json:"paused"`
			UpdateStrategy struct {
				Type          string `json:"type"`
				RollingUpdate struct {
					Partition int64 `json:"partition"`
				} `json:"rollingUpdate"`
			} `json:"updateStrategy"`
		} `json:"spec"`
		Status *struct {
			ObservedGeneration     int64                                   `json:"observedGeneration"`
			Replicas               int64                                   `json:"replicas"`
			Ready                  int64                                   `json:"readyReplicas"`
			Available              int64                                   `json:"availableReplicas"`
			Updated                int64                                   `json:"updatedReplicas"`
			CurrentRevision        string                                  `json:"currentRevision"`
			UpdateRevision         string                                  `json:"updateRevision"`
			DesiredNumberScheduled int64                                   `json:"desiredNumberScheduled"`
			NumberReady            int64                                   `json:"numberReady"`
			NumberAvailable        int64                                   `json:"numberAvailable"`
			UpdatedNumberScheduled int64                                   `json:"updatedNumberScheduled"`
			NumberMisscheduled     int64                                   `json:"numberMisscheduled"`
			Conditions             []struct{ Type, Status, Reason string } `json:"conditions"`
		} `json:"status"`
	}
	if json.Unmarshal(object.Data, &data) != nil || data.Status == nil {
		result.State = "unknown"
		return
	}
	s := data.Status
	desired := int64(1)
	if data.Spec.Replicas != nil {
		desired = *data.Spec.Replicas
	}
	ready, available, updated, actual := s.Ready, s.Available, s.Updated, s.Replicas
	if kind == "daemonset" {
		desired, ready, available, updated, actual = s.DesiredNumberScheduled, s.NumberReady, s.NumberAvailable, s.UpdatedNumberScheduled, s.DesiredNumberScheduled+s.NumberMisscheduled
	}
	result.Fields["desired_replicas"] = fmt.Sprint(desired)
	result.Fields["ready"] = fmt.Sprintf("%d / %d", ready, desired)
	if kind != "statefulset" {
		result.Fields["available"] = fmt.Sprintf("%d / %d", available, desired)
	}
	if kind != "replicaset" {
		result.Fields["updated"] = fmt.Sprintf("%d / %d", updated, desired)
	}
	result.State = "progressing"
	observed := s.ObservedGeneration >= object.Generation
	complete := actual == desired && ready == desired
	switch kind {
	case "deployment", "daemonset":
		complete = complete && updated == desired && available == desired
	case "statefulset":
		result.Fields["current_revision"], result.Fields["update_revision"] = s.CurrentRevision, s.UpdateRevision
		partition := data.Spec.UpdateStrategy.RollingUpdate.Partition
		if data.Spec.UpdateStrategy.Type == "OnDelete" {
			partition = 0
		}
		required := max(int64(0), desired-partition)
		result.Fields["update_target"] = fmt.Sprint(required)
		complete = complete && updated >= required
		if partition == 0 && desired > 0 {
			complete = complete && s.CurrentRevision != "" && s.CurrentRevision == s.UpdateRevision
		}
	}
	if observed && complete {
		result.State = "healthy"
	}
	if observed && desired == 0 && actual == 0 {
		result.State = "scaled_down"
	}
	if observed && ready == desired && !complete && data.Spec.UpdateStrategy.Type == "OnDelete" {
		result.State, result.Reason = "warning", "UpdateRequiresRecreation"
	}
	if kind == "deployment" && data.Spec.Paused {
		result.State = "paused"
	}
	for _, c := range s.Conditions {
		if (c.Type == "ReplicaFailure" && c.Status == "True") || (c.Type == "Progressing" && c.Status == "False") {
			result.State, result.Reason = "failed", c.Reason
		}
	}
}
