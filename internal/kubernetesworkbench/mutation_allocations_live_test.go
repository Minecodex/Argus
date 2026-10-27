package kubernetesworkbench

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

func runLiveAllocatedObjectWorkflows(t *testing.T, f *mutationFixture, namespace string) {
	t.Helper()
	f.exec("INSERT INTO role_permissions(role_id,permission_id) SELECT binding.role_id,permission.id FROM role_bindings binding CROSS JOIN permissions permission WHERE binding.enterprise_id=$1 AND permission.id IN ('kubernetes.workload.manage','kubernetes.network.manage') ON CONFLICT DO NOTHING", f.actor.EnterpriseID)
	w := Worker{Service: f.service}
	for _, test := range []struct{ kind, name, api, manifest string }{
		{"service", "internal", "v1", `"type":"ClusterIP","ports":[{"port":8080}]`},
		{"service", "allocated-port", "v1", `"type":"NodePort","ports":[{"port":8080}]`},
		{"job", "task", "batch/v1", `"backoffLimit":0,"template":{"spec":{"restartPolicy":"Never","terminationGracePeriodSeconds":1,"containers":[{"name":"main","image":"alpine:3.21","imagePullPolicy":"Never","command":["/bin/sh","-c","sleep 4; echo plan8-workflow-job"]}]}}`},
	} {
		t.Run(test.kind+"/"+test.name, func(t *testing.T) {
			kind := "Service"
			if test.kind == "job" {
				kind = "Job"
			}
			metadata, _ := json.Marshal(map[string]string{"name": test.name, "namespace": namespace})
			manifest := json.RawMessage(`{"apiVersion":"` + test.api + `","kind":"` + kind + `","metadata":` + string(metadata) + `,"spec":{` + test.manifest + `}}`)
			input := MutationInput{ClusterID: f.cluster, NamespaceID: f.namespace, Operation: "create", Type: test.kind, Name: test.name, Object: manifest}
			id, execution := f.queueInput(input)
			if err := w.execute(t.Context(), operationTask{f.actor.EnterpriseID, id}); err != nil {
				t.Fatal(err)
			}
			if err := wait.PollUntilContextTimeout(t.Context(), 300*time.Millisecond, 35*time.Second, true, func(ctx context.Context) (bool, error) {
				row := f.operation(id)
				if row.Status == "failed" || row.Status == "superseded" {
					t.Fatalf("allocated object workflow: %s %s", row.Status, row.ErrorCode)
				}
				if row.Status == "succeeded" {
					return true, nil
				}
				return false, w.observe(ctx, row)
			}); err != nil {
				t.Fatal(err)
			}
			f.reconcileExecution(execution)
		})
	}
}
