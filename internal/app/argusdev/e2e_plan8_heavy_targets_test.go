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

func TestPlan8AdditionalActualTargetRolesAndVersions(t *testing.T) {
	if os.Getenv("ARGUS_PLAN8_EXTRA_FIXTURES") != "1" {
		t.Skip("explicit owned target fixtures required")
	}
	filter := os.Getenv("ARGUS_PLAN8_TARGET_FILTER")
	for _, engine := range []string{"mongodb", "elasticsearch", "oracle", "kafka"} {
		if filter != "" && filter != engine {
			continue
		}
		t.Run(engine, func(t *testing.T) {
			root, _ := filepath.Abs(filepath.Join("..", "..", ".."))
			a := App{root: root, runner: Runner{Dir: root, Stdout: io.Discard, Stderr: io.Discard}}
			owner := "plan8-actual-" + engine + "-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
			network, err := a.runner.Output(t.Context(), nil, "docker", "network", "create", "--internal", "--label", "argus.owner="+owner, owner)
			if err != nil {
				t.Fatal(err)
			}
			network = strings.TrimSpace(network)
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				current, e := a.runner.Output(ctx, nil, "docker", "network", "inspect", "--format", `{{index .Labels "argus.owner"}}`, network)
				if e != nil || strings.TrimSpace(current) != owner {
					t.Error("owned network identity differs")
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
			var target plan8ExtraTarget
			switch engine {
			case "mongodb", "elasticsearch":
				target, err = a.preparePlan8DocumentTarget(t.Context(), env, host, engine)
			case "oracle":
				target, err = a.preparePlan8OracleTarget(t.Context(), env, host)
			case "kafka":
				target, err = a.preparePlan8KafkaTarget(t.Context(), env, host)
			}
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
			if target.version == "" || !strings.HasPrefix(target.imageID, "sha256:") {
				t.Fatal("actual target version or image identity unavailable")
			}
			body, err := os.ReadFile(filepath.Join(env.Options.Artifacts, "plan8-"+engine+"-ownership.json"))
			if err != nil || strings.Contains(string(body), target.password) {
				t.Fatal("actual target ownership evidence invalid or private")
			}
			if output := os.Getenv("ARGUS_PLAN8_ACTUAL_VERSION_DIRECTORY"); output != "" {
				if err = os.WriteFile(filepath.Join(output, engine+".json"), body, 0600); err != nil {
					t.Fatal(err)
				}
			}
			t.Logf("actual %s runtime=%s image=%s, monitoring login and explicit write rejection checked", engine, target.version, target.imageID)
		})
	}
}
