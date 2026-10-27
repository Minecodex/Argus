package dashboard

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/resourceref"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

type namespaceRuntimeBackend struct {
	t                              *testing.T
	application, namespace, source uuid.UUID
	calls                          int
	revoke                         func()
}

func (b *namespaceRuntimeBackend) ExecuteEngineQuery(_ context.Context, request queryengine.Request) (queryengine.Result, error) {
	b.calls++
	if !slices.Equal(request.Scope.Resources, []resourceref.Ref{{Type: "application", ID: b.application}}) || !slices.Equal(request.Scope.ResourceIDs, []uuid.UUID{b.application}) || !slices.Equal(request.Scope.SourceKeys, []string{b.source.String() + ":1"}) {
		b.t.Fatal("Namespace authority did not become a bounded physical source scope")
	}
	if b.revoke != nil {
		b.revoke()
	}
	return queryengine.Result{Language: request.Language, ResultType: "log_entries", Data: []map[string]any{{"body": "controlled query fixture"}}, Meta: queryengine.QueryMeta{ScannedBytes: 1, ReturnedRows: 1}}, nil
}
func (b *namespaceRuntimeBackend) DiscoverData(_ context.Context, request telemetry.DataCatalogRequest) (telemetry.DataCatalogResult, error) {
	if !slices.Equal(request.Resources, []resourceref.Ref{{Type: "application", ID: b.application}}) || !slices.Equal(request.ResourceIDs, []uuid.UUID{b.application}) {
		b.t.Fatal("candidate lookup used a virtual Namespace as a storage identity")
	}
	return telemetry.DataCatalogResult{Values: []string{"controlled query fixture"}, Complete: true, Membership: map[string]bool{}}, nil
}

