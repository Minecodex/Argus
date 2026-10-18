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

	"github.com/kakj-go/Argus/internal/remoteexecution"
)

func (a *App) plan7ModelActivity(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) (failure error) {
	model, err := a.plan7Model(ctx, env)
	if err != nil {
		return err
	}
	other, err := a.plan7ModelConversation(ctx, env, model, "concurrent-operation")
	if err != nil {
		return err
	}
	const script = "p7_private=other\nprintf 'running\\n' > \"$ARGUS_OUTPUT_DIR/activity.marker\"\nsleep 900"
	prompt := fmt.Sprintf("为主机 %s 申请一个 Bash connect 批次，脚本必须逐字为下面这段，只创建预览并等待确认。不要改写命令或添加其他步骤。超时设置为 1200 秒。\n%s", host.HostID, script)
	run, err := a.plan7StartModelMessage(ctx, env, other, "concurrent-operation", prompt)
	if err != nil {
		return err
	}
	preview, err := a.plan7WaitRequest(ctx, env, other, 1, func(v remoteexecution.View) bool {
		return v.Spec.RunID.String() == run && v.Status == "awaiting_confirmation"
	}, run)
	if err != nil {
		return err
	}
	if len(preview.Spec.Steps) != 1 || preview.Spec.Steps[0].Target.ResourceID.String() != host.HostID || preview.Spec.Steps[0].Interpreter != "bash" || preview.Spec.Steps[0].TimeoutSeconds != 1200 || strings.TrimSpace(preview.Spec.Steps[0].Script) != script {
		return fmt.Errorf("GLM concurrent fixture differs from the approved bounded script")
	}
	if err = a.plan7NoDispatchBeforeApproval(ctx, env, preview); err != nil {
		return err
	}
	if err = a.plan7Approve(ctx, env, preview); err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
		defer cancel()
		client, e := scenarioHTTP(env)
		if e == nil {
			_, e = client.JSON(cleanup, "p7-stop-concurrent-fixture", "enterprise", http.MethodPost, "/conversations/"+other+"/remote-executions/"+preview.ID.String()+"/stop", 200, nil, enterpriseHeaders(env, "p7-stop-concurrent-fixture"))
		}
		if e == nil {
			_, e = a.plan7WaitRequest(cleanup, env, other, 1, func(v remoteexecution.View) bool {
				return v.ID == preview.ID && (v.Status == "cancelled" || v.Status == "succeeded")
			})
		}
		failure = errors.Join(failure, e)
	}()
	step := preview.Spec.Steps[0]
	marker := "/tmp/argus/" + step.SessionID.String() + "/" + step.Generation.String() + "/" + preview.ID.String() + "/output/activity.marker"
	deadline := time.Now().Add(45 * time.Second)
	for {
		if value, e := a.plan6DockerScript(ctx, host.Docker, host.Docker.Host, "cat "+marker+"\n"); e == nil && strings.TrimSpace(value) == "running" {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("concurrent operation never reached the actual remote process")
		}
		if err = waitContext(ctx, 250*time.Millisecond); err != nil {
			return err
		}
	}
	observer, err := a.plan7ModelConversation(ctx, env, model, "private-activity-context")
	if err != nil {
		return err
	}
	const observerScript = "test -z \"${p7_private+x}\" && printf 'independent\\n' > \"$ARGUS_OUTPUT_DIR/overlap.marker\""
	observerRun, err := a.plan7StartModelMessage(ctx, env, observer, "private-activity-context", fmt.Sprintf("请为授权主机 %s 创建一次 Bash connect，逐字使用下面脚本（超时 60 秒），等确认后执行，再在 Workspace 查看自动回传的文件并给出简短结论。不得停止其他执行，不机械复述服务端内部数据块，不追加其他远程操作。\n%s", host.HostID, observerScript))
	if err != nil {
		return err
	}
	approved, err := a.plan7FinishModel(ctx, env, observer, observerRun, "private-activity-context", host.HostID, 1, func(v remoteexecution.View) error {
		if len(v.Spec.Steps) != 1 || strings.TrimSpace(v.Spec.Steps[0].Script) != observerScript || v.Spec.Steps[0].TimeoutSeconds != 60 {
			return fmt.Errorf("observer command differs from its approved bounded script")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(approved) != 1 {
		return fmt.Errorf("observer did not execute its independently approved batch")
	}
	overlap, err := a.postgresQuery(ctx, env, fmt.Sprintf(`SELECT count(*) FROM remote_execution_steps a JOIN remote_execution_steps b ON a.session_id<>b.session_id JOIN remote_execution_requests r ON r.id=b.request_id WHERE a.request_id='%s' AND r.run_id='%s' AND a.started_at<=b.started_at AND b.completed_at<=coalesce(a.completed_at,now()) AND b.status='succeeded'`, preview.ID, observerRun))
	if err != nil || strings.TrimSpace(overlap) != "1" {
		return fmt.Errorf("independent resource commands did not actually overlap: %v", err)
	}
	query := fmt.Sprintf(`SELECT coalesce(json_agg(t),'[]') FROM (SELECT o.model_call_id,encode(o.projection_hash,'hex') AS projection_hash,o.authorization_version,o.activity FROM remote_execution_context_observations o JOIN model_calls c ON c.id=o.model_call_id WHERE o.run_id='%s' AND c.model_id='%s' AND c.status='succeeded' AND o.projection_hash=c.projection_hash AND o.activity->'activities' @> '[{"execution_id":"%s","status":"running"}]'::jsonb LIMIT 8) t`, observerRun, model, preview.ID)
	evidence, err := a.postgresQuery(ctx, env, query)
	if err != nil {
		return err
	}
	var observations []map[string]any
	if json.Unmarshal([]byte(evidence), &observations) != nil || len(observations) == 0 {
		return fmt.Errorf("actual GLM request did not contain the permitted concurrent operation")
	}
	if !strings.Contains(evidence, "sleep (arguments withheld)") {
		return fmt.Errorf("concurrent context omitted its bounded command description")
	}
	visible, err := a.postgresQuery(ctx, env, fmt.Sprintf(`SELECT count(*) FROM conversation_events WHERE conversation_id='%s' AND (payload::text LIKE '%%argus.remote_activity/v1%%' OR payload::text LIKE '%%"concurrent_activity"%%')`, observer))
	if err != nil || strings.TrimSpace(visible) != "0" {
		return fmt.Errorf("private concurrent projection leaked into conversation events: %v", err)
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	for _, path := range []string{"/conversations/" + observer, "/runs/" + observerRun} {
		value, e := client.JSON(ctx, "p7-activity-public-dto", "enterprise", http.MethodGet, path, 200, nil, enterpriseHeaders(env, ""))
		if e != nil {
			return e
		}
		raw, _ := json.Marshal(value)
		if strings.Contains(string(raw), "argus.remote_activity/v1") || strings.Contains(string(raw), "concurrent_activity") {
			return fmt.Errorf("private concurrent projection leaked into public DTO")
		}
	}
	env.State.Values["p7_activity_conversation"] = observer
	if err = a.runPlaywright(ctx, env, "e2e/plan7-model-real.spec.ts", map[string]string{
		"ARGUS_PLAN7_E2E": "1", "ARGUS_E2E_ARTIFACT_SUFFIX": "model",
		"ARGUS_PLAN7_USERNAME": env.State.Values["enterprise_username"], "ARGUS_PLAN7_PASSWORD": env.State.Values["enterprise_password"],
		"ARGUS_PLAN7_ACTIVITY_CHAT": observer, "ARGUS_PLAN7_OTHER_EXECUTION": preview.ID.String(),
		"ARGUS_PLAN7_REAL_REPAIR_CHAT":   env.State.Values["p7_model_conversation"],
		"ARGUS_PLAN7_REAL_RECOVERY_CHAT": env.State.Values["p7_recovery_conversation"],
	}); err != nil {
		return err
	}
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan7-model-activity.json"), []byte(evidence))
}
