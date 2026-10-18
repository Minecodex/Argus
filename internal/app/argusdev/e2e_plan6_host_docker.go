package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type plan6DockerFixture struct {
	RuntimeImage string `json:"runtime_image,omitempty"`
	Owner        string `json:"owner"`
	Networks     []struct {
		Name     string `json:"name"`
		ID       string `json:"id"`
		Internal bool   `json:"internal"`
	} `json:"networks"`
	Containers []struct {
		Role string `json:"role"`
		Name string `json:"name"`
		ID   string `json:"id"`
	} `json:"containers"`
	Host, Bastion, HostIP, BastionIP string
}

type plan6DockerInspection struct {
	ID     string `json:"Id"`
	Config struct {
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	State struct {
		Running bool `json:"Running"`
	} `json:"State"`
	HostConfig struct {
		CgroupnsMode string `json:"CgroupnsMode"`
	} `json:"HostConfig"`
	NetworkSettings struct {
		Networks map[string]struct {
			IP string `json:"IPAddress"`
		} `json:"Networks"`
	} `json:"NetworkSettings"`
}

func (a *App) plan6InspectDocker(ctx context.Context, id, owner string) (plan6DockerInspection, error) {
	var rows []plan6DockerInspection
	raw, err := a.runner.Output(ctx, nil, "docker", "inspect", id)
	if err != nil {
		return plan6DockerInspection{}, err
	}
	if err = json.Unmarshal([]byte(raw), &rows); err != nil || len(rows) != 1 {
		return plan6DockerInspection{}, fmt.Errorf("invalid Docker inspection: %w", err)
	}
	d := rows[0]
	if d.ID != id || d.Config.Labels["io.argus.e2e.run"] != owner {
		return d, fmt.Errorf("Docker fixture ownership mismatch")
	}
	return d, nil
}

func (a *App) loadPlan6DockerFixture(ctx context.Context, name string) (*plan6DockerFixture, error) {
	raw, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	var f plan6DockerFixture
	if err = json.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	id := regexp.MustCompile(`^[a-f0-9]{64}$`)
	if !windowsE2EIdentifier.MatchString(f.Owner) || len(f.Containers) != 2 || len(f.Networks) != 2 {
		return nil, fmt.Errorf("invalid owned Docker host manifest")
	}
	inside := ""
	for _, n := range f.Networks {
		if !id.MatchString(n.ID) {
			return nil, fmt.Errorf("invalid Docker network identity")
		}
		r, e := a.runner.Output(ctx, nil, "docker", "network", "inspect", n.ID)
		if e != nil {
			return nil, e
		}
		var values []struct {
			ID       string            `json:"Id"`
			Internal bool              `json:"Internal"`
			Labels   map[string]string `json:"Labels"`
		}
		if json.Unmarshal([]byte(r), &values) != nil || len(values) != 1 || values[0].ID != n.ID || values[0].Labels["io.argus.e2e.run"] != f.Owner || values[0].Internal != n.Internal {
			return nil, fmt.Errorf("Docker network ownership or isolation mismatch")
		}
		if n.Internal {
			if inside != "" {
				return nil, fmt.Errorf("ambiguous isolated network")
			}
			inside = n.Name
		}
	}
	if inside == "" {
		return nil, fmt.Errorf("Host requires an internal-only Docker network")
	}
	for _, c := range f.Containers {
		if !id.MatchString(c.ID) {
			return nil, fmt.Errorf("invalid Docker container identity")
		}
		d, e := a.plan6InspectDocker(ctx, c.ID, f.Owner)
		if e != nil {
			return nil, e
		}
		v, ok := d.NetworkSettings.Networks[inside]
		if !d.State.Running || d.HostConfig.CgroupnsMode != "private" || !ok || net.ParseIP(v.IP) == nil {
			return nil, fmt.Errorf("Docker fixture is not a running private-cgroup peer")
		}
		switch c.Role {
		case "host":
			if f.Host != "" || len(d.NetworkSettings.Networks) != 1 {
				return nil, fmt.Errorf("Host must have exactly one internal network")
			}
			f.Host, f.HostIP = c.ID, v.IP
		case "bastion":
			if f.Bastion != "" || len(d.NetworkSettings.Networks) != 2 {
				return nil, fmt.Errorf("Bastion must bridge only the two owned networks")
			}
			f.Bastion, f.BastionIP = c.ID, v.IP
		default:
			return nil, fmt.Errorf("unknown Docker host role")
		}
	}
	if f.Host == "" || f.Bastion == "" {
		return nil, fmt.Errorf("both Host and Bastion are required")
	}
	return &f, nil
}

func (a *App) plan6DockerScript(ctx context.Context, f *plan6DockerFixture, id, script string) (string, error) {
	if id != f.Host && id != f.Bastion {
		return "", fmt.Errorf("Docker command target is not owned")
	}
	if _, err := a.plan6InspectDocker(ctx, id, f.Owner); err != nil {
		return "", err
	}
	// Enrollment instructions stay on stdin, never a logged process argument.
	cmd := exec.CommandContext(ctx, "docker", "exec", "--interactive", id, "/bin/bash", "-se")
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("owned Docker command failed: %w", err)
	}
	return string(out), nil
}

