package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type plan7ServiceFault struct{ Directory, Unit string }

func (a *App) preparePlan7ServiceFault(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) (plan7ServiceFault, error) {
	name := kubernetesNameForDev("argus-plan7-" + env.Options.RunID)
	fault := plan7ServiceFault{Directory: "/tmp/" + name, Unit: name + ".service"}
	script := fmt.Sprintf(`set -eu
mkdir -p %[1]s
printf '0\n' > %[1]s/workers
cat > %[1]s/run <<'P7SERVICE'
#!/bin/sh
set -eu
workers=$(cat %[1]s/workers)
if [ "$workers" != 2 ]; then echo 'workers configuration must be 2' >&2; exit 64; fi
trap 'exit 0' TERM INT
while :; do date -u '+%%s' > %[1]s/heartbeat; sleep 1; done
P7SERVICE
cat > %[1]s/health <<'P7HEALTH'
#!/bin/sh
set -eu
systemctl is-active --quiet %[2]s
test "$(cat %[1]s/workers)" = 2
now=$(date -u '+%%s')
heartbeat=$(cat %[1]s/heartbeat)
test "$((now-heartbeat))" -le 5
printf 'healthy\n'
P7HEALTH
chmod 700 %[1]s/run %[1]s/health
cat > /etc/systemd/system/%[2]s <<'P7UNIT'
[Unit]
Description=Owned Argus Plan7 acceptance service
[Service]
Type=simple
WorkingDirectory=%[1]s
ExecStart=/bin/sh %[1]s/run
Restart=no
KillMode=control-group
TimeoutStopSec=5
P7UNIT
systemctl daemon-reload
systemctl start %[2]s || true
sleep 1
state=$(systemctl is-active %[2]s || true)
test "$state" = failed
if %[1]s/health; then exit 1; fi
printf 'owned service failure observed\n'
`, fault.Directory, fault.Unit)
	result, err := a.plan6DockerScript(ctx, host.Docker, host.Docker.Host, script)
	if err != nil || !strings.Contains(result, "owned service failure observed") {
		return fault, fmt.Errorf("Plan7 service fault not established: %v", err)
	}
	return fault, nil
}

func (a *App) verifyPlan7ServiceRecovery(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop, fault plan7ServiceFault) error {
	script := fmt.Sprintf(`set -eu
test "$(cat %[1]s/workers.bak)" = 0
%[1]s/health
pid=$(systemctl show --property=MainPID --value %[2]s)
test "$pid" -gt 0
test -r /proc/$pid/cmdline
tr '\000' ' ' < /proc/$pid/cmdline | grep -F '%[1]s/run' >/dev/null
printf 'main_pid=%%s\n' "$pid"
systemctl show --property=ActiveState,SubState,InvocationID %[2]s
printf 'heartbeat_utc_epoch='
cat %[1]s/heartbeat
sha256sum %[1]s/workers %[1]s/workers.bak
`, fault.Directory, fault.Unit)
	proof, err := a.plan6DockerScript(ctx, host.Docker, host.Docker.Host, script)
	if err != nil || !strings.Contains(proof, "healthy") || !strings.Contains(proof, "ActiveState=active") {
		return fmt.Errorf("Plan7 owned service has not recovered: %v", err)
	}
	raw, err := json.MarshalIndent(map[string]any{"run_id": env.Options.RunID, "resource_id": host.HostID, "unit": fault.Unit, "directory": fault.Directory, "scope": "owned systemd process, configuration, backup and fresh heartbeat; no claim about unrelated production services", "proof": proof}, "", "  ")
	if err != nil {
		return err
	}
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan7-service-recovery.json"), raw)
}
