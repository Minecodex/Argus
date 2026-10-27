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

func TestPlan8PostgresActualRoleVersionAndOwnership(t *testing.T) {
	if os.Getenv("ARGUS_PLAN8_EXTRA_FIXTURES") != "1" {
		t.Skip("explicit owned Docker target fixture required")
	}
	root, _ := filepath.Abs(filepath.Join("..", "..", ".."))
	a := App{root: root, runner: Runner{Dir: root, Stdout: io.Discard, Stderr: io.Discard}}
	owner := "plan8-postgres-fixture-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	network, err := a.runner.Output(t.Context(), nil, "docker", "network", "create", "--internal", "--label", "argus.owner="+owner, owner)
	if err != nil {
		t.Fatal(err)
	}
	network = strings.TrimSpace(network)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		found, e := a.runner.Output(ctx, nil, "docker", "network", "inspect", "--format", `{{index .Labels "argus.owner"}}`, network)
		if e != nil || strings.TrimSpace(found) != owner {
			t.Error("owned PostgreSQL test network changed")
			return
		}
		if e = a.runner.RunIO(ctx, nil, nil, io.Discard, io.Discard, "docker", "network", "rm", network); e != nil {
			t.Error(e)
		}
	})
	env := &E2EEnvironment{Root: root, WorkDir: t.TempDir(), Options: E2EOptions{RunID: owner, Artifacts: t.TempDir()}}
	host := &plan6HostLoop{Docker: &plan6DockerFixture{}}
	raw, _ := json.Marshal(map[string]any{"networks": []map[string]any{{"name": owner, "id": network, "internal": true}}})
	if err = json.Unmarshal(raw, host.Docker); err != nil {
		t.Fatal(err)
	}
	target, err := a.preparePlan8PostgresTarget(t.Context(), env, host)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if target.owner != "" {
			if e := a.cleanupPlan8Databases(ctx, []plan8DatabaseFixture{target.plan8DatabaseFixture}); e != nil {
				t.Error(e)
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if target.database != "plan8" || target.version == "" || !strings.HasPrefix(target.imageID, "sha256:") || target.password == "" {
		t.Fatal("actual PostgreSQL role, version or immutable image identity missing")
	}
	proof, err := os.ReadFile(filepath.Join(env.Options.Artifacts, "plan8-postgresql-ownership.json"))
	if err != nil || !json.Valid(proof) || strings.Contains(string(proof), target.password) || strings.Contains(string(proof), "Root-P8-") {
		t.Fatal("public fixture ownership evidence leaked private values or is unavailable")
	}
	t.Log("actual PostgreSQL monitor role cannot create a database; runtime server version and image identity recorded; owned target and network cleanup remain separate from deployed pipeline acceptance")
}
