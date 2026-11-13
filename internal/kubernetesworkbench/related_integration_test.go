package kubernetesworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"strconv"
	"testing"
	"time"
)

type relatedBackend struct {
	calls    []kubernetesinventory.Query
	after    func()
	replaced bool
}

func (b *relatedBackend) GetInventory(_ context.Context, _ db.KubernetesCluster, q kubernetesinventory.Query) (kubernetesinventory.Object, error) {
	d, _ := kubernetesinventory.Lookup(q.Type)
	uid := q.UID
	if b.replaced {
		uid = "replacement"
	}
	return kubernetesinventory.Object{Definition: d, UID: uid, Name: q.Name, Namespace: q.Namespace, NamespaceUID: q.NamespaceUID, ResourceVersion: "1", Data: json.RawMessage(`{"spec":{"containers":[]}}`)}, nil
}
func (b *relatedBackend) ListInventory(_ context.Context, _ db.KubernetesCluster, q kubernetesinventory.Query) (kubernetesinventory.Page, error) {
	b.calls = append(b.calls, q)
	if b.after != nil {
		b.after()
		b.after = nil
	}
	d, _ := kubernetesinventory.Lookup(q.Type)
	offset, _ := strconv.Atoi(q.Continue)
	out := kubernetesinventory.Page{SchemaVersion: kubernetesinventory.SchemaVersion, ResourceVersion: "1", ObservedAt: time.Now(), Complete: true, Items: []kubernetesinventory.Object{}}
	if q.Type != "event" {
		return out, nil
	}
	for index := offset; index < min(offset+int(q.Limit), 1055); index++ {
		// Interleaved irrelevant events must not consume output slots or be exposed.
		uid := "pod-uid"
		if index%2 == 1 {
			uid = "other-pod"
		}
		out.Items = append(out.Items, kubernetesinventory.Object{Definition: d, UID: fmt.Sprint("event-", index), Name: fmt.Sprint("event-", index), Namespace: q.Namespace, NamespaceUID: q.NamespaceUID, ResourceVersion: "1", Data: json.RawMessage(fmt.Sprintf(`{"type":"Warning","involvedObject":{"uid":%q}}`, uid))})
	}
	if offset+int(q.Limit) < 1055 {
		out.Continue = strconv.Itoa(offset + int(q.Limit))
		out.Complete = false
	}
	return out, nil
}
func TestPostgresRelatedPaginationIdentityAndRevocation(t *testing.T) {
	f := newMutationFixture(t)
	backend := &relatedBackend{}
	f.service.Backend = backend
	input := RelatedInput{ObjectInput: ObjectInput{Type: "pod", Name: "api", UID: "pod-uid", NamespaceID: f.namespace}, View: "events", Limit: 30}
	first, err := f.service.Related(t.Context(), f.actor, f.cluster, input)
	if err != nil || len(first.Items) != 15 || first.NextCursor == "" {
		t.Fatal(first, err)
	}
	token := first.NextCursor
	seen := map[string]bool{}
	for _, row := range first.Items {
		seen[row.UID] = true
	}
	for cursor := token; cursor != ""; {
		input.Cursor = cursor
		page, e := f.service.Related(t.Context(), f.actor, f.cluster, input)
		if e != nil {
			t.Fatal(e)
		}
		for _, row := range page.Items {
			if seen[row.UID] {
				t.Fatal("duplicate page", row)
			}
			seen[row.UID] = true
			if row.NamespaceID == nil || *row.NamespaceID != f.namespace {
				t.Fatal("unbound namespace", row)
			}
		}
		cursor = page.NextCursor
	}
	if len(seen) != 528 {
		t.Fatal("scan truncated or unrelated event exposed", len(seen))
	}
	for _, q := range backend.calls {
		if q.AllNamespaces || q.Namespace != "orders" || q.NamespaceUID != "orders-uid" || q.Limit > 30 {
			t.Fatal("unbounded or broadened read", q)
		}
	}
	input.Cursor = token
	input.View = "relations"
	if _, err = f.service.Related(t.Context(), f.actor, f.cluster, input); err == nil {
		t.Fatal("cursor reused for a different view")
	}
	input.View = "events"
	backend.replaced = true
	if _, err = f.service.Related(t.Context(), f.actor, f.cluster, input); !errors.Is(err, kubernetesinventory.ErrIdentityChanged) {
		t.Fatal("replacement accepted", err)
	}
	backend.replaced = false
	backend.after = func() {
		f.exec("UPDATE data_authorization_grants SET status='disabled' WHERE enterprise_id=$1 AND subject_id=$2", f.actor.EnterpriseID, f.actor.SubjectID)
	}
	page, err := f.service.Related(t.Context(), f.actor, f.cluster, input)
	if !errors.Is(err, authorization.ErrAccessDenied) || len(page.Items) > 0 {
		t.Fatal("revoked response exposed", page, err)
	}
}
