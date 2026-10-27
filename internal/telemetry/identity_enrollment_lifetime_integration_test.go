package telemetry

import (
	"crypto/sha256"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPostgresTelemetryEnrollmentLifetimeBoundsAndRollback(t *testing.T) {
	address := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("owned PostgreSQL required")
	}
	pool, err := pgxpool.New(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	schema := "plan8_token_bounds_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	q := db.New(connection)
	for _, seconds := range []int32{-1, 0, 601, 2147483647} {
		digest := sha256.Sum256([]byte(uuid.NewString()))
		_, err := q.CreateTelemetryEnrollmentToken(t.Context(), db.CreateTelemetryEnrollmentTokenParams{ID: uuid.New(), CollectorID: uuid.New(), TokenHash: digest[:], LifetimeSeconds: seconds})
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("invalid lifetime %d was accepted or broke the transaction: %v", seconds, err)
		}
	}
	for _, seconds := range []int32{1, 300, 600} {
		digest := sha256.Sum256([]byte(uuid.NewString()))
		record, err := q.CreateTelemetryEnrollmentToken(t.Context(), db.CreateTelemetryEnrollmentTokenParams{ID: uuid.New(), CollectorID: uuid.New(), TokenHash: digest[:], LifetimeSeconds: seconds})
		if err != nil || record.ExpiresAt.Time.Sub(record.CreatedAt.Time) != time.Duration(seconds)*time.Second {
			t.Fatalf("bounded database lifetime %d failed: %v", seconds, err)
		}
	}
	tx, err := connection.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(uuid.NewString()))
	_, err = db.New(tx).CreateTelemetryEnrollmentToken(t.Context(), db.CreateTelemetryEnrollmentTokenParams{ID: uuid.New(), CollectorID: uuid.New(), TokenHash: digest[:], LifetimeSeconds: 600})
	if err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if err = tx.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err = connection.QueryRow(t.Context(), "SELECT count(*) FROM telemetry_enrollment_tokens WHERE token_hash=$1", digest[:]).Scan(&rows); err != nil || rows != 0 {
		t.Fatal("rolled-back issuance left a token")
	}
}
