package sandbox

import (
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
)

func TestQuotaCreateAndUpdateRequireTheObservedVersion(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires a disposable migrated database")
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	enterprise := uuid.New()
	if _, err := store.Pool.Exec(t.Context(), "INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'P5 quota',$2,'UTC')", enterprise, "p5-"+enterprise.String()); err != nil {
		t.Fatal(err)
	}
	service := Service{Store: store}
	if _, err := service.UpdateQuota(t.Context(), enterprise, 2, 125, 5); !errors.Is(err, ErrVersionConflict) {
		t.Fatal("nonzero version created an absent quota")
	}
	created, err := service.UpdateQuota(t.Context(), enterprise, 2, 125, 0)
	if err != nil || created.Version != 1 {
		t.Fatalf("create failed: %v", err)
	}
	if _, err := service.UpdateQuota(t.Context(), enterprise, 9, 999, 0); !errors.Is(err, ErrVersionConflict) {
		t.Fatal("create version overwrote an existing quota")
	}
	updated, err := service.UpdateQuota(t.Context(), enterprise, 3, 125, created.Version)
	if err != nil || updated.Version != 2 || updated.MonthlySessionSeconds != 125 {
		t.Fatalf("quota update failed: %v", err)
	}
	if _, err := service.UpdateQuota(t.Context(), enterprise, 9, 999, created.Version); !errors.Is(err, ErrVersionConflict) {
		t.Fatal("stale quota update succeeded")
	}
}
