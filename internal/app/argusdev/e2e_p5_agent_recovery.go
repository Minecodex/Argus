package argusdev

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Seed the durable boundary left by an interrupted worker, without deleting or
// rewriting existing ConversationEvents. The stopped deployment is owned by
// this E2E namespace; its exact original replica count is restored on all exits.
func (a *App) verifyP5AgentRecovery(ctx context.Context, env *E2EEnvironment) (failure error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	if err := a.verifyP5AdmissionBoundary(ctx, env); err != nil {
		return err
	}
	deployment, err := env.Kube.Client.AppsV1().Deployments(env.SystemNS).Get(ctx, "argus-worker", metav1.GetOptions{})
	if err != nil {
		return err
	}
	replicas := int32(1)
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}
	restored := false
	restore := func(ctx context.Context) error {
		if err := env.Kube.ScaleDeployment(ctx, env.SystemNS, "argus-worker", replicas); err != nil {
			return err
		}
		if err := env.Kube.WaitDeployment(ctx, env.SystemNS, "argus-worker", 2*time.Minute); err != nil {
			return err
		}
		restored = true
		return nil
	}
	defer func() {
		if !restored {
			cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			failure = errors.Join(failure, restore(cleanup))
		}
	}()
	if err := env.Kube.ScaleDeployment(ctx, env.SystemNS, "argus-worker", 0); err != nil {
		return err
	}
	if err := env.Kube.WaitDeployment(ctx, env.SystemNS, "argus-worker", time.Minute); err != nil {
		return err
	}
	convo, err := a.p5Conversation(ctx, env, "Preview restart boundary")
	if err != nil {
		return err
	}
	run, err := a.p5StartRun(ctx, env, convo, "preview-restart", nil, []string{})
	if err != nil {
		return err
	}
	preview, err := client.JSON(ctx, "p5-recovery-preview", "enterprise", http.MethodPost, "/enterprise/hosts/actions/preview-create", 201, p5RecoveryPreviewInput(env.Options.RunID), enterpriseHeaders(env, "p5-recovery-preview"))
	if err != nil {
		return err
	}
	ref, err := stringField(preview, "action_ref")
	if err != nil {
		return err
	}
	if _, err := a.postgresQuery(ctx, env, "UPDATE pending_actions SET run_id='"+run+"' WHERE action_ref='"+ref+"' AND enterprise_id='"+env.State.Values["enterprise_id"]+"';"); err != nil {
		return err
	}
	if err := a.p5SeedFinishedTool(ctx, env, run, map[string]any{"action_ref": ref}); err != nil {
		return err
	}
	failedConvo, err := a.p5Conversation(ctx, env, "Compaction failure terminal")
	if err != nil {
		return err
	}
	failedRun, err := a.p5StartRun(ctx, env, failedConvo, "compaction-failure", nil, []string{})
	if err != nil {
		return err
	}
	if err := a.p5SeedFinishedTool(ctx, env, failedRun, map[string]any{"summary": strings.Repeat("x", 25000)}); err != nil {
		return err
	}
	if _, err := a.postgresQuery(ctx, env, "UPDATE runs SET status='waiting_system',stop_reason='context_compaction' WHERE id='"+failedRun+"'; UPDATE runtime_tasks SET queue='compaction',payload=jsonb_set(payload,'{reason}','\"hard_limit\"') WHERE run_id='"+failedRun+"';"); err != nil {
		return err
	}
	if err := restore(ctx); err != nil {
		return err
	}
	if err := a.waitRunStatus(ctx, env, run, "waiting_input", time.Minute); err != nil {
		return err
	}
	if err := a.waitPostgresValue(ctx, env, "SELECT count(*) FROM conversation_events WHERE run_id='"+run+"' AND event_type='pending_action_created';", "1", time.Second); err != nil {
		return err
	}
	if err := a.waitPostgresValue(ctx, env, "SELECT count(*) FROM model_calls WHERE run_id='"+run+"';", "0", time.Second); err != nil {
		return err
	}
	if _, err := client.JSON(ctx, "p5-recovery-cancel", "enterprise", http.MethodPost, "/enterprise/pending-actions/"+ref+"/cancel", 200, nil, enterpriseHeaders(env, "p5-recovery-cancel")); err != nil {
		return err
	}
	if err := a.waitRunStatus(ctx, env, run, "cancelled", 10*time.Second); err != nil {
		return err
	}
	if err := a.waitRunStatus(ctx, env, failedRun, "failed", time.Minute); err != nil {
		return err
	}
	if err := a.waitPostgresValue(ctx, env, "SELECT count(*) FROM conversation_events WHERE run_id='"+failedRun+"' AND event_type='run_state_changed' AND payload->>'status'='failed';", "1", time.Second); err != nil {
		return err
	}
	env.State.Values["p5_compaction_failure_conversation"] = failedConvo
	evidence, _ := json.Marshal(map[string]any{"preview_recovery": "waiting_input", "preview_model_calls": 0, "pending_action_events": 1, "single_cancel": "cancelled", "compaction": "failed", "failure_terminal_events": 1, "worker_original_replicas": replicas, "worker_restored": restored, "fixture": "persisted boundary before new worker start"})
	return writePrivate(filepath.Join(env.Options.Artifacts, "p5-agent-recovery.json"), append(evidence, '\n'))
}

