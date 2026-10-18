package argusdev

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kakj-go/Argus/internal/remoteexecution"
)

// The real provider chooses tools and handles the file exchange. Approval is
// limited to exact scripts with effects restricted to the run-owned directories.
func (a *App) plan7ModelPlatforms(ctx context.Context, env *E2EEnvironment) error {
	model, err := a.plan7Model(ctx, env)
	if err != nil {
		return err
	}
	host := env.State.Values["p7_windows_host_id"]
	if host == "" {
		return fmt.Errorf("Windows enrollment fixture missing")
	}
	conversation, err := a.plan7ModelConversation(ctx, env, model, "windows-state-and-files")
	if err != nil {
		return err
	}
	scripts := []string{
		`$p7ModelState = Get-Content -Raw -LiteralPath (Join-Path $env:ARGUS_INPUT_DIR 'state.json') | ConvertFrom-Json; if ($p7ModelState.Count -ne 41) { throw 'input mismatch' }; [IO.File]::WriteAllText((Join-Path $env:ARGUS_OUTPUT_DIR 'state.txt'),[string]$p7ModelState.Count,[Text.UTF8Encoding]::new($false))`,
		`$p7ModelState.Count += 1; if ($p7ModelState.Count -ne 42) { throw 'context mismatch' }; [IO.File]::WriteAllText((Join-Path $env:ARGUS_OUTPUT_DIR 'state.txt'),[string]$p7ModelState.Count,[Text.UTF8Encoding]::new($false))`,
	}
	var previous remoteexecution.Step
	for index, script := range scripts {
		label := fmt.Sprintf("windows-state-%d", index+1)
		prompt := fmt.Sprintf("主机 %s 是本轮 Windows 11 测试资源。通过 connect 使用 powershell 解释器，逐字申请下面唯一脚本，等待确认，之后在 Workspace 检查自动回传的 state.txt 并报告实际数值，不要添加任何其他远程命令。\n%s", host, script)
		if index == 0 {
			prompt = "先在当前 Workspace 的 remote/input/state.json 写入 {\"Count\":41}，由平台自动传递。\n" + prompt
		}
		run, e := a.plan7StartModelMessage(ctx, env, conversation, label, prompt)
		if e != nil {
			return e
		}
		approved, e := a.plan7FinishScopedModel(ctx, env, conversation, run, label, 1, func(s remoteexecution.Step) bool {
			return s.Target.ResourceType == "host" && s.Target.ResourceID.String() == host && s.Interpreter == "powershell"
		}, exactPlan7Scripts(script))
		if e != nil {
			return e
		}
		if len(approved) != 1 {
			return fmt.Errorf("Windows model skipped its explicit confirmation")
		}
		view, e := a.plan7WaitRequest(ctx, env, conversation, index+1, func(v remoteexecution.View) bool { return v.ID == approved[0].ID && v.Status == "succeeded" })
		if e != nil {
			return e
		}
		step := view.Spec.Steps[0]
		if index > 0 && (step.SessionID != previous.SessionID || step.Generation != previous.Generation) {
			return fmt.Errorf("Windows model lost shared object state")
		}
		previous = step
		file := filepath.Join(env.WorkDir, "plan7-windows-exchange", step.SessionID.String(), step.Generation.String(), view.ID.String(), "output", "state.txt")
		value, e := os.ReadFile(file)
		if e != nil || string(value) != fmt.Sprint(41+index) {
			return fmt.Errorf("Windows model real file mismatch: %v", e)
		}
		if e = a.plan7ModelCompletedEvidence(ctx, env, conversation, run, label, view, "state.txt", string(value)); e != nil {
			return e
		}
	}
	env.State.Values["p7_windows_model_conversation"] = conversation
	return a.plan7KubernetesModel(ctx, env, model)
}

