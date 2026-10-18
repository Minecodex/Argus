package runtime

import (
	"context"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestProcessorPoolOverlapsRemoteTasksAndKeepsControlAvailable(t *testing.T) {
	address := os.Getenv("ARGUS_P7_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires a disposable migrated database")
	}
	admin, err := pgx.Connect(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	schema := "p7_pool_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(t.Context(), "CREATE SCHEMA "+identifier); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	if _, err := admin.Exec(t.Context(), "CREATE TABLE "+identifier+".runtime_tasks (LIKE public.runtime_tasks INCLUDING ALL)"); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(address)
	if err != nil {
		t.Fatal(err)
	}
	parameters := parsed.Query()
	parameters.Set("search_path", schema)
	parsed.RawQuery = parameters.Encode()
	store, err := postgres.Open(t.Context(), parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	enterprise := uuid.New()
	add := func(queue string) uuid.UUID {
		t.Helper()
		id := uuid.New()
		_, err := store.Queries.CreateRuntimeTask(ctx, db.CreateRuntimeTaskParams{ID: id, EnterpriseID: uuid.NullUUID{UUID: enterprise, Valid: true}, Queue: queue, RunID: uuid.NullUUID{UUID: uuid.New(), Valid: true}, Payload: []byte(`{}`), MaxAttempts: 1, AvailableAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	for range 3 {
		add("remote_execution")
	}
	started := make(chan Task, 3)
	release := make(chan struct{})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	done := make(chan error, 2)
	go func() {
		done <- (Processor{Store: store, Queue: "remote_execution", Owner: "pool-test", Concurrency: 2, Poll: 10 * time.Millisecond, Logger: logger, Handle: HandlerFunc(func(ctx context.Context, task Task) error {
			started <- task
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})}).Run(ctx)
	}()
	consumers := 1
	defer func() {
		cancel()
		for range consumers {
			<-done
		}
	}()
	owners := map[string]bool{}
	for range 2 {
		select {
		case task := <-started:
			owners[task.LeaseOwner.String] = true
		case <-ctx.Done():
			t.Fatal("independent remote tasks were serialized")
		}
	}
	if len(owners) != 2 {
		t.Fatal("parallel consumers did not retain independent lease owners")
	}
	var pending int
	if err := store.Pool.QueryRow(ctx, "SELECT count(*) FROM runtime_tasks WHERE status='pending' AND queue='remote_execution'").Scan(&pending); err != nil || pending != 1 {
		t.Fatalf("execution capacity was not bounded: %d %v", pending, err)
	}
	control := add("remote_execution_control")
	controlled := make(chan uuid.UUID, 1)
	consumers++
	go func() {
		done <- (Processor{Store: store, Queue: "remote_execution_control", Owner: "control-test", Poll: 10 * time.Millisecond, Logger: logger, Handle: HandlerFunc(func(_ context.Context, task Task) error { controlled <- task.ID; return nil })}).Run(ctx)
	}()
	select {
	case id := <-controlled:
		if id != control {
			t.Fatal("wrong control task")
		}
	case <-ctx.Done():
		t.Fatal("saturated execution workers starved control")
	}
	close(release)
	for {
		var succeeded int
		if err := store.Pool.QueryRow(ctx, "SELECT count(*) FROM runtime_tasks WHERE status='succeeded'").Scan(&succeeded); err != nil {
			t.Fatal(err)
		}
		if succeeded == 4 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("pool did not finish all tasks")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