func (a *App) verifyP5AdmissionBoundary(ctx context.Context, env *E2EEnvironment) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	budget := modelprovider.NewContextBudget(32768, 1024)
	for _, delta := range []int{-1, 0, 1, 1985} {
		key := fmt.Sprintf("capacity-boundary-%d", delta)
		convo, err := a.p5Conversation(ctx, env, key)
		if err != nil {
			return err
		}
		base, err := client.JSON(ctx, key+"-baseline", "enterprise", http.MethodPost, "/conversations/"+convo+"/preflight", 200, map[string]any{"content": "x", "file_ids": []string{}}, enterpriseHeaders(env, ""))
		if err != nil {
			return err
		}
		estimate, ok := base["estimated_tokens"].(float64)
		if !ok {
			return fmt.Errorf("P5 preflight lacks input estimate")
		}
		content := strings.Repeat("x", 1+budget.HardLimit+delta-int(estimate))
		status := 422
		if delta < 0 {
			status = 200
		}
		if _, err := client.JSON(ctx, key+"-preflight", "enterprise", http.MethodPost, "/conversations/"+convo+"/preflight", status, map[string]any{"content": content, "file_ids": []string{}}, enterpriseHeaders(env, "")); err != nil {
			return err
		}
		if delta < 0 {
			status = 202
		}
		result, err := client.JSON(ctx, key+"-message", "enterprise", http.MethodPost, "/conversations/"+convo+"/messages", status, map[string]any{"content": content, "file_ids": []string{}}, enterpriseHeaders(env, p5RequestKey("p5-capacity", key)))
		if err != nil {
			return err
		}
		if delta < 0 {
			run, err := stringField(result, "run", "run_id")
			if err != nil {
				return err
			}
			if err := a.waitRunTerminal(ctx, env, run); err != nil {
				return err
			}
			continue
		}
		if err := a.waitPostgresValue(ctx, env, "SELECT (SELECT count(*) FROM runs WHERE conversation_id='"+convo+"')+(SELECT count(*) FROM conversation_events WHERE conversation_id='"+convo+"');", "0", time.Second); err != nil {
			return err
		}
	}
	evidence, _ := json.Marshal(map[string]any{"hard_limit": budget.HardLimit, "accepted_estimates": []int{budget.HardLimit - 1}, "rejected_estimates": []int{budget.HardLimit, budget.HardLimit + 1, budget.HardLimit + 1985}, "rejected_runs_and_events": 0})
	return writePrivate(filepath.Join(env.Options.Artifacts, "p5-capacity-boundary.json"), append(evidence, '\n'))
}

