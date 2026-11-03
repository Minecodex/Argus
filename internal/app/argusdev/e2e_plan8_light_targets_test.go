package argusdev

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPlan8LightTargetActualVersionsAndCleanup(t *testing.T) {
	if os.Getenv("ARGUS_PLAN8_EXTRA_FIXTURES") != "1" {
		t.Skip("explicit owned fixture required")
	}
	root, _ := filepath.Abs(filepath.Join("..", "..", ".."))
	a := App{root: root, runner: Runner{Dir: root, Stdout: io.Discard, Stderr: io.Discard}}
	owner := "plan8-light-fixture-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	id, err := a.runner.Output(t.Context(), nil, "docker", "network", "create", "--internal", "--label", "argus.owner="+owner, owner)
	if err != nil {
		t.Fatal(err)
	}
	id = strings.TrimSpace(id)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		actual, e := a.runner.Output(ctx, nil, "docker", "network", "inspect", "--format", `{{index .Labels "argus.owner"}}`, id)
		if e != nil || strings.TrimSpace(actual) != owner {
			t.Error("owned network changed")
			return
		}
		if e = a.runner.RunIO(ctx, nil, nil, io.Discard, io.Discard, "docker", "network", "rm", id); e != nil {
			t.Error(e)
		}
	})
	env := &E2EEnvironment{Root: root, WorkDir: t.TempDir(), Options: E2EOptions{RunID: owner, Artifacts: t.TempDir()}}
	host := &plan6HostLoop{Docker: &plan6DockerFixture{}}
	raw, _ := json.Marshal(map[string]any{"networks": []map[string]any{{"name": owner, "id": id, "internal": true}}})
	if err = json.Unmarshal(raw, host.Docker); err != nil {
		t.Fatal(err)
	}
	rows, err := a.preparePlan8LightTargets(t.Context(), env, host)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		targets := []plan8DatabaseFixture{}
		for _, row := range rows {
			targets = append(targets, row.plan8DatabaseFixture)
		}
		if e := a.cleanupPlan8Databases(ctx, targets); e != nil {
			t.Error(e)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatal("actual target set incomplete")
	}
	body, err := os.ReadFile(filepath.Join(env.Options.Artifacts, "plan8-light-targets-ownership.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.engine == "memcached" {
			deadline := time.Now().Add(12 * time.Second)
			for {
				stats, e := a.runner.Output(t.Context(), nil, "docker", "exec", row.id, "sh", "-c", `printf 'stats\r\n' | nc -w 1 127.0.0.1 11211`)
				counts := map[string]int64{}
				for _, line := range strings.Split(stats, "\n") {
					fields := strings.Fields(line)
					if len(fields) == 3 && fields[0] == "STAT" {
						counts[fields[1]], _ = strconv.ParseInt(fields[2], 10, 64)
					}
				}
				if e == nil && counts["get_hits"] > 0 && counts["get_misses"] > 0 {
					t.Log("actual cache set/get traffic produced hits and misses")
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("owned cache workload did not produce real hits/misses", e)
				}
				if e = waitContext(t.Context(), time.Second); e != nil {
					t.Fatal(e)
				}
			}
		}
		if row.version == "" || !strings.HasPrefix(row.imageID, "sha256:") {
			t.Fatal("runtime identity missing", row.engine)
		}
		if row.password != "" && strings.Contains(string(body), row.password) {
			t.Fatal("private authentication reached evidence")
		}
		t.Logf("actual %s runtime=%s image=%s", row.engine, row.version, row.imageID)
	}
	if output := os.Getenv("ARGUS_PLAN8_LIGHT_VERSION_EVIDENCE"); output != "" {
		if err = os.WriteFile(output, body, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
