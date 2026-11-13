package kubernetesinventory

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Summary is derived only from the public projected object. Missing observations
// remain unknown, not healthy or numeric zero.
type Summary struct {
	State  string            `json:"state"`
	Reason string            `json:"reason,omitempty"`
	Fields map[string]string `json:"fields"`
}
type ActionAvailability struct {
	Action string `json:"action"`
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
}

// Summarize is shared by inventory lists, detail views and dashboard snapshots.
func Summarize(object Object) Summary {
	result := Summary{State: "unknown", Fields: map[string]string{}}
	owners := []string{}
	for _, owner := range object.Owners {
		owners = append(owners, owner.Kind+"/"+owner.Name)
	}
	if len(owners) > 0 {
		result.Fields["owner"] = strings.Join(owners, ", ")
	}
	var data map[string]any
	if json.Unmarshal(object.Data, &data) != nil {
		return result
	}
	value := func(path ...string) any {
		var v any = data
		for _, key := range path {
			m, ok := v.(map[string]any)
			if !ok {
				return nil
			}
			v = m[key]
		}
		return v
	}
	text := func(path ...string) string {
		v := value(path...)
		if v == nil {
			return ""
		}
		switch v.(type) {
		case string, bool, float64:
			return fmt.Sprint(v)
		}
		return ""
	}
	field := func(key string, path ...string) {
		if v := text(path...); v != "" {
			result.Fields[key] = v
		}
	}
	condition := func(kind string) string {
		rows, _ := value("status", "conditions").([]any)
		for _, raw := range rows {
			row, _ := raw.(map[string]any)
			if row["type"] == kind {
				return fmt.Sprint(row["status"])
			}
		}
		return ""
	}
	images := func(path ...string) {
		rows, _ := value(path...).([]any)
		list := []string{}
		for _, raw := range rows {
			row, _ := raw.(map[string]any)
			if v, ok := row["image"].(string); ok && v != "" {
				list = append(list, v)
			}
		}
		if len(list) > 0 {
			result.Fields["images"] = strings.Join(list, ", ")
		}
	}
	ratio := func(key string, a, b float64) { result.Fields[key] = fmt.Sprintf("%g / %g", a, b) }
	statusPresent := value("status") != nil
	switch object.Definition.Key {
	case "pod":
		podRequestSummary(object.Data, result.Fields)
		field("node", "spec", "nodeName")
		field("pod_ip", "status", "podIP")
		field("phase", "status", "phase")
		images("spec", "containers")
		containers, _ := value("spec", "containers").([]any)
		ready, restarts := 0., 0.
		considerReason := func(reason string) {
			if podReasonPriority(reason) > podReasonPriority(result.Reason) {
				result.Reason = reason
			}
			if podReasonPriority(reason) >= 3 {
				result.State = "failed"
			}
		}
		considerTerminated := func(state map[string]any) {
			terminated, _ := state["terminated"].(map[string]any)
			exit, _ := terminated["exitCode"].(float64)
			signal, _ := terminated["signal"].(float64)
			if exit != 0 || signal != 0 {
				reason, _ := terminated["reason"].(string)
				if reason == "" {
					reason = "Error"
				}
				considerReason(reason)
				result.State = "failed"
			}
		}
		statuses, _ := value("status", "containerStatuses").([]any)
		for _, raw := range statuses {
			row, _ := raw.(map[string]any)
			if row["ready"] == true {
				ready++
			}
			count, _ := row["restartCount"].(float64)
			restarts += count
			state, _ := row["state"].(map[string]any)
			waiting, _ := state["waiting"].(map[string]any)
			reason, _ := waiting["reason"].(string)
			considerReason(reason)
			considerTerminated(state)
		}
		restartable := map[string]bool{}
		initializers, _ := value("spec", "initContainers").([]any)
		for _, raw := range initializers {
			row, _ := raw.(map[string]any)
			if row["restartPolicy"] == "Always" {
				name, _ := row["name"].(string)
				restartable[name] = true
			}
		}
		initStatuses, _ := value("status", "initContainerStatuses").([]any)
		for _, raw := range initStatuses {
			row, _ := raw.(map[string]any)
			count, _ := row["restartCount"].(float64)
			restarts += count
			name, _ := row["name"].(string)
			if restartable[name] && row["ready"] == true {
				ready++
			}
			state, _ := row["state"].(map[string]any)
			waiting, _ := state["waiting"].(map[string]any)
			reason, _ := waiting["reason"].(string)
			considerReason(reason)
			considerTerminated(state)
		}
		if statusPresent {
			ratio("ready", ready, float64(len(containers)+len(restartable)))
			result.Fields["restarts"] = fmt.Sprint(restarts)
		}
		if result.State != "failed" {
			switch text("status", "phase") {
			case "Succeeded":
				result.State = "completed"
			case "Failed":
				result.State = "failed"
			case "Pending":
				result.State = "pending"
			case "Running":
				if condition("Ready") == "True" {
					result.State = "healthy"
				} else {
					result.State = "warning"
				}
			}
		}
	case "deployment", "statefulset", "replicaset":
		images("spec", "template", "spec", "containers")
		field("strategy", "spec", "strategy", "type")
		if object.Definition.Key == "statefulset" {
			field("strategy", "spec", "updateStrategy", "type")
			field("service", "spec", "serviceName")
		}
	case "daemonset":
		field("strategy", "spec", "updateStrategy", "type")
		images("spec", "template", "spec", "containers")
	case "job":
		field("active", "status", "active")
		field("succeeded", "status", "succeeded")
		field("failed", "status", "failed")
		field("started_at", "status", "startTime")
		field("completed_at", "status", "completionTime")
		images("spec", "template", "spec", "containers")
		if condition("Failed") == "True" || condition("FailureTarget") == "True" {
			result.State = "failed"
		} else if condition("Complete") == "True" {
			result.State = "completed"
		} else if value("spec", "suspend") == true {
			result.State = "paused"
		} else if statusPresent {
			result.State = "progressing"
		}
	case "cronjob":
		result.State = "configured"
		if value("spec", "suspend") == true {
			result.State = "paused"
		}
		field("schedule", "spec", "schedule")
		field("timezone", "spec", "timeZone")
		field("concurrency", "spec", "concurrencyPolicy")
		field("last_schedule", "status", "lastScheduleTime")
		field("last_success", "status", "lastSuccessfulTime")
		rows, _ := value("status", "active").([]any)
		if statusPresent {
			result.Fields["active"] = strconv.Itoa(len(rows))
		}
	case "node":
		field("kubelet_version", "status", "nodeInfo", "kubeletVersion")
		field("os", "status", "nodeInfo", "osImage")
		field("cpu_allocatable", "status", "allocatable", "cpu")
		field("memory_allocatable", "status", "allocatable", "memory")
		field("pod_capacity", "status", "allocatable", "pods")
		if condition("Ready") == "True" {
			result.State = "healthy"
		} else if condition("Ready") == "False" {
			result.State = "failed"
		}
		if value("spec", "unschedulable") == true {
			result.Fields["scheduling"] = "disabled"
		} else {
			result.Fields["scheduling"] = "enabled"
		}
		for _, c := range []string{"MemoryPressure", "DiskPressure", "PIDPressure", "NetworkUnavailable"} {
			if condition(c) == "True" {
				result.State = "warning"
				result.Reason = c
			}
		}
	case "pvc":
		field("requested_capacity", "spec", "resources", "requests", "storage")
		field("actual_capacity", "status", "capacity", "storage")
		field("storage_class", "spec", "storageClassName")
		field("volume", "spec", "volumeName")
		field("phase", "status", "phase")
		switch text("status", "phase") {
		case "Bound":
			result.State = "healthy"
		case "Pending":
			result.State = "pending"
		case "Lost":
			result.State = "failed"
		}
		for _, c := range []string{"Resizing", "FileSystemResizePending"} {
			if condition(c) == "True" {
				result.State = "progressing"
				result.Reason = c
			}
		}
	case "pv":
		field("capacity", "spec", "capacity", "storage")
		field("storage_class", "spec", "storageClassName")
		field("reclaim_policy", "spec", "persistentVolumeReclaimPolicy")
		field("claim", "spec", "claimRef", "name")
		field("phase", "status", "phase")
		switch text("status", "phase") {
		case "Bound", "Available":
			result.State = "healthy"
		case "Released":
			result.State = "warning"
		case "Failed":
			result.State = "failed"
		}
	case "storageclass":
		result.State = "configured"
		field("provisioner", "provisioner")
		field("binding_mode", "volumeBindingMode")
		field("reclaim_policy", "reclaimPolicy")
		field("expandable", "allowVolumeExpansion")
	case "service":
		result.State = "configured"
		field("service_type", "spec", "type")
		field("cluster_ip", "spec", "clusterIP")
		field("external_name", "spec", "externalName")
		ports, _ := value("spec", "ports").([]any)
		list := []string{}
		for _, raw := range ports {
			p, _ := raw.(map[string]any)
			list = append(list, fmt.Sprintf("%v → %v/%v", p["port"], p["targetPort"], p["protocol"]))
		}
		result.Fields["ports"] = strings.Join(list, ", ")
	case "ingress":
		result.State = "configured"
		field("ingress_class", "spec", "ingressClassName")
		rules, _ := value("spec", "rules").([]any)
		hosts := []string{}
		for _, raw := range rules {
			row, _ := raw.(map[string]any)
			if host, ok := row["host"].(string); ok {
				hosts = append(hosts, host)
			}
		}
		result.Fields["hosts"] = strings.Join(hosts, ", ")
	case "ingressclass":
		result.State = "configured"
		field("controller", "spec", "controller")
	case "hpa":
		field("current_replicas", "status", "currentReplicas")
		field("desired_replicas", "status", "desiredReplicas")
		field("min_replicas", "spec", "minReplicas")
		field("max_replicas", "spec", "maxReplicas")
		field("target", "spec", "scaleTargetRef", "name")
		if condition("ScalingActive") == "True" {
			result.State = "healthy"
		} else if condition("ScalingActive") == "False" {
			result.State = "warning"
		}
	case "pdb":
		field("healthy", "status", "currentHealthy")
		field("desired_healthy", "status", "desiredHealthy")
		field("disruptions_allowed", "status", "disruptionsAllowed")
		field("min_available", "spec", "minAvailable")
		field("max_unavailable", "spec", "maxUnavailable")
		if statusPresent {
			result.State = "configured"
		}
	case "configmap", "secret":
		result.State = "configured"
		field("configuration_type", "type")
		keys := map[string]bool{}
		for _, k := range []string{"data", "binaryData", "stringData"} {
			m, _ := value(k).(map[string]any)
			for key := range m {
				keys[key] = true
			}
		}
		result.Fields["keys"] = strconv.Itoa(len(keys))
	case "namespace":
		field("phase", "status", "phase")
		switch text("status", "phase") {
		case "Active":
			result.State = "healthy"
		case "Terminating":
			result.State = "terminating"
		}
	case "networkpolicy":
		result.State = "configured"
		for _, direction := range []string{"ingress", "egress"} {
			rows, _ := value("spec", direction).([]any)
			result.Fields[direction+"_rules"] = strconv.Itoa(len(rows))
		}
	case "role", "clusterrole":
		result.State = "configured"
		rows, _ := value("rules").([]any)
		result.Fields["rules"] = strconv.Itoa(len(rows))
	case "rolebinding", "clusterrolebinding":
		result.State = "configured"
		field("role", "roleRef", "name")
		rows, _ := value("subjects").([]any)
		result.Fields["subjects"] = strconv.Itoa(len(rows))
	case "serviceaccount":
		result.State = "configured"
		field("automount_token", "automountServiceAccountToken")
	case "controllerrevision":
		result.State = "configured"
		field("revision", "revision")
	case "resourcequota":
		result.State = "configured"
		hard, _ := value("status", "hard").(map[string]any)
		used, _ := value("status", "used").(map[string]any)
		keys := []string{}
		for k := range hard {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := []string{}
		for _, k := range keys {
			u := "—"
			if used[k] != nil {
				u = fmt.Sprint(used[k])
			}
			parts = append(parts, k+": "+u+" / "+fmt.Sprint(hard[k]))
		}
		result.Fields["quota"] = strings.Join(parts, ", ")
	case "limitrange":
		result.State = "configured"
		rows, _ := value("spec", "limits").([]any)
		result.Fields["limits"] = strconv.Itoa(len(rows))
	case "endpointslice":
		result.State = "configured"
		field("address_type", "addressType")
		rows, _ := value("endpoints").([]any)
		result.Fields["endpoints"] = strconv.Itoa(len(rows))
	case "event":
		result.State = "configured"
		field("involved_uid", "involvedObject", "uid")
		if result.Fields["involved_uid"] == "" {
			field("involved_uid", "regarding", "uid")
		}
		field("event_type", "type")
		field("event_reason", "reason")
		field("message", "message")
		field("event_time", "lastTimestamp")
		if result.Fields["event_time"] == "" {
			field("event_time", "eventTime")
		}
		field("count", "count")
		if text("type") == "Warning" {
			result.State = "warning"
		}
	case "crd":
		field("scope", "spec", "scope")
		field("group", "spec", "group")
		field("kind", "spec", "names", "kind")
		if condition("Established") == "True" {
			result.State = "configured"
		}
	default:
		field("phase", "status", "phase")
		if condition("Ready") == "True" {
			result.State = "healthy"
		} else if condition("Ready") == "False" {
			result.State = "warning"
		}
	}
	summarizeRollout(object, &result)
	enrichSummary(object, &result)
	if text("metadata", "deletionTimestamp") != "" {
		result.State = "terminating"
	}
	if result.Reason == "" {
		result.Reason = text("status", "reason")
	}
	for key, v := range result.Fields {
		if len([]rune(v)) > 1024 {
			result.Fields[key] = string([]rune(v)[:1024]) + "…"
		}
	}
	return result
}
