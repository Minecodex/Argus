package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/logcollection"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type logWorkloadFixture struct {
	namespaces map[uuid.UUID]db.KubernetesNamespaceResource
	parent     string
}

func (f logWorkloadFixture) ResolveLogWorkload(_ context.Context, _ Actor, _ uuid.UUID, w logcollection.Workload) (logcollection.Workload, error) {
	ns := f.namespaces[w.NamespaceID]
	w.Namespace, w.NamespaceUID = ns.Name, ns.NamespaceUid
	w.Ancestors = nil
	if w.Type == "pod" {
		w.Ancestors = []string{f.parent}
	}
	return w, nil
}

func TestPostgresKubernetesLogNamespaceClaimsAndRecreation(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	collector := f.collector()
	cluster := uuid.New()
	f.exec("INSERT INTO kubernetes_clusters(id,enterprise_id,name,api_server,connection_mode,default_namespace,environment,labels_hash) VALUES($1,$2,'log cluster','https://cluster.example.test','in_cluster','default','development',decode(repeat('00',32),'hex'))", cluster, f.a.EnterpriseID)
	f.exec("UPDATE collector_instances SET resource_type='kubernetes_cluster',resource_id=$2 WHERE id=$1", collector, cluster)
	f.exec("UPDATE collector_distribution_versions SET components=ARRAY['filelog','file_storage','k8sattributes','argus_log_identity'] WHERE id=(SELECT distribution_version_id FROM collector_instances WHERE id=$1)", collector)
	f.exec("INSERT INTO role_permissions(role_id,permission_id) SELECT role_id,'telemetry.collector.manage' FROM role_bindings WHERE enterprise_id=$1", f.a.EnterpriseID)
	resolver := logWorkloadFixture{namespaces: map[uuid.UUID]db.KubernetesNamespaceResource{}, parent: uuid.NewString()}
	for _, name := range []string{"orders", "payments"} {
		id := uuid.New()
		ns, e := f.s.Store.Queries.UpsertKubernetesNamespaceResource(ctx, db.UpsertKubernetesNamespaceResourceParams{ID: id, EnterpriseID: f.a.EnterpriseID, ClusterID: cluster, Name: name, NamespaceUid: uuid.NewString()})
		if e != nil {
			t.Fatal(e)
		}
		resolver.namespaces[id] = ns
	}
	f.s.Workloads = resolver
	var own, foreign db.KubernetesNamespaceResource
	for _, ns := range resolver.namespaces {
		if ns.Name == "orders" {
			own = ns
		} else {
			foreign = ns
		}
	}
	for _, grant := range []struct{ subject, id uuid.UUID }{{f.a.SubjectID, own.ID}, {f.other.SubjectID, foreign.ID}} {
		f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'kubernetes_namespace',$4)", uuid.New(), f.a.EnterpriseID, grant.subject, grant.id)
	}
	create := func(ns db.KubernetesNamespaceResource, kind, uid string) Mutation {
		raw, _ := json.Marshal(logcollection.Config{Scope: "component", Workload: &logcollection.Workload{NamespaceID: ns.ID, Namespace: "client-forgery", NamespaceUID: "client-forgery", Type: kind, Name: "orders", UID: uid, Containers: []string{"main"}, Ancestors: []string{"client-forgery"}}})
		return Mutation{Kind: "collection", Operation: "create", Collection: &CollectionInput{Name: "Scoped workload logs", Integration: "k8slog", CollectorID: collector, Enabled: true, Config: raw}}
	}
	first, err := f.apply(create(own, "deployment", resolver.parent))
	if err != nil {
		t.Fatal(err)
	}
	row, err := f.s.Collection(ctx, f.a, first.ResourceID)
	if err != nil || row.Target.Kind != "kubernetes_cluster" || row.Target.ID != cluster {
		t.Fatalf("namespace-owned collection: %+v %v", row, err)
	}
	var stored logcollection.Config
	if json.Unmarshal(row.Config, &stored) != nil || stored.Workload.NamespaceUID != own.NamespaceUid || stored.Workload.Namespace != "orders" || len(stored.Workload.Ancestors) != 0 {
		t.Fatal("client ownership claims survived normalization")
	}
	if err = authorization.RequireResource(ctx, f.s.Store.Queries, f.a, "kubernetes_cluster", cluster); err == nil {
		t.Fatal("log assignment granted cluster-wide access")
	}
	choices, err := f.s.CollectorChoices(ctx, f.a, ListInput{})
	if err != nil || len(choices.Items) != 1 || choices.Items[0].Resource.ID!=cluster {
		t.Fatalf("namespace Collector navigation: %d %v", len(choices.Items), err)
	}
	if _, err = f.s.Collection(ctx, f.other, first.ResourceID); !errors.Is(err, ErrDenied) {
		t.Fatalf("other Namespace read collection: %v", err)
	}
	page, err := f.s.Collections(ctx, f.other, ListInput{})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("other Namespace listed rule: %v", err)
	}
	page, err = f.s.Collections(ctx, f.a, ListInput{})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("Namespace list omitted own rule: %v", err)
	}
	if _, err = f.apply(create(foreign, "deployment", uuid.NewString())); !errors.Is(err, ErrDenied) {
		t.Fatalf("foreign Namespace configured: %v", err)
	}
	if _, err = f.apply(create(own, "pod", uuid.NewString())); !errors.Is(err, ErrLogOverlap) {
		t.Fatalf("Pod/controller overlap missed: %v", err)
	}
	if err = f.s.Store.Queries.RetireReplacedKubernetesNamespace(ctx, db.RetireReplacedKubernetesNamespaceParams{EnterpriseID: f.a.EnterpriseID, ClusterID: cluster, Name: own.Name, NamespaceUid: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Store.Queries.UpsertKubernetesNamespaceResource(ctx, db.UpsertKubernetesNamespaceResourceParams{ID: uuid.New(), EnterpriseID: f.a.EnterpriseID, ClusterID: cluster, Name: own.Name, NamespaceUid: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	page, err = f.s.Collections(ctx, f.a, ListInput{})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("recreated Namespace inherited old log rule: %v", err)
	}
	if _, err = f.apply(Mutation{Kind: "collection", Operation: "set_enabled", ID: first.ResourceID, ExpectedVersion: 1, Collection: &CollectionInput{Enabled: false}}); !errors.Is(err, ErrDenied) {
		t.Fatalf("retired Namespace grant controlled rule: %v", err)
	}
	// A cluster administrator can retire an orphaned rule without reading a
	// deleted workload, but cannot reactivate its retired Namespace identity.
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'kubernetes_cluster',$4)", uuid.New(), f.a.EnterpriseID, f.a.SubjectID, cluster)
	if _, err = f.apply(Mutation{Kind: "collection", Operation: "set_enabled", ID: first.ResourceID, ExpectedVersion: 1, Collection: &CollectionInput{Enabled: false}}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.apply(Mutation{Kind: "collection", Operation: "set_enabled", ID: first.ResourceID, ExpectedVersion: 2, Collection: &CollectionInput{Enabled: true}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("retired Namespace rule re-enabled: %v", err)
	}
}
