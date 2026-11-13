package kubernetesworkbench

import (
	"context"
	"encoding/json"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
)

// Object actions describe which guarded previews a caller can open. Native API
// privileges/admission are verified again by the existing preview/write path.
func (s Service) objectActions(ctx context.Context, a Actor, objects []kubernetesinventory.Object) {
	permissions := map[string]bool{}
	allowed := func(permission string) bool {
		if permission == "" {
			return false
		}
		v, ok := permissions[permission]
		if !ok {
			v = authorization.Authorize(ctx, s.Store.Queries, a, permission) == nil
			permissions[permission] = v
		}
		return v
	}
	for index := range objects {
		obj := &objects[index]
		summary := kubernetesinventory.Summarize(*obj)
		obj.Summary = &summary
		obj.Actions = []kubernetesinventory.ActionAvailability{}
		add := func(action, permission, reason string) {
			if !allowed(permission) {
				return
			}
			state := "requires_check"
			if reason != "" {
				state = "unavailable"
			}
			obj.Actions = append(obj.Actions, kubernetesinventory.ActionAvailability{Action: action, State: state, Reason: reason})
		}
		key := obj.Definition.Key
		readonly := key == "pv" || key == "storageclass" || key == "event" || key == "endpointslice"
		reason := ""
		if summary.State == "terminating" {
			reason = "terminating"
		}
		if !readonly {
			add("edit", obj.Definition.ManagePermission, reason)
			add("delete", obj.Definition.ManagePermission, reason)
		}
		switch key {
		case "deployment", "statefulset", "replicaset":
			add("scale", obj.Definition.ManagePermission, reason)
		}
		switch key {
		case "deployment", "statefulset", "daemonset":
			add("image", obj.Definition.ManagePermission, reason)
			if key == "deployment" {
				if summary.State == "paused" {
					add("resume", obj.Definition.ManagePermission, reason)
				} else {
					add("suspend", obj.Definition.ManagePermission, reason)
				}
			}
			add("restart", obj.Definition.ManagePermission, reason)
			add("rollback", obj.Definition.ManagePermission, reason)
			add("configure", obj.Definition.ManagePermission, reason)
		case "cronjob":
			add("run", obj.Definition.ManagePermission, reason)
			if summary.State == "paused" {
				add("resume", obj.Definition.ManagePermission, reason)
			} else {
				add("suspend", obj.Definition.ManagePermission, reason)
			}
			add("schedule", obj.Definition.ManagePermission, reason)
		case "job":
			if summary.State == "completed" || summary.State == "failed" {
				reason = "job_finished"
			}
			if summary.State == "paused" {
				add("resume", obj.Definition.ManagePermission, reason)
			} else {
				add("suspend", obj.Definition.ManagePermission, reason)
			}
		case "pvc":
			if summary.State != "healthy" {
				reason = "volume_not_bound"
			}
			add("expand", obj.Definition.ManagePermission, reason)
		case "node":
			var data struct{ Spec struct{ Unschedulable bool } }
			_ = json.Unmarshal(obj.Data, &data)
			if data.Spec.Unschedulable {
				add("uncordon", obj.Definition.ManagePermission, reason)
			} else {
				add("cordon", obj.Definition.ManagePermission, reason)
			}
			add("drain", "kubernetes.node.drain", reason)
		case "pod":
			add("logs", "kubernetes.operational_logs.read", reason)
			if summary.Fields["phase"] != "Running" {
				reason = "container_not_running"
			}
			add("exec", "kubernetes.exec", reason)
		case "configmap", "secret", "hpa", "pdb", "service", "ingress", "networkpolicy", "role", "clusterrole", "rolebinding", "clusterrolebinding", "resourcequota", "limitrange", "namespace", "serviceaccount", "ingressclass":
			add("configure", obj.Definition.ManagePermission, reason)
		}
	}
}
