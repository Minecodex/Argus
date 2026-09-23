package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (a *App) p5WaitCommandMarker(ctx context.Context, env *E2EEnvironment, conversation, marker string) (*corev1.Pod, string, error) {
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		pod, err := a.p5WorkspacePod(ctx, env, conversation)
		if err == nil {
			for _, container := range pod.Spec.Containers {
				if len(container.Command) != 1 || container.Command[0] != "/usr/local/bin/argus-workspace-supervisor" {
					continue
				}
				if _, err := env.Kube.Exec(ctx, env.SandboxNS, "argus.io/workspace-id="+pod.Labels["argus.io/workspace-id"], container.Name, "/bin/sh", "-c", "test -s /workspace/"+marker); err == nil {
					return pod, container.Name, nil
				}
			}
		}
		if err := waitContext(ctx, time.Second); err != nil {
			return nil, "", err
		}
	}
	return nil, "", fmt.Errorf("P5 command did not reach its fault-injection marker")
}

func (a *App) p5CrashActiveSupervisor(ctx context.Context, env *E2EEnvironment, conversation string, before *corev1.Pod, container string) error {
	run, err := a.p5StartRun(ctx, env, conversation, "active-supervisor-crash", []p5ToolStep{{"bash", map[string]any{"command": "printf x >> /workspace/crash-started; sleep 120", "timeout_seconds": float64(120)}}}, []string{})
	if err != nil {
		return err
	}
	pod, _, err := a.p5WaitCommandMarker(ctx, env, conversation, "crash-started")
	if err != nil {
		return err
	}
	if pod.UID != before.UID {
		return fmt.Errorf("P5 active crash fixture unexpectedly replaced its warm Pod")
	}
	_, _ = env.Kube.Exec(ctx, env.SandboxNS, "argus.io/workspace-id="+pod.Labels["argus.io/workspace-id"], container, "/bin/sh", "-c", "kill -ABRT 1")
	if err := a.waitPostgresValue(ctx, env, "SELECT count(*) FROM runs WHERE id='"+run+"' AND status='failed' AND stop_reason='result_unknown';", "1", 90*time.Second); err != nil {
		return fmt.Errorf("P5 crashed command did not stop as result unknown: %w", err)
	}
	return a.waitPostgresValue(ctx, env, "SELECT count(*) FROM tool_calls WHERE run_id='"+run+"' AND tool_id='bash' AND status='result_unknown';", "1", time.Second)
}

func (a *App) verifyP5WorkspaceControlFaults(ctx context.Context, env *E2EEnvironment, conversation string) error {
	if err := a.verifyP5WorkspaceAPIOutage(ctx, env, conversation); err != nil {
		return err
	}
	if err := a.verifyP5WorkspaceProfileRace(ctx, env, conversation); err != nil {
		return err
	}
	return a.verifyP5WorkspaceWorkerTakeover(ctx, env, conversation)
}

