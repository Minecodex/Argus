package telemetry

import (
	"crypto/sha256"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// A real owned schema keeps both the production upper-bound constraint and
// the application service entry point. No wall clock or public table changes.
func TestPostgresTelemetryEnrollmentDeadlineUsesOneDatabaseClock(t *testing.T) {
	address := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("owned PostgreSQL required")
	}
	pool, err := pgxpool.New(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	schema := "plan8_token_clock_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = pool.Exec(t.Context(), "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := pool.Exec(t.Context(), "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	if _, err = pool.Exec(t.Context(), "CREATE TABLE "+quoted+".telemetry_enrollment_tokens (LIKE public.telemetry_enrollment_tokens INCLUDING ALL)"); err != nil {
		t.Fatal(err)
	}
	connection, err := pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Release()
	if _, err = connection.Exec(t.Context(), "SET search_path TO "+quoted+", public"); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"statement-default", "longer-transaction-default"} {
		t.Run(kind, func(t *testing.T) {
			def := "statement_timestamp()"
			if kind == "longer-transaction-default" {
				def = "now()"
			}
			if _, err := connection.Exec(t.Context(), "ALTER TABLE telemetry_enrollment_tokens ALTER COLUMN created_at SET DEFAULT "+def); err != nil {
				t.Fatal(err)
			}
			tx, err := connection.Begin(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(t.Context())
			if _, err = tx.Exec(t.Context(), "SELECT pg_sleep(0.15)"); err != nil {
				t.Fatal(err)
			}
			token, err := (IdentityService{}).CreateEnrollmentToken(t.Context(), db.New(tx), uuid.New())
			if err != nil {
				t.Fatalf("ordinary service could not mint its bounded token after transaction work: %v", err)
			}
			digest := sha256.Sum256([]byte(token))
			var seconds float64
			var created, expires time.Time
			if err = tx.QueryRow(t.Context(), "SELECT EXTRACT(EPOCH FROM expires_at-created_at)::float8,created_at,expires_at FROM telemetry_enrollment_tokens WHERE token_hash=$1", digest[:]).Scan(&seconds, &created, &expires); err != nil {
				t.Fatal(err)
			}
			if seconds != 600 || expires.Sub(created) != 10*time.Minute {
				t.Fatalf("ten-minute token was measured across different clocks: lifetime_seconds=%f", seconds)
			}
		})
	}
	t.Run("existing-database-upper-bound-retained", func(t *testing.T) {
		digest := sha256.Sum256([]byte(uuid.NewString()))
		_, err := connection.Exec(t.Context(), "INSERT INTO telemetry_enrollment_tokens(id,collector_id,token_hash,created_at,expires_at) VALUES($1,$2,$3,statement_timestamp(),statement_timestamp()+interval '10 minutes 1 microsecond')", uuid.New(), uuid.New(), digest[:])
		var rejected *pgconn.PgError
		if !errors.As(err, &rejected) || rejected.Code != "23514" {
			t.Fatal("database accepted a token longer than the existing ten-minute limit")
		}
	})
}
