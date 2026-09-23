package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Only this suite's freshly installed workloads receive the shorter idle window.
// Product defaults remain 900 seconds; the same reconciler and lease path run.
func (a *App) configureP5IdleWindow(ctx context.Context, env *E2EEnvironment) error {
	for _, name := range []string{"argus-server", "argus-worker"} {
		deployment, err := env.Kube.Client.AppsV1().Deployments(env.SystemNS).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if deployment.Labels["argus.io/release-id"] != env.ReleaseID {
			return fmt.Errorf("P5 idle fixture deployment ownership mismatch")
		}
		for index := range deployment.Spec.Template.Spec.Containers {
			deployment.Spec.Template.Spec.Containers[index].Env = append(deployment.Spec.Template.Spec.Containers[index].Env, corev1.EnvVar{Name: "ARGUS_WORKSPACE_IDLE_SECONDS", Value: "60"})
		}
		if _, err := env.Kube.Client.AppsV1().Deployments(env.SystemNS).Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
			return err
		}
		if err := env.Kube.WaitDeployment(ctx, env.SystemNS, name, 3*time.Minute); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) p5WorkspacePod(ctx context.Context, env *E2EEnvironment, conversation string) (*corev1.Pod, error) {
	value, err := a.postgresQuery(ctx, env, "SELECT id::text FROM workspaces WHERE conversation_id='"+conversation+"' AND status='ready';")
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(value)
	if id == "" {
		return nil, fmt.Errorf("P5 ready workspace missing")
	}
	pods, err := env.Kube.Client.CoreV1().Pods(env.SandboxNS).List(ctx, metav1.ListOptions{LabelSelector: "argus.io/workspace-id=" + id})
	if err != nil {
		return nil, err
	}
	if len(pods.Items) != 1 || pods.Items[0].DeletionTimestamp != nil {
		return nil, fmt.Errorf("P5 expected exactly one retained Workspace Pod, got %d", len(pods.Items))
	}
	return &pods.Items[0], nil
}

func (a *App) verifyP5WarmReuse(ctx context.Context, env *E2EEnvironment, conversation string, cold time.Duration, original *corev1.Pod) error {
	start := time.Now()
	if err := a.p5BashProof(ctx, env, conversation, "warm-reuse", "test -s /workspace/report.txt && printf warm-reuse-proved"); err != nil {
		return err
	}
	hot := time.Since(start)
	pod, err := a.p5WorkspacePod(ctx, env, conversation)
	if err != nil {
		return err
	}
	if pod.UID != original.UID {
		return fmt.Errorf("P5 hot execution replaced the compute Pod")
	}
	clock, err := a.postgresQuery(ctx, env, "SELECT count(*) FROM workspaces w WHERE conversation_id='"+conversation+"' AND last_used_at>=(SELECT max(created_at) FROM runs WHERE conversation_id=w.conversation_id);")
	if err != nil || strings.TrimSpace(clock) != "1" {
		return fmt.Errorf("P5 hot execution did not reset the durable idle clock: %v", err)
	}
	if cold > 180*time.Second || hot > 30*time.Second {
		return fmt.Errorf("P5 startup budget exceeded: cold=%s hot=%s", cold, hot)
	}
	data, _ := json.MarshalIndent(map[string]any{"cold_command_ms": cold.Milliseconds(), "hot_command_ms": hot.Milliseconds(), "cold_budget_ms": 180000, "hot_budget_ms": 30000, "same_pod_uid": true, "pod_uid": pod.UID}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "p5-workspace-reuse.json"), data)
}

func (a *App) verifyP5ComputeRecoveryAndIdle(ctx context.Context, env *E2EEnvironment, conversation string) error {
	before, err := a.p5WorkspacePod(ctx, env, conversation)
	if err != nil {
		return err
	}
	container := ""
	for _, item := range before.Spec.Containers {
		if len(item.Command) == 1 && item.Command[0] == "/usr/local/bin/argus-workspace-supervisor" {
			container = item.Name
		}
	}
	if container == "" {
		return fmt.Errorf("P5 trusted supervisor container absent")
	}
	if err := a.p5CrashActiveSupervisor(ctx, env, conversation, before, container); err != nil {
		return err
	}
	if err := a.p5BashProof(ctx, env, conversation, "supervisor-recovery", "test -s /workspace/report.txt && test \"$(cat /workspace/crash-started)\" = x && printf crash-recovery-proved"); err != nil {
		return err
	}
	after, err := a.p5WorkspacePod(ctx, env, conversation)
	if err != nil {
		return err
	}
	if after.UID == before.UID {
		return fmt.Errorf("P5 crashed workload was reused")
	}
	// Do not modify timestamps or delete the Pod: observe natural idle expiry.
	deadline := time.Now().Add(100 * time.Second)
	retired := false
	for time.Now().Before(deadline) {
		value, err := a.postgresQuery(ctx, env, "SELECT (active_pod_name IS NULL)::text FROM workspaces WHERE conversation_id='"+conversation+"' AND status='ready';")
		if err != nil {
			return err
		}
		if strings.TrimSpace(value) == "true" {
			retired = true
			break
		}
		if err := waitContext(ctx, 2*time.Second); err != nil {
			return err
		}
	}
	if !retired {
		return fmt.Errorf("P5 natural idle expiry did not clear the compute attachment")
	}
	pods, err := env.Kube.Client.CoreV1().Pods(env.SandboxNS).List(ctx, metav1.ListOptions{LabelSelector: "argus.io/workspace-id=" + before.Labels["argus.io/workspace-id"]})
	if err != nil || len(pods.Items) != 0 {
		return fmt.Errorf("P5 idle writer still present: %v", err)
	}
	if err := a.p5BashProof(ctx, env, conversation, "idle-remount", "test -s /workspace/report.txt && printf idle-retention-proved"); err != nil {
		return err
	}
	data, _ := json.Marshal(map[string]any{"supervisor_crash_recovered": true, "old_pod_uid": before.UID, "replacement_pod_uid": after.UID, "natural_idle_seconds": 60, "idle_pods": 0, "remounted_file_preserved": true})
	return writePrivate(filepath.Join(env.Options.Artifacts, "p5-workspace-recovery.json"), data)
}
