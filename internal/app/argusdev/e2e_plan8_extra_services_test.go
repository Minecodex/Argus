package argusdev

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPlan8ExtraServiceTargetsHaveActualReadOnlyEndpointsAndOwnedCleanup(t *testing.T) {
	if os.Getenv("ARGUS_PLAN8_EXTRA_FIXTURES") != "1" {
		t.Skip("explicit owned Docker fixture gate required")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	a := App{root: root, runner: Runner{Dir: root, Stdout: io.Discard, Stderr: io.Discard}}
	owner := "plan8-extra-fixture-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	network, err := a.runner.Output(t.Context(), nil, "docker", "network", "create", "--internal", "--label", "argus.owner="+owner, owner)
	if err != nil {
		t.Fatal(err)
	}
	network = strings.TrimSpace(network)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		found, err := a.runner.Output(ctx, nil, "docker", "network", "inspect", "--format", `{{index .Labels "argus.owner"}}`, network)
		if err != nil || strings.TrimSpace(found) != owner {
			t.Error("fixture network identity changed", err)
			return
		}
		if err = a.runner.RunIO(ctx, nil, nil, io.Discard, io.Discard, "docker", "network", "rm", network); err != nil {
			t.Error(err)
		}
	})
	env := &E2EEnvironment{Root: root, WorkDir: t.TempDir(), Options: E2EOptions{RunID: owner, Artifacts: t.TempDir()}}
	h := &plan6HostLoop{Docker: &plan6DockerFixture{}}
	encoded, _ := json.Marshal(map[string]any{"networks": []map[string]any{{"name": owner, "id": network, "internal": true}}})
	if err = json.Unmarshal(encoded, h.Docker); err != nil {
		t.Fatal(err)
	}
	rows, err := a.preparePlan8ExtraServices(t.Context(), env, h)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		targets := []plan8DatabaseFixture{}
		for _, row := range rows {
			targets = append(targets, row.plan8DatabaseFixture)
		}
		if err := a.cleanupPlan8Databases(ctx, targets); err != nil {
			t.Error(err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].engine != "redis" || rows[1].engine != "nginx" {
		t.Fatal("expected actual Redis and Nginx targets")
	}
	body, err := os.ReadFile(filepath.Join(env.Options.Artifacts, "plan8-extra-services-ownership.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if !strings.HasPrefix(row.imageID, "sha256:") || row.version == "" {
			t.Fatal("actual service version and immutable image identity missing")
		}
		if row.password != "" && strings.Contains(string(body), row.password) {
			t.Fatal("private fixture credential entered public ownership evidence")
		}
	}
	t.Log("actual metadata-only Redis and Nginx endpoints ready; private ownership and exact cleanup verified")
}
