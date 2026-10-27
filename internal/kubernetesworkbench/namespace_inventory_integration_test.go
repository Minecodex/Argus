package kubernetesworkbench

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type namespaceSnapshotBackend struct {
	inventoryFixtureBackend
	pages     []kubernetesinventory.Page
	calls     int
	objectErr error
	hook      func()
}

func (b *namespaceSnapshotBackend) ListInventory(_ context.Context, _ db.KubernetesCluster, q kubernetesinventory.Query) (kubernetesinventory.Page, error) {
	if q.Type != "namespace" && q.AllNamespaces {
		return kubernetesinventory.Page{Complete: true}, b.objectErr
	}
	if q.Type != "namespace" || q.Namespace != "" || q.AllNamespaces {
		return kubernetesinventory.Page{}, kubernetesinventory.ErrInvalid
	}
	if b.hook != nil {
		hook := b.hook
		b.hook = nil
		hook()
	}
	index := b.calls
	b.calls++
	if index >= len(b.pages) {
		return kubernetesinventory.Page{}, kubernetesinventory.ErrUnavailable
	}
	return b.pages[index], nil
}

func TestPostgresNamespaceInventorySnapshotAndRaceFences(t *testing.T) {
	for _, scenario := range []string{"replace_and_delete", "partial_snapshot", "object_read_failed", "lease_expired", "route_changed", "foreground_newer"} {
		t.Run(scenario, func(t *testing.T) {
			f := newMutationFixture(t)
			ctx := t.Context()
			definition, _ := kubernetesinventory.Lookup("namespace")
			uid := uuid.NewString()
			object := kubernetesinventory.Object{Definition: definition, UID: uid, Name: "orders", ResourceVersion: "101"}
			backend := &namespaceSnapshotBackend{pages: []kubernetesinventory.Page{{Items: []kubernetesinventory.Object{object}, Complete: true, ResourceVersion: "101"}}}
			f.service.Backend = backend
			f.exec("INSERT INTO kubernetes_namespace_sync_states(cluster_id,enterprise_id,lease_token,lease_expires_at,last_attempt_at,snapshot_started_at,observed_cluster_version,status) VALUES($1,$2,$3,now()+interval '1 minute',now(),clock_timestamp(),1,'syncing')", f.cluster, f.actor.EnterpriseID, uuid.New())
			claim, err := f.service.Store.Queries.GetKubernetesNamespaceSyncState(ctx, db.GetKubernetesNamespaceSyncStateParams{EnterpriseID: f.actor.EnterpriseID, ClusterID: f.cluster})
			if err != nil {
				t.Fatal(err)
			}
			var foreground uuid.UUID
			switch scenario {
			case "object_read_failed":
				backend.objectErr = kubernetesinventory.ErrUnavailable
			case "partial_snapshot":
				backend.pages[0].Continue, backend.pages[0].Complete = "next", false
			case "lease_expired":
				backend.hook = func() {
					f.exec("UPDATE kubernetes_namespace_sync_states SET lease_expires_at=now()-interval '1 second' WHERE cluster_id=$1", f.cluster)
				}
			case "route_changed":
				backend.hook = func() {
					f.exec("UPDATE kubernetes_clusters SET resource_version=resource_version+1 WHERE id=$1", f.cluster)
				}
			case "foreground_newer":
				backend.hook = func() {
					current := object
					current.UID, current.ResourceVersion = uuid.NewString(), "102"
					refs, e := f.service.observeNamespaces(ctx, f.actor.EnterpriseID, f.cluster, []kubernetesinventory.Object{current})
					if e != nil {
						t.Fatal(e)
					}
					foreground = refs[current.UID]
				}
			}
			err = f.service.syncClaimedNamespaces(ctx, claim)
			old, e := f.service.Store.Queries.GetKubernetesNamespaceResource(ctx, db.GetKubernetesNamespaceResourceParams{EnterpriseID: f.actor.EnterpriseID, ID: f.namespace})
			if e != nil {
				t.Fatal(e)
			}
			if scenario == "partial_snapshot" || scenario == "object_read_failed" || scenario == "lease_expired" || scenario == "route_changed" {
				if err == nil || old.Status != "active" {
					t.Fatal("incomplete/stale inventory replaced authoritative metadata", err)
				}
				return
			}
			if err != nil || old.Status != "deleted" {
				t.Fatal("replacement did not retire the old UID", err)
			}
			current, err := f.service.Store.Queries.ResolveKubernetesNamespaceResource(ctx, db.ResolveKubernetesNamespaceResourceParams{EnterpriseID: f.actor.EnterpriseID, ClusterID: f.cluster, Name: "orders"})
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "foreground_newer" && (current.ID != foreground || current.NamespaceUid == uid) {
				t.Fatal("background snapshot undid a newer trusted read")
			}
			if err := authorization.RequireResource(ctx, f.service.Store.Queries, f.actor, "kubernetes_namespace", current.ID); !errors.Is(err, authorization.ErrAccessDenied) {
				t.Fatal("same-name namespace inherited an old grant", err)
			}
		})
	}
}
