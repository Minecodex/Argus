package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/kakj-go/Argus/internal/remoteexecution"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (a *App) plan7WorkerFault(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) (failure error) {
	namespace, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, env.SystemNS, metav1.GetOptions{})
	if err != nil || namespace.Labels["argus.io/release-id"] != env.ReleaseID {
		return fmt.Errorf("Plan7 worker fault requires its owned namespace: %v", err)
	}
	worker, err := env.Kube.Client.AppsV1().Deployments(env.SystemNS).Get(ctx, "argus-worker", metav1.GetOptions{})
	if err != nil || worker.UID == "" || worker.Spec.Replicas == nil {
		return fmt.Errorf("Plan7 worker identity unavailable: %v", err)
	}
	replicas := *worker.Spec.Replicas
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
		defer cancel()
		current, e := env.Kube.Client.AppsV1().Deployments(env.SystemNS).Get(cleanup, worker.Name, metav1.GetOptions{})
		if e == nil && current.UID != worker.UID {
			e = fmt.Errorf("refusing to restore a replaced worker Deployment")
		}
		if e == nil {
			e = env.Kube.ScaleDeployment(cleanup, env.SystemNS, worker.Name, replicas)
		}
		if e == nil {
			e = env.Kube.WaitDeployment(cleanup, env.SystemNS, worker.Name, 2*time.Minute)
		}
		failure = errors.Join(failure, e)
	}()
	conversation, err := a.p5Conversation(ctx, env, "Plan7 durable worker restart")
	if err != nil {
		return err
	}
	steps := []map[string]any{
		plan7ToolStep(host.HostID, "bash", `printf 'first\n' >> "$ARGUS_OUTPUT_DIR/effects"`),
		plan7ToolStep(host.HostID, "bash", `printf 'second\n' >> "$ARGUS_OUTPUT_DIR/effects"; sleep 30`),
		plan7ToolStep(host.HostID, "bash", `printf 'third\n' >> "$ARGUS_OUTPUT_DIR/effects"`),
	}
	run, err := a.p5StartRun(ctx, env, conversation, "plan7-worker-fault", plan7PlanToolSequence(plan7PreviewTool("plan", steps...)), []string{})
	if err != nil {
		return err
	}
	view, err := a.plan7WaitRequest(ctx, env, conversation, 1, func(v remoteexecution.View) bool { return v.Status == "awaiting_confirmation" }, run)
	if err != nil {
		return err
	}
	if err = a.plan7NoDispatchBeforeApproval(ctx, env, view); err != nil {
		return err
	}
	if err = a.plan7Approve(ctx, env, view); err != nil {
		return err
	}
	step := view.Spec.Steps[0]
	path := "/tmp/argus/" + step.SessionID.String() + "/" + step.Generation.String() + "/" + view.ID.String() + "/output/effects"
	deadline := time.Now().Add(time.Minute)
	for {
		text, e := a.plan6DockerScript(ctx, host.Docker, host.Docker.Host, "cat "+path+"\n")
		if e == nil && strings.TrimSpace(text) == "first\nsecond" {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("worker fault never reached its known/active boundary")
		}
		if err = waitContext(ctx, 250*time.Millisecond); err != nil {
			return err
		}
	}
	pods, err := env.Kube.Client.CoreV1().Pods(env.SystemNS).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=argus-worker"})
	if err != nil || len(pods.Items) != 1 {
		return fmt.Errorf("worker fault expected one owned process: %v", err)
	}
	pod := pods.Items[0]
	if err = env.Kube.VerifyDeploymentPod(ctx, env.SystemNS, worker.Name, env.ReleaseID, pod); err != nil {
		return err
	}
	zero := int64(0)
	uid := pod.UID
	if err = env.Kube.Client.CoreV1().Pods(env.SystemNS).Delete(ctx, pod.Name, metav1.DeleteOptions{GracePeriodSeconds: &zero, Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil {
		return err
	}
	password, err := dataCredentialValue(ctx, env, "redis-password")
	if err != nil {
		return err
	}
	_, err = env.Kube.Exec(ctx, env.SystemNS, "app.kubernetes.io/name=argus-redis", "redis", "redis-cli", "-a", password, "FLUSHALL")
	password = ""
	if err != nil {
		return err
	}
	if err = env.Kube.ScaleDeployment(ctx, env.SystemNS, worker.Name, 2); err != nil {
		return err
	}
	if err = env.Kube.WaitDeployment(ctx, env.SystemNS, worker.Name, 3*time.Minute); err != nil {
		return err
	}
	result, err := a.plan7WaitRequest(ctx, env, conversation, 1, func(v remoteexecution.View) bool {
		return v.ID == view.ID && (v.Status == "succeeded" || v.Status == "result_unknown" || v.Status == "paused_for_recovery")
	})
	if err != nil {
		return err
	}
	if len(result.Results) != 3 || result.Results[0].Status != "succeeded" {
		return fmt.Errorf("worker restart lost the successful first step")
	}
	effects, err := a.plan6DockerScript(ctx, host.Docker, host.Docker.Host, "cat "+path+"\n")
	if err != nil {
		return err
	}
	observed, _ := json.MarshalIndent(map[string]any{"result": result, "actual_effects": effects, "worker_uid": worker.UID, "killed_pod_uid": pod.UID}, "", "  ")
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan7-worker-fault-observed.json"), observed); err != nil {
		return err
	}
	want, wantCommands, err := plan7WorkerFaultExpectation(result)
	if err != nil {
		return err
	}
	if strings.TrimSpace(effects) != want {
		return fmt.Errorf("worker restart replayed or lost an effect: %q", effects)
	}
	commands, err := a.postgresQuery(ctx, env, "SELECT count(*) FROM connector_commands WHERE command_type='remote_execution_batch' AND payload->>'requestId'='"+view.ID.String()+"';")
	if err != nil {
		return err
	}
	if strings.TrimSpace(commands) != fmt.Sprint(wantCommands) {
		return fmt.Errorf("worker restart duplicated command records: %s", commands)
	}
	if wantCommands == 2 {
		unsent, e := a.postgresQuery(ctx, env, "SELECT count(*) FROM remote_execution_steps WHERE request_id='"+view.ID.String()+"' AND sequence=3 AND command_id<>'';")
		if e != nil || strings.TrimSpace(unsent) != "0" {
			return fmt.Errorf("worker restart dispatched the paused final step: %v", e)
		}
	}
	raw, _ := json.MarshalIndent(map[string]any{"conversation_id": conversation, "run_id": run, "worker_uid": worker.UID, "killed_pod_uid": pod.UID, "redis_flush": true, "two_workers": true, "result": result, "actual_effects": effects, "durable_commands": commands}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan7-worker-fault.json"), raw)
}

func plan7WorkerFaultExpectation(result remoteexecution.View) (string, int, error) {
	if len(result.Results) != 3 || result.Results[0].Status != "succeeded" {
		return "", 0, fmt.Errorf("worker restart lost the successful first step")
	}
	if result.Status == "succeeded" && result.Results[1].Status == "succeeded" && result.Results[2].Status == "succeeded" {
		return "first\nsecond\nthird", 3, nil
	}
	// Redis loss can also break a transport after a known receipt. Preparing
	// the next step then pauses before dispatch, instead of marking it skipped
	// behind an earlier unknown step. Both must preserve exactly two effects.
	if result.Status == "paused_for_recovery" || result.Status == "result_unknown" {
		last := result.Results[2].Status
		if last == "skipped" || (result.Status == "paused_for_recovery" && result.Results[1].Status == "succeeded" && last == "paused_for_recovery") {
			return "first\nsecond", 2, nil
		}
	}
	return "", 0, fmt.Errorf("worker restart did not preserve a completed or safely paused plan")
}
