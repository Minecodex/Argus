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
)

func TestPostgresTelemetryEnrollmentConsumptionUsesCurrentDatabaseTime(t *testing.T) {
	address := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("owned PostgreSQL required")
	}
	pool, err := pgxpool.New(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	schema := "plan8_token_consume_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	if _, err = connection.Exec(t.Context(), "SET search_path TO "+quoted+",public"); err != nil {
		t.Fatal(err)
	}
	create := func(t *testing.T, seconds float64, consumed bool) (uuid.UUID, []byte) {
		t.Helper()
		id := uuid.New()
		digest := sha256.Sum256([]byte(uuid.NewString()))
		if _, err := connection.Exec(t.Context(), "INSERT INTO telemetry_enrollment_tokens(id,collector_id,token_hash,created_at,expires_at,consumed_at) VALUES($1,$2,$3,statement_timestamp()-interval '1 second',statement_timestamp()+($4::float8*interval '1 second'),CASE WHEN $5::boolean THEN statement_timestamp() ELSE NULL END)", id, uuid.New(), digest[:], seconds, consumed); err != nil {
			t.Fatal(err)
		}
		return id, digest[:]
	}
	t.Run("expired-and-consumed-records-not-admitted", func(t *testing.T) {
		for _, consumed := range []bool{false, true} {
			seconds := float64(-0.1)
			if consumed {
				seconds = 30
			}
			_, hash := create(t, seconds, consumed)
			if _, err := db.New(connection).GetTelemetryEnrollmentTokenForUpdate(t.Context(), hash); !errors.Is(err, pgx.ErrNoRows) {
				t.Fatal("expired or consumed token passed the database admission gate")
			}
		}
	})
	t.Run("expiry-during-transaction-cannot-use-transaction-start-time", func(t *testing.T) {
		id, hash := create(t, 1, false)
		tx, err := connection.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(t.Context())
		q := db.New(tx)
		if _, err = q.GetTelemetryEnrollmentTokenForUpdate(t.Context(), hash); err != nil {
			t.Fatal("fresh token was not admitted", err)
		}
		if _, err = tx.Exec(t.Context(), "SELECT pg_sleep(1.1)"); err != nil {
			t.Fatal(err)
		}
		if _, err = q.ConsumeTelemetryEnrollmentToken(t.Context(), id); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal("token was consumed after expiry using the earlier transaction timestamp")
		}
	})
	t.Run("successful-use-is-one-time", func(t *testing.T) {
		id, hash := create(t, 30, false)
		q := db.New(connection)
		if _, err = q.GetTelemetryEnrollmentTokenForUpdate(t.Context(), hash); err != nil {
			t.Fatal(err)
		}
		if _, err = q.ConsumeTelemetryEnrollmentToken(t.Context(), id); err != nil {
			t.Fatal(err)
		}
		if _, err = q.ConsumeTelemetryEnrollmentToken(t.Context(), id); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal("duplicate consumption was accepted")
		}
		if _, err = q.GetTelemetryEnrollmentTokenForUpdate(t.Context(), hash); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal("used token was admitted again")
		}
	})
}
