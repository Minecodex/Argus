package telemetry

import (
	"errors"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"os"
	"path/filepath"
	"testing"
)

func TestActualOpaqueKubernetesUIDIsResolvedOnlyFromTrustedInventory(t *testing.T) {
	address := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("owned disposable PostgreSQL required")
	}
	ctx := t.Context()
	if err := postgres.RunMigrations(ctx, address, filepath.Join("..", "..", "migrations", "postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	store, err := postgres.Open(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	e, c := uuid.New(), uuid.New()
	exec(`INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Opaque UID',$2,'UTC')`, e, e.String())
	exec(`INSERT INTO kubernetes_clusters(id,enterprise_id,name,api_server,connection_mode,default_namespace,environment,labels_hash) VALUES($1,$2,'Opaque UID','https://cluster.example.test','in_cluster','default','development',decode(repeat('00',32),'hex'))`, c, e)
	n := uuid.NewString()
	_, err = store.Queries.UpsertKubernetesNamespaceResource(ctx, db.UpsertKubernetesNamespaceResourceParams{ID: uuid.New(), EnterpriseID: e, ClusterID: c, Name: "kube-system", NamespaceUid: n})
	if err != nil {
		t.Fatal(err)
	}
	// A real static Pod UID observed in saved native Kubernetes captures.
	pod := "0e74ad7780cc02c361bb66f53ee8fdc5"
	exec(`INSERT INTO kubernetes_namespace_object_identities(enterprise_id,cluster_id,object_uid,object_type,namespace_name,namespace_uid) VALUES($1,$2,$3,'pod','kube-system',$4)`, e, c, pod, n)
	control := PostgresIngestControl{Queries: store.Queries}
	identity := TrustedIdentity{EnterpriseID: e, CollectorID: uuid.New()}
	source := SourceIdentity{ResourceType: "kubernetes_cluster", ResourceID: c, Type: "k8s_cluster"}
	got, err := control.ResolveMetricObjectNamespace(ctx, identity, source, "kube-system", []MetricNamespaceObject{{Type: "pod", UID: pod}})
	if err != nil || got != n {
		t.Fatalf("trusted opaque Kubernetes UID rejected: namespace=%s error=%v", got, err)
	}
	_, err = control.ResolveMetricObjectNamespace(ctx, identity, source, "other", []MetricNamespaceObject{{Type: "pod", UID: pod}})
	if !errors.Is(err, errIngestRejected) {
		t.Fatal("opaque UID gained another Namespace", err)
	}
	for _, bad := range []string{"", " ", "with space", "with\ncontrol", "with\x00nul"} {
		_, err = control.ResolveMetricObjectNamespace(ctx, identity, source, "kube-system", []MetricNamespaceObject{{Type: "pod", UID: bad}})
		if !errors.Is(err, errIngestRejected) {
			t.Fatal("malformed object UID accepted", err)
		}
	}
	_, err = control.ResolveMetricObjectNamespace(ctx, identity, source, "kube-system", []MetricNamespaceObject{{Type: "deployment", UID: pod}})
	if !errors.Is(err, errIngestRejected) {
		t.Fatal("opaque UID gained another object type", err)
	}
	source.ResourceID = uuid.New()
	_, err = control.ResolveMetricObjectNamespace(ctx, identity, source, "kube-system", []MetricNamespaceObject{{Type: "pod", UID: pod}})
	if !errors.Is(err, errNamespaceAttributionUnverified) {
		t.Fatal("opaque UID gained another Cluster", err)
	}
}
