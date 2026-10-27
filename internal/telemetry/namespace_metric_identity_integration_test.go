package telemetry

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresMetricNamespaceUIDCannotFollowRecreatedName(t *testing.T) {
	address := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires owned disposable PostgreSQL")
	}
	if err := postgres.RunMigrations(t.Context(), address, filepath.Join("..", "..", "migrations", "postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(t.Context(), sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	enterprise, cluster := uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Namespace admission',$2,'UTC')", enterprise, enterprise.String())
	exec("INSERT INTO kubernetes_clusters(id,enterprise_id,name,api_server,connection_mode,default_namespace,environment,labels_hash) VALUES($1,$2,'Namespace admission','https://cluster.example.test','in_cluster','default','development',decode(repeat('00',32),'hex'))", cluster, enterprise)
	uid := uuid.NewString()
	row, err := store.Queries.UpsertKubernetesNamespaceResource(t.Context(), db.UpsertKubernetesNamespaceResourceParams{ID: uuid.New(), EnterpriseID: enterprise, ClusterID: cluster, Name: "orders", NamespaceUid: uid})
	if err != nil {
		t.Fatal(err)
	}
	control := PostgresIngestControl{Queries: store.Queries}
	identity := TrustedIdentity{EnterpriseID: enterprise, CollectorID: uuid.New()}
	source := SourceIdentity{ResourceType: "kubernetes_cluster", ResourceID: cluster, Type: "k8s_cluster"}
	podUID := uuid.NewString()
	exec("INSERT INTO kubernetes_namespace_object_identities(enterprise_id,cluster_id,object_uid,object_type,namespace_name,namespace_uid) VALUES($1,$2,$3,'pod','orders',$4)", enterprise, cluster, podUID, uid)
	objects := []MetricNamespaceObject{{Type: "pod", UID: podUID}}
	if actual, err := control.ResolveMetricObjectNamespace(t.Context(), identity, source, "orders", objects); err != nil || actual != uid {
		t.Fatal("trusted Pod to Namespace identity denied", err)
	}
	unknown := MetricNamespaceObject{Type: "pod", UID: uuid.NewString()}
	if _, err := control.ResolveMetricObjectNamespace(t.Context(), identity, source, "orders", []MetricNamespaceObject{unknown, objects[0]}); !errors.Is(err, errNamespaceAttributionUnverified) {
		t.Fatal("unknown object was inferred or hid trusted lookup failure", err)
	}
	for _, mixed := range [][]MetricNamespaceObject{{unknown, {Type: "deployment", UID: podUID}}, {{Type: "deployment", UID: podUID}, unknown}} {
		if _, err := control.ResolveMetricObjectNamespace(t.Context(), identity, source, "orders", mixed); !errors.Is(err, errIngestRejected) {
			t.Fatal("unknown UID masked a known type contradiction", err)
		}
	}
	if _, err := control.ResolveMetricObjectNamespace(t.Context(), identity, source, "other", []MetricNamespaceObject{unknown, objects[0]}); !errors.Is(err, errIngestRejected) {
		t.Fatal("unknown UID masked a known Namespace contradiction", err)
	}
	if actual, err := control.ResolveMetricNamespace(t.Context(), identity, source, "orders", uid); err != nil || actual != uid {
		t.Fatal("valid namespace denied", err)
	}
	if _, err := control.ResolveMetricNamespace(t.Context(), identity, source, "orders", ""); !errors.Is(err, ErrSourceAwaitingApply) {
		t.Fatal("missing native UID was inferred by name", err)
	}
	exec("UPDATE kubernetes_namespace_resources SET status='deleted',version=version+1 WHERE id=$1", row.ID)
	_, err = store.Queries.UpsertKubernetesNamespaceResource(t.Context(), db.UpsertKubernetesNamespaceResourceParams{ID: uuid.New(), EnterpriseID: enterprise, ClusterID: cluster, Name: "orders", NamespaceUid: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := control.ResolveMetricNamespace(t.Context(), identity, source, "orders", uid); err == nil {
		t.Fatal("old UID was rebound to a recreated namespace")
	}
	if _, err := control.ResolveMetricObjectNamespace(t.Context(), identity, source, "orders", objects); !errors.Is(err, errNamespaceAttributionUnverified) {
		t.Fatal("old Pod UID was rebound by a recreated Namespace name", err)
	}
	if err := store.Queries.ObserveKubernetesNamespaceObjectIdentity(t.Context(), db.ObserveKubernetesNamespaceObjectIdentityParams{EnterpriseID: enterprise, ClusterID: cluster, ObjectUid: podUID, ObjectType: "pod", NamespaceName: "orders", NamespaceUid: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	if _, err := control.ResolveMetricObjectNamespace(t.Context(), identity, source, "other", objects); !errors.Is(err, errIngestRejected) {
		t.Fatal("Pod UID accepted under a different Namespace name", err)
	}
	other := source
	other.ResourceID = uuid.New()
	if _, err := control.ResolveMetricNamespace(t.Context(), identity, other, "orders", uid); !errors.Is(err, ErrSourceAwaitingApply) {
		t.Fatal("unobserved foreign Cluster namespace accepted", err)
	}
}