func (a *App) verifyP5WorkspaceWorkerTakeover(ctx context.Context, env *E2EEnvironment, conversation string) error {
	run, err := a.p5StartRun(ctx, env, conversation, "workspace-worker-crash", []p5ToolStep{{"bash", map[string]any{"command": "printf x >> /workspace/worker-crash-started; sleep 120", "timeout_seconds": float64(120)}}}, []string{})
	if err != nil {
		return err
	}
	before, _, err := a.p5WaitCommandMarker(ctx, env, conversation, "worker-crash-started")
	if err != nil {
		return err
	}
	ns, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, env.SystemNS, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if ns.Labels["argus.io/release-id"] != env.ReleaseID {
		return fmt.Errorf("P5 worker namespace ownership mismatch")
	}
	workers, err := env.Kube.Client.CoreV1().Pods(env.SystemNS).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=argus-worker"})
	if err != nil {
		return err
	}
	if len(workers.Items) != 1 {
		return fmt.Errorf("P5 worker crash requires one owned worker Pod")
	}
	worker := workers.Items[0]
	zero := int64(0)
	if err := env.Kube.Client.CoreV1().Pods(env.SystemNS).Delete(ctx, worker.Name, metav1.DeleteOptions{GracePeriodSeconds: &zero, Preconditions: &metav1.Preconditions{UID: &worker.UID}}); err != nil {
		return err
	}
	if err := env.Kube.WaitDeployment(ctx, env.SystemNS, "argus-worker", 2*time.Minute); err != nil {
		return err
	}
	if err := a.waitPostgresValue(ctx, env, "SELECT count(*) FROM runs WHERE id='"+run+"' AND status='failed' AND stop_reason='result_unknown';", "1", 2*time.Minute); err != nil {
		return fmt.Errorf("P5 worker interruption result: %w", err)
	}
	// A crash can interrupt the 60-second cleanup path after it reserves its
	// 90-second protective lease. Wait through that documented grace and the
	// reconciler tick; do not bypass the lease to make takeover appear faster.
	if err := a.waitPostgresValue(ctx, env, "SELECT count(*) FROM workspaces WHERE conversation_id='"+conversation+"' AND (lease_until IS NULL OR lease_until<now());", "1", 2*time.Minute); err != nil {
		return fmt.Errorf("P5 worker cleanup lease: %w", err)
	}
	if err := a.p5BashProof(ctx, env, conversation, "workspace-worker-takeover", "test \"$(cat /workspace/worker-crash-started)\" = x && test -s /workspace/report.txt && printf worker-takeover-proved"); err != nil {
		return err
	}
	after, err := a.p5WorkspacePod(ctx, env, conversation)
	if err != nil {
		return err
	}
	if before.UID == after.UID {
		return fmt.Errorf("P5 expired owner was allowed to hand over a reusable Pod")
	}
	count, err := a.postgresQuery(ctx, env, "SELECT count(*) FROM tool_calls WHERE run_id='"+run+"' AND tool_id='bash' AND status='result_unknown';")
	if err != nil || strings.TrimSpace(count) != "1" {
		return fmt.Errorf("P5 abandoned command was repeated or lost: %v", err)
	}
	data, _ := json.Marshal(map[string]any{"worker_forced_exit": true, "unknown_commands": 1, "command_executions": 1, "old_workspace_pod_uid": before.UID, "new_workspace_pod_uid": after.UID, "abandoned_owner_reused": false, "files_retained": true})
	return writePrivate(filepath.Join(env.Options.Artifacts, "p5-workspace-worker-takeover.json"), data)
}

func (a *App) verifyP5WorkspaceAPIOutage(ctx context.Context, env *E2EEnvironment, conversation string) (failure error) {
	ns, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, env.SandboxNS, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if ns.Labels["argus.io/release-id"] != env.ReleaseID {
		return fmt.Errorf("P5 Sandbox namespace ownership mismatch")
	}
	deployment, err := env.Kube.Client.AppsV1().Deployments(env.SandboxNS).Get(ctx, "opensandbox-server", metav1.GetOptions{})
	if err != nil {
		return err
	}
	replicas := int32(1)
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}
	run, err := a.p5StartRun(ctx, env, conversation, "sandbox-api-outage", []p5ToolStep{{"bash", map[string]any{"command": "printf x > /workspace/api-outage-started; sleep 35; printf preserved > /workspace/api-outage-result", "timeout_seconds": float64(120)}}}, []string{})
	if err != nil {
		return err
	}
	if _, _, err := a.p5WaitCommandMarker(ctx, env, conversation, "api-outage-started"); err != nil {
		return err
	}
	restored := false
	restore := func() error {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := env.Kube.ScaleDeployment(cleanup, env.SandboxNS, "opensandbox-server", replicas); err != nil {
			return err
		}
		if err := env.Kube.WaitDeployment(cleanup, env.SandboxNS, "opensandbox-server", time.Minute); err != nil {
			return err
		}
		restored = true
		return nil
	}
	defer func() {
		if !restored {
			failure = errors.Join(failure, restore())
		}
	}()
	if err := env.Kube.ScaleDeployment(ctx, env.SandboxNS, "opensandbox-server", 0); err != nil {
		return err
	}
	if err := env.Kube.WaitDeployment(ctx, env.SandboxNS, "opensandbox-server", time.Minute); err != nil {
		return err
	}
	// The known supervisor result remains authoritative while the lifecycle API
	// is down. Cleanup uses Kubernetes directly, without replaying the command.
	if err := a.waitPostgresValue(ctx, env, "SELECT count(*) FROM tool_calls WHERE run_id='"+run+"' AND tool_id='bash' AND status='succeeded';", "1", 90*time.Second); err != nil {
		return err
	}
	if err := restore(); err != nil {
		return err
	}
	if err := a.waitRunTerminal(ctx, env, run); err != nil {
		return err
	}
	if err := a.p5BashProof(ctx, env, conversation, "sandbox-api-restored", "test \"$(cat /workspace/api-outage-result)\" = preserved && printf api-recovery-proved"); err != nil {
		return err
	}
	data, _ := json.Marshal(map[string]any{"api_replicas_during_execution": 0, "restored_replicas": replicas, "command_results": 1, "file_preserved": true})
	return writePrivate(filepath.Join(env.Options.Artifacts, "p5-workspace-api-outage.json"), data)
}

