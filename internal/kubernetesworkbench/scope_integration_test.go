package kubernetesworkbench

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"strings"
)

type inventoryFixtureBackend struct {
	calls   []kubernetesinventory.Query
	foreign bool
	onWatch func()
}

func (b *inventoryFixtureBackend) WatchInventory(_ context.Context, _ db.KubernetesCluster, q kubernetesinventory.Query) (kubernetesinventory.WatchBatch, error) {
	if b.onWatch != nil {
		b.onWatch()
	}
	d, _ := kubernetesinventory.Lookup(q.Type)
	return kubernetesinventory.WatchBatch{SchemaVersion: kubernetesinventory.SchemaVersion, ResourceVersion: "43", ObservedAt: time.Now(), Events: []kubernetesinventory.Event{{Type: "MODIFIED", Object: kubernetesinventory.Object{Definition: d, UID: "pod-" + q.Namespace, Name: "pod", Namespace: q.Namespace, NamespaceUID: q.NamespaceUID, ResourceVersion: "43"}}}}, nil
}

func (b *inventoryFixtureBackend) ListInventory(_ context.Context, _ db.KubernetesCluster, q kubernetesinventory.Query) (kubernetesinventory.Page, error) {
	b.calls = append(b.calls, q)
	d, _ := kubernetesinventory.Lookup(q.Type)
	namespace := q.Namespace
	if b.foreign {
		namespace = "foreign"
	}
	return kubernetesinventory.Page{SchemaVersion: kubernetesinventory.SchemaVersion, ResourceVersion: "42", ObservedAt: time.Now(), Complete: true, Items: []kubernetesinventory.Object{{Definition: d, UID: "pod-" + q.Namespace, Name: "pod", Namespace: namespace, NamespaceUID: q.NamespaceUID, ResourceVersion: "42"}}}, nil
}
func (b *inventoryFixtureBackend) GetInventory(_ context.Context, _ db.KubernetesCluster, q kubernetesinventory.Query) (kubernetesinventory.Object, error) {
	b.calls = append(b.calls, q)
	d, _ := kubernetesinventory.Lookup(q.Type)
	return kubernetesinventory.Object{Definition: d, UID: q.UID, Name: q.Name, ResourceVersion: "43", ObservedAt: time.Now()}, nil
}