func TestPostgresNamespaceDashboardUsesPhysicalSourcesAndRechecksExpiry(t *testing.T) {
	f := newAnalysisFixture(t)
	ctx := t.Context()
	execute := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.store.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	cluster, namespace, application, service, collector, distribution, generation, source, collection := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	namespaceUID := uuid.NewString()
	execute("INSERT INTO kubernetes_clusters(id,enterprise_id,name,api_server,connection_mode,default_namespace,environment,labels_hash) VALUES($1,$2,'scope cluster','https://cluster.example.test','in_cluster','default','development',decode(repeat('00',32),'hex'))", cluster, f.actor.EnterpriseID)
	execute("INSERT INTO kubernetes_namespace_resources(id,enterprise_id,cluster_id,name,namespace_uid,resource_version) VALUES($1,$2,$3,'orders',$4,'1')", namespace, f.actor.EnterpriseID, cluster, namespaceUID)
	execute("INSERT INTO monitoring_applications(id,enterprise_id,application_key,name,environment,created_by) VALUES($1,$2,$3,'Private application','production',$4)", application, f.actor.EnterpriseID, application.String(), f.actor.SubjectID)
	execute("INSERT INTO monitoring_application_services(id,enterprise_id,application_id,service_name,service_namespace,runtime) VALUES($1,$2,$3,'orders','business','controlled-fixture')", service, f.actor.EnterpriseID, application)
	execute("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'kubernetes_namespace',$4)", uuid.New(), f.actor.EnterpriseID, f.actor.SubjectID, namespace)
	execute("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,artifact_manifest,catalog_revision) VALUES($1,$2,'fixture','fixture','fixture','supported','[]',1)", distribution, distribution.String())
	execute("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,source_generation,desired_revision,effective_revision) VALUES($1,$2,'kubernetes_cluster',$3,$4,'linux_amd64','direct','converged',$5,1,1)", collector, f.actor.EnterpriseID, cluster, distribution, generation)
	execute("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,1,'{}','{}',decode(repeat('00',32),'hex'),'effective')", uuid.New(), collector)
	execute("INSERT INTO monitoring_collections(id,enterprise_id,name,integration_key,target_type,target_id,collector_id,application_id,service_id,created_by) VALUES($1,$2,'Controlled source','k8slog','application',$3,$4,$3,$5,$6)", collection, f.actor.EnterpriseID, application, collector, service, f.actor.SubjectID)
	if _, err := f.store.Queries.SnapshotMonitoringCollection(ctx, db.SnapshotMonitoringCollectionParams{EnterpriseID: f.actor.EnterpriseID, CollectionID: collection, ActorID: f.actor.SubjectID}); err != nil {
		t.Fatal(err)
	}
	execute("INSERT INTO monitoring_collection_assignments(collection_id,enterprise_id,epoch,collector_id) VALUES($1,$2,1,$3)", collection, f.actor.EnterpriseID, collector)
	execute("INSERT INTO telemetry_sources(id,enterprise_id,collector_id,generation,resource_type,resource_id,source_key,source_type,signals,config_revision,config_hash,capability_version,collection_id,collection_revision,assignment_epoch,application_service_id,namespace_uid) VALUES($1,$2,$3,$4,'application',$5,'kubernetes_agent/filelog/fixture','filelog',ARRAY['logs'],1,decode(repeat('00',32),'hex'),'v1',$6,1,1,$7,$8)", source, f.actor.EnterpriseID, collector, generation, application, collection, service, namespaceUID)
	role, binding := uuid.New(), uuid.New()
	execute("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Temporary log reader')", role, f.actor.EnterpriseID)
	execute("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'kubernetes.operational_logs.read')", role)
	execute("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id,valid_until) VALUES($1,$2,'user',$3,$4,now()+interval '1 hour')", binding, f.actor.EnterpriseID, f.actor.SubjectID, role)
	backend := &namespaceRuntimeBackend{t: t, application: application, namespace: namespace, source: source}
	runtime := Runtime{Store: f.store, Backend: backend}
	panel := validPanel()
	panel.ID, panel.Title, panel.Type, panel.Signal = "logs", "Scoped logs", "logs", "logs"
	panel.ApplicableResourceTypes = []string{"kubernetes_namespace"}
	panel.SourceBinding = SourceBinding{SourceType: "filelog", CapabilityVersion: "v1"}
	panel.Targets = []Target{{ID: "logs", Language: queryengine.LanguageKQL, SourceDefinition: Definition{Builder: &Builder{Operation: "records", Limit: 10}}}}
	spec := EmptySpec()
	spec.Panels = []Panel{panel}
	input := ExecutionInput{ResourceMode: "explicit", ResourceTypes: []string{"kubernetes_namespace"}, ResourceIDs: []uuid.UUID{namespace}}
	result, err := runtime.executeSpec(ctx, f.actor, spec, input)
	if err != nil || backend.calls != 1 || len(result.Panels) != 1 || result.Panels[0].Targets[0].Status != "success" {
		t.Fatalf("Namespace log execution: %+v %v", result.Panels, err)
	}
	if result.ScopeSelection == nil || result.Resources[0].Type != "kubernetes_namespace" || result.Resources[0].ID != namespace {
		t.Fatal("public selection authority was replaced with application access")
	}
	if _, err = resolveResources(ctx, f.store.Queries, f.actor, []uuid.UUID{application}); !errors.Is(err, ErrDenied) {
		t.Fatalf("Namespace read granted application identity: %v", err)
	}
	if _, err = runtime.Catalog(ctx, f.actor, CatalogInput{ResourceMode: "explicit", ResourceTypes: input.ResourceTypes, ResourceIDs: input.ResourceIDs, SourceBinding: panel.SourceBinding, Signal: "logs", Kind: "values", Field: "body", From: result.From, To: result.To, Limit: 10}); err != nil {
		t.Fatal(err)
	}
	backend.revoke = func() { execute("UPDATE role_bindings SET valid_until=now()-interval '1 second' WHERE id=$1", binding) }
	if _, err = runtime.executeSpec(ctx, f.actor, spec, input); !errors.Is(err, ErrDenied) {
		t.Fatalf("role expiry during query released data: %v", err)
	}
}
