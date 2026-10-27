package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Restore an actual pg_dump into a different owned database, rather than
// accepting a migration script or an empty-schema rollback as backup evidence.
func TestPlan8PostgresUpgradeInterruptedDDLAndBackupRestore(t *testing.T) {
	address := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL")
	container := os.Getenv("ARGUS_PLAN8_BACKUP_CONTAINER")
	if address == "" || container == "" {
		t.Skip("requires an explicitly owned disposable PostgreSQL container")
	}
	ctx := t.Context()
	var state []struct {
		Config          struct{ Labels map[string]string }
		NetworkSettings struct {
			Ports map[string][]struct{ HostIP, HostPort string }
		}
	}
	raw, err := exec.CommandContext(ctx, "docker", "inspect", container).Output()
	if err != nil || json.Unmarshal(raw, &state) != nil || len(state) != 1 || !strings.HasPrefix(state[0].Config.Labels["argus.owner"], "plan8-") {
		t.Fatal("backup container ownership was not verified")
	}
	base, err := url.Parse(address)
	if err != nil || base.User == nil || base.Hostname() != "127.0.0.1" {
		t.Fatal("backup requires an explicit loopback test URL")
	}
	ports := state[0].NetworkSettings.Ports["5432/tcp"]
	if len(ports) == 0 || ports[0].HostPort != base.Port() {
		t.Fatal("backup URL does not resolve to the owned container")
	}
	admin, err := sql.Open("pgx", address)
	if err != nil {
		t.Fatal("open owned PostgreSQL")
	}
	t.Cleanup(func() { _ = admin.Close() })
	owner := strings.ReplaceAll(uuid.NewString(), "-", "")
	source, restored := "p8_backup_"+owner, "p8_restore_"+owner
	for _, name := range []string{source, restored} {
		if _, err := admin.ExecContext(ctx, `CREATE DATABASE "`+name+`" TEMPLATE template0`); err != nil {
			t.Fatal(err)
		}
		name := name
		t.Cleanup(func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if _, err := admin.ExecContext(cleanup, `DROP DATABASE IF EXISTS "`+name+`" WITH (FORCE)`); err != nil {
				t.Error("owned backup database cleanup", err)
			}
		})
	}
	connect := func(name string) string { u := *base; u.Path = "/" + name; return u.String() }
	sourceDB, err := sql.Open("pgx", connect(source))
	if err != nil {
		t.Fatal("open migration source")
	}
	t.Cleanup(func() { _ = sourceDB.Close() })
	path := filepath.Join("..", "..", "..", "migrations", "postgresql")
	prior := t.TempDir()
	files, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		var version int
		if _, err := fmt.Sscanf(file.Name(), "%d_", &version); err != nil || version > 44 {
			continue
		}
		body, err := os.ReadFile(filepath.Join(path, file.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(prior, file.Name()), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := RunMigrations(ctx, connect(source), prior, MigrationUp); err != nil {
		t.Fatal(err)
	}
	canary := "protected-unrelated-data-" + uuid.NewString()
	if _, err := sourceDB.ExecContext(ctx, "CREATE TABLE plan8_preservation_proof(id integer PRIMARY KEY,value bytea NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	if _, err := sourceDB.ExecContext(ctx, "INSERT INTO plan8_preservation_proof VALUES(1,convert_to($1,'UTF8'))", canary); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "owned-baseline.dump")
	file, err := os.OpenFile(backup, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	dump := exec.CommandContext(ctx, "docker", "exec", container, "pg_dump", "-U", base.User.Username(), "-d", source, "-Fc", "--no-owner", "--no-acl")
	dump.Stdout = file
	err = dump.Run()
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatal("actual pg_dump failed")
	}
	body, err := os.ReadFile(backup)
	if err != nil || len(body) == 0 {
		t.Fatal("empty backup")
	}
	digest := sha256.Sum256(body)
	if err := RunMigrations(ctx, connect(source), path, MigrationUp); err != nil {
		t.Fatal(err)
	}
	// A deliberate failed migration must not persist its DDL or version row.
	interrupted := t.TempDir()
	for _, entry := range files {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		data, e := os.ReadFile(filepath.Join(path, entry.Name()))
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(interrupted, entry.Name()), data, 0600); e != nil {
			t.Fatal(e)
		}
	}
	if err := os.WriteFile(filepath.Join(interrupted, "99999_owned_interrupted.sql"), []byte("-- +goose Up\nCREATE TABLE plan8_interrupted_ddl(id integer);\nSELECT 1/0;\n-- +goose Down\nDROP TABLE plan8_interrupted_ddl;\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(ctx, connect(source), interrupted, MigrationUp); err == nil {
		t.Fatal("interrupted DDL was accepted")
	}
	var marker sql.NullString
	if err := sourceDB.QueryRowContext(ctx, "SELECT to_regclass('plan8_interrupted_ddl')::text").Scan(&marker); err != nil || marker.Valid {
		t.Fatal("failed DDL was not rolled back", err)
	}
	restore := exec.CommandContext(ctx, "docker", "exec", "-i", container, "pg_restore", "-U", base.User.Username(), "-d", restored, "--exit-on-error", "--no-owner", "--no-acl")
	restore.Stdin = bytes.NewReader(body)
	if err := restore.Run(); err != nil {
		t.Fatal("actual pg_restore failed")
	}
	if err := RunMigrations(ctx, connect(restored), path, MigrationUp); err != nil {
		t.Fatal(err)
	}
	copyDB, err := sql.Open("pgx", connect(restored))
	if err != nil {
		t.Fatal("open restored database")
	}
	defer copyDB.Close()
	for name, database := range map[string]*sql.DB{"upgraded": sourceDB, "restored": copyDB} {
		var value []byte
		if err := database.QueryRowContext(ctx, "SELECT value FROM plan8_preservation_proof WHERE id=1").Scan(&value); err != nil || string(value) != canary {
			t.Fatal(name+" lost unrelated bytes", err)
		}
		var version int64
		if err := database.QueryRowContext(ctx, "SELECT max(version_id) FROM goose_db_version WHERE is_applied").Scan(&version); err != nil || version < 48 {
			t.Fatal(name+" migration version invalid", err)
		}
	}
	t.Logf("actual PostgreSQL baseline44 upgrade, failed-DDL rollback and pg_dump/pg_restore into a separate owned database passed; backup_sha256=%s", hex.EncodeToString(digest[:]))
}
