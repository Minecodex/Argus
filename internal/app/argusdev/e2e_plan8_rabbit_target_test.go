package argusdev

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlan8RabbitActualRoleVersionAndCleanup(t *testing.T) {
	if os.Getenv("ARGUS_PLAN8_EXTRA_FIXTURES") != "1" {
		t.Skip("explicit owned fixture required")
	}
	root, _ := filepath.Abs(filepath.Join("..", "..", ".."))
	a := App{root: root, runner: Runner{Dir: root, Stdout: io.Discard, Stderr: io.Discard}}
	owner := "plan8-rabbit-fixture-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
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
	row, err := a.preparePlan8RabbitTarget(t.Context(), env, host)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if row.owner != "" {
			if e := a.cleanupPlan8Databases(ctx, []plan8DatabaseFixture{row.plan8DatabaseFixture}); e != nil {
				t.Error(e)
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(env.Options.Artifacts, "plan8-rabbitmq-ownership.json"))
	if err != nil || strings.Contains(string(body), row.password) || strings.Contains(string(body), "Root-P8-") {
		t.Fatal("RabbitMQ public ownership evidence invalid")
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		status, data, e := a.plan8TargetHTTP(t.Context(), row.id, "15672", "GET", "/api/overview", "monitor", row.password, nil)
		var traffic struct {
			Stats struct {
				Publish   int64 `json:"publish"`
				Delivered int64 `json:"deliver_get"`
				Ack       int64 `json:"ack"`
			} `json:"message_stats"`
		}
		if e == nil && status == 200 && json.Unmarshal(data, &traffic) == nil && traffic.Stats.Publish > 0 && traffic.Stats.Delivered > 0 && traffic.Stats.Ack > 0 {
			t.Log("actual publish/get/ack traffic observed by the monitoring identity")
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("owned RabbitMQ workload did not produce monitored publish/get/ack counters", status, e, traffic)
		}
		if e = waitContext(t.Context(), time.Second); e != nil {
			t.Fatal(e)
		}
	}
	t.Logf("actual RabbitMQ runtime=%s image=%s; monitor read succeeds, queue write and wrong password explicitly rejected", row.version, row.imageID)
	if output := os.Getenv("ARGUS_PLAN8_RABBIT_VERSION_EVIDENCE"); output != "" {
		if err = os.WriteFile(output, body, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
