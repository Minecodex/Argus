package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) createPlan7DockerFixture(ctx context.Context, env *E2EEnvironment) (*plan6DockerFixture, error) {
	image := os.Getenv("ARGUS_P7_SYSTEMD_IMAGE")
	if image == "" {
		return nil, fmt.Errorf("Plan7 requires ARGUS_P7_SYSTEMD_IMAGE to reference the selected cached SSH/systemd image")
	}
	if _, err := a.runner.Output(ctx, nil, "docker", "image", "inspect", image, "--format", "{{.Id}}"); err != nil {
		return nil, fmt.Errorf("Plan7 systemd image is not cached: %w", err)
	}
	owner := "plan7-" + env.Options.RunID
	manifest := map[string]any{"owner": owner, "runtime_image": image, "networks": []map[string]any{}, "containers": []map[string]any{}}
	env.Options.Plan6HostFixtures = filepath.Join(env.Options.Artifacts, "plan7-docker-ownership.json")
	save := func() error {
		raw, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return err
		}
		return writePrivate(env.Options.Plan6HostFixtures, raw)
	}
	if err := save(); err != nil {
		return nil, err
	}
	networks := []string{}
	for _, role := range []string{"internal", "outbound"} {
		name := kubernetesNameForDev("argus-" + owner + "-" + role)
		args := []string{"network", "create", "--label", "io.argus.e2e.run=" + owner}
		if role == "internal" {
			args = append(args, "--internal")
		}
		args = append(args, name)
		id, err := a.runner.Output(ctx, nil, "docker", args...)
		if err != nil {
			return nil, err
		}
		id = strings.TrimSpace(id)
		manifest["networks"] = append(manifest["networks"].([]map[string]any), map[string]any{"name": name, "id": id, "internal": role == "internal"})
		networks = append(networks, name)
		if err = save(); err != nil {
			return nil, err
		}
	}
	for _, role := range []string{"host", "bastion"} {
		name := kubernetesNameForDev("argus-" + owner + "-" + role)
		id, err := a.runner.Output(ctx, nil, "docker", "run", "--detach", "--name", name, "--hostname", name, "--label", "io.argus.e2e.run="+owner, "--privileged", "--cgroupns", "private", "--memory", "768m", "--cpus", "1", "--tmpfs", "/run", "--tmpfs", "/run/lock", "--network", networks[0], image, "/sbin/init")
		if err != nil {
			return nil, err
		}
		id = strings.TrimSpace(id)
		manifest["containers"] = append(manifest["containers"].([]map[string]any), map[string]any{"role": role, "name": name, "id": id})
		if err = save(); err != nil {
			return nil, err
		}
		if role == "bastion" {
			if err = a.runner.Run(ctx, nil, "docker", "network", "connect", networks[1], id); err != nil {
				return nil, err
			}
		}
		machineID := strings.ReplaceAll(uuid.NewString(), "-", "")
		if err = a.runner.RunIO(ctx, nil, strings.NewReader(machineID+"\n"), a.stdout, a.stderr, "docker", "exec", "--interactive", id, "/bin/sh", "-c", "cat > /etc/machine-id"); err != nil {
			return nil, err
		}
		if err = a.runner.Run(ctx, nil, "docker", "restart", id); err != nil {
			return nil, err
		}
		deadline := time.Now().Add(time.Minute)
		for {
			state, e := a.runner.Output(ctx, nil, "docker", "exec", id, "systemctl", "is-active", "ssh")
			if e == nil && strings.TrimSpace(state) == "active" {
				break
			}
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("Plan7 %s SSH/systemd did not become ready", role)
			}
			if e = waitContext(ctx, time.Second); e != nil {
				return nil, e
			}
		}
	}
	return a.loadPlan6DockerFixture(ctx, env.Options.Plan6HostFixtures)
}
