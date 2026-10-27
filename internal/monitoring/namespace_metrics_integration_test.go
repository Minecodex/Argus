package monitoring

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/observationscope"
	"github.com/kakj-go/Argus/internal/resourceref"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresNamespaceMetricSourcesKeepUIDBoundSelectors(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	collector, cluster, collection, source := f.collector(), uuid.New(), uuid.New(), uuid.New()
	f.exec("INSERT INTO kubernetes_clusters(id,enterprise_id,name,api_server,connection_mode,default_namespace,environment,labels_hash) VALUES($1,$2,'Namespace metric scope','https://cluster.example.test','in_cluster','default','development',decode(repeat('00',32),'hex'))", cluster, f.a.EnterpriseID)
	f.exec("UPDATE collector_instances SET resource_type='kubernetes_cluster',resource_id=$2 WHERE id=$1", collector, cluster)
	f.exec("INSERT INTO monitoring_collections(id,enterprise_id,name,integration_key,target_type,target_id,collector_id,created_by) VALUES($1,$2,'Cluster state','kubernetes_cluster_metrics','kubernetes_cluster',$3,$4,$5)", collection, f.a.EnterpriseID, cluster, collector, f.a.SubjectID)
	f.exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,1,'{}','{}',decode(repeat('00',32),'hex'),'effective')", uuid.New(), collector)
	if _, err := f.s.Store.Queries.SnapshotMonitoringCollection(ctx, db.SnapshotMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, CollectionID: collection, ActorID: f.a.SubjectID}); err != nil {
		t.Fatal(err)
	}
	row, err := f.s.Store.Queries.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: f.a.EnterpriseID, ID: collector})
	if err != nil {
		t.Fatal(err)
	}
	err = f.s.Store.Queries.RegisterCollectionTelemetrySource(ctx, db.RegisterCollectionTelemetrySourceParams{ID: source, EnterpriseID: f.a.EnterpriseID, CollectorID: collector, Generation: row.SourceGeneration, ResourceType: "kubernetes_cluster", ResourceID: cluster, SourceKey: "gateway/k8s_cluster/test", SourceType: "k8s_cluster", Signals: []string{"metrics"}, ConfigRevision: 1, ConfigHash: bytes.Repeat([]byte{1}, 32), CapabilityVersion: "v1", CollectionID: nullID(collection), CollectionRevision: pgtype.Int8{Int64: 1, Valid: true}, AssignmentEpoch: pgtype.Int8{Int64: 1, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	ns, err := f.s.Store.Queries.UpsertKubernetesNamespaceResource(ctx, db.UpsertKubernetesNamespaceResourceParams{ID: uuid.New(), EnterpriseID: f.a.EnterpriseID, ClusterID: cluster, Name: "orders", NamespaceUid: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'kubernetes_namespace',$4)", uuid.New(), f.a.EnterpriseID, f.other.SubjectID, ns.ID)
	resolver := observationscope.Resolver{Queries: f.s.Store.Queries}
	selection := observationscope.SourceSelection{Resources: []resourceref.Ref{{Type: "kubernetes_namespace", ID: ns.ID}}, Signal: "metrics"}
	result, err := resolver.Sources(ctx, f.other, selection)
	base := source.String() + ":1"
	if err != nil || !slices.Equal(result.Keys, []string{base + "@" + ns.NamespaceUid}) || len(result.DataResources) != 1 || result.DataResources[0].ID != cluster {
		t.Fatal("Namespace source became empty or unbounded", result.Keys, err)
	}
	selection.Keys = []string{base + "@" + uuid.NewString()}
	if _, err = resolver.Sources(ctx, f.other, selection); !errors.Is(err, observationscope.ErrDenied) {
		t.Fatal("foreign Namespace selector accepted", err)
	}
	selection.Keys = []string{base}
	selected, err := resolver.Sources(ctx, f.other, selection)
	if err != nil || !slices.Equal(selected.Keys, result.Keys) {
		t.Fatal("public source selection removed its Namespace boundary", err)
	}
	f.exec("UPDATE kubernetes_namespace_resources SET status='deleted',version=version+1 WHERE id=$1", ns.ID)
	if _, err = resolver.Sources(ctx, f.other, selection); err == nil {
		t.Fatal("deleted/replaced Namespace kept its source authority")
	}
}
