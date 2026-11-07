package dashboard

import (
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresDashboardSourceNamesUseRegisteredReadableOrigins(t *testing.T) {
	f := newAnalysisFixture(t)
	ctx := t.Context()
	e, user := f.actor.EnterpriseID, f.actor.SubjectID
	host, app, collector, distribution, generation := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	collection, endpoint, journalSource, otlpSource := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := f.store.Pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,labels_hash,role,control_path)
		VALUES($1,$2,'Linux node','127.0.0.1',22,'development','linux',decode(repeat('00',32),'hex'),'managed_host','direct')`, host, e)
	exec(`INSERT INTO monitoring_applications(id,enterprise_id,application_key,name,environment,created_by)
		VALUES($1,$2,'app','Orders','production',$3)`, app, e, user)
	resources := []ResourceScope{{Type: "host", ID: host}, {Type: "application", ID: app}}
	for _, resource := range resources {
		exec(`INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id)
			VALUES($1,$2,'user',$3,$4,$5)`, uuid.New(), e, user, resource.Type, resource.ID)
	}
	exec(`INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,artifact_manifest,catalog_revision)
		VALUES($1,'Source names fixture',$2,'fixture','v1','supported','[]',1)`, distribution, distribution.String())
	exec(`INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,source_generation,desired_revision,effective_revision)
		VALUES($1,$2,'host',$3,$4,'linux_amd64','direct','converged',$5,1,1)`, collector, e, host, distribution, generation)
	exec(`INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status)
		VALUES($1,$2,1,'{}','{}',decode(repeat('00',32),'hex'),'effective')`, uuid.New(), collector)
	exec(`INSERT INTO monitoring_collections(id,enterprise_id,name,integration_key,target_type,target_id,collector_id,created_by)
		VALUES($1,$2,'Host Journal','journald','host',$3,$4,$5)`, collection, e, host, collector, user)
	if _, err := f.store.Queries.SnapshotMonitoringCollection(ctx, db.SnapshotMonitoringCollectionParams{EnterpriseID: e, CollectionID: collection, ActorID: user}); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO monitoring_collection_assignments(collection_id,enterprise_id,epoch,collector_id)
		VALUES($1,$2,1,$3)`, collection, e, collector)
	exec(`INSERT INTO monitoring_ingest_endpoints(id,enterprise_id,application_id,name,protocol,signals,collector_id,created_by)
		VALUES($1,$2,$3,'Orders OTLP','otlp',ARRAY['logs'],$4,$5)`, endpoint, e, app, collector, user)
	exec(`INSERT INTO telemetry_sources(id,enterprise_id,collector_id,generation,resource_type,resource_id,source_key,source_type,signals,config_revision,config_hash,capability_version,collection_id,collection_revision,assignment_epoch,ingest_endpoint_id,ingest_endpoint_version) VALUES
		($1,$3,$4,$5,'host',$6,'journal','journald',ARRAY['logs'],1,decode(repeat('00',32),'hex'),'v1',$8,1,1,NULL,NULL),
		($2,$3,$4,$5,'application',$7,'otlp','otlp',ARRAY['logs'],1,decode(repeat('00',32),'hex'),'v1',NULL,NULL,NULL,$9,1)`, journalSource, otlpSource, e, collector, generation, host, app, collection, endpoint)

	sources := []ResolvedSource{
		{ID: journalSource, Revision: 1, ResourceType: "host", ResourceID: host},
		{ID: otlpSource, Revision: 1, ResourceType: "application", ResourceID: app},
		{ID: journalSource, Revision: 2, ResourceType: "host", ResourceID: host},
	}
	check := func(scope []ResourceScope, journal, otlp string) {
		t.Helper()
		if err := populateSourceNames(ctx, f.store.Queries, f.actor, scope, sources); err != nil {
			t.Fatal(err)
		}
		if sources[0].Name != journal || sources[1].Name != otlp || sources[2].Name != "" {
			t.Fatalf("unexpected source labels: %q, %q, %q", sources[0].Name, sources[1].Name, sources[2].Name)
		}
	}
	check(resources, "", "")
	exec(`INSERT INTO role_permissions(role_id,permission_id) SELECT id,'monitoring.read' FROM roles WHERE enterprise_id=$1`, e)
	check(resources, "Host Journal", "")
	exec(`INSERT INTO role_permissions(role_id,permission_id) SELECT id,'application.read' FROM roles WHERE enterprise_id=$1`, e)
	check(resources, "Host Journal", "Orders OTLP")
	check(resources[:1], "Host Journal", "")
}
