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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (a *App) verifyM4RunLifecycle(ctx context.Context, env *E2EEnvironment, hostID string) (failure error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	convo, run, ref, err := a.m4AgentPreview(ctx, env, hostID, "rejected")
	if err != nil {
		return err
	}
	confirmed, err := client.JSON(ctx, "m4-run-confirm-rejected", "enterprise", http.MethodPost, "/enterprise/pending-actions/"+ref+"/confirm", 200, nil, enterpriseHeaders(env, "m4-run-confirm-rejected"))
	if err != nil {
		return err
	}
	approval, err := stringField(confirmed, "approval_request", "approval_request_id")
	if err != nil {
		return err
	}
	if err := a.waitRunStatus(ctx, env, run, "waiting_approval", 30*time.Second); err != nil {
		return err
	}
	if err := a.m4DecideRunAction(ctx, env, approval, "rejected", "reject-run"); err != nil {
		return err
	}
	if err := a.waitRunStatus(ctx, env, run, "failed", 30*time.Second); err != nil {
		return err
	}
	if err := a.waitPostgresValue(ctx, env, "SELECT count(*) FROM conversation_events WHERE run_id='"+run+"' AND event_type='run_state_changed' AND payload->>'error_code'='APPROVAL_REJECTED';", "1", time.Second); err != nil {
		return err
	}
	if _, err := a.p5Run(ctx, env, convo, "continue-after-rejection", nil, []string{}); err != nil {
		return err
	}
	// Grant the independent member the ordinary conversation route permissions;
	// owner checks must still reject access to the creator's private Run.
	if _, err := a.postgresQuery(ctx, env, `DO $role$ DECLARE rid uuid:=gen_random_uuid(); uid uuid; BEGIN
 SELECT id INTO STRICT uid FROM enterprise_users WHERE enterprise_id='`+env.State.Values["enterprise_id"]+`' AND username='m4-approver';
 INSERT INTO roles(id,enterprise_id,name,builtin) VALUES(rid,'`+env.State.Values["enterprise_id"]+`','M4 conversation member',false);
 INSERT INTO role_permissions(role_id,permission_id) VALUES(rid,'conversation.read'),(rid,'conversation.use');
 INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES(gen_random_uuid(),'`+env.State.Values["enterprise_id"]+`','user',uid,rid);
 UPDATE enterprise_users SET authorization_version=authorization_version+1 WHERE id=uid; END $role$;`); err != nil {
		return err
	}
	if err := a.refreshM4ApproverLogin(ctx, env); err != nil {
		return err
	}
	otherHeaders := map[string]string{"Origin": env.EnterpriseOrigin(), "X-CSRF-Token": env.State.Values["m4_approver_csrf"], "Idempotency-Key": p5RequestKey("m4-owned", run)}
	if _, err := client.JSON(ctx, "m4-foreign-run-read", "m4-approver", http.MethodGet, "/runs/"+run, 404, nil, otherHeaders); err != nil {
		return err
	}
	for _, operation := range []string{"compact", "cancel"} {
		if _, err := client.JSON(ctx, "m4-foreign-run-"+operation, "m4-approver", http.MethodPost, "/runs/"+run+"/"+operation, 404, nil, otherHeaders); err != nil {
			return err
		}
	}
	if err := a.waitPostgresValue(ctx, env, "SELECT count(*) FROM runtime_tasks WHERE run_id='"+run+"' AND queue='compaction';", "0", time.Second); err != nil {
		return err
	}
	if _, err := client.JSON(ctx, "m4-delete-private-conversation", "enterprise", http.MethodDelete, "/conversations/"+convo, 202, nil, enterpriseHeaders(env, "m4-delete-private-conversation")); err != nil {
		return err
	}
	if _, err := client.JSON(ctx, "m4-deleted-run-read", "enterprise", http.MethodGet, "/runs/"+run, 404, nil, enterpriseHeaders(env, "")); err != nil {
		return err
	}
	if _, err := client.JSON(ctx, "m4-deleted-run-compact", "enterprise", http.MethodPost, "/runs/"+run+"/compact", 404, nil, enterpriseHeaders(env, "m4-deleted-run-compact")); err != nil {
		return err
	}
	invalidConvo, invalidRun, invalidRef, err := a.m4AgentPreview(ctx, env, hostID, "invalidated")
	if err != nil {
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
	confirmed, err = client.JSON(ctx, "m4-run-confirm-invalidated", "enterprise", http.MethodPost, "/enterprise/pending-actions/"+invalidRef+"/confirm", 200, nil, enterpriseHeaders(env, "m4-run-confirm-invalidated"))
	if err != nil {
		return err
	}
	approval, err = stringField(confirmed, "approval_request", "approval_request_id")
	if err != nil {
		return err
	}
	if err := a.m4DecideRunAction(ctx, env, approval, "approved", "approve-before-invalidation"); err != nil {
		return err
	}
	if _, err := a.postgresQuery(ctx, env, "UPDATE approval_policies SET version=version+1 WHERE enterprise_id='"+env.State.Values["enterprise_id"]+"' AND name='M4 host changes';"); err != nil {
		return err
	}
	if err := restore(ctx); err != nil {
		return err
	}
	if err := a.waitRunStatus(ctx, env, invalidRun, "failed", time.Minute); err != nil {
		return err
	}
	if err := a.waitPostgresValue(ctx, env, "SELECT count(*) FROM executions WHERE run_id='"+invalidRun+"' AND status='failed' AND error_code='ACTION_INVALIDATED';", "1", time.Second); err != nil {
		return err
	}
	if err := a.waitPostgresValue(ctx, env, "SELECT count(*) FROM hosts WHERE id='"+hostID+"' AND labels->>'p5-proof'='invalidated';", "0", time.Second); err != nil {
		return err
	}
	if _, err := a.p5Run(ctx, env, invalidConvo, "continue-after-invalidation", nil, []string{}); err != nil {
		return err
	}
	evidence, _ := json.Marshal(map[string]any{"rejected_run": "failed", "rejected_terminal_events": 1, "invalidated_execution": "failed", "invalidated_business_changes": 0, "following_messages": "succeeded", "foreign_run_read_compact_cancel": "404", "deleted_run_read_compact": "404", "foreign_compaction_tasks": 0, "worker_replicas_restored": replicas})
	return writePrivate(filepath.Join(env.Options.Artifacts, "m4-run-lifecycle.json"), append(evidence, '\n'))
}

func (a *App) m4AgentPreview(ctx context.Context, env *E2EEnvironment, hostID, label string) (string, string, string, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return "", "", "", err
	}
	host, err := client.JSON(ctx, "m4-run-host-"+label, "enterprise", http.MethodGet, "/enterprise/hosts/"+hostID, 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return "", "", "", err
	}
	version, err := numberField(host, "resource_version")
	if err != nil {
		return "", "", "", err
	}
	convo, err := a.p5Conversation(ctx, env, "M4 run "+label)
	if err != nil {
		return "", "", "", err
	}
	run, err := a.p5StartRun(ctx, env, convo, "m4-"+label, []p5ToolStep{{"tool.describe", map[string]any{"category": "host", "name": "update.preview"}}, {"tool.invoke", map[string]any{"category": "host", "name": "update.preview", "arguments": map[string]any{"host_id": hostID, "expected_version": version, "labels": map[string]string{"p5-proof": label}}}}}, []string{})
	if err != nil {
		return "", "", "", err
	}
	if err := a.waitRunStatus(ctx, env, run, "waiting_input", time.Minute); err != nil {
		return "", "", "", err
	}
	ref, err := a.postgresQuery(ctx, env, "SELECT action_ref FROM pending_actions WHERE run_id='"+run+"' AND status='awaiting_confirmation';")
	if err != nil {
		return "", "", "", err
	}
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.ContainsAny(ref, "\r\n") {
		return "", "", "", fmt.Errorf("M4 Run preview not bound to one action")
	}
	return convo, run, ref, nil
}

func (a *App) m4DecideRunAction(ctx context.Context, env *E2EEnvironment, approval, decision, key string) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	_, err = client.JSON(ctx, "m4-run-decision-"+key, "m4-approver", http.MethodPost, "/enterprise/approval-requests/"+approval+"/decisions", 200, map[string]any{"decision": decision, "reason": "M4 Run lifecycle regression"}, map[string]string{"Origin": env.EnterpriseOrigin(), "X-CSRF-Token": env.State.Values["m4_approver_csrf"], "Idempotency-Key": p5RequestKey("m4-decision", key+env.Options.RunID)})
	return err
}
