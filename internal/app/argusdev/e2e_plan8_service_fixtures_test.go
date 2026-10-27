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

func TestPlan8PreparedDatabaseFixturesUsePrivateInputsAndCleanOwnedTargets(t *testing.T) {
	if os.Getenv("ARGUS_PLAN8_SERVICE_FIXTURE") != "1" {
		t.Skip("explicit owned Docker target gate required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	a := &App{root: root, runner: Runner{Dir: root, Stdout: io.Discard, Stderr: io.Discard}}
	run := "plan8-fixture-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	network, err := a.runner.Output(ctx, nil, "docker", "network", "create", "--internal", "--label", "io.argus.e2e.run="+run, run)
	if err != nil {
		t.Fatal(err)
	}
	network = strings.TrimSpace(network)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		raw, err := a.runner.Output(cleanup, nil, "docker", "network", "inspect", network)
		if err != nil {
			t.Error("inspect owned service network")
			return
		}
		var rows []struct {
			ID     string
			Labels map[string]string
		}
		if json.Unmarshal([]byte(raw), &rows) != nil || len(rows) != 1 || rows[0].ID != network || rows[0].Labels["io.argus.e2e.run"] != run {
			t.Error("service network ownership changed")
			return
		}
		if err := a.runner.RunIO(cleanup, nil, nil, io.Discard, io.Discard, "docker", "network", "rm", network); err != nil {
			t.Error("remove owned service network", err)
		}
	})
	h := &plan6HostLoop{Docker: &plan6DockerFixture{}}
	body, _ := json.Marshal(map[string]any{"networks": []map[string]any{{"name": run, "id": network, "internal": true}}})
	if err = json.Unmarshal(body, h.Docker); err != nil {
		t.Fatal(err)
	}
	env := &E2EEnvironment{Root: root, WorkDir: t.TempDir(), Options: E2EOptions{RunID: run, Artifacts: t.TempDir()}}
	rows, err := a.preparePlan8Databases(ctx, env, h)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		if err := a.cleanupPlan8Databases(cleanup, rows); err != nil {
			t.Error(err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatal("required two MySQL and one SQL Server targets were not created")
	}
	proof, err := os.ReadFile(filepath.Join(env.Options.Artifacts, "plan8-services-ownership.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if !strings.HasPrefix(row.imageID, "sha256:") || row.version == "" {
			t.Fatal("actual database version and immutable image identity missing")
		}
		if strings.Contains(string(proof), row.password) {
			t.Fatal("credential leaked into public ownership evidence")
		}
	}
	t.Log("actual two MySQL and SQL Server targets initialized using private env/stdin, monitoring accounts rejected writes, ownership evidence excluded credentials")
}