func (a *App) plan7KubernetesModel(ctx context.Context, env *E2EEnvironment, model string) error {
	cluster, namespace := env.State.Values["p7_cluster_id"], env.State.Values["p7_target_namespace"]
	pod, uid := env.State.Values["p7_target_pod"], env.State.Values["p7_target_pod_uid"]
	if cluster == "" || namespace == "" || pod == "" || uid == "" {
		return fmt.Errorf("Kubernetes model target missing")
	}
	conversation, err := a.plan7ModelConversation(ctx, env, model, "kubernetes-state-and-files")
	if err != nil {
		return err
	}
	cases := []struct{ interpreter, script, filename, value string }{
		{"kubectl", "kubectl get configmap plan7-marker -n " + namespace + " -o jsonpath='{.data.value}' > \"$ARGUS_OUTPUT_DIR/marker.txt\"", "marker.txt", "approved"},
		{"pod_bash", `p7_model_value=41; printf '%s' "$p7_model_value" > "$ARGUS_OUTPUT_DIR/state.txt"`, "state.txt", "41"},
		{"pod_bash", `p7_model_value=$((p7_model_value+1)); test "$p7_model_value" = 42; printf '%s' "$p7_model_value" > "$ARGUS_OUTPUT_DIR/state.txt"`, "state.txt", "42"},
	}
	var previous remoteexecution.Step
	for index, tc := range cases {
		label := fmt.Sprintf("kubernetes-model-%d", index+1)
		target := fmt.Sprintf("集群 %s、namespace %s，解释器 %s", cluster, namespace, tc.interpreter)
		if tc.interpreter == "pod_bash" {
			target += fmt.Sprintf("，固定 Pod %s，Pod UID %s，container target", pod, uid)
		}
		prompt := fmt.Sprintf("对本轮隔离测试目标 %s 创建一次 connect，必须逐字使用以下脚本。等待本次确认后执行，并在 Workspace 检查自动回传的 %s，报告实际内容和来源，不申请其他远程操作。\n%s", target, tc.filename, tc.script)
		run, e := a.plan7StartModelMessage(ctx, env, conversation, label, prompt)
		if e != nil {
			return e
		}
		approved, e := a.plan7FinishScopedModel(ctx, env, conversation, run, label, 1, func(s remoteexecution.Step) bool {
			if s.Target.ResourceType != "kubernetes_cluster" || s.Target.ResourceID.String() != cluster || s.Target.Namespace != namespace || s.Interpreter != tc.interpreter {
				return false
			}
			return tc.interpreter != "pod_bash" || (s.Target.Pod == pod && s.Target.PodUID == uid && s.Target.Container == "target")
		}, exactPlan7Scripts(tc.script))
		if e != nil {
			return e
		}
		if len(approved) != 1 {
			return fmt.Errorf("Kubernetes model skipped confirmation")
		}
		view, e := a.plan7WaitRequest(ctx, env, conversation, index+1, func(v remoteexecution.View) bool { return v.ID == approved[0].ID && v.Status == "succeeded" })
		if e != nil {
			return e
		}
		step := view.Spec.Steps[0]
		if index == 2 && (step.SessionID != previous.SessionID || step.Generation != previous.Generation) {
			return fmt.Errorf("Pod model lost its shared variable")
		}
		previous = step
		if e = a.plan7ModelCompletedEvidence(ctx, env, conversation, run, label, view, tc.filename, tc.value); e != nil {
			return e
		}
	}
	env.State.Values["p7_kubernetes_model_conversation"] = conversation
	return nil
}

func (a *App) plan7ModelCompletedEvidence(ctx context.Context, env *E2EEnvironment, conversation, run, label string, view remoteexecution.View, file, value string) error {
	if len(view.Results) != 1 || !view.Results[0].Terminated || !view.Results[0].OutputsCollected {
		return fmt.Errorf("model target lacks complete output receipts")
	}
	found := false
	for _, f := range view.Results[0].OutputFiles {
		if f.Path == "results/"+file && f.Hash == fmt.Sprintf("%x", sha256.Sum256([]byte(value))) {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("model target output hash mismatch")
	}
	analysis, err := a.postgresQuery(ctx, env, fmt.Sprintf("SELECT count(*) FROM tool_calls WHERE run_id='%s' AND tool_id IN ('read','bash') AND status='succeeded' AND created_at>(SELECT max(completed_at) FROM remote_execution_steps WHERE request_id='%s')", run, view.ID))
	if err != nil || strings.TrimSpace(analysis) == "0" {
		return fmt.Errorf("model did not inspect returned file in Workspace: %v", err)
	}
	conclusion, err := a.planV2ModelReply(ctx, env, run)
	if err != nil || !strings.Contains(conclusion, value) {
		return fmt.Errorf("model omitted the actual observed file content: %v", err)
	}
	raw, _ := json.MarshalIndent(map[string]any{"scope": "real model, real API, approved platform execution and Workspace analysis", "conversation_id": conversation, "run_id": run, "view": view, "conclusion": conclusion}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan7-"+label+"-result.json"), raw)
}
