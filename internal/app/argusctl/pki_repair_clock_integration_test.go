package argusctl

import (
	"crypto/sha256"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"os"
	"strings"
	"testing"
	"time"
)

// Only owned schema fixtures are changed. This calls the actual repair token
// transaction and proves both its returned deadline and failed-issue rollback.
func TestCollectorPKIRepairUsesDatabaseDeadlineAndRollsBackRevocation(t *testing.T) {
	address := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("owned PostgreSQL required")
	}
	owner, err := pgxpool.New(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	schema := "plan8_repair_clock_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = owner.Exec(t.Context(), "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := owner.Exec(t.Context(), "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	for _, name := range []string{"collector_instances", "telemetry_enrollment_tokens", "telemetry_certificates", "pki_certificate_identities"} {
		if _, err = owner.Exec(t.Context(), "CREATE TABLE "+quoted+"."+name+" (LIKE public."+name+" INCLUDING ALL)"); err != nil {
			t.Fatal(err)
		}
	}
	config, err := pgxpool.ParseConfig(address)
	if err != nil {
		t.Fatal("owned database configuration unavailable")
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := &postgres.Store{Pool: pool, Queries: db.New(pool)}
	session := &pkiSession{store: store}
	create := func() uuid.UUID {
		t.Helper()
		id := uuid.New()
		if _, err := pool.Exec(t.Context(), "INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,source_generation,desired_revision,effective_revision) VALUES($1,$2,'host',$3,$4,'linux_amd64','direct','converged',$5,1,1)", id, uuid.New(), uuid.New(), uuid.New(), uuid.New()); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(t.Context(), "INSERT INTO telemetry_certificates(id,collector_id,serial_number,uri_san,csr_hash,certificate_hash,certificate_request_name,issuer_generation,not_before,not_after,certificate_usage) VALUES($1,$2,$3,$4,decode(repeat('00',32),'hex'),decode(repeat('01',32),'hex'),'owned-repair-clock',1,statement_timestamp()-interval '1 minute',statement_timestamp()+interval '1 hour','clientAuth')", uuid.New(), id, strings.ReplaceAll(uuid.NewString(), "-", ""), "spiffe://argus/telemetry/collectors/"+id.String()); err != nil {
			t.Fatal(err)
		}
		return id
	}
	t.Run("successful-repair-returns-stored-deadline", func(t *testing.T) {
		id := create()
		row, token, expires, err := issueCollectorRepairToken(t.Context(), session, id, "host")
		if err != nil {
			t.Fatal(err)
		}
		if row.ID != id || token == "" {
			t.Fatal("repair identity missing")
		}
		digest := sha256.Sum256([]byte(token))
		var stored, created time.Time
		if err = pool.QueryRow(t.Context(), "SELECT expires_at,created_at FROM telemetry_enrollment_tokens WHERE token_hash=$1 AND collector_id=$2", digest[:], id).Scan(&stored, &created); err != nil {
			t.Fatal(err)
		}
		if !stored.Equal(expires) || stored.Sub(created) != 10*time.Minute {
			t.Fatal("repair advertised a deadline different from its authoritative bounded database record")
		}
		var revoked bool
		if err = pool.QueryRow(t.Context(), "SELECT revoked_at IS NOT NULL AND revoke_reason='pki_repair' FROM telemetry_certificates WHERE collector_id=$1", id).Scan(&revoked); err != nil || !revoked {
			t.Fatal("successful repair did not revoke the preceding certificate")
		}
	})
	t.Run("failed-issuance-rolls-back-certificate-revocation", func(t *testing.T) {
		id := create()
		if _, err := pool.Exec(t.Context(), "ALTER TABLE telemetry_enrollment_tokens ADD CONSTRAINT owned_repair_issue_denied CHECK (collector_id <> '"+id.String()+"'::uuid)"); err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := issueCollectorRepairToken(t.Context(), session, id, "host"); err == nil {
			t.Fatal("forced owned issuance rejection was ignored")
		}
		var revoked bool
		var tokens int
		if err := pool.QueryRow(t.Context(), "SELECT revoked_at IS NOT NULL FROM telemetry_certificates WHERE collector_id=$1", id).Scan(&revoked); err != nil || revoked {
			t.Fatal("failed issuance retained a certificate revocation")
		}
		if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM telemetry_enrollment_tokens WHERE collector_id=$1", id).Scan(&tokens); err != nil || tokens != 0 {
			t.Fatal("failed issuance retained a token")
		}
	})
}
