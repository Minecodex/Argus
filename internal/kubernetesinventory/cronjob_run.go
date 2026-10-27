package kubernetesinventory

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation"
)

type ObjectVersionReference struct {
	Name            string `json:"name"`
	UID             string `json:"uid"`
	ResourceVersion string `json:"resource_version"`
}

// Only the execution-side API identity reads the complete template. Cleartext
// environment values never need to round-trip through the portal/action plan.
func (c Client) jobFromCronJob(ctx context.Context, input Mutation) (*unstructured.Unstructured, error) {
	ref := input.CronJobSource
	if ref == nil || ref.UID == "" || len(ref.UID) > 128 || ref.ResourceVersion == "" || len(ref.ResourceVersion) > 256 || len(validation.IsDNS1123Subdomain(ref.Name)) > 0 {
		return nil, ErrMutationInvalid
	}
	d, _ := Lookup("cronjob")
	source, err := c.Dynamic.Resource(d.GVR()).Namespace(input.Query.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil, mutationError(err)
	}
	if string(source.GetUID()) != ref.UID || source.GetResourceVersion() != ref.ResourceVersion || source.GetNamespace() != input.Query.Namespace {
		return nil, ErrMutationConflict
	}
	if source.GetDeletionTimestamp() != nil {
		return nil, ErrMutationConflict
	}
	spec, found, err := unstructured.NestedMap(source.Object, "spec", "jobTemplate", "spec")
	if err != nil || !found {
		return nil, ErrMutationInvalid
	}
	labels, _, err := unstructured.NestedStringMap(source.Object, "spec", "jobTemplate", "metadata", "labels")
	if err != nil {
		return nil, ErrMutationInvalid
	}
	annotations, _, err := unstructured.NestedStringMap(source.Object, "spec", "jobTemplate", "metadata", "annotations")
	if err != nil {
		return nil, ErrMutationInvalid
	}
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations["cronjob.kubernetes.io/instantiate"] = "manual"
	job := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "batch/v1", "kind": "Job", "metadata": map[string]any{"name": input.Query.Name, "namespace": input.Query.Namespace}, "spec": spec,
	}}
	job.SetLabels(labels)
	job.SetAnnotations(annotations)
	controller := true
	// Do not set blockOwnerDeletion: that would require an additional
	// cronjobs/finalizers privilege on the Kubernetes API connection.
	job.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "batch/v1", Kind: "CronJob", Name: source.GetName(), UID: source.GetUID(), Controller: &controller}})
	return job, nil
}