func TestPostgresWorkbenchNamespaceScopesAndCursorRevocation(t *testing.T) {
	databaseURL := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	if err := postgres.RunMigrations(t.Context(), databaseURL, filepath.Join("..", "..", "migrations", "postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	store, err := postgres.Open(t.Context(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	enterprise, department, user, role, cluster := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	execute := func(sql string, args ...any) {
		t.Helper()
		if _, e := store.Pool.Exec(t.Context(), sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	execute("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Plan8 inventory',$2,'UTC')", enterprise, enterprise.String())
	execute("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'operators')", department, enterprise)
	execute("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'operator')", user, enterprise, department, user.String())
	execute("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'namespace reader')", role, enterprise)
	execute("INSERT INTO permissions(id,description,registry_version) VALUES('kubernetes.read','read',1) ON CONFLICT(id) DO NOTHING")
	execute("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'kubernetes.read')", role)
	execute("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), enterprise, user, role)
	execute("INSERT INTO kubernetes_clusters(id,enterprise_id,name,api_server,connection_mode,default_namespace,environment,labels_hash) VALUES($1,$2,'owned cluster','https://cluster.example.test','in_cluster','default','development',decode(repeat('00',32),'hex'))", cluster, enterprise)
	ids := []uuid.UUID{uuid.New(), uuid.New()}
	for index, id := range ids {
		name := []string{"orders", "payments"}[index]
		_, err = store.Queries.UpsertKubernetesNamespaceResource(t.Context(), db.UpsertKubernetesNamespaceResourceParams{ID: id, EnterpriseID: enterprise, ClusterID: cluster, Name: name, NamespaceUid: name + "-uid"})
		if err != nil {
			t.Fatal(err)
		}
	}
	grant := func(id uuid.UUID) {
		execute("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'kubernetes_namespace',$4)", uuid.New(), enterprise, user, id)
	}
	grant(ids[0])
	backend := &inventoryFixtureBackend{}
	service := Service{Store: store, Backend: backend, CursorKey: bytes.Repeat([]byte{7}, 32)}
	actor := Actor{EnterpriseID: enterprise, SubjectID: user, SubjectType: "user", AuthorizationVersion: 1}
	input := ListInput{Type: "pod", Namespaces: Selection{Mode: "all_authorized"}, Limit: 1}
	page, err := service.List(t.Context(), actor, cluster, input)
	if err != nil || len(page.Items) != 1 || page.Items[0].Namespace != "orders" || backend.calls[0].AllNamespaces || backend.calls[0].NamespaceUID != "orders-uid" {
		t.Fatalf("namespace grant expanded: %+v %v", page, err)
	}
	before := len(backend.calls)
	input.Namespaces = Selection{Mode: "explicit", IDs: []uuid.UUID{}}
	page, err = service.List(t.Context(), actor, cluster, input)
	if err != nil || len(page.Items) != 0 || len(backend.calls) != before {
		t.Fatal("explicit empty selection queried resources")
	}
	input.Namespaces.IDs = []uuid.UUID{ids[1]}
	if _, err = service.List(t.Context(), actor, cluster, input); !errors.Is(err, authorization.ErrAccessDenied) {
		t.Fatalf("foreign namespace allowed: %v", err)
	}
	input = ListInput{Type: "node", Namespaces: Selection{Mode: "all_authorized"}, Limit: 1}
	if _, err = service.List(t.Context(), actor, cluster, input); !errors.Is(err, authorization.ErrAccessDenied) {
		t.Fatalf("namespace grant allowed nodes: %v", err)
	}
	grant(ids[1])
	input = ListInput{Type: "pod", Namespaces: Selection{Mode: "all_authorized"}, Limit: 1}
	page, err = service.List(t.Context(), actor, cluster, input)
	if err != nil || page.NextCursor == "" || page.Complete {
		t.Fatalf("namespace fanout cursor: %+v %v", page, err)
	}
	input.Cursor = page.NextCursor
	watchResult, err := service.Watch(t.Context(), actor, cluster, page.WatchToken)
	if err != nil || len(watchResult.Events) != 1 || watchResult.Events[0].Object.Namespace != page.Items[0].Namespace {
		t.Fatalf("watch escaped its page scope: %+v %v", watchResult, err)
	}
	foreign := actor
	foreign.SubjectID = uuid.New()
	if _, err = service.Watch(t.Context(), foreign, cluster, page.WatchToken); err == nil {
		t.Fatal("watch token replayed by another actor")
	}
	second, err := service.List(t.Context(), actor, cluster, input)
	if err != nil || len(second.Items) != 1 || second.Items[0].Namespace == page.Items[0].Namespace || !second.Complete {
		t.Fatalf("namespace fanout repeated/skipped: %+v %v", second, err)
	}
	execute("UPDATE enterprise_users SET authorization_version=authorization_version+1 WHERE id=$1", user)
	if _, err = service.List(t.Context(), actor, cluster, input); !errors.Is(err, authorization.ErrAccessDenied) {
		t.Fatalf("revoked cursor accepted: %v", err)
	}
	actor.AuthorizationVersion = 2
	input.Cursor = ""
	backend.foreign = true
	if _, err = service.List(t.Context(), actor, cluster, input); !errors.Is(err, kubernetesinventory.ErrIdentityChanged) {
		t.Fatalf("backend returned another namespace: %v", err)
	}
	logs := &legacyLogFixture{}
	legacy := resource.Service{Store: store, Kubernetes: logs, Access: resource.AccessService{}}
	subject := resource.Subject{ActorID: user.String(), ActorType: "enterprise_user", AuthorizationVersion: 2, AuthorizedResourceIDs: []uuid.UUID{cluster}}
	request := resource.PodLogsQuery{Namespace: "orders", Pod: "app", TailLines: 20}
	execute("INSERT INTO permissions(id,description,registry_version) VALUES('kubernetes.logs','logs',1),('kubernetes.operational_logs.read','ops logs',1) ON CONFLICT(id) DO NOTHING")
	execute("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'kubernetes.logs')", role)
	if _, _, err = legacy.GetKubernetesPodLogs(t.Context(), subject, enterprise, cluster, request); !errors.Is(err, resource.ErrResourceDenied) || logs.calls != 0 {
		t.Fatal("ordinary cluster log permission implied cross-application operations access")
	}
	execute("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'kubernetes.operational_logs.read')", role)
	content, _, err := legacy.GetKubernetesPodLogs(t.Context(), subject, enterprise, cluster, request)
	if err != nil || logs.calls != 1 || strings.Contains(string(content), "private-canary") || !strings.Contains(string(content), "[REDACTED]") {
		t.Fatalf("explicit operations log access/projection failed: %v", err)
	}
	backend.foreign = false
	input.Cursor = ""
	page, err = service.List(t.Context(), actor, cluster, input)
	if err != nil {
		t.Fatal(err)
	}
	backend.onWatch = func() {
		execute("UPDATE enterprise_users SET authorization_version=authorization_version+1 WHERE id=$1", user)
	}
	watchResult, err = service.Watch(t.Context(), actor, cluster, page.WatchToken)
	if !errors.Is(err, authorization.ErrAccessDenied) || len(watchResult.Events) != 0 {
		t.Fatalf("revocation during watch released data: %+v %v", watchResult, err)
	}
}

type legacyLogFixture struct{ calls int }

func (*legacyLogFixture) List(context.Context, db.KubernetesCluster, resource.KubernetesQuery) ([]resource.KubernetesObject, error) {
	return nil, nil
}
func (f *legacyLogFixture) PodLogs(context.Context, db.KubernetesCluster, resource.PodLogsQuery) ([]byte, bool, error) {
	f.calls++
	return []byte("password=private-canary\nnormal event"), false, nil
}
