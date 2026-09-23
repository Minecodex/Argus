package sandbox

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestFrozenWorkspaceRuntimeRejectsConfigurationChanges(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires a disposable migrated database")
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := Service{Store: store}
	backend, err := service.CreateBackend(t.Context(), BackendInput{Name: "identity-" + uuid.NewString(), Endpoint: "http://127.0.0.1:9", Status: "enabled"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Queries.SetSandboxBackendHealth(t.Context(), db.SetSandboxBackendHealthParams{ID: backend.ID, HealthStatus: "healthy"}); err != nil {
		t.Fatal(err)
	}
	image, err := service.CreateImage(t.Context(), ImageInput{BackendID: backend.ID, Name: "identity-" + uuid.NewString(), ImageRef: "argus/workspace:fixed", Digest: "sha256:" + strings.Repeat("1", 64), Status: "enabled"})
	if err != nil {
		t.Fatal(err)
	}
	input := ProfileInput{Name: "identity-" + uuid.NewString(), BackendID: backend.ID, ImageID: image.ID, TaskKinds: []string{"agent_workspace"}, CPUMillis: 500, MemoryMiB: 512, TimeoutSeconds: 900, NetworkMode: "none", Status: "enabled"}
	profile, err := service.CreateProfile(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := service.WorkspaceRuntimeByID(t.Context(), profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	// A new eligible profile at the same revision cannot replace this Run's choice.
	replacement := input
	replacement.Name = "replacement-" + uuid.NewString()
	other, err := service.CreateProfile(t.Context(), replacement)
	if err != nil || other.Revision != profile.Revision {
		t.Fatalf("replacement fixture: %v", err)
	}
	resolved, err := service.ResolveWorkspaceRuntime(t.Context(), frozen.Identity())
	if err != nil || resolved.Profile.ID != profile.ID {
		t.Fatalf("new profile replaced a frozen choice: %v", err)
	}
	for _, target := range []string{"profile", "image", "backend"} {
		t.Run(target, func(t *testing.T) {
			current, err := service.WorkspaceRuntimeByID(t.Context(), profile.ID)
			if err != nil {
				t.Fatal(err)
			}
			switch target {
			case "profile":
				input.ExpectedVersion = current.Profile.Version
				input.MemoryMiB += 128
				_, err = service.UpdateProfile(t.Context(), profile.ID, input)
			case "image":
				_, err = service.UpdateImage(t.Context(), image.ID, ImageInput{BackendID: backend.ID, Name: "changed-" + uuid.NewString(), ImageRef: current.Image.ImageRef, Digest: "sha256:" + strings.Repeat("2", 64), Status: "enabled", ExpectedVersion: current.Image.Version})
			case "backend":
				_, err = service.UpdateBackend(t.Context(), backend.ID, BackendInput{Name: "changed-" + uuid.NewString(), Endpoint: "http://127.0.0.1:10", Status: "enabled", ExpectedVersion: current.Backend.Version})
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.ResolveWorkspaceRuntime(t.Context(), current.Identity()); !errors.Is(err, ErrWorkspaceRuntimeChanged) {
				t.Fatalf("old %s snapshot executed against changed configuration: %v", target, err)
			}
		})
	}
}
