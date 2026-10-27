// Package postgrestest owns disposable databases for tests of global workers.
package postgrestest

import (
	"context"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/storage/postgres"
)

func New(t *testing.T, environment, purpose, migrations string) *postgres.Store {
	t.Helper()
	address := os.Getenv(environment)
	if address == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	if !regexp.MustCompile(`^[a-z][a-z0-9_]{2,20}$`).MatchString(purpose) {
		t.Fatal("invalid isolated database purpose")
	}
	destination, err := url.Parse(address)
	if err != nil || (destination.Scheme != "postgres" && destination.Scheme != "postgresql") {
		t.Fatal("isolated tests require a PostgreSQL URL")
	}
	admin, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	id := uuid.NewString()
	name := "p8_" + purpose + "_" + strings.ReplaceAll(id, "-", "")
	owner := "argus.test/" + purpose + "/" + id
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Pool.Exec(t.Context(), "CREATE DATABASE "+quoted); err != nil {
		t.Fatal(err)
	}
	var oid, role uint32
	stamped := false
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if oid == 0 {
			t.Error("isolated database identity was not verified; cleanup withheld", name)
			return
		}
		var currentOID, currentRole uint32
		var comment string
		if err := admin.Pool.QueryRow(cleanup, "SELECT oid,datdba,COALESCE(shobj_description(oid,'pg_database'),'') FROM pg_database WHERE datname=$1", name).Scan(&currentOID, &currentRole, &comment); err != nil || currentOID != oid || currentRole != role || stamped && comment != owner {
			t.Error("isolated database ownership changed", name, err)
			return
		}
		if _, err := admin.Pool.Exec(cleanup, "DROP DATABASE "+quoted); err != nil {
			t.Error("isolated database cleanup", name, err)
		} else {
			t.Log("owned isolated database removed", name)
		}
	})
	if err = admin.Pool.QueryRow(t.Context(), "SELECT oid,datdba FROM pg_database WHERE datname=$1", name).Scan(&oid, &role); err != nil {
		t.Fatal("isolated database identity lookup", err)
	}
	if _, err = admin.Pool.Exec(t.Context(), "COMMENT ON DATABASE "+quoted+" IS '"+owner+"'"); err != nil {
		t.Fatal(err)
	}
	stamped = true
	destination.Path = "/" + name
	store, err := postgres.Open(t.Context(), destination.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	var actual string
	if err = store.Pool.QueryRow(t.Context(), "SELECT current_database()").Scan(&actual); err != nil || actual != name {
		t.Fatal("refusing to write to a different test database", err)
	}
	if err = postgres.RunMigrations(t.Context(), destination.String(), migrations, postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	return store
}
