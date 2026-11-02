package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
)

// Real cache/message operations supply rate/ratio panels with a nonempty
// population. No synthetic telemetry is inserted or missing series zero-filled.
func (a *App) plan8StartTargetLoad(ctx context.Context, env *E2EEnvironment, row plan8ExtraTarget) error {
	owner, err := a.runner.Output(ctx, nil, "docker", "inspect", "--format", `{{index .Config.Labels "io.argus.e2e.run"}}`, row.id)
	if err != nil || strings.TrimSpace(owner) != row.owner {
		return errors.New("workload fixture ownership changed")
	}
	var command string
	switch row.engine {
	case "memcached":
		command = `i=0; while [ "$i" -lt 1800 ]; do printf 'set argus_probe 0 30 1\r\n1\r\nget argus_probe\r\nget argus_missing\r\n' | nc -w 1 127.0.0.1 11211 >/dev/null 2>&1; i=$((i+1)); sleep 1; done`
	case "rabbitmq":
		architecture, err := a.runner.Output(ctx, nil, "docker", "image", "inspect", row.imageID, "--format", "{{.Architecture}}")
		if err != nil || strings.TrimSpace(architecture) != "amd64" && strings.TrimSpace(architecture) != "arm64" {
			return errors.New("workload fixture architecture unavailable")
		}
		binary := filepath.Join(env.WorkDir, "argus-e2e-target-load")
		if err = a.runner.Run(ctx, map[string]string{"GOOS": "linux", "GOARCH": strings.TrimSpace(architecture), "CGO_ENABLED": "0"}, "go", "build", "-trimpath", "-o", binary, "./tests/e2e/targetload"); err != nil {
			return err
		}
		if err = a.runner.RunIO(ctx, nil, nil, io.Discard, io.Discard, "docker", "cp", binary, row.id+":/tmp/argus-e2e-target-load"); err != nil {
			return err
		}
		if err = a.runner.RunIO(ctx, nil, nil, io.Discard, io.Discard, "docker", "exec", row.id, "chmod", "0755", "/tmp/argus-e2e-target-load"); err != nil {
			return err
		}
		command = "exec /tmp/argus-e2e-target-load"
	default:
		return errors.New("unsupported test workload")
	}
	shell := "sh"
	if row.engine == "rabbitmq" {
		shell = "bash"
	}
	if err = a.runner.RunIO(ctx, nil, nil, io.Discard, io.Discard, "docker", "exec", "--detach", row.id, shell, "-c", command); err != nil {
		return errors.New("owned workload did not start")
	}
	data, err := json.MarshalIndent(map[string]any{"integration": row.engine, "owner": row.owner, "container_id": row.id, "bounded_iterations": 1800, "actual_operations": map[string]string{"memcached": "set, successful get and missing-key get", "rabbitmq": "publish, get, acknowledge"}[row.engine], "synthetic_telemetry": false}, "", "  ")
	if err == nil {
		err = writePrivate(filepath.Join(env.Options.Artifacts, "plan8-load-"+row.engine+".json"), data)
	}
	return err
}
