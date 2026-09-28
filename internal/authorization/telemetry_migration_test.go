package authorization

import (
	"context"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPostgresRetiredTelemetryPrivilegesInvalidateWithoutGrantExpansion(t *testing.T) {
	address := os.Getenv("ARGUS_DASHBOARD_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	if err := postgres.RunMigrations(t.Context(), address, filepath.Join("..", "..", "migrations", "postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	tx, err := store.Pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(t.Context(), query, args...); err != nil {
			t.Fatal(err)
		}
	}
	e, d, user, account, role := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Permission migration',$2,'UTC')", e, e.String())
	exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Migration')", d, e)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Legacy reader')", user, e, d, user.String())
	exec("INSERT INTO service_accounts(id,enterprise_id,name) VALUES($1,$2,'Legacy reader')", account, e)
	exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Legacy signal reader')", role, e)
	exec("INSERT INTO permissions(id,description,registry_version) VALUES('telemetry.query.metrics','retired fixture',10)")
	exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'telemetry.query.metrics')", role)
	for _, binding := range []struct {
		kind string
		id   uuid.UUID
	}{{"department", d}, {"service_account", account}} {
		exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,$3,$4,$5)", uuid.New(), e, binding.kind, binding.id, role)
	}
	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "postgresql", "00009_telemetry_object_permissions.sql"))
	if err != nil {
		t.Fatal(err)
	}
	exec(strings.Split(string(migration), "-- +goose Down")[0])
	var userVersion, accountVersion, roleVersion int64
	if err = tx.QueryRow(t.Context(), "SELECT authorization_version FROM enterprise_users WHERE id=$1", user).Scan(&userVersion); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(t.Context(), "SELECT authorization_version FROM service_accounts WHERE id=$1", account).Scan(&accountVersion); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(t.Context(), "SELECT version FROM roles WHERE id=$1", role).Scan(&roleVersion); err != nil {
		t.Fatal(err)
	}
	if userVersion != 2 || accountVersion != 2 || roleVersion != 2 {
		t.Fatalf("authorization or role baseline was not invalidated: %d %d %d", userVersion, accountVersion, roleVersion)
	}
	var remaining int
	if err = tx.QueryRow(t.Context(), "SELECT count(*) FROM role_permissions WHERE role_id=$1", role).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatal("signal privilege was retained or expanded to object access")
	}
}
