package kubernetesworkbench

import (
	"encoding/json"
	"testing"

	"github.com/kakj-go/Argus/internal/kubernetesinventory"
)

func TestPVCResizeWaitsForActualCapacityAndFilesystem(t *testing.T) {
	input := kubernetesinventory.Mutation{Operation: "patch", Query: kubernetesinventory.Query{Type: "pvc", UID: "claim"}}
	for _, tc := range []struct {
		name, status string
		ready        bool
		code         string
	}{
		{"API accepted, old capacity", `{"phase":"Bound","capacity":{"storage":"1Gi"}}`, false, ""},
		{"not bound", `{"phase":"Pending","capacity":{"storage":"2Gi"}}`, false, ""},
		{"controller still resizing", `{"phase":"Bound","capacity":{"storage":"2Gi"},"conditions":[{"type":"Resizing","status":"True"}]}`, false, ""},
		{"filesystem pending", `{"phase":"Bound","capacity":{"storage":"2Gi"},"conditions":[{"type":"FileSystemResizePending","status":"True"}]}`, false, ""},
		{"actual expansion complete", `{"phase":"Bound","capacity":{"storage":"2048Mi"}}`, true, ""},
		{"volume lost", `{"phase":"Lost"}`, false, "KUBERNETES_VOLUME_LOST"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			object := &kubernetesinventory.Object{UID: "claim", Data: json.RawMessage(`{"apiVersion":"v1","kind":"PersistentVolumeClaim","spec":{"resources":{"requests":{"storage":"2Gi"}}},"status":` + tc.status + `}`)}
			ready, code := mutationConverged(input, kubernetesinventory.MutationResult{After: object}, nil)
			if ready != tc.ready || code != tc.code {
				t.Fatalf("convergence: ready=%v code=%s", ready, code)
			}
		})
	}
}
