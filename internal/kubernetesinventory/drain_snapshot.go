package kubernetesinventory

import (
	"context"
	"encoding/json"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func (c Client) drainSnapshot(ctx context.Context, node string) (*DrainPlan, map[string]*unstructured.Unstructured, error) {
	plan := &DrainPlan{Pods: []DrainPod{}}
	objects := map[string]*unstructured.Unstructured{}
	cursor := ""
	for {
		page, err := c.Dynamic.Resource(drainPods).List(ctx, metav1.ListOptions{FieldSelector: fields.OneTermEqualSelector("spec.nodeName", node).String(), Limit: drainLimit, Continue: cursor})
		if err != nil {
			return nil, nil, apiError(err)
		}
		for i := range page.Items {
			pod := &page.Items[i]
			assigned, _, _ := unstructured.NestedString(pod.Object, "spec", "nodeName")
			if assigned != node || pod.GetUID() == "" {
				return nil, nil, ErrProtocol
			}
			if len(plan.Pods) >= drainLimit {
				return nil, nil, ErrBudget
			}
			nsUID, err := c.namespaceUID(ctx, pod.GetNamespace(), "")
			if err != nil {
				return nil, nil, err
			}
			entry := DrainPod{Namespace: pod.GetNamespace(), NamespaceUID: nsUID, Name: pod.GetName(), UID: string(pod.GetUID()), Volumes: []DrainVolume{}}
			entry.SpecDigest = drainHash([]any{pod.Object["spec"], pod.GetOwnerReferences(), pod.GetLabels(), pod.GetAnnotations()["kubernetes.io/config.mirror"]})
			entry.Disposition, err = c.drainDisposition(ctx, pod)
			if err != nil {
				return nil, nil, err
			}
			if entry.Disposition == "evict" {
				entry.Volumes, err = c.drainVolumes(ctx, pod)
				if err != nil {
					return nil, nil, err
				}
				for _, volume := range entry.Volumes {
					if volume.Local {
						entry.Disposition = "local_storage"
					}
				}
			}
			plan.Pods = append(plan.Pods, entry)
			objects[entry.UID] = pod.DeepCopy()
		}
		cursor = page.GetContinue()
		if cursor == "" {
			break
		}
	}
	sort.Slice(plan.Pods, func(i, j int) bool {
		a, b := plan.Pods[i], plan.Pods[j]
		return a.Namespace+"/"+a.Name < a.Namespace+"/"+b.Name
	})
	return plan, objects, nil
}

func (c Client) drainDisposition(ctx context.Context, pod *unstructured.Unstructured) (string, error) {
	if pod.GetAnnotations()["kubernetes.io/config.mirror"] != "" {
		return "mirror", nil
	}
	phase, _, _ := unstructured.NestedString(pod.Object, "status", "phase")
	if phase == "Succeeded" || phase == "Failed" {
		return "completed", nil
	}
	for _, owner := range pod.GetOwnerReferences() {
		if owner.Controller == nil || !*owner.Controller {
			continue
		}
		var gvr schema.GroupVersionResource
		switch {
		case owner.APIVersion == "apps/v1" && owner.Kind == "DaemonSet":
			gvr = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}
		case owner.APIVersion == "apps/v1" && owner.Kind == "ReplicaSet":
			gvr = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}
		case owner.APIVersion == "apps/v1" && owner.Kind == "StatefulSet":
			gvr = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
		case owner.APIVersion == "batch/v1" && owner.Kind == "Job":
			gvr = schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}
		case owner.APIVersion == "v1" && owner.Kind == "ReplicationController":
			gvr = schema.GroupVersionResource{Version: "v1", Resource: "replicationcontrollers"}
		default:
			return "unmanaged", nil
		}
		object, err := c.Dynamic.Resource(gvr).Namespace(pod.GetNamespace()).Get(ctx, owner.Name, metav1.GetOptions{})
		if err != nil {
			return "", apiError(err)
		}
		if object.GetUID() != owner.UID {
			return "", ErrIdentityChanged
		}
		if owner.Kind == "DaemonSet" {
			return "daemonset", nil
		}
		return "evict", nil
	}
	return "unmanaged", nil
}

