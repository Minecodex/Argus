package argusdev

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestLivePlan7OwnedServiceFaultAndRecoveryFixture(t *testing.T) {
	image := os.Getenv("ARGUS_P7_SYSTEMD_IMAGE")
	if os.Getenv("ARGUS_P7_SERVICE_FIXTURE_LIVE") != "1" || image == "" {
		t.Skip("requires owned Docker systemd fixture opt-in")
	}
	root, err := findRepoRoot("")
	if err != nil {
		t.Fatal(err)
	}
	a := &App{root: root, stdout: io.Discard, stderr: io.Discard, runner: Runner{Dir: root, Stdout: io.Discard, Stderr: io.Discard}}
	name := "argus-p7-service-" + uuid.NewString()[:8]
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	id, err := a.runner.Output(ctx, nil, "docker", "run", "--detach", "--name", name, "--label", "io.argus.e2e.run="+name, "--privileged", "--cgroupns", "private", "--network", "none", "--memory", "256m", "--tmpfs", "/run", "--tmpfs", "/run/lock", image, "/sbin/init")
	if err != nil {
		t.Fatal(err)
	}
	id = strings.TrimSpace(id)
	t.Logf("owned container=%s id=%s", name, id)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		if _, err := a.plan6InspectDocker(cleanup, id, name); err != nil {
			t.Error("ownership changed; retained", err)
			return
		}
		if err := a.runner.Run(cleanup, nil, "docker", "rm", "--force", id); err != nil {
			t.Error(err)
		}
	})
	for {
		if err = a.runner.Run(ctx, nil, "docker", "exec", id, "test", "-S", "/run/systemd/private"); err == nil {
			break
		}
		if err = waitContext(ctx, 200*time.Millisecond); err != nil {
			t.Fatal(err)
		}
	}
	env := &E2EEnvironment{Options: E2EOptions{RunID: name, Artifacts: t.TempDir()}}
	host := &plan6HostLoop{Docker: &plan6DockerFixture{Owner: name, Host: id}, HostID: uuid.NewString()}
	fault, err := a.preparePlan7ServiceFault(ctx, env, host)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.plan6DockerScript(ctx, host.Docker, id, "cp "+fault.Directory+"/workers "+fault.Directory+"/workers.bak\nprintf '2\\n' > "+fault.Directory+"/workers\nsystemctl restart "+fault.Unit+"\nsleep 1\n"); err != nil {
		t.Fatal(err)
	}
	if err = a.verifyPlan7ServiceRecovery(ctx, env, host, fault); err != nil {
		t.Fatal(err)
	}
	proof, err := os.ReadFile(filepath.Join(env.Options.Artifacts, "plan7-service-recovery.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(proof))
}