func p5RecoveryPreviewInput(runID string) map[string]any {
	return map[string]any{"name": "p5-recovery-" + runID, "platform": "linux", "role": "managed_host", "control_path": "direct",
		"install_method": "manual", "ssh_path": "none", "architecture": "amd64", "environment": "development", "labels": map[string]any{}}
}

func (a *App) p5SeedFinishedTool(ctx context.Context, env *E2EEnvironment, run string, projection map[string]any) error {
	encoded, _ := json.Marshal(projection)
	_, err := a.postgresQuery(ctx, env, p5CompletedToolFixtureSQL(run, env.State.Values["enterprise_id"], encoded))
	return err
}

func p5CompletedToolFixtureSQL(run, enterprise string, encoded []byte) string {
	return fmt.Sprintf(`DO $fixture$ DECLARE
 r runs%%ROWTYPE; step uuid:=gen_random_uuid(); call uuid:=gen_random_uuid(); artifact uuid:=gen_random_uuid(); scope text; seq bigint; body jsonb;
 result jsonb:=convert_from(decode('%s','hex'),'UTF8')::jsonb;
BEGIN
 SELECT * INTO STRICT r FROM runs WHERE id='%s' AND enterprise_id='%s';
 SELECT t.authorization_scope INTO STRICT scope FROM tool_calls t JOIN runs owner ON owner.id=t.run_id
 WHERE t.enterprise_id=r.enterprise_id AND owner.actor_user_id=r.actor_user_id AND t.authorization_scope<>'' ORDER BY t.created_at DESC LIMIT 1;
 INSERT INTO run_steps(id,run_id,enterprise_id,sequence,step_type,status) VALUES(step,r.id,r.enterprise_id,1,'model_call','succeeded');
 INSERT INTO tool_calls(id,call_id,enterprise_id,run_id,step_id,tool_id,source,input,input_hash,status,authorization_scope)
 VALUES(call,call::text,r.enterprise_id,r.id,step,'tool.invoke','argus','{}',sha256('{}'::bytea),'succeeded',scope);
 INSERT INTO artifacts(id,result_ref,enterprise_id,conversation_id,run_id,content_type,data_classification,content,content_hash,byte_size,authorization_scope)
 VALUES(artifact,'result_'||call,r.enterprise_id,r.conversation_id,r.id,'application/json','internal',convert_to(result::text,'UTF8'),sha256(convert_to(result::text,'UTF8')),octet_length(convert_to(result::text,'UTF8')),scope);
 INSERT INTO tool_results(id,tool_call_id,enterprise_id,artifact_id,projection,projection_hash,projection_bytes,partial)
 VALUES(gen_random_uuid(),call,r.enterprise_id,artifact,result,sha256(convert_to(result::text,'UTF8')),octet_length(convert_to(result::text,'UTF8')),false);
 UPDATE conversations SET event_sequence=event_sequence+2 WHERE id=r.conversation_id RETURNING event_sequence-1 INTO seq;
 body:=jsonb_build_object('content','','tool_calls',jsonb_build_array(jsonb_build_object('id',call,'name','tool.invoke','arguments','{}')),'authorization_scope',scope);
 INSERT INTO conversation_events(id,enterprise_id,conversation_id,run_id,step_id,sequence,event_type,actor_type,actor_id,payload,content_hash,data_classification)
 VALUES(gen_random_uuid(),r.enterprise_id,r.conversation_id,r.id,step,seq,'assistant_message','model','fixture',body,sha256(convert_to(body::text,'UTF8')),'internal');
 body:=jsonb_build_object('tool_call_id',call,'projection',result,'authorization_scope',scope,'status','succeeded');
 INSERT INTO conversation_events(id,enterprise_id,conversation_id,run_id,step_id,sequence,event_type,actor_type,actor_id,payload,content_hash,data_classification)
 VALUES(gen_random_uuid(),r.enterprise_id,r.conversation_id,r.id,step,seq+1,'tool_call_result','service','fixture',body,sha256(convert_to(body::text,'UTF8')),'internal');
END $fixture$;`, hex.EncodeToString(encoded), run, enterprise)
}
