package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (a *App) templatePresentationRecovery(ctx context.Context, env *E2EEnvironment, convo string, calls []string) error {
	client, _ := scenarioHTTP(env)
	before := map[string]string{}
	for index, call := range calls {
		data, err := client.JSON(ctx, fmt.Sprintf("templates-before-restart-%d", index), "enterprise", http.MethodPost, "/conversations/"+convo+"/tool-presentations/"+call+"/read", 200, map[string]any{"operation": "result.detail", "parameters": map[string]any{}}, enterpriseHeaders(env, ""))
		if err != nil || data["status"] != "ready" {
			return fmt.Errorf("presentation not ready before restart: %s %v", call, err)
		}
		raw, _ := json.Marshal(data)
		before[call] = string(raw)
	}
	pods, err := env.Kube.Client.CoreV1().Pods(env.SystemNS).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=argus-worker"})
	if err != nil || len(pods.Items) != 1 {
		return fmt.Errorf("expected one owned Worker: %v", err)
	}
	pod := pods.Items[0]
	if err = env.Kube.VerifyDeploymentPod(ctx, env.SystemNS, "argus-worker", env.ReleaseID, pod); err != nil {
		return err
	}
	zero, uid := int64(0), pod.UID
	if err = env.Kube.Client.CoreV1().Pods(env.SystemNS).Delete(ctx, pod.Name, metav1.DeleteOptions{GracePeriodSeconds: &zero, Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil {
		return err
	}
	if err = env.Kube.WaitDeployment(ctx, env.SystemNS, "argus-worker", 3*time.Minute); err != nil {
		return err
	}
	for index, call := range calls {
		data, err := client.JSON(ctx, fmt.Sprintf("templates-after-restart-%d", index), "enterprise", http.MethodPost, "/conversations/"+convo+"/tool-presentations/"+call+"/read", 200, map[string]any{"operation": "result.detail", "parameters": map[string]any{}}, enterpriseHeaders(env, ""))
		if err != nil {
			return err
		}
		raw, _ := json.Marshal(data)
		if string(raw) != before[call] {
			return fmt.Errorf("sealed presentation changed after restart: %s", call)
		}
	}
	duplicates, err := a.postgresQuery(ctx, env, "SELECT count(*) FROM (SELECT payload->>'presentation_id' FROM conversation_events WHERE conversation_id='"+convo+"' AND event_type='tool_presentation' AND payload->>'status'='data_ready' GROUP BY payload->>'presentation_id' HAVING count(*) <> 1) repeated;")
	if err != nil || duplicates != "0" {
		return fmt.Errorf("duplicate ready notifications after restart: %s %v", duplicates, err)
	}
	proof, _ := json.MarshalIndent(map[string]any{"kind": "template-v2-worker-recovery", "worker_uid_before": uid, "presentation_count": len(calls), "unchanged_reads": len(before), "duplicate_ready_events": 0}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "tool-template-v2-worker-recovery.json"), proof)
}
