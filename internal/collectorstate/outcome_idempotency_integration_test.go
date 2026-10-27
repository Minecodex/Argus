package collectorstate

import (
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/testsupport/postgrestest"
)

func TestPostgresRepeatedCollectorOutcomeDoesNotInvalidateUnchangedIntent(t *testing.T) {
	store := postgrestest.New(t, "ARGUS_MONITORING_TEST_DATABASE_URL", "outcome_idempotency", filepath.Join("..", "..", "migrations", "postgresql"))
	ctx := t.Context()
	e, h, c, d := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO enterprises(id,name,code,timezone)VALUES($1,'Owned outcome replay',$2,'UTC')", e, e.String())
	exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,labels_hash)VALUES($1,$2,'Owned outcome host','127.0.0.1',22,'development','linux','managed_host','direct',decode(repeat('00',32),'hex'))", h, e)
	exec("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,artifact_manifest,catalog_revision)VALUES($1,$2,'fixture','fixture','fixture','supported','[]',1)", d, d.String())
	exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,desired_revision,effective_revision)VALUES($1,$2,'host',$3,$4,'linux_amd64','direct','installing',2,1)", c, e, h, d)
	apply := func(revision int64, status string, applied bool) int64 {
		t.Helper()
		n, err := store.Queries.ApplyCollectorRevisionOutcome(ctx, db.ApplyCollectorRevisionOutcomeParams{EnterpriseID: e, CollectorID: c, Revision: revision, Status: status, Applied: applied})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	read := func() db.CollectorInstance {
		t.Helper()
		row, err := store.Queries.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: e, ID: c})
		if err != nil {
			t.Fatal(err)
		}
		return row
	}
	before := read()
	if apply(2, "converged", true) != 1 {
		t.Fatal("first current result rejected")
	}
	first := read()
	if first.Version != before.Version+1 || first.EffectiveRevision != 2 || first.Status != "converged" {
		t.Fatal("first result did not change the actual projection")
	}
	if apply(2, "converged", true) != 1 {
		t.Fatal("current duplicate must remain recognizable")
	}
	duplicate := read()
	if duplicate.Version != first.Version || !duplicate.UpdatedAt.Time.Equal(first.UpdatedAt.Time) {
		t.Fatal("duplicate current result changed the version of an unchanged Collector intent")
	}
	if apply(1, "converged", true) != 0 || apply(2, "degraded", false) != 0 {
		t.Fatal("late/failed result overrode an effective revision")
	}
	final := read()
	if final.Version != first.Version || final.Status != "converged" || final.EffectiveRevision != 2 {
		t.Fatal("rejected result changed verified state")
	}
}
