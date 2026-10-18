package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/kakj-go/Argus/internal/remoteexecution"
)

func (a *App) plan7ModelRecovery(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) error {
	model, err := a.plan7Model(ctx, env)
	if err != nil {
		return err
	}
	conversation, err := a.plan7ModelConversation(ctx, env, model, "lost-context-recovery")
	if err != nil {
		return err
	}
	const initialize = `p7_result="$ARGUS_OUTPUT_DIR/effects"; printf 'first\n' >> "$p7_result"; printf '%s %s\n' "$PPID" "$BASHPID" > "$ARGUS_OUTPUT_DIR/processes"; printf 'result_path=%s\n' "$p7_result"`
	initializeJSON, _ := json.Marshal(initialize)
	run, err := a.plan7StartModelMessage(ctx, env, conversation, "recovery-initialize", fmt.Sprintf("对主机 %s 创建一次 Bash connect，等待确认后执行，然后报告结果。下面是 script 字段的完整 JSON 字符串；工具参数 script 必须等于它解码后的值。逐字保留分号、空格、引号及反斜杠，不增加换行、注释、防御语句或额外命令；不要改写成语义等价的脚本，也不要执行其他远程操作。\n```json\n%s\n```", host.HostID, initializeJSON))
	if err != nil {
		return err
	}
	first, err := a.plan7FinishModel(ctx, env, conversation, run, "recovery-initialize", host.HostID, 1, exactPlan7Scripts(initialize))
	if err != nil {
		return err
	}
	if len(first) != 1 {
		return fmt.Errorf("recovery fixture has no separately approved initialization")
	}
	firstStep := first[0].Spec.Steps[0]
	firstRoot := "/tmp/argus/" + firstStep.SessionID.String() + "/" + firstStep.Generation.String() + "/" + first[0].ID.String() + "/output"
	if err = a.plan7SetMode(ctx, env, conversation, "plan"); err != nil {
		return err
	}
	const uncertain = `printf 'second\n' >> "$p7_result"; printf 'reached\n' > "$ARGUS_OUTPUT_DIR/second.marker"; sleep 120`
	const remaining = `printf 'third\n' >> "$p7_result"`
	fixtureJSON, _ := json.Marshal([]map[string]any{{"script": uncertain, "timeout_seconds": 180}, {"script": remaining, "timeout_seconds": 60}})
	prompt := fmt.Sprintf("继续主机 %s 的同一 Bash 上下文，创建严格两个步骤的固定计划，先等确认。下方 JSON 给出了每步 script 和 timeout_seconds 的确切值；逐字使用解码后的 script，不增加换行、注释、防御语句或改写为等价脚本。补充目标和说明即可。最终 effects 应依次只有 first、second、third 各一行。若连接或解释器失效，必须先基于服务端已成功、未知和未执行事实分析；重新确认恢复检查和后续操作，不重放已成功修改，不盲目重试结果未知的写入，不声称恢复所有旧变量。\n```json\n%s\n```", host.HostID, fixtureJSON)
	run, err = a.plan7StartModelMessage(ctx, env, conversation, "recovery-plan", prompt)
	if err != nil {
		return err
	}
	plan, err := a.plan7WaitRequest(ctx, env, conversation, 2, func(v remoteexecution.View) bool {
		return v.Spec.RunID.String() == run && v.Status == "awaiting_confirmation"
	}, run)
	if err != nil {
		return err
	}
	if plan.Spec.Kind != "plan" || exactPlan7Scripts(uncertain, remaining)(plan) != nil {
		if err = savePlan7RejectedPreview(env, "recovery-plan", plan); err != nil {
			return err
		}
		return fmt.Errorf("recovery fixture plan was changed")
	}
	for _, step := range plan.Spec.Steps {
		if step.Target.ResourceID.String() != host.HostID || step.Interpreter != "bash" || step.SessionID != firstStep.SessionID || step.Generation != firstStep.Generation {
			return fmt.Errorf("recovery fixture did not retain the approved target context")
		}
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	if _, err = client.JSON(ctx, "p7-recovery-compact", "enterprise", http.MethodPost, "/runs/"+run+"/compact", 202, nil, enterpriseHeaders(env, "p7-recovery-compact")); err != nil {
		return err
	}
	if err = a.waitPostgresValue(ctx, env, "SELECT count(*) FROM context_snapshots WHERE conversation_id='"+conversation+"' AND status='active';", "1", 3*time.Minute); err != nil {
		return err
	}
	if err = a.plan7NoDispatchBeforeApproval(ctx, env, plan); err != nil {
		return err
	}
	if err = a.plan7Approve(ctx, env, plan); err != nil {
		return err
	}
	marker := "/tmp/argus/" + firstStep.SessionID.String() + "/" + firstStep.Generation.String() + "/" + plan.ID.String() + "/output/second.marker"
	deadline := time.Now().Add(time.Minute)
	for {
		if text, e := a.plan6DockerScript(ctx, host.Docker, host.Docker.Host, "cat "+marker+"\n"); e == nil && strings.TrimSpace(text) == "reached" {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("uncertain step never reached its real effect")
		}
		if err = waitContext(ctx, 250*time.Millisecond); err != nil {
			return err
		}
	}
	// Only the exact helper and Bash process group recorded by the approved
	// initialization may be faulted, within this run's owned Docker container.
	kill := fmt.Sprintf(`set -eu
read -r supervisor shell < %[1]s/processes
case "$supervisor:$shell" in *[!0-9:]*|'') exit 71;; esac
test "$supervisor" -gt 1
test "$shell" -gt 1
command=$(tr '\000' ' ' < /proc/$supervisor/cmdline)
case "$command" in *argus-connector\ remote-runtime*) ;; *) exit 72;; esac
test "$(awk '{print $4}' /proc/$shell/stat)" = "$supervisor"
test "$(awk '{print $5}' /proc/$shell/stat)" = "$shell"
kill -KILL "$supervisor"
kill -KILL -- "-$shell" 2>/dev/null || true
printf 'owned interpreter stopped after the second effect\n'
`, firstRoot)
	if _, err = a.plan6DockerScript(ctx, host.Docker, host.Docker.Host, kill); err != nil {
		return err
	}
	unknown, err := a.plan7WaitRequest(ctx, env, conversation, 2, func(v remoteexecution.View) bool {
		return v.ID == plan.ID && (v.Status == "result_unknown" || v.Status == "paused_for_recovery")
	})
	if err != nil {
		return err
	}
	if len(unknown.Results) != 2 || unknown.Results[0].Status != "result_unknown" || unknown.Results[1].Status != "skipped" {
		return fmt.Errorf("lost receipt did not preserve unknown and skipped facts")
	}
	recovery, err := a.plan7FinishModel(ctx, env, conversation, run, "lost-context-recovery", host.HostID, 3, func(v remoteexecution.View) error {
		for _, step := range v.Spec.Steps {
			if step.Script == initialize || step.Script == uncertain {
				return fmt.Errorf("model attempted to replay a completed or uncertain effect")
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	linked := false
	for _, v := range recovery {
		if v.Spec.RecoveryOfID != nil && *v.Spec.RecoveryOfID == plan.ID {
			linked = true
			for _, step := range v.Spec.Steps {
				if step.Generation == firstStep.Generation {
					return fmt.Errorf("recovery reused the lost interpreter")
				}
			}
		}
	}
	if !linked {
		return fmt.Errorf("GLM recovery did not link its new confirmation to the lost plan")
	}
	effects, err := a.plan6DockerScript(ctx, host.Docker, host.Docker.Host, "cat "+firstRoot+"/effects\n")
	if err != nil || strings.TrimSpace(effects) != "first\nsecond\nthird" {
		return fmt.Errorf("recovery replayed or missed a side effect: %v", err)
	}
	views, err := a.plan7Requests(ctx, env, conversation)
	if err != nil {
		return err
	}
	for _, v := range views {
		if v.ID == plan.ID && v.Results[0].Status != "result_unknown" {
			return fmt.Errorf("recovery fabricated the old missing receipt")
		}
	}
	used, err := a.postgresQuery(ctx, env, "SELECT count(*) FROM model_calls WHERE run_id='"+run+"' AND call_kind='inference' AND context_snapshot_id IS NOT NULL AND status='succeeded';")
	if err != nil || strings.TrimSpace(used) == "0" {
		return fmt.Errorf("recovery model did not use the actual compacted context: %v", err)
	}
	proof, _ := json.MarshalIndent(map[string]any{"model_id": model, "conversation_id": conversation, "run_id": run, "initialization": first[0], "unknown_plan": unknown, "recovery_approvals": recovery, "observed_effects": effects, "old_receipt_remains_unknown": true, "compacted_model_calls": used}, "", "  ")
	env.State.Values["p7_recovery_conversation"] = conversation
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan7-model-recovery.json"), proof)
}

func exactPlan7Scripts(scripts ...string) func(remoteexecution.View) error {
	return func(v remoteexecution.View) error {
		if len(v.Spec.Steps) != len(scripts) {
			return fmt.Errorf("unexpected bounded fixture step count")
		}
		for i, script := range scripts {
			if strings.TrimSpace(v.Spec.Steps[i].Script) != script {
				return fmt.Errorf("fixture script was changed")
			}
		}
		return nil
	}
}
