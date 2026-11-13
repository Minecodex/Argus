package kubernetesworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"testing"
)

type observationReader struct {
	value kubernetesinventory.Object
	err   error
	reads int
	query kubernetesinventory.Query
}

func (b *observationReader) GetInventory(_ context.Context, _ db.KubernetesCluster, q kubernetesinventory.Query) (kubernetesinventory.Object, error) {
	b.reads++
	b.query = q
	return b.value, b.err
}
func TestSupersededObservationRequiresAcknowledgedSameObjectAndNewerGeneration(t *testing.T) {
	definition, _ := kubernetesinventory.Lookup("job")
	original := kubernetesinventory.Object{Definition: definition, Name: "task", UID: "accepted-uid", Namespace: "owned", NamespaceUID: "namespace-uid", Generation: 2}
	input := kubernetesinventory.Mutation{Operation: "create", Query: kubernetesinventory.Query{Type: "job", Name: "task", Namespace: "owned", NamespaceUID: "namespace-uid"}}
	accepted := json.RawMessage(`{"accepted":true,"uid":"accepted-uid","generation":1}`)
	for _, tc := range []struct {
		name   string
		change func(*observationReader, *json.RawMessage, *kubernetesinventory.Mutation)
		want   bool
	}{
		{"later edit", func(*observationReader, *json.RawMessage, *kubernetesinventory.Mutation) {}, true},
		{"lost write response", func(_ *observationReader, p *json.RawMessage, _ *kubernetesinventory.Mutation) {
			*p = json.RawMessage(`{"accepted":false,"uid":"accepted-uid","generation":1}`)
		}, false},
		{"missing acknowledgement", func(_ *observationReader, p *json.RawMessage, _ *kubernetesinventory.Mutation) {
			*p = json.RawMessage(`{}`)
		}, false},
		{"same generation", func(b *observationReader, _ *json.RawMessage, _ *kubernetesinventory.Mutation) {
			b.value.Generation = 1
		}, false},
		{"replaced object", func(b *observationReader, _ *json.RawMessage, _ *kubernetesinventory.Mutation) {
			b.value.UID = "new-uid"
		}, false},
		{"replaced namespace", func(b *observationReader, _ *json.RawMessage, _ *kubernetesinventory.Mutation) {
			b.value.NamespaceUID = "new-namespace"
		}, false},
		{"wrong type", func(b *observationReader, _ *json.RawMessage, _ *kubernetesinventory.Mutation) {
			b.value.Definition.Key = "deployment"
		}, false},
		{"read unavailable", func(b *observationReader, _ *json.RawMessage, _ *kubernetesinventory.Mutation) {
			b.err = errors.New("unavailable")
		}, false},
		{"delete", func(_ *observationReader, _ *json.RawMessage, p *kubernetesinventory.Mutation) {
			p.Operation = "delete"
		}, false},
		{"drain", func(_ *observationReader, _ *json.RawMessage, p *kubernetesinventory.Mutation) { p.Operation = "drain" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := &observationReader{value: original}
			body := append(json.RawMessage(nil), accepted...)
			in := input
			tc.change(backend, &body, &in)
			value, ok := supersededObservation(t.Context(), backend, db.KubernetesCluster{}, in, body)
			if ok != tc.want || ok && (value.UID != "accepted-uid" || backend.query.UID != "accepted-uid") {
				t.Fatalf("settled=%v result=%+v query=%+v", ok, value, backend.query)
			}
		})
	}
}