func (c Client) drainVolumes(ctx context.Context, pod *unstructured.Unstructured) ([]DrainVolume, error) {
	result := []DrainVolume{}
	volumes, _, _ := unstructured.NestedSlice(pod.Object, "spec", "volumes")
	for _, raw := range volumes {
		value, ok := raw.(map[string]any)
		if !ok {
			return nil, ErrProtocol
		}
		name, _ := value["name"].(string)
		for kind := range value {
			switch kind {
			case "name", "emptyDir", "hostPath", "ephemeral", "persistentVolumeClaim", "projected", "secret", "configMap", "downwardAPI", "image":
				continue
			}
			result = append(result, DrainVolume{Name: name, Kind: kind, Local: true})
		}
		for _, kind := range []string{"emptyDir", "hostPath", "ephemeral", "persistentVolumeClaim"} {
			if value[kind] == nil {
				continue
			}
			v := DrainVolume{Name: name, Kind: kind, Local: kind != "persistentVolumeClaim"}
			if kind == "persistentVolumeClaim" {
				claim, _, _ := unstructured.NestedString(value, kind, "claimName")
				pvc, err := c.Dynamic.Resource(drainPVCs).Namespace(pod.GetNamespace()).Get(ctx, claim, metav1.GetOptions{})
				if err != nil {
					return nil, apiError(err)
				}
				v.Claim, v.ClaimUID = claim, string(pvc.GetUID())
				v.Volume, _, _ = unstructured.NestedString(pvc.Object, "spec", "volumeName")
				if v.Volume == "" {
					v.Local = true
				} else {
					pv, err := c.Dynamic.Resource(drainPVs).Get(ctx, v.Volume, metav1.GetOptions{})
					if err != nil {
						return nil, apiError(err)
					}
					v.VolumeUID = string(pv.GetUID())
					boundUID, _, _ := unstructured.NestedString(pv.Object, "spec", "claimRef", "uid")
					if boundUID != v.ClaimUID {
						return nil, ErrIdentityChanged
					}
					local, _, _ := unstructured.NestedMap(pv.Object, "spec", "local")
					hostPath, _, _ := unstructured.NestedMap(pv.Object, "spec", "hostPath")
					affinity, _, _ := unstructured.NestedMap(pv.Object, "spec", "nodeAffinity")
					v.Local = local != nil || hostPath != nil || affinity != nil
				}
			}
			result = append(result, v)
		}
	}
	return result, nil
}

func validateDrainSubset(approved, current *DrainPlan) error {
	if approved == nil || len(approved.Pods) > drainLimit {
		return ErrMutationInvalid
	}
	known := map[string]DrainPod{}
	for _, p := range approved.Pods {
		if p.UID == "" || known[p.UID].UID != "" {
			return ErrMutationInvalid
		}
		known[p.UID] = p
	}
	for _, p := range current.Pods {
		old, ok := known[p.UID]
		if !ok || drainHash(old) != drainHash(p) {
			return ErrMutationConflict
		}
	}
	return nil
}

func (c Client) drainBudgets(ctx context.Context, pod *unstructured.Unstructured) ([]DrainBudget, error) {
	result := []DrainBudget{}
	page, err := c.Dynamic.Resource(drainPDBs).Namespace(pod.GetNamespace()).List(ctx, metav1.ListOptions{Limit: drainLimit})
	if err != nil {
		return nil, apiError(err)
	}
	if page.GetContinue() != "" || len(page.Items) > drainLimit {
		return nil, ErrBudget
	}
	for _, value := range page.Items {
		raw, found, _ := unstructured.NestedMap(value.Object, "spec", "selector")
		if !found {
			continue
		}
		var selector metav1.LabelSelector
		data, _ := json.Marshal(raw)
		if json.Unmarshal(data, &selector) != nil {
			return nil, ErrProtocol
		}
		match, err := metav1.LabelSelectorAsSelector(&selector)
		if err != nil {
			return nil, ErrProtocol
		}
		if !match.Matches(labels.Set(pod.GetLabels())) {
			continue
		}
		allowed, _, _ := unstructured.NestedInt64(value.Object, "status", "disruptionsAllowed")
		result = append(result, DrainBudget{Name: value.GetName(), UID: string(value.GetUID()), DisruptionsAllowed: allowed})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}
