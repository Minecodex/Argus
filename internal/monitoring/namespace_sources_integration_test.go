package monitoring

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/logcollection"
	"github.com/kakj-go/Argus/internal/observationscope"
	"github.com/kakj-go/Argus/internal/resourceref"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Tests selection/source authority with real PostgreSQL and domain-created
// collections. Pod I/O and native Collector gates have separate live evidence.
func TestPostgresNamespaceSourcesOperationalPermissionAndFrozenKeys(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	collector, cluster := f.collector(), uuid.New()
	f.exec("INSERT INTO kubernetes_clusters(id,enterprise_id,name,api_server,connection_mode,default_namespace,environment,labels_hash) VALUES($1,$2,'source cluster','https://cluster.example.test','in_cluster','default','development',decode(repeat('00',32),'hex'))", cluster, f.a.EnterpriseID)
	f.exec("UPDATE collector_instances SET resource_type='kubernetes_cluster',resource_id=$2 WHERE id=$1", collector, cluster)
	f.exec("UPDATE collector_distribution_versions SET components=ARRAY['filelog','file_storage','k8sattributes','argus_log_identity'] WHERE id=(SELECT distribution_version_id FROM collector_instances WHERE id=$1)", collector)
	f.exec("INSERT INTO role_permissions(role_id,permission_id) SELECT role_id,'telemetry.collector.manage' FROM role_bindings WHERE enterprise_id=$1", f.a.EnterpriseID)
	ns, err := f.s.Store.Queries.UpsertKubernetesNamespaceResource(ctx, db.UpsertKubernetesNamespaceResourceParams{ID: uuid.New(), EnterpriseID: f.a.EnterpriseID, ClusterID: cluster, Name: "orders", NamespaceUid: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	f.s.Workloads = logWorkloadFixture{namespaces: map[uuid.UUID]db.KubernetesNamespaceResource{ns.ID: ns}}
	for _, subject := range []uuid.UUID{f.a.SubjectID, f.other.SubjectID} {
		f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'kubernetes_namespace',$4)", uuid.New(), f.a.EnterpriseID, subject, ns.ID)
	}
	apps := []Application{f.application("App A", "production"), f.application("App B", "production")}
	collectorRow, err := f.s.Store.Queries.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: f.a.EnterpriseID, ID: collector})
	if err != nil {
		t.Fatal(err)
	}
	f.exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,1,'{}','{}',$3,'effective')", uuid.New(), collector, bytes.Repeat([]byte{1}, 32))
	for i := 0; i < 3; i++ {
		rule := logcollection.Config{Scope: "component", Workload: &logcollection.Workload{NamespaceID: ns.ID, Type: "deployment", Name: "orders", UID: uuid.NewString(), Containers: []string{"main"}}}
		input := &CollectionInput{Name: uuid.NewString(), Integration: "k8slog", CollectorID: collector, Enabled: true}
		if i > 0 {
			app := apps[i-1]
			service, e := f.apply(Mutation{Kind: "service", Operation: "create", Service: &ApplicationServiceInput{ApplicationID: app.ID, Name: "orders"}})
			if e != nil {
				t.Fatal(e)
			}
			input.ApplicationID, input.ServiceID = app.ID, service.ResourceID
			rule.Scope = "business"
		}
		input.Config, _ = json.Marshal(rule)
		result, e := f.apply(Mutation{Kind: "collection", Operation: "create", Collection: input})
		if e != nil {
			t.Fatal(e)
		}
		row, e := f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: result.ResourceID})
		if e != nil {
			t.Fatal(e)
		}
		e = f.s.Store.Queries.RegisterCollectionTelemetrySource(ctx, db.RegisterCollectionTelemetrySourceParams{ID: uuid.New(), EnterpriseID: f.a.EnterpriseID, CollectorID: collector, Generation: collectorRow.SourceGeneration, ResourceType: row.TargetType, ResourceID: row.TargetID, SourceKey: "kubernetes_agent/filelog/" + row.ID.String(), SourceType: "filelog", Signals: []string{"logs"}, ConfigRevision: 1, ConfigHash: bytes.Repeat([]byte{1}, 32), CapabilityVersion: "v1", CollectionID: nullID(row.ID), CollectionRevision: pgtype.Int8{Int64: 1, Valid: true}, AssignmentEpoch: pgtype.Int8{Int64: 1, Valid: true}, ApplicationServiceID: row.ServiceID, NamespaceUid: ns.NamespaceUid})
		if e != nil {
			t.Fatal(e)
		}
	}
	resolver := observationscope.Resolver{Queries: f.s.Store.Queries}
	selection := observationscope.SourceSelection{Resources: []resourceref.Ref{{Type: "kubernetes_namespace", ID: ns.ID}}, Signal: "logs"}
	result, err := resolver.Sources(ctx, f.other, selection)
	if err != nil || len(result.Rows) != 1 || result.Rows[0].ResourceType != "kubernetes_cluster" {
		t.Fatalf("plain Namespace exposed business logs: %+v %v", result.Keys, err)
	}
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'application',$4)", uuid.New(), f.a.EnterpriseID, f.other.SubjectID, apps[0].ID)
	result, err = resolver.Sources(ctx, f.other, selection)
	if err != nil || len(result.Rows) != 2 {
		t.Fatalf("application + Namespace intersection: %v", err)
	}
	for _, row := range result.Rows {
		if row.ResourceID == apps[1].ID {
			t.Fatal("ungranted application read")
		}
	}
	role, binding := uuid.New(), uuid.New()
	f.exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'temporary operational logs')", role, f.a.EnterpriseID)
	f.exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'kubernetes.operational_logs.read')", role)
	f.exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id,valid_until) VALUES($1,$2,'user',$3,$4,now()+interval '1 hour')", binding, f.a.EnterpriseID, f.other.SubjectID, role)
	result, err = resolver.Sources(ctx, f.other, selection)
	if err != nil || len(result.Rows) != 3 || len(result.DataResources) != 3 {
		t.Fatalf("explicit operational grant: %v", err)
	}
	for _, row := range result.Rows {
		if !slices.Equal(row.ScopeResourceIds, []uuid.UUID{ns.ID}) {
			t.Fatal("selection authority replaced with physical resource grant")
		}
	}
	if err = authorization.RequireResource(ctx, f.s.Store.Queries, f.other, "application", apps[1].ID); err == nil {
		t.Fatal("operational log read granted application access")
	}
	frozen := slices.Clone(result.Keys)
	filtered := selection
	filtered.Applications = &observationscope.ApplicationSelection{Mode: observationscope.Explicit, IDs: []uuid.UUID{apps[0].ID}}
	result, err = resolver.Sources(ctx, f.other, filtered)
	if err != nil || len(result.Rows) != 1 || result.Rows[0].ResourceID != apps[0].ID {
		t.Fatalf("application filter included unrelated logs: %v", err)
	}
	for _, signal := range []string{"traces", "metrics"} {
		empty := selection
		empty.Signal = signal
		value, e := resolver.Sources(ctx, f.other, empty)
		if e != nil || len(value.Rows) != 0 {
			t.Fatalf("Namespace logs widened to %s: %v", signal, e)
		}
	}
	empty := selection
	empty.Keys = []string{}
	result, err = resolver.Sources(ctx, f.other, empty)
	if err != nil || len(result.Rows) != 0 {
		t.Fatal("explicit empty sources widened")
	}
	empty.Resources = []resourceref.Ref{}
	empty.Keys = nil
	result, err = resolver.Sources(ctx, f.other, empty)
	if err != nil || len(result.Rows) != 0 {
		t.Fatal("explicit empty resources widened")
	}
	// Natural role expiry need not bump the subject version. Revalidating the
	// frozen keys must still reject data the actor can no longer read.
	f.exec("UPDATE role_bindings SET valid_until=now()-interval '1 second' WHERE id=$1", binding)
	selection.Keys = frozen
	if _, err = resolver.Sources(ctx, f.other, selection); !errors.Is(err, ErrDenied) {
		t.Fatalf("expired operational authority retained frozen sources: %v", err)
	}
	if err = f.s.Store.Queries.RetireReplacedKubernetesNamespace(ctx, db.RetireReplacedKubernetesNamespaceParams{EnterpriseID: f.a.EnterpriseID, ClusterID: cluster, Name: ns.Name, NamespaceUid: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	if _, err = resolver.Sources(ctx, f.other, selection); !errors.Is(err, ErrDenied) {
		t.Fatalf("retired Namespace retained source authority: %v", err)
	}
}
