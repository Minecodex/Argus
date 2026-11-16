package hostremoval

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Only an explicitly supplied disposable database is used. Temporary tables
// shadow the migrated schema so no application or fixture facts escape the test.
func TestBastionRemovalDependenciesIgnoreVerifiedRetiredTelemetry(t *testing.T) {
	url := os.Getenv("ARGUS_REMOVAL_DEPENDENCY_DATABASE_URL")
	if url == "" {
		t.Skip("set ARGUS_REMOVAL_DEPENDENCY_DATABASE_URL to a disposable database")
	}
	ctx := context.Background()
	root, _ := filepath.Abs("../..")
	if err := postgres.RunMigrations(ctx, url, filepath.Join(root, "migrations/postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"hosts", "host_onboarding_operations", "host_removal_operations", "collector_instances", "telemetry_routes", "remote_access_sessions", "connector_commands", "bastion_scopes", "credential_leases", "connector_control_tunnels", "telemetry_tunnels", "telemetry_collector_operations"} {
		exec("CREATE TEMP TABLE " + table + " (LIKE public." + table + " INCLUDING DEFAULTS)")
	}
	enterprise, scope, host, member, connector, rootCollector, leafCollector := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO bastion_scopes(id,enterprise_id,name,active_connector_id,environment,labels_hash,onboarding_mode) VALUES($1,$2,'Gateway',$3,'testing',decode(repeat('00',32),'hex'),'direct_install_tunnel')", scope, enterprise, connector)
	exec("INSERT INTO hosts(id,enterprise_id,name,platform,status,local_cleanup,bastion_scope_id,environment,labels_hash,role,control_path) VALUES($1,$2,'Retired member','linux','deleted','verified',$3,'testing',decode(repeat('00',32),'hex'),'managed_host','bastion_relay')", member, enterprise, scope)
	for _, item := range []struct {
		id, resource uuid.UUID
		role         string
	}{{rootCollector, host, "edge_gateway"}, {leafCollector, member, "leaf"}} {
		exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status) VALUES($1,$2,'host',$3,$4,'linux_amd64',$5,'uninstalled')", item.id, enterprise, item.resource, uuid.New(), item.role)
	}
	exec("INSERT INTO telemetry_routes(id,enterprise_id,collector_id,kind,status,transport) VALUES($1,$2,$3,'direct_argus','active','executor_tunnel')", uuid.New(), enterprise, rootCollector)
	tunnel := uuid.New()
	exec("INSERT INTO telemetry_tunnels(id,enterprise_id,host_id,collector_id,connector_id,credential_id,credential_version,target_address,target_port,target_username,pinned_host_key,initiator,transport,loopback_port,forward_target,status) VALUES($1,$2,$3,$4,$5,$6,1,'192.0.2.33',22,'root','SHA256:test','connector','bastion_tunnel',14317,'gateway','down')", tunnel, enterprise, member, leafCollector, connector, uuid.New())
	queries := db.New(conn)
	params := db.ListBastionRemovalDependenciesParams{EnterpriseID: enterprise, BastionScopeID: uuid.NullUUID{UUID: scope, Valid: true}, ResourceID: host}
	check := func(want ...string) {
		t.Helper()
		rows, err := queries.ListBastionRemovalDependencies(ctx, params)
		if err != nil {
			t.Fatal(err)
		}
		actual := map[string]bool{}
		for _, row := range rows {
			actual[row.DependencyType] = true
		}
		if len(actual) != len(want) {
			t.Fatalf("dependency kinds: got %v, want %v", actual, want)
		}
		for _, kind := range want {
			if !actual[kind] {
				t.Fatalf("missing dependency %s: %v", kind, actual)
			}
		}
	}
	check()
	exec("UPDATE collector_instances SET status='converged' WHERE id=$1", rootCollector)
	check("collector", "telemetry_route")
	exec("UPDATE collector_instances SET status='uninstalled' WHERE id=$1", rootCollector)
	exec("UPDATE hosts SET local_cleanup='unknown' WHERE id=$1", member)
	check("telemetry_tunnel")
	exec("UPDATE hosts SET local_cleanup='verified' WHERE id=$1", member)
	exec("UPDATE telemetry_tunnels SET status='established' WHERE id=$1", tunnel)
	check("telemetry_tunnel")
	exec("UPDATE telemetry_tunnels SET status='down' WHERE id=$1", tunnel)
	exec("UPDATE hosts SET status='active' WHERE id=$1", member)
	check("member_host", "telemetry_tunnel")
	exec("DELETE FROM hosts WHERE id=$1", member)
	check("telemetry_tunnel")
	params.EnterpriseID = uuid.New()
	check()
}
