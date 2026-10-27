package kubernetesworkbench

import (
	"encoding/json"
	"testing"

	"github.com/kakj-go/Argus/internal/kubernetesinventory"
)

func TestJobConvergenceRequiresActualPodReadinessOrCompletion(t *testing.T) {
	for _, tc := range []struct {
		name, spec, status string
		ready              bool
		code               string
	}{
		{"accepted only", `{}`, `{}`, false, ""},
		{"pod active but not ready", `{}`, `{"active":1,"ready":0}`, false, ""},
		{"pod actually ready", `{}`, `{"active":1,"ready":1}`, true, ""},
		{"parallelism incomplete", `{"parallelism":2}`, `{"active":2,"ready":1}`, false, ""},
		{"completed", `{}`, `{"conditions":[{"type":"Complete","status":"True"}]}`, true, ""},
		{"failed", `{}`, `{"conditions":[{"type":"Failed","status":"True"}]}`, false, "KUBERNETES_WORKLOAD_FAILED"},
		{"suspend not observed", `{"suspend":true}`, `{"active":1}`, false, ""},
		{"suspend observed", `{"suspend":true}`, `{"active":0,"conditions":[{"type":"Suspended","status":"True"}]}`, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := kubernetesinventory.Mutation{Operation: "create", Query: kubernetesinventory.Query{Type: "job"}}
			after := &kubernetesinventory.Object{Data: json.RawMessage(`{"apiVersion":"batch/v1","kind":"Job","spec":` + tc.spec + `,"status":` + tc.status + `}`)}
			ready, code := mutationConverged(input, kubernetesinventory.MutationResult{After: after}, nil)
			if ready != tc.ready || code != tc.code {
				t.Fatalf("ready=%v code=%s", ready, code)
			}
		})
	}
}
