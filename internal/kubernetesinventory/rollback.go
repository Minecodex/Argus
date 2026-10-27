package kubernetesinventory

import (
	"context"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation"
)

// RollbackReference freezes a concrete controller-owned history object. The
// executable Pod template is always read from Kubernetes, never from a caller.
type RollbackReference struct {
	Name            string `json:"name"`
	UID             string `json:"uid"`
	ResourceVersion string `json:"resource_version"`
	Revision        int64  `json:"revision"`
}

func rollbackHistoryType(workload string) string {
	switch workload {
	case "deployment":
		return "replicaset"
	case "statefulset", "daemonset":
		return "controllerrevision"
	default:
		return ""
	}
}

func (c Client) rollbackObject(ctx context.Context, input Mutation, definition Definition, before *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	ref := input.Rollback
	historyType := rollbackHistoryType(definition.Key)
	if before == nil || historyType == "" || ref == nil || ref.UID == "" || len(ref.UID) > 128 || ref.ResourceVersion == "" || len(ref.ResourceVersion) > 256 || ref.Revision < 1 || len(validation.IsDNS1123Subdomain(ref.Name)) > 0 || len(input.Object) > 0 || input.Replicas != nil || input.RestartAt != "" {
		return nil, ErrMutationInvalid
	}
	if paused, _, _ := unstructured.NestedBool(before.Object, "spec", "paused"); paused {
		return nil, ErrMutationInvalid
	}
	historyDefinition, _ := Lookup(historyType)
	history, err := c.Dynamic.Resource(historyDefinition.GVR()).Namespace(input.Query.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil, mutationError(err)
	}
	if string(history.GetUID()) != ref.UID || history.GetResourceVersion() != ref.ResourceVersion || history.GetNamespace() != before.GetNamespace() {
		return nil, ErrMutationConflict
	}
	owned := false
	for _, owner := range history.GetOwnerReferences() {
		if owner.Controller != nil && *owner.Controller && owner.UID == before.GetUID() && owner.Name == before.GetName() && owner.Kind == definition.Kind && owner.APIVersion == "apps/v1" {
			owned = true
		}
	}
	if !owned {
		return nil, ErrIdentityChanged
	}
	var template map[string]any
	var revision int64
	if historyType == "replicaset" {
		revision, err = strconv.ParseInt(history.GetAnnotations()["deployment.kubernetes.io/revision"], 10, 64)
		if err == nil {
			template, _, err = unstructured.NestedMap(history.Object, "spec", "template")
		}
		metadata, _ := template["metadata"].(map[string]any)
		if labels, ok := metadata["labels"].(map[string]any); ok {
			delete(labels, "pod-template-hash")
		}
	} else {
		revision, _, err = unstructured.NestedInt64(history.Object, "revision")
		if err == nil {
			// ControllerRevisions hold strategic patches. Only the workload
			// template is a rollback input; metadata or other spec fields in a
			// forged history patch must never become delegated API mutations.
			data, _ := history.Object["data"].(map[string]any)
			if len(data) != 1 || data["spec"] == nil {
				return nil, ErrMutationInvalid
			}
			spec, _ := data["spec"].(map[string]any)
			if len(spec) != 1 || spec["template"] == nil {
				return nil, ErrMutationInvalid
			}
			template, _, err = unstructured.NestedMap(spec, "template")
			if err != nil || template == nil {
				return nil, ErrMutationInvalid
			}
			if directive, exists := template["$patch"]; exists && directive != "replace" {
				return nil, ErrMutationInvalid
			}
			delete(template, "$patch")
		}
	}
	if err != nil || revision != ref.Revision || template == nil {
		return nil, ErrMutationConflict
	}
	desired := before.DeepCopy()
	if err = unstructured.SetNestedMap(desired.Object, template, "spec", "template"); err != nil {
		return nil, ErrMutationInvalid
	}
	return desired, nil
}
