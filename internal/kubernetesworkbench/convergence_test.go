package kubernetesworkbench

import (
	"encoding/json"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"testing"
)

func TestMutationConvergenceRequiresObservedReadyReplicas(t *testing.T) {
	replicas := int64(2)
	input := kubernetesinventory.Mutation{Operation: "scale", Query: kubernetesinventory.Query{Type: "deployment", UID: "uid"}, Replicas: &replicas}
	object := &kubernetesinventory.Object{UID: "uid", Generation: 3}
	result := kubernetesinventory.MutationResult{After: object}
	for _, test := range []struct {
		status string
		ready  bool
	}{
		{`{"observedGeneration":2,"replicas":2,"updatedReplicas":2,"readyReplicas":2,"availableReplicas":2}`, false},
		{`{"observedGeneration":3,"replicas":2,"updatedReplicas":2,"readyReplicas":1,"availableReplicas":1}`, false},
		{`{"observedGeneration":3,"replicas":2,"updatedReplicas":2,"readyReplicas":2,"availableReplicas":2}`, true},
	} {
		object.Data = json.RawMessage(`{"apiVersion":"apps/v1","kind":"Deployment","spec":{"replicas":2},"status":` + test.status + `}`)
		ready, code := mutationConverged(input, result, []byte(`{"generation":3,"uid":"uid"}`))
		if ready != test.ready || code != "" {
			t.Fatalf("readiness %s: %v %s", test.status, ready, code)
		}
	}
	object.Generation = 4
	if _, code := mutationConverged(input, result, []byte(`{"generation":3,"uid":"uid"}`)); code != "KUBERNETES_OPERATION_SUPERSEDED" {
		t.Fatalf("external edit presented as approved state: %s", code)
	}
}
