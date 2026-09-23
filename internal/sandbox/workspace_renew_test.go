package sandbox

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestWorkspaceRenewalReservesBeforeDispatchAndRotatesAtMonthBoundary(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires a disposable migrated database")
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var dispatched atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { dispatched.Add(1); _, _ = fmt.Fprint(w, `{}`) }))
	defer upstream.Close()
	service := Service{Store: store}
	backend, err := service.CreateBackend(t.Context(), BackendInput{Name: uuid.NewString(), Endpoint: upstream.URL, Status: "enabled"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Queries.SetSandboxBackendHealth(t.Context(), db.SetSandboxBackendHealthParams{ID: backend.ID, HealthStatus: "healthy"}); err != nil {
		t.Fatal(err)
	}
	image, err := service.CreateImage(t.Context(), ImageInput{BackendID: backend.ID, Name: uuid.NewString(), ImageRef: "argus/offline:fixed", Digest: "sha256:" + strings.Repeat("1", 64), Status: "enabled"})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := service.CreateProfile(t.Context(), ProfileInput{Name: uuid.NewString(), BackendID: backend.ID, ImageID: image.ID, TaskKinds: []string{"agent_workspace"}, CPUMillis: 500, MemoryMiB: 512, TimeoutSeconds: 900, NetworkMode: "none", Status: "enabled"})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := service.WorkspaceRuntimeByID(t.Context(), profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	e, session := uuid.New(), uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(t.Context(), sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Renewal quota',$2,'UTC')", e, "p5-"+e.String())
	quota, err := service.UpdateQuota(t.Context(), e, 1, 60, 0)
	if err != nil {
		t.Fatal(err)
	}
	exec("INSERT INTO runtime_tasks(id,enterprise_id,queue) VALUES($1,$2,'sandbox')", session, e)
	exec("INSERT INTO sandbox_sessions(id,task_id,enterprise_id,profile_id,profile_revision,upstream_session_id,status,expires_at,started_at) VALUES($1,$1,$2,$3,$4,$5,'running',now()+interval '30 seconds',now())", session, e, profile.ID, profile.Revision, session.String())
	if err := service.RenewWorkspaceSession(t.Context(), session, runtime, 120); !errors.Is(err, ErrQuotaExceeded) || dispatched.Load() != 0 {
		t.Fatalf("over-quota extension dispatched: %v / %d", err, dispatched.Load())
	}
	if _, err := service.UpdateQuota(t.Context(), e, 1, 500, quota.Version); err != nil {
		t.Fatal(err)
	}
	if err := service.RenewWorkspaceSession(t.Context(), session, runtime, 120); err != nil || dispatched.Load() != 1 {
		t.Fatalf("approved extension failed: %v / %d", err, dispatched.Load())
	}
	current, err := store.Queries.GetSandboxSession(t.Context(), session)
	if err != nil || time.Until(current.ExpiresAt.Time) < 115*time.Second {
		t.Fatalf("extended reservation not persisted: %v", err)
	}
	exec("UPDATE sandbox_sessions SET created_at=date_trunc('month',now())-interval '1 day' WHERE id=$1", session)
	if err := service.RenewWorkspaceSession(t.Context(), session, runtime, 120); !errors.Is(err, ErrQuotaExceeded) || dispatched.Load() != 1 {
		t.Fatalf("old-month reservation reused new-month quota: %v / %d", err, dispatched.Load())
	}
	start := make(chan struct{})
	finished := make(chan error, 8)
	for index := 0; index < 8; index++ {
		go func() { <-start; _, err := service.FinishWorkspaceSession(t.Context(), session); finished <- err }()
	}
	close(start)
	for index := 0; index < 8; index++ {
		if err := <-finished; err != nil {
			t.Fatalf("concurrent settlement failed: %v", err)
		}
	}
	var count int64
	if err := store.Pool.QueryRow(t.Context(), "SELECT coalesce(sum(session_count),0) FROM sandbox_usage WHERE enterprise_id=$1", e).Scan(&count); err != nil || count != 1 {
		t.Fatalf("session charged more than once: %d / %v", count, err)
	}
	if err := store.Pool.QueryRow(t.Context(), "SELECT coalesce(sum(session_count),0) FROM sandbox_usage WHERE enterprise_id=$1 AND month=(date_trunc('month',now())-interval '1 month')::date", e).Scan(&count); err != nil || count != 1 {
		t.Fatalf("settlement changed reservation month: %d / %v", count, err)
	}
	if _, err := store.Queries.UpdateSandboxSessionStatus(t.Context(), db.UpdateSandboxSessionStatusParams{ID: session, Status: "running"}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("stale observation revived a settled session: %v", err)
	}
	if _, err := store.Queries.UpdateSandboxSessionExpiry(t.Context(), db.UpdateSandboxSessionExpiryParams{ID: session, Status: "running", ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Minute), Valid: true}}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("stale renewal revived a settled session: %v", err)
	}
}
