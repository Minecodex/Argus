package collectorstate

import (
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/testsupport/postgrestest"
)

// Controlled projection facts exercise real SQL boundaries. This fixture does
// not claim that an SSH tunnel or a native Collector was installed here.
func TestPostgresRouteRecoveryRequiresCurrentVerifiedConfigAndTunnelLease(t *testing.T) {
	store := postgrestest.New(t, "ARGUS_MONITORING_TEST_DATABASE_URL", "route_recovery", filepath.Join("..", "..", "migrations", "postgresql"))
	ctx := t.Context()
	e, h, c, d, s, credential, tunnel := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO enterprises(id,name,code,timezone)VALUES($1,'Owned route projection',$2,'UTC')", e, e.String())
	exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,labels_hash)VALUES($1,$2,'Owned route host','127.0.0.1',22,'development','linux','managed_host','direct',decode(repeat('00',32),'hex'))", h, e)
	exec("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,artifact_manifest,catalog_revision)VALUES($1,$2,'fixture','fixture','fixture','supported','[]',1)", d, d.String())
	exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,desired_revision,effective_revision)VALUES($1,$2,'host',$3,$4,'linux_amd64','direct','converged',2,2)", c, e, h, d)
	exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status)VALUES($1,$2,2,'{}','{}',decode(repeat('00',32),'hex'),'effective')", uuid.New(), c)
	exec("INSERT INTO secrets(id,enterprise_id,name,type,created_by)VALUES($1,$2,'Unused route metadata','ssh_password',$3)", s, e, uuid.New())
	exec("INSERT INTO credentials(id,enterprise_id,name,protocol,username,secret_id)VALUES($1,$2,'Unused route metadata','ssh','owned',$3)", credential, e, s)
	exec("INSERT INTO telemetry_routes(id,enterprise_id,collector_id,kind,transport,status,loopback_port)VALUES($1,$2,$3,'direct_argus','executor_tunnel','degraded',14318)", uuid.New(), e, c)
	exec("INSERT INTO telemetry_tunnels(id,enterprise_id,host_id,collector_id,credential_id,credential_version,target_address,target_port,target_username,pinned_host_key,initiator,transport,loopback_port,forward_target,status,epoch,fence,lease_owner,lease_expires_at)VALUES($1,$2,$3,$4,$5,1,'127.0.0.1',22,'owned','owned-key','direct_executor','executor_tunnel',14318,'owned-ingest:4317','established',3,4,'owned-recovery',now()+interval '90 seconds')", tunnel, e, h, c, credential)
	observed := db.TelemetryTunnel{ID: tunnel, EnterpriseID: e, CollectorID: c, Epoch: 3, Fence: 4, LeaseOwner: "owned-recovery"}
	apply := func(value db.TelemetryTunnel) {
		t.Helper()
		if err := store.InReadCommittedTx(ctx, func(q *db.Queries) error { return ActivateVerifiedTunnelRoute(ctx, q, value) }); err != nil {
			t.Fatal(err)
		}
	}
	assert := func(want string) {
		t.Helper()
		var status string
		if err := store.Pool.QueryRow(ctx, "SELECT status FROM telemetry_routes WHERE collector_id=$1", c).Scan(&status); err != nil || status != want {
			t.Fatal("route did not match verified current facts", status, want, err)
		}
	}
	for _, state := range []string{"installing", "result_unknown", "degraded", "uninstalled"} {
		exec("UPDATE collector_instances SET status=$2 WHERE id=$1", c, state)
		apply(observed)
		assert("degraded")
	}
	exec("UPDATE collector_instances SET status='converged',desired_revision=3 WHERE id=$1", c)
	apply(observed)
	assert("degraded")
	exec("UPDATE collector_instances SET desired_revision=2 WHERE id=$1", c)
	bad := observed
	bad.Fence++
	apply(bad)
	assert("degraded")
	bad = observed
	bad.Epoch++
	apply(bad)
	assert("degraded")
	bad = observed
	bad.LeaseOwner = "old-owner"
	apply(bad)
	assert("degraded")
	bad = observed
	bad.EnterpriseID = uuid.New()
	apply(bad)
	assert("degraded")
	exec("UPDATE telemetry_tunnels SET lease_expires_at=now()-interval '1 second' WHERE id=$1", tunnel)
	apply(observed)
	assert("degraded")
	exec("UPDATE telemetry_tunnels SET lease_expires_at=now()+interval '90 seconds' WHERE id=$1", tunnel)
	exec("UPDATE credentials SET status='disabled' WHERE id=$1", credential)
	apply(observed)
	assert("degraded")
	exec("UPDATE credentials SET status='active',version=2 WHERE id=$1", credential)
	apply(observed)
	assert("degraded")
	exec("UPDATE credentials SET version=1 WHERE id=$1", credential)
	exec("UPDATE secrets SET status='disabled' WHERE id=$1", s)
	apply(observed)
	assert("degraded")
	exec("UPDATE secrets SET status='active' WHERE id=$1", s)
	exec("UPDATE collector_config_revisions SET status='failed' WHERE collector_id=$1", c)
	apply(observed)
	assert("degraded")
	exec("UPDATE collector_config_revisions SET status='effective' WHERE collector_id=$1", c)
	apply(observed)
	assert("active")
	var before, after int64
	if err := store.Pool.QueryRow(ctx, "SELECT version FROM telemetry_routes WHERE collector_id=$1", c).Scan(&before); err != nil {
		t.Fatal(err)
	}
	apply(observed)
	if err := store.Pool.QueryRow(ctx, "SELECT version FROM telemetry_routes WHERE collector_id=$1", c).Scan(&after); err != nil || before != after {
		t.Fatal("steady heartbeat churned route versions", before, after, err)
	}
}
