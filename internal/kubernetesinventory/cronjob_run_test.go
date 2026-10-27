package kubernetesinventory

import (
	"bytes"
	"errors"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

func cronJobRunFixture() (*unstructured.Unstructured, Mutation) {
	cron := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "batch/v1", "kind": "CronJob", "metadata": map[string]any{"name": "scheduled", "namespace": "owned", "uid": "source-uid", "resourceVersion": "4"},
		"spec": map[string]any{"suspend": true, "schedule": "0 * * * *", "jobTemplate": map[string]any{
			"metadata": map[string]any{"labels": map[string]any{"app": "batch"}, "annotations": map[string]any{"purpose": "report"}},
			"spec":     map[string]any{"backoffLimit": int64(1), "template": map[string]any{"spec": map[string]any{"automountServiceAccountToken": false, "restartPolicy": "Never", "containers": []any{map[string]any{"name": "main", "image": "alpine:3.21", "env": []any{map[string]any{"name": "PASSWORD", "value": "source-private-canary"}}}}}}},
		}},
	}}
	return cron, Mutation{ID: "manual-operation", Operation: "create", Query: Query{Type: "job", Name: "manual-job", Namespace: "owned", NamespaceUID: "namespace-uid"}, CronJobSource: &ObjectVersionReference{Name: "scheduled", UID: "source-uid", ResourceVersion: "4"}}
}

func TestCronJobRunCopiesFrozenTemplateAndProjectsPrivateValues(t *testing.T) {
	cron, input := cronJobRunFixture()
	c := Client{Dynamic: fake.NewSimpleDynamicClient(runtime.NewScheme(), cron)}
	job, err := c.jobFromCronJob(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if job.GetName() != "manual-job" || job.GetNamespace() != "owned" || job.GetAPIVersion() != "batch/v1" || job.GetKind() != "Job" {
		t.Fatal("manual job target was not fixed")
	}
	owners := job.GetOwnerReferences()
	if len(owners) != 1 || owners[0].UID != cron.GetUID() || owners[0].BlockOwnerDeletion != nil || owners[0].Controller == nil || !*owners[0].Controller {
		t.Fatal("manual job ownership requires unintended finalizer privilege")
	}
	if job.GetAnnotations()["cronjob.kubernetes.io/instantiate"] != "manual" || job.GetLabels()["app"] != "batch" {
		t.Fatal("manual job metadata lost")
	}
	raw, _ := job.MarshalJSON()
	if !bytes.Contains(raw, []byte("source-private-canary")) {
		t.Fatal("execution-side template lost its existing private input")
	}
	job.SetUID("new-job-uid")
	job.SetResourceVersion("5")
	d, _ := Lookup("job")
	projected, err := Project(d, *job, input.Query.NamespaceUID, time.Now())
	if err != nil || bytes.Contains(projected.Data, []byte("source-private-canary")) {
		t.Fatal("private source template escaped public projection")
	}
	if err = validateMutationPolicy(d, job, false, MutationPolicy{}); err != nil {
		t.Fatal(err)
	}
	if err = unstructured.SetNestedField(job.Object, int64(10), "spec", "backoffLimit"); err != nil {
		t.Fatal(err)
	}
	limit, _, _ := unstructured.NestedInt64(cron.Object, "spec", "jobTemplate", "spec", "backoffLimit")
	if limit != 1 {
		t.Fatal("copy mutated the source CronJob")
	}
}

func TestCronJobRunRejectsChangedSourceAndInheritedPrivileges(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*unstructured.Unstructured, *Mutation)
		want error
	}{
		{"uid", func(c *unstructured.Unstructured, _ *Mutation) { c.SetUID("replacement") }, ErrMutationConflict},
		{"version", func(c *unstructured.Unstructured, _ *Mutation) { c.SetResourceVersion("5") }, ErrMutationConflict},
		{"deleted", func(c *unstructured.Unstructured, _ *Mutation) { at := metav1.Now(); c.SetDeletionTimestamp(&at) }, ErrMutationConflict},
		{"missing reference", func(_ *unstructured.Unstructured, in *Mutation) { in.CronJobSource = nil }, ErrMutationInvalid},
		{"invalid name", func(_ *unstructured.Unstructured, in *Mutation) { in.CronJobSource.Name = "../other" }, ErrMutationInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			cron, in := cronJobRunFixture()
			test.edit(cron, &in)
			c := Client{Dynamic: fake.NewSimpleDynamicClient(runtime.NewScheme(), cron)}
			if _, err := c.jobFromCronJob(t.Context(), in); !errors.Is(err, test.want) {
				t.Fatalf("got %v want %v", err, test.want)
			}
		})
	}
	cron, in := cronJobRunFixture()
	_ = unstructured.SetNestedField(cron.Object, true, "spec", "jobTemplate", "spec", "template", "spec", "hostPID")
	c := Client{Dynamic: fake.NewSimpleDynamicClient(runtime.NewScheme(), cron)}
	job, err := c.jobFromCronJob(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := Lookup("job")
	if err = validateMutationPolicy(d, job, false, MutationPolicy{}); !errors.Is(err, ErrForbidden) {
		t.Fatal("source template bypassed current workload privileges")
	}
}
