package telemetry

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresSourceApplyWindowIsRetryableAndRetiredConfigurationIsFenced(t *testing.T) {
	url := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	ctx := t.Context()
	if err := postgres.RunMigrations(ctx, url, filepath.Join("..", "..", "migrations", "postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	store, err := postgres.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	e, host, collector, generation, distribution, source, creator, target, collection, collectionSource := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Source apply window',$2,'UTC')", e, e.String())
	exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,labels_hash) VALUES($1,$2,'Collector','127.0.0.1',22,'development','linux','managed_host','direct',decode(repeat('00',32),'hex'))", host, e)
	exec("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,artifact_manifest,catalog_revision) VALUES($1,$2,'fixture','fixture','fixture','supported','[]',1)", distribution, distribution.String())
	exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,source_generation,desired_revision,effective_revision) VALUES($1,$2,'host',$3,$4,'linux_amd64','direct','converged',$5,2,1)", collector, e, host, distribution, generation)
	for revision, state := range map[int]string{1: "effective", 2: "applying"} {
		exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,$3,'{}','{}',decode(repeat('00',32),'hex'),$4)", uuid.New(), collector, revision, state)
	}
	for _, revision := range []int{1, 2} {
		exec("INSERT INTO telemetry_sources(id,enterprise_id,collector_id,generation,resource_type,resource_id,source_key,source_type,signals,config_revision,config_hash,capability_version) VALUES($1,$2,$3,$4,'host',$5,'host/hostmetrics','hostmetrics',ARRAY['metrics'],$6,decode(repeat('00',32),'hex'),'v1')", source, e, collector, generation, host, revision)
	}
	exec("INSERT INTO monitoring_targets(id,enterprise_id,name,kind,engine,identity_config,created_by) VALUES($1,$2,'Controlled database','database','mysql','{}',$3)", target, e, creator)
	exec("INSERT INTO monitoring_collections(id,enterprise_id,name,integration_key,target_type,target_id,collector_id,effective_revision,created_by) VALUES($1,$2,'Controlled receiver','mysql','monitoring_target',$3,$4,1,$5)", collection, e, target, collector, creator)
	for _, revision := range []int64{1, 2} {
		exec("UPDATE monitoring_collections SET desired_revision=$2 WHERE id=$1", collection, revision)
		if _, err := store.Queries.SnapshotMonitoringCollection(ctx, db.SnapshotMonitoringCollectionParams{EnterpriseID: e, CollectionID: collection, ActorID: creator}); err != nil {
			t.Fatal(err)
		}
		exec("INSERT INTO telemetry_sources(id,enterprise_id,collector_id,generation,resource_type,resource_id,source_key,source_type,signals,config_revision,config_hash,capability_version,collection_id,collection_revision,assignment_epoch) VALUES($1,$2,$3,$4,'monitoring_target',$5,'host/mysql','mysql',ARRAY['metrics'],$6,decode(repeat('00',32),'hex'),'v1',$7,$6,1)", collectionSource, e, collector, generation, target, revision, collection)
	}
	exec("INSERT INTO monitoring_collection_assignments(collection_id,enterprise_id,epoch,collector_id,activated_at) VALUES($1,$2,1,$3,now())", collection, e, collector)
	exec("UPDATE monitoring_collections SET effective_collector_id=$2,effective_assignment_epoch=1 WHERE id=$1", collection, collector)
	control := PostgresIngestControl{Queries: store.Queries}
	identity := TrustedIdentity{EnterpriseID: e, CollectorID: collector, ResourceID: host, ResourceType: "host"}
	resolve := func(id uuid.UUID, revision int64, want error) {
		t.Helper()
		_, err := control.ResolveSource(ctx, identity, id, revision, "metrics")
		if !errors.Is(err, want) {
			t.Fatalf("source %s:%d err=%v want=%v", id, revision, err, want)
		}
	}
	for _, id := range []uuid.UUID{source, collectionSource} {
		resolve(id, 1, nil)
		resolve(id, 2, ErrSourceAwaitingApply)
	}
	resolve(uuid.New(), 2, errIngestRejected)
	if _, err = control.ResolveSource(ctx, identity, source, 2, "logs"); !errors.Is(err, errIngestRejected) {
		t.Fatal("pending status admitted a different signal")
	}
	exec("UPDATE collector_config_revisions SET status='failed' WHERE collector_id=$1 AND revision=2", collector)
	for _, id := range []uuid.UUID{source, collectionSource} {
		resolve(id, 1, nil)
		resolve(id, 2, errIngestRejected)
	}
	exec("UPDATE collector_config_revisions SET status='applying' WHERE collector_id=$1 AND revision=2", collector)
	exec("UPDATE monitoring_collections SET desired_enabled=false WHERE id=$1", collection)
	if err := requireUnusedCollector(ctx, store.Queries, e, collector); !errors.Is(err, ErrCollectorDependenciesExist) {
		t.Fatal("uninstall ignored the still-effective collection while stop was pending", err)
	}
	resolve(collectionSource, 2, errIngestRejected)
	exec("UPDATE monitoring_collections SET desired_enabled=true,effective_revision=2 WHERE id=$1", collection)
	exec("UPDATE collector_instances SET effective_revision=2 WHERE id=$1", collector)
	exec("UPDATE collector_config_revisions SET status='effective' WHERE collector_id=$1 AND revision=2", collector)
	for _, id := range []uuid.UUID{source, collectionSource} {
		resolve(id, 2, nil)
		resolve(id, 1, errIngestRejected)
	}
	collectorRow, err := store.Queries.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: e, ID: collector})
	if err != nil {
		t.Fatal(err)
	}
	legacy := collectorActionPlan{ProfileKeys: []string{"host-basic@1", "collector-self@1"}}
	action := db.PendingAction{EnterpriseID: e, CreatorSubjectID: creator}
	for i := 0; i < 2; i++ {
		if err = store.InTx(ctx, func(q *db.Queries) error { return materializeBuiltinProfiles(ctx, q, action, collectorRow, legacy) }); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			exec("UPDATE monitoring_collections SET desired_enabled=false WHERE id=$1", builtincollection.StableID("host", host, "host_metrics"))
		}
	}
	var builtinRows int
	if err = store.Pool.QueryRow(ctx, "SELECT count(*) FROM monitoring_collections WHERE enterprise_id=$1 AND integration_key IN ('host_metrics','collector_metrics')", e).Scan(&builtinRows); err != nil || builtinRows != 2 {
		t.Fatal("legacy metrics did not become exactly two fixed instances")
	}
	own, err := store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: e, ID: builtincollection.StableID("host", host, "host_metrics")})
	if err != nil || own.DesiredEnabled || own.EffectiveRevision != 0 || own.LastSampleAt.Valid {
		t.Fatal("migration re-enabled an explicit off choice or fabricated runtime/sample state")
	}
	// A real driver failure is availability, never permanent identity rejection.
	store.Close()
	if _, err = control.ResolveSource(ctx, identity, source, 2, "metrics"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("control outage classified as rejection: %v", err)
	}
	if _, _, err = control.ResolveCollectorIdentity(ctx, "missing"); !errors.Is(err, ErrUnavailable) {
		t.Fatal("identity database outage classified as permanent authentication failure")
	}
}