func (a *App) preparePlan6DockerRouting(ctx context.Context, env *E2EEnvironment, f *plan6DockerFixture) error {
	hosts, err := e2eExposureHosts(env.ConfigPath)
	if err != nil {
		return err
	}
	bridge, err := a.plan6DockerIngressBridge(ctx, env, f)
	if err != nil {
		return err
	}
	lines := "\n"
	for _, host := range hosts {
		lines += bridge + " " + host + "\n"
	}
	config, err := env.Kube.Client.CoreV1().ConfigMaps(env.SystemNS).Get(ctx, "argus-runtime-config", metav1.GetOptions{})
	if err != nil {
		return err
	}
	for _, key := range []string{"ARGUS_TELEMETRY_INGEST_GRPC_ENDPOINT", "ARGUS_TELEMETRY_INGEST_HTTP_ENDPOINT"} {
		u, err := url.Parse(config.Data[key])
		if err != nil || u.Hostname() == "" {
			return fmt.Errorf("managed telemetry endpoint unavailable")
		}
		lines += bridge + " " + u.Hostname() + "\n"
	}
	for _, id := range []string{f.Bastion, f.Host} {
		if _, err := a.plan6DockerScript(ctx, f, id, "printf '%s' '"+lines+"' >> /etc/hosts\nsystemctl is-active ssh\n"); err != nil {
			return err
		}
	}
	if _, err := a.plan6DockerScript(ctx, f, f.Host, "timeout 3 bash -c '</dev/tcp/"+bridge+"/443'\n"); err == nil {
		return fmt.Errorf("internal Docker Host unexpectedly has direct platform access")
	}
	proof := map[string]any{"docker_engine": true, "host_id": f.Host, "bastion_id": f.Bastion, "host_address": f.HostIP, "bastion_address": f.BastionIP, "private_cgroups": true, "host_direct_platform_access": false, "shared_linux_kernel": true, "synthetic_otlp_injection": false}
	raw, _ := json.MarshalIndent(proof, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "host-docker-topology.json"), raw)
}

func (a *App) cleanupPlan6DockerFixture(ctx context.Context, env *E2EEnvironment) error {
	if env.Options.Plan6HostFixtures == "" {
		return nil
	}
	raw, err := os.ReadFile(env.Options.Plan6HostFixtures)
	if err != nil {
		return err
	}
	var f plan6DockerFixture
	if json.Unmarshal(raw, &f) != nil || !windowsE2EIdentifier.MatchString(f.Owner) {
		return fmt.Errorf("invalid Docker cleanup manifest")
	}
	var failures []error
	if bridge := env.State.Values["host_docker_bridge_id"]; bridge != "" {
		if _, err = a.plan6InspectDocker(ctx, bridge, env.State.Values["host_docker_bridge_owner"]); err != nil {
			failures = append(failures, err)
		} else {
			failures = append(failures, a.runner.Run(ctx, nil, "docker", "rm", "--force", "--volumes", bridge))
		}
	}
	for _, c := range f.Containers {
		if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(c.ID) {
			return fmt.Errorf("invalid owned container ID")
		}
		if _, err = a.plan6InspectDocker(ctx, c.ID, f.Owner); err != nil {
			failures = append(failures, err)
			continue
		}
		failures = append(failures, a.runner.Run(ctx, nil, "docker", "rm", "--force", "--volumes", c.ID))
	}
	for _, n := range f.Networks {
		if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(n.ID) {
			return fmt.Errorf("invalid owned network ID")
		}
		text, e := a.runner.Output(ctx, nil, "docker", "network", "inspect", n.ID)
		if e != nil {
			failures = append(failures, e)
			continue
		}
		var rows []struct {
			ID         string            `json:"Id"`
			Labels     map[string]string `json:"Labels"`
			Containers map[string]any    `json:"Containers"`
		}
		if json.Unmarshal([]byte(text), &rows) != nil || len(rows) != 1 || rows[0].ID != n.ID || rows[0].Labels["io.argus.e2e.run"] != f.Owner || len(rows[0].Containers) != 0 {
			failures = append(failures, fmt.Errorf("network ownership or remaining members changed"))
			continue
		}
		failures = append(failures, a.runner.Run(ctx, nil, "docker", "network", "rm", n.ID))
	}
	result := errors.Join(failures...)
	data, _ := json.MarshalIndent(map[string]any{"owner": f.Owner, "passed": result == nil, "checked_ids": f.Containers, "completed_at": time.Now().UTC()}, "", "  ")
	return errors.Join(result, writePrivate(filepath.Join(env.Options.Artifacts, "host-docker-cleanup.json"), data))
}

func (a *App) plan6DockerDiagnostics(ctx context.Context, env *E2EEnvironment) error {
	f, err := a.loadPlan6DockerFixture(ctx, env.Options.Plan6HostFixtures)
	if err != nil {
		return err
	}
	var failures []error
	for _, target := range f.Containers {
		// Fixed observations avoid identity/token/key files and installation argv.
		script := "ip route\nss -lnt\n"
		for _, unit := range []string{"argus-connector", "argus-connector-privileged", "argus-otelcol"} {
			script += "systemctl show " + unit + " --property=ActiveState,SubState,ExecMainStatus,NRestarts --no-pager || true\n"
			script += "journalctl -u " + unit + " --no-pager --output=short-iso -n 500 || true\n"
		}
		output, err := a.plan6DockerScript(ctx, f, target.ID, script)
		if err == nil {
			err = writePrivate(filepath.Join(env.Options.Artifacts, "diagnostics", "docker-"+target.Role+".journal.log"), redactDiagnostic([]byte(output)))
		}
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}