func (a *App) verifyP5WorkspaceProfileRace(ctx context.Context, env *E2EEnvironment, conversation string) error {
	run, err := a.p5StartRun(ctx, env, conversation, "profile-race", []p5ToolStep{
		{"bash", map[string]any{"command": "printf ready > /workspace/profile-race-started; sleep 12", "timeout_seconds": float64(60)}},
		{"bash", map[string]any{"command": "printf unsafe > /workspace/stale-profile-executed"}},
	}, []string{})
	if err != nil {
		return err
	}
	pod, _, err := a.p5WaitCommandMarker(ctx, env, conversation, "profile-race-started")
	if err != nil {
		return err
	}
	id := pod.Labels["argus.io/runtime-profile-id"]
	value, err := a.postgresQuery(ctx, env, "SELECT json_build_object('name',name,'backend_id',backend_id,'image_id',image_id,'task_kinds',task_kinds,'cpu_millis',cpu_millis,'memory_mib',memory_mib+128,'timeout_seconds',timeout_seconds,'network_mode',network_mode,'status',status,'expected_version',version) FROM sandbox_profiles WHERE id='"+id+"';")
	if err != nil {
		return err
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(value), &input); err != nil {
		return err
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	if _, err := client.JSON(ctx, "p5-profile-race-update", "platform", http.MethodPut, "/platform/sandbox/profiles/"+id, http.StatusOK, input, map[string]string{"Origin": env.PlatformOrigin(), "X-CSRF-Token": env.State.Values["platform_csrf"]}); err != nil {
		return err
	}
	if err := a.waitRunTerminal(ctx, env, run); err != nil {
		return err
	}
	count, err := a.postgresQuery(ctx, env, "SELECT count(*) FROM tool_calls WHERE run_id='"+run+"' AND tool_id='bash' AND status='failed' AND error_code='TOOL_VERSION_UNAVAILABLE';")
	if err != nil || strings.TrimSpace(count) != "1" {
		return fmt.Errorf("P5 stale runtime invocation was not explicitly rejected: %v", err)
	}
	if err := a.p5BashProof(ctx, env, conversation, "new-profile-run", "test ! -e /workspace/stale-profile-executed && test -s /workspace/report.txt && printf profile-revision-proved"); err != nil {
		return err
	}
	after, err := a.p5WorkspacePod(ctx, env, conversation)
	if err != nil {
		return err
	}
	if after.UID == pod.UID || after.Labels["argus.io/runtime-version"] == pod.Labels["argus.io/runtime-version"] {
		return fmt.Errorf("P5 new Run did not use the new runtime revision")
	}
	data, _ := json.Marshal(map[string]any{"stale_calls_rejected": 1, "stale_command_writes": 0, "new_run_uses_new_revision": true})
	return writePrivate(filepath.Join(env.Options.Artifacts, "p5-workspace-profile-race.json"), data)
}
