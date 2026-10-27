package kubernetesworkbench

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
)

func runLiveCronJobWorkflow(t *testing.T, f *mutationFixture, namespace string) {
	t.Helper()
	backend := f.service.Backend.(liveOperationBackend)
	d, _ := kubernetesinventory.Lookup("cronjob")
	api := backend.client.Dynamic.Resource(d.GVR()).Namespace(namespace)
	private := "cronjob-private-canary-" + uuid.NewString()
	source := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "batch/v1", "kind": "CronJob", "metadata": map[string]any{"name": "scheduled-template", "namespace": namespace},
		"spec": map[string]any{"suspend": true, "schedule": "0 * * * *", "jobTemplate": map[string]any{"spec": map[string]any{"backoffLimit": int64(0), "template": map[string]any{"spec": map[string]any{
			"automountServiceAccountToken": false, "restartPolicy": "Never", "terminationGracePeriodSeconds": int64(1),
			"containers": []any{map[string]any{"name": "main", "image": "alpine:3.21", "imagePullPolicy": "Never", "command": []any{"/bin/sh", "-c", "sleep 2; echo manual-completed"}, "env": []any{map[string]any{"name": "PASSWORD", "value": private}}}},
		}}}}},
	}}
	current, err := api.Create(t.Context(), source, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	input := MutationInput{ClusterID: f.cluster, NamespaceID: f.namespace, Operation: "create", Type: "job", Name: "manual-stale", CronJobSource: &kubernetesinventory.ObjectVersionReference{Name: current.GetName(), UID: string(current.GetUID()), ResourceVersion: current.GetResourceVersion()}}
	id, execution := f.queueInput(input)
	current.SetAnnotations(map[string]string{"changed": "after-confirmation"})
	current, err = api.Update(t.Context(), current, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	w := Worker{Service: f.service}
	if err = w.execute(t.Context(), operationTask{f.actor.EnterpriseID, id}); err != nil {
		t.Fatal(err)
	}
	row := f.operation(id)
	if row.Status != "failed" || row.ErrorCode != kubernetesinventory.ErrMutationConflict.Error() {
		t.Fatalf("stale source workflow result: %s %s", row.Status, row.ErrorCode)
	}
	f.reconcileExecutionStatus(execution, "failed")
	jobDefinition, _ := kubernetesinventory.Lookup("job")
	jobs := backend.client.Dynamic.Resource(jobDefinition.GVR()).Namespace(namespace)
	if _, err = jobs.Get(t.Context(), input.Name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatal("stale approved source created a Job")
	}
	input.Name = "manual-approved"
	input.CronJobSource.ResourceVersion = current.GetResourceVersion()
	id, execution = f.queueInput(input)
	if err = w.execute(t.Context(), operationTask{f.actor.EnterpriseID, id}); err != nil {
		t.Fatal(err)
	}
	if err = wait.PollUntilContextTimeout(t.Context(), 200*time.Millisecond, 35*time.Second, true, func(ctx context.Context) (bool, error) {
		job, err := jobs.Get(ctx, input.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		conditions, _, _ := unstructured.NestedSlice(job.Object, "status", "conditions")
		for _, raw := range conditions {
			value := raw.(map[string]any)
			if value["type"] == "Failed" && value["status"] == "True" {
				t.Fatal("manual workflow Job failed")
			}
			if value["type"] == "Complete" && value["status"] == "True" {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	row = f.operation(id)
	if row.Status != "succeeded" {
		if err = w.observe(t.Context(), row); err != nil {
			t.Fatal(err)
		}
	}
	if row = f.operation(id); row.Status != "succeeded" {
		t.Fatalf("completed manual Job workflow: %s %s", row.Status, row.ErrorCode)
	}
	f.reconcileExecution(execution)
	if strings.Contains(string(row.PrivatePlan), private) || strings.Contains(string(row.Result), private) {
		t.Fatal("private source template persisted in operation plan/result")
	}
	var leaked bool
	err = f.service.Store.Pool.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM pending_actions WHERE enterprise_id=$1 AND (preview::text||diff::text) LIKE '%'||$2||'%') OR EXISTS(SELECT 1 FROM pending_action_plans WHERE enterprise_id=$1 AND immutable_plan::text LIKE '%'||$2||'%') OR EXISTS(SELECT 1 FROM audit_events WHERE enterprise_id=$1 AND details::text LIKE '%'||$2||'%')`, f.actor.EnterpriseID, private).Scan(&leaked)
	if err != nil || leaked {
		t.Fatalf("source template privacy leak=%v error=%v", leaked, err)
	}
	t.Log("actual PostgreSQL approval/worker rejected post-confirmation CronJob changes, settled failed execution, then newly approved manual Job completed without persisting its existing private template")
}
