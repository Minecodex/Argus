package authorization

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/testsupport/postgrestest"
)

// Independent subjects intentionally share one tenant audit chain. A small
// fixture-only delay overlaps serializable snapshots without changing grants,
// permissions or application responses. No failed operation is retried by the
// test: all six API-domain calls must finish once with the original version.
func TestPostgresConcurrentGrantChangesKeepVersionsAndAuditExactlyOnce(t *testing.T) {
	store := postgrestest.New(t, "ARGUS_MONITORING_TEST_DATABASE_URL", "auth_grants", filepath.Join("..", "..", "migrations", "postgresql"))
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	exec := func(query string, values ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, query, values...); err != nil {
			t.Fatal(err)
		}
	}
	enterprise, department, admin, host := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,timezone) VALUES($1,'Concurrent grants','UTC')", enterprise)
	exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Concurrent readers')", department, enterprise)
	users := []uuid.UUID{admin}
	for i := 0; i < 6; i++ {
		users = append(users, uuid.New())
	}
	for _, user := range users {
		exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Grant reader')", user, enterprise, department, user.String())
	}
	exec("INSERT INTO hosts(id,enterprise_id,name,address,port,platform,role,control_path,environment,labels_hash) VALUES($1,$2,'Concurrent Host','127.0.0.1',22,'linux','managed_host','direct','development',decode(repeat('00',32),'hex'))", host, enterprise)
	if err := audit.InitializeChain(ctx, store.Queries, "enterprise", uuid.NullUUID{UUID: enterprise, Valid: true}); err != nil {
		t.Fatal(err)
	}
	// This is a disposable database created by postgrestest.New. The trigger
	// only widens the overlap; PostgreSQL itself produces any SQLSTATE error.
	exec("CREATE FUNCTION grant_fixture_overlap() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(0.05); RETURN NEW; END $$")
	exec("CREATE TRIGGER grant_fixture_overlap BEFORE INSERT OR UPDATE ON data_authorization_grants FOR EACH ROW EXECUTE FUNCTION grant_fixture_overlap()")
	service := Service{Store: store}
	for _, remove := range []bool{false, true} {
		barrier := make(chan struct{})
		results := make(chan error, len(users)-1)
		version := int64(1)
		if remove {
			version = 2
		}
		for _, user := range users[1:] {
			go func(user uuid.UUID) {
				<-barrier
				results <- service.UpdateGrantBatch(ctx, admin.String(), enterprise, GrantBatchInput{SubjectType: "user", SubjectID: user, ResourceType: "host", ResourceIDs: []uuid.UUID{host}, ExpectedVersion: version, Remove: remove})
			}(user)
		}
		close(barrier)
		failures := 0
		for range users[1:] {
			if err := <-results; err != nil {
				failures++
				var pg *pgconn.PgError
				if errors.As(err, &pg) {
					t.Logf("Concurrent remove=%t PostgreSQL error SQLSTATE=%s: %s", remove, pg.Code, pg.Message)
				} else {
					t.Logf("Concurrent remove=%t domain error: %v", remove, err)
				}
			}
		}
		if failures != 0 {
			t.Fatalf("%d concurrent grant changes failed; no test-side retries", failures)
		}
		for _, user := range users[1:] {
			current, err := service.CurrentAuthorizationVersion(ctx, enterprise, user, "user")
			if err != nil || current != version+1 {
				t.Fatal("expected version was lost or advanced more than once", current, err)
			}
		}
		var events int
		if err := store.Pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE enterprise_id=$1 AND action='data_authorization.update'", enterprise).Scan(&events); err != nil {
			t.Fatal(err)
		}
		want := 6
		if remove {
			want = 12
		}
		if events != want {
			t.Fatal("aborted grant attempt produced duplicate audit records", events, want)
		}
	}
	// A stale user-visible version remains a real conflict, never silently
	// refreshed by the server's internal database transaction handling.
	if err := service.UpdateGrantBatch(ctx, admin.String(), enterprise, GrantBatchInput{SubjectType: "user", SubjectID: users[1], ResourceType: "host", ResourceIDs: []uuid.UUID{host}, ExpectedVersion: 1}); !errors.Is(err, ErrAuthorizationConflict) {
		t.Fatal("stale caller version was replaced automatically", err)
	}
	var active int
	if err := store.Pool.QueryRow(ctx, "SELECT count(*) FROM data_authorization_grants WHERE enterprise_id=$1 AND status='active'", enterprise).Scan(&active); err != nil || active != 0 {
		t.Fatal("failed/stale mutation left active grants", active, err)
	}
	// Two different changes with the same subject/version are not merged by
	// retry: one commits and the other must return the original stale conflict.
	otherHost := uuid.New()
	exec("INSERT INTO hosts(id,enterprise_id,name,address,port,platform,role,control_path,environment,labels_hash) VALUES($1,$2,'Other concurrent Host','127.0.0.1',22,'linux','managed_host','direct','development',decode(repeat('00',32),'hex'))", otherHost, enterprise)
	type competingResult struct {
		resource uuid.UUID
		err      error
	}
	start := make(chan struct{})
	competing := make(chan competingResult, 2)
	for _, resource := range []uuid.UUID{host, otherHost} {
		go func(resource uuid.UUID) {
			<-start
			competing <- competingResult{resource, service.UpdateGrantBatch(ctx, admin.String(), enterprise, GrantBatchInput{SubjectType: "user", SubjectID: users[1], ResourceType: "host", ResourceIDs: []uuid.UUID{resource}, ExpectedVersion: 3})}
		}(resource)
	}
	close(start)
	winners, conflicts := 0, 0
	var winner uuid.UUID
	for range 2 {
		result := <-competing
		if result.err == nil {
			winners++
			winner = result.resource
		} else if errors.Is(result.err, ErrAuthorizationConflict) {
			conflicts++
		} else {
			t.Fatal("competing subject failed without its version conflict", result.err)
		}
	}
	var committed uuid.UUID
	if winners != 1 || conflicts != 1 {
		t.Fatal("same-subject caller versions were merged or replayed", winners, conflicts)
	}
	if err := store.Pool.QueryRow(ctx, "SELECT resource_id FROM data_authorization_grants WHERE enterprise_id=$1 AND subject_id=$2 AND status='active'", enterprise, users[1]).Scan(&committed); err != nil || committed != winner {
		t.Fatal("aborted competing change broadened resource grants", committed, winner, err)
	}
	current, err := service.CurrentAuthorizationVersion(ctx, enterprise, users[1], "user")
	if err != nil || current != 4 {
		t.Fatal("competing failure advanced authorization version", current, err)
	}
	var events int
	if err := store.Pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE enterprise_id=$1 AND action='data_authorization.update'", enterprise).Scan(&events); err != nil || events != 13 {
		t.Fatal("competing aborted transaction duplicated audit effects", events, err)
	}
	t.Log(fmt.Sprintf("12 concurrent grant/revoke operations and one competing winner committed once; 13 audit records, one preserved stale conflict"))
}
