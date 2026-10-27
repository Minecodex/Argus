package kubernetesinventory

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
)

func TestLiveManualJobFromCronJobChecksSourceAndActualCompletion(t *testing.T) {
	f := newLiveInventoryNamespace(t, "cronjob-run")
	source, _ := cronJobRunFixture()
	source.SetNamespace(f.name)
	source.SetUID("")
	source.SetResourceVersion("")
	containers, _, _ := unstructured.NestedSlice(source.Object, "spec", "jobTemplate", "spec", "template", "spec", "containers")
	container := containers[0].(map[string]any)
	container["imagePullPolicy"] = "Never"
	container["command"] = []any{"/bin/sh", "-c", "echo plan8-manual-job"}
	_ = unstructured.SetNestedSlice(source.Object, containers, "spec", "jobTemplate", "spec", "template", "spec", "containers")
	_ = unstructured.SetNestedField(source.Object, int64(1), "spec", "jobTemplate", "spec", "template", "spec", "terminationGracePeriodSeconds")
	d, _ := Lookup("cronjob")
	api := f.client.Dynamic.Resource(d.GVR()).Namespace(f.name)
	created, err := api.Create(f.ctx, source, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	input := Mutation{ID: uuid.NewString(), Operation: "create", Query: Query{Type: "job", Name: "manual-run", Namespace: f.name, NamespaceUID: f.uid}, CronJobSource: &ObjectVersionReference{Name: created.GetName(), UID: string(created.GetUID()), ResourceVersion: created.GetResourceVersion()}}
	preview, err := f.client.Mutate(f.ctx, input, true)
	if err != nil || preview.After == nil || !preview.DryRun || preview.Accepted {
		t.Fatalf("manual job preview: %v", err)
	}
	if bytes.Contains(preview.After.Data, []byte("source-private-canary")) {
		t.Fatal("existing private template entered public dry-run")
	}
	jobDefinition, _ := Lookup("job")
	jobs := f.client.Dynamic.Resource(jobDefinition.GVR()).Namespace(f.name)
	if _, err = jobs.Get(f.ctx, input.Query.Name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatal("preview created a Job")
	}
	created.SetAnnotations(map[string]string{"updated": "after-preview"})
	current, err := api.Update(f.ctx, created, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.client.Mutate(f.ctx, input, false); !errors.Is(err, ErrMutationConflict) {
		t.Fatalf("changed CronJob source was accepted: %v", err)
	}
	if _, err = jobs.Get(f.ctx, input.Query.Name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatal("source conflict still created a Job")
	}
	input.CronJobSource.ResourceVersion = current.GetResourceVersion()
	preview, err = f.client.Mutate(f.ctx, input, true)
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedDigest = preview.Digest
	if _, err = f.client.Mutate(f.ctx, input, false); err != nil {
		t.Fatal(err)
	}
	var job *unstructured.Unstructured
	if err = wait.PollUntilContextTimeout(f.ctx, 200*time.Millisecond, 35*time.Second, true, func(ctx context.Context) (bool, error) {
		var err error
		job, err = jobs.Get(ctx, input.Query.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		conditions, _, _ := unstructured.NestedSlice(job.Object, "status", "conditions")
		for _, value := range conditions {
			condition := value.(map[string]any)
			if condition["type"] == "Failed" && condition["status"] == "True" {
				t.Fatal("manual Job failed")
			}
			if condition["type"] == "Complete" && condition["status"] == "True" {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	owners := job.GetOwnerReferences()
	if len(owners) != 1 || owners[0].UID != created.GetUID() || owners[0].BlockOwnerDeletion != nil || job.GetAnnotations()["cronjob.kubernetes.io/instantiate"] != "manual" {
		t.Fatal("actual manual Job lost controlled ownership")
	}
	raw, _ := job.MarshalJSON()
	if !bytes.Contains(raw, []byte("source-private-canary")) {
		t.Fatal("existing execution-side template value lost")
	}
	t.Log("suspended actual CronJob produced no scheduled run; manual preview had no effect; source version change denied; freshly approved Job inherited private template only at execution and actually completed")
}
