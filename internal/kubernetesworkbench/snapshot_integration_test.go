package kubernetesworkbench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"testing"
	"time"
)

type snapshotScopeBackend struct {
	calls []kubernetesinventory.Query
	after func()
	fail  string
}

func (b *snapshotScopeBackend) ListInventory(_ context.Context, _ db.KubernetesCluster, q kubernetesinventory.Query) (kubernetesinventory.Page, error) {
	b.calls = append(b.calls, q)
	if b.after != nil {
		b.after()
		b.after = nil
	}
	if q.Type == b.fail {
		return kubernetesinventory.Page{}, kubernetesinventory.ErrForbidden
	}
	d, _ := kubernetesinventory.Lookup(q.Type)
	raw := json.RawMessage(`{"spec":{"containers":[]},"status":{"phase":"Running","conditions":[{"type":"Ready","status":"True"}]}}`)
	if q.Type == "secret" {
		raw = json.RawMessage(`{"data":{"password":"must-not-appear-in-snapshot"}}`)
	}
	object := kubernetesinventory.Object{Definition: d, UID: q.Type + "-uid", Name: q.Type, Namespace: q.Namespace, NamespaceUID: q.NamespaceUID, ResourceVersion: "1", ObservedAt: time.Now(), Data: raw}
	return kubernetesinventory.Page{SchemaVersion: kubernetesinventory.SchemaVersion, ResourceVersion: "1", ObservedAt: time.Now(), Complete: true, Items: []kubernetesinventory.Object{object}}, nil
}
func (b *snapshotScopeBackend) GetInventory(context.Context, db.KubernetesCluster, kubernetesinventory.Query) (kubernetesinventory.Object, error) {
	return kubernetesinventory.Object{}, kubernetesinventory.ErrUnsupported
}
func TestPostgresSnapshotNamespaceScopeBudgetAndRevocation(t *testing.T) {
	f := newMutationFixture(t)
	backend := &snapshotScopeBackend{}
	f.service.Backend = backend
	input := SnapshotInput{Types: []string{"node", "pod", "secret"}, Namespaces: Selection{Mode: "all_authorized"}, MaxItems: 10}
	value, err := f.service.Snapshot(t.Context(), f.actor, f.cluster, input)
	if err != nil || !value.Complete || value.ClusterWide || len(value.Resources) != 2 || value.Counts["node"] != nil {
		t.Fatal(value, err)
	}
	for _, q := range backend.calls {
		if q.AllNamespaces || q.Namespace != "orders" || q.NamespaceUID != "orders-uid" || q.Type == "node" {
			t.Fatal("scope expanded", q)
		}
	}
	raw, _ := json.Marshal(value)
	if bytes.Contains(raw, []byte("must-not-appear")) {
		t.Fatal("raw secret exposed")
	}
	for _, ref := range value.Resources {
		if ref.NamespaceID == nil || *ref.NamespaceID != f.namespace {
			t.Fatal("unbound namespace reference", ref)
		}
	}
	input.MaxItems = 1
	value, err = f.service.Snapshot(t.Context(), f.actor, f.cluster, input)
	if err != nil || value.Complete || value.Errors["secret"] != "KUBERNETES_RESULT_BUDGET_EXCEEDED" {
		t.Fatal("truncated snapshot claimed completeness", value, err)
	}
	input.MaxItems = 10
	backend.fail = "pod"
	value, err = f.service.Snapshot(t.Context(), f.actor, f.cluster, input)
	if err != nil || value.Complete || value.Counts["secret"]["total"] != 1 || value.Errors["pod"] == "" {
		t.Fatal("one source failure erased unrelated data", value, err)
	}
	backend.fail = ""
	backend.after = func() {
		f.exec("UPDATE data_authorization_grants SET status='disabled' WHERE enterprise_id=$1 AND subject_id=$2", f.actor.EnterpriseID, f.actor.SubjectID)
	}
	value, err = f.service.Snapshot(t.Context(), f.actor, f.cluster, input)
	if !errors.Is(err, authorization.ErrAccessDenied) || len(value.Resources) > 0 {
		t.Fatal("revoked snapshot was returned", value, err)
	}
}
