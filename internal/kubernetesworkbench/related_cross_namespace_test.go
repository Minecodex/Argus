package kubernetesworkbench

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"testing"
	"time"
)

type crossNamespaceBackend struct {
	objects []kubernetesinventory.Object
	calls   []kubernetesinventory.Query
}

func (b *crossNamespaceBackend) GetInventory(_ context.Context, _ db.KubernetesCluster, q kubernetesinventory.Query) (kubernetesinventory.Object, error) {
	for _, o := range b.objects {
		if o.Definition.Key == q.Type && o.Name == q.Name && o.Namespace == q.Namespace && o.UID == q.UID {
			return o, nil
		}
	}
	return kubernetesinventory.Object{}, kubernetesinventory.ErrIdentityChanged
}
func (b *crossNamespaceBackend) ListInventory(_ context.Context, _ db.KubernetesCluster, q kubernetesinventory.Query) (kubernetesinventory.Page, error) {
	b.calls = append(b.calls, q)
	out := kubernetesinventory.Page{SchemaVersion: kubernetesinventory.SchemaVersion, ObservedAt: time.Now(), ResourceVersion: "1", Complete: true, Items: []kubernetesinventory.Object{}}
	for _, o := range b.objects {
		if o.Definition.Key == q.Type && (o.Namespace == q.Namespace || q.AllNamespaces) {
			out.Items = append(out.Items, o)
		}
	}
	return out, nil
}
func TestPostgresCrossNamespaceRelationsRemainAuthorized(t *testing.T) {
	f := newMutationFixture(t)
	shared := uuid.New()
	_, err := f.service.Store.Queries.UpsertKubernetesNamespaceResource(t.Context(), db.UpsertKubernetesNamespaceResourceParams{ID: shared, EnterpriseID: f.actor.EnterpriseID, ClusterID: f.cluster, Name: "shared", NamespaceUid: "shared-uid"})
	if err != nil {
		t.Fatal(err)
	}
	obj := func(kind, name, uid, ns, raw string) kubernetesinventory.Object {
		def, _ := kubernetesinventory.Lookup(kind)
		return kubernetesinventory.Object{Definition: def, Name: name, UID: uid, Namespace: ns, NamespaceUID: ns + "-uid", ResourceVersion: "1", Data: json.RawMessage(raw)}
	}
	binding := obj("rolebinding", "grant", "binding", "orders", `{"subjects":[{"kind":"ServiceAccount","name":"app","namespace":"shared"}]}`)
	account := obj("serviceaccount", "app", "account", "shared", `{}`)
	backend := &crossNamespaceBackend{objects: []kubernetesinventory.Object{binding, account}}
	f.service.Backend = backend
	read := func(input RelatedInput) []SnapshotResource {
		t.Helper()
		result := []SnapshotResource{}
		for i := 0; i < 20; i++ {
			page, e := f.service.Related(t.Context(), f.actor, f.cluster, input)
			if e != nil {
				t.Fatal(e)
			}
			result = append(result, page.Items...)
			if page.NextCursor == "" {
				return result
			}
			input.Cursor = page.NextCursor
		}
		t.Fatal("pagination did not terminate")
		return nil
	}
	input := RelatedInput{ObjectInput: ObjectInput{Type: "rolebinding", Name: "grant", UID: "binding", NamespaceID: f.namespace}, View: "relations", Limit: 30}
	if result := read(input); len(result) != 0 {
		t.Fatal("ungranted target leaked", result)
	}
	for _, q := range backend.calls {
		if q.AllNamespaces || q.Namespace == "shared" {
			t.Fatal("ungranted namespace queried", q)
		}
	}
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'kubernetes_namespace',$4)", uuid.New(), f.actor.EnterpriseID, f.actor.SubjectID, shared)
	if result := read(input); len(result) != 1 || result[0].UID != "account" || result[0].NamespaceID == nil || *result[0].NamespaceID != shared {
		t.Fatal("cross namespace target missing", result)
	}
	reverse := RelatedInput{ObjectInput: ObjectInput{Type: "serviceaccount", Name: "app", UID: "account", NamespaceID: shared}, View: "relations", Limit: 30}
	if result := read(reverse); len(result) != 1 || result[0].UID != "binding" {
		t.Fatal("reverse relation missing", result)
	}
	f.exec("UPDATE data_authorization_grants SET status='disabled' WHERE enterprise_id=$1 AND subject_id=$2 AND resource_id=$3", f.actor.EnterpriseID, f.actor.SubjectID, f.namespace)
	backend.calls = nil
	if result := read(reverse); len(result) != 0 {
		t.Fatal("revoked target remained", result)
	}
	for _, q := range backend.calls {
		if q.AllNamespaces || q.Namespace == "orders" {
			t.Fatal("revoked namespace queried", q)
		}
	}
}
