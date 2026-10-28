package httpapi

import (
	"net/url"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/telemetry"
)

func modelGovernanceRoleStores(t *testing.T, f *auditWorkflowFixture, admin driver.Conn, address, password string) (*postgres.Store, *postgres.Store, driver.Conn, driver.Conn) {
	t.Helper()
	ctx := t.Context()
	for _, role := range []string{"argus_telemetry_query", "argus_telemetry_writer"} {
		f.exec(`DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='` + role + `') THEN CREATE ROLE ` + role + ` LOGIN PASSWORD 'modelgov-test-only'; END IF; END $$`)
	}
	connection := f.store.Pool.Config().ConnString()
	if _, err := postgres.ReconcileRuntimeRolePermissions(ctx, connection); err != nil {
		t.Fatal(err)
	}
	open := func(role string) *postgres.Store {
		u, err := url.Parse(connection)
		if err != nil {
			t.Fatal(err)
		}
		u.User = url.UserPassword(role, "modelgov-test-only")
		store, err := postgres.Open(ctx, u.String())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(store.Close)
		return store
	}
	read, write := open("argus_telemetry_query"), open("argus_telemetry_writer")
	if _, err := read.Pool.Exec(ctx, `SELECT ciphertext FROM ai_model_credentials LIMIT 1`); err == nil {
		t.Fatal("query role can read model credentials")
	}
	if _, err := write.Pool.Exec(ctx, `SELECT * FROM model_calls LIMIT 1`); err == nil {
		t.Fatal("projector role can read original model calls")
	}
	for _, entry := range []struct{ user, grant string }{{"modelgov_read", "SELECT"}, {"modelgov_write", "INSERT"}} {
		if err := admin.Exec(ctx, `CREATE USER IF NOT EXISTS `+entry.user+` IDENTIFIED WITH sha256_password BY 'modelgov-test-only'`); err != nil {
			t.Fatal(err)
		}
		if err := admin.Exec(ctx, `GRANT `+entry.grant+` ON argus_telemetry.* TO `+entry.user); err != nil {
			t.Fatal(err)
		}
	}
	openCH := func(user string) driver.Conn {
		conn, err := telemetry.OpenClickHouse(address, "argus_telemetry", user, password)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		return conn
	}
	return read, write, openCH("modelgov_read"), openCH("modelgov_write")
}
