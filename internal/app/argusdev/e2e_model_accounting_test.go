package argusdev

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestModelAccountingWaitsForBackgroundCompactionAndSettlement(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires a disposable PostgreSQL database")
	}
	connection, err := pgx.Connect(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(context.Background())
	// Connection-local tables isolate the accounting transitions from live or
	// other integration-test tasks, while exercising the exact production SQL.
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := connection.Exec(t.Context(), query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("CREATE TEMP TABLE model_calls(id uuid,run_id uuid,status text); CREATE TEMP TABLE runtime_tasks(run_id uuid,queue text,status text); CREATE TEMP TABLE model_quota_reservations(model_call_id uuid,status text)")
	run, other, call := uuid.New(), uuid.New(), uuid.New()
	check := func(want int) {
		t.Helper()
		var got int
		if err := connection.QueryRow(t.Context(), p5PendingModelAccountingSQL(run.String())).Scan(&got); err != nil || got != want {
			t.Fatalf("pending accounting=%d want=%d err=%v", got, want, err)
		}
	}
	exec("INSERT INTO runtime_tasks VALUES($1,'compaction','running'),($2,'agent','running')", other, run)
	check(0)
	exec("INSERT INTO runtime_tasks VALUES($1,'compaction','pending')", run)
	for _, status := range []string{"pending", "leased", "running"} {
		exec("UPDATE runtime_tasks SET status=$2 WHERE run_id=$1 AND queue='compaction'", run, status)
		check(1)
	}
	exec("INSERT INTO model_calls VALUES($1,$2,'reserved')", call, run)
	check(2)
	exec("UPDATE model_calls SET status='running' WHERE id=$1", call)
	check(2)
	exec("UPDATE model_calls SET status='succeeded' WHERE id=$1", call)
	exec("INSERT INTO model_quota_reservations VALUES($1,'active')", call)
	check(2)
	exec("UPDATE runtime_tasks SET status='succeeded' WHERE run_id=$1 AND queue='compaction'", run)
	check(1)
	exec("UPDATE model_quota_reservations SET status='settled'")
	check(0)
	for _, status := range []string{"failed", "cancelled", "timed_out"} {
		exec("UPDATE runtime_tasks SET status=$2 WHERE run_id=$1 AND queue='compaction'", run, status)
		check(0) // Settled is not passed; the strict usage gate rejects failures.
	}
}
