package kubernetesmetrics

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	corev1 "k8s.io/api/core/v1"
	resourcehelper "k8s.io/component-helpers/resource"
	"slices"
)

func number(v float64) *float64 { return &v }
func rowFrom(o kubernetesinventory.Object, refs map[string]uuid.UUID) row {
	r := row{UID: o.UID, Name: o.Name, Namespace: o.Namespace, NamespaceID: refs[o.NamespaceUID], NamespaceUID: o.NamespaceUID, Created: o.CreatedAt, Capacity: map[string]float64{}, Allocatable: map[string]float64{}}
	if o.Definition.Key == "namespace" {
		r.NamespaceID = refs[o.UID]
		r.Namespace = o.Name
		r.NamespaceUID = o.UID
	}
	if o.Definition.Key == "node" {
		var n corev1.Node
		if json.Unmarshal(o.Data, &n) == nil {
			for key, name := range map[string]corev1.ResourceName{"cpu": corev1.ResourceCPU, "memory": corev1.ResourceMemory, "storage": corev1.ResourceEphemeralStorage} {
				q := n.Status.Capacity[name]
				r.Capacity[key] = q.AsApproximateFloat64()
				q = n.Status.Allocatable[name]
				r.Allocatable[key] = q.AsApproximateFloat64()
			}
		}
	}
	if o.Definition.Key == "pvc" {
		var p corev1.PersistentVolumeClaim
		if json.Unmarshal(o.Data, &p) == nil {
			r.Capacity["storage"] = p.Status.Capacity.Storage().AsApproximateFloat64()
		}
	}
	return r
}
func podNode(o kubernetesinventory.Object) string {
	var p struct {
		Spec struct {
			NodeName string `json:"nodeName"`
		} `json:"spec"`
	}
	_ = json.Unmarshal(o.Data, &p)
	return p.Spec.NodeName
}
func podFrom(o kubernetesinventory.Object, refs map[string]uuid.UUID) pod {
	p := pod{UID: o.UID, Name: o.Name, Namespace: o.Namespace, NamespaceID: refs[o.NamespaceUID], NamespaceUID: o.NamespaceUID, Created: o.CreatedAt, Budgets: map[string]budget{}}
	var native corev1.Pod
	if json.Unmarshal(o.Data, &native) != nil {
		return p
	}
	p.Terminal = native.Status.Phase == corev1.PodSucceeded || native.Status.Phase == corev1.PodFailed
	p.Containers = len(native.Spec.Containers)
	for _, c := range native.Spec.InitContainers {
		if c.RestartPolicy != nil && *c.RestartPolicy == corev1.ContainerRestartPolicyAlways {
			p.Containers++
		}
	}
	if native.Annotations["kubernetes.io/config.source"] == "file" {
		p.Alias = native.Annotations["kubernetes.io/config.mirror"]
	}
	requests := resourcehelper.PodRequests(&native, resourcehelper.PodResourcesOptions{})
	limits := resourcehelper.PodLimits(&native, resourcehelper.PodResourcesOptions{})
	for key, name := range map[string]corev1.ResourceName{"cpu": corev1.ResourceCPU, "memory": corev1.ResourceMemory, "storage": corev1.ResourceEphemeralStorage} {
		q := requests[name]
		l := limits[name]
		b := budget{}
		if q.Sign() > 0 {
			b.Request = number(q.AsApproximateFloat64())
		}
		if l.Sign() > 0 {
			b.Limit = number(l.AsApproximateFloat64())
		}
		rqCount, lmCount := 0, 0
		for _, c := range native.Spec.Containers {
			q := c.Resources.Requests[name]
			l := c.Resources.Limits[name]
			if q.Sign() > 0 {
				rqCount++
			}
			if l.Sign() > 0 {
				lmCount++
			}
		}
		b.Partial = rqCount > 0 && rqCount < len(native.Spec.Containers) || lmCount > 0 && lmCount < len(native.Spec.Containers)
		podLimitSet := false
		if native.Spec.Resources != nil {
			q := native.Spec.Resources.Limits[name]
			podLimitSet = q.Sign() > 0
		}
		if lmCount < len(native.Spec.Containers) && !podLimitSet {
			b.Limit = nil
		}
		p.Budgets[key] = b
	}
	// Container gauges cannot reconstruct historical Pod-level/init/overhead
	// scheduling semantics. Keep actual usage and current effective configuration.
	if native.Spec.Resources != nil || len(native.Spec.Overhead) > 0 {
		p.Containers = -1
	}
	for _, c := range native.Spec.InitContainers {
		if c.RestartPolicy == nil || *c.RestartPolicy != corev1.ContainerRestartPolicyAlways {
			p.Containers = -1
		}
	}
	for _, v := range native.Spec.Volumes {
		if v.PersistentVolumeClaim != nil {
			p.Volumes = append(p.Volumes, v.PersistentVolumeClaim.ClaimName)
		}
	}
	return p
}
func aggregateBudgets(pods []pod, parent string) map[string]budget {
	out := map[string]budget{}
	for _, key := range []string{"cpu", "memory", "storage"} {
		n := 0
		rq, lm := 0., 0.
		allR, allL := true, true
		partial := false
		for _, p := range pods {
			if !slices.Contains(p.Parents, parent) {
				continue
			}
			n++
			b := p.Budgets[key]
			partial = partial || b.Partial
			if b.Request != nil {
				rq += *b.Request
			} else {
				allR = false
			}
			if b.Limit != nil {
				lm += *b.Limit
			} else {
				allL = false
			}
		}
		b := budget{Partial: partial}
		if n > 0 && allR {
			b.Request = number(rq)
		}
		if n > 0 && allL {
			b.Limit = number(lm)
		}
		out[key] = b
	}
	return out
}
