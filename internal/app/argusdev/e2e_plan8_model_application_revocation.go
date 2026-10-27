package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Existing observations come from this deployment's real SDK, not injected rows.
func (a *App) plan8ModelApplicationRevocation(ctx context.Context, env *E2EEnvironment) (failure error) {
	var sdk struct {
		Passed bool `json:"passed"`
		Cases  []struct {
			Protocol string    `json:"protocol"`
			App      uuid.UUID `json:"application_id"`
		} `json:"cases"`
	}
	raw, err := os.ReadFile(filepath.Join(env.Options.Artifacts, "plan8-deployed-sdk.json"))
	if err != nil || json.Unmarshal(raw, &sdk) != nil || !sdk.Passed {
		return errors.New("revocation model requires this run's actual SDK observations")
	}
	apps := []uuid.UUID{}
	for _, row := range sdk.Cases {
		if row.Protocol == "otlp_http" || row.Protocol == "otlp_grpc" {
			apps = append(apps, row.App)
		}
	}
	if len(apps) != 2 || apps[0] == uuid.Nil || apps[1] == uuid.Nil || apps[0] == apps[1] {
		return errors.New("invalid owned application scopes")
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	user := env.State.Values["admin_user_id"]
	if uuid.Validate(user) != nil {
		return errors.New("invalid current human actor")
	}
	grantPath := "/enterprise/data-authorizations/user/" + user
	changeGrant := func(c context.Context, remove bool) error {
		state, e := client.JSON(c, "model-revocation-grant-version", "enterprise", http.MethodGet, grantPath+"?resource_type=application", 200, nil, enterpriseHeaders(env, ""))
		if e != nil {
			return e
		}
		_, e = client.JSON(c, "model-revocation-grant-change", "enterprise", http.MethodPost, grantPath, 204, map[string]any{"resource_type": "application", "resource_ids": []string{apps[0].String()}, "remove": remove, "expected_version": state["authorization_version"]}, enterpriseHeaders(env, "model-revocation-grant-"+uuid.NewString()))
		if e != nil {
			return e
		}
		// This changes the current user's own authorization. Keep the same
		// Conversation, but authenticate normally for its new permission version.
		return a.refreshEnterpriseLogin(c, env)
	}
	model, err := a.plan7Model(bounded, env)
	if err != nil {
		return err
	}
	conversation, err := a.plan7ModelConversation(bounded, env, model, "Plan8 same-conversation application revocation")
	if err != nil {
		return err
	}
	first, err := a.realModelTask(bounded, env, conversation, "plan8-before-app-revocation", fmt.Sprintf("请只读查询应用 %s 最近30分钟的实际APM和业务日志，读取Workspace结果后说明样本和来源限制。不要查询其他应用，不修改配置或发布新文件。", apps[0]), nil, nil)
	var second *p5BenchmarkSample
	defer func() {
		failure = errors.Join(failure, a.planV2ModelEvidence(ctx, env, conversation, first, failure))
		if second != nil {
			failure = errors.Join(failure, a.planV2ModelEvidence(ctx, env, conversation, *second, failure))
		}
	}()
	if err != nil {
		return err
	}
	if first.RunStatus != "succeeded" || !first.UsageComplete || !first.CachedUsageComplete || first.ModelCalls < 1 || first.ModelCalls > 32 {
		return errors.New("initial revocation model run or usage incomplete")
	}
	filesRaw, err := a.postgresQuery(bounded, env, "SELECT coalesce(json_agg(json_build_object('id',id,'path',path,'refs',source_result_refs)),'[]') FROM workspace_files WHERE conversation_id='"+conversation+"' AND deleted_at IS NULL AND cardinality(source_result_refs)>0")
	if err != nil {
		return err
	}
	var files []struct {
		ID   uuid.UUID
		Path string
		Refs []string
	}
	if json.Unmarshal([]byte(filesRaw), &files) != nil || len(files) == 0 {
		return errors.New("model did not obtain source-bound actual Workspace files")
	}
	reads, err := a.postgresQuery(bounded, env, "SELECT count(*) FROM tool_calls WHERE run_id='"+first.RunID+"' AND status='succeeded' AND tool_id IN ('read','bash')")
	if err != nil || strings.TrimSpace(reads) == "0" {
		return errors.New("model did not read actual files before revocation")
	}
	for _, file := range files {
		if file.ID == uuid.Nil || len(file.Refs) == 0 {
			return errors.New("missing immutable file identity")
		}
		if _, _, err = p5FileHTTP(bounded, client, env, http.MethodGet, "/conversations/"+conversation+"/workspace/files/"+file.ID.String()+"/content", nil, 200, ""); err != nil {
			return err
		}
	}
	if err = changeGrant(bounded, true); err != nil {
		return err
	}
	defer func() {
		restore, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		failure = errors.Join(failure, changeGrant(restore, false))
	}()
	if _, err = client.JSON(bounded, "model-revoked-application-http", "enterprise", http.MethodGet, "/applications/"+apps[0].String(), 403, nil, enterpriseHeaders(env, "")); err != nil {
		return err
	}
	if _, err = client.JSON(bounded, "model-retained-application-http", "enterprise", http.MethodGet, "/applications/"+apps[1].String(), 200, nil, enterpriseHeaders(env, "")); err != nil {
		return err
	}
	for _, file := range files {
		if _, _, err = p5FileHTTP(bounded, client, env, http.MethodGet, "/conversations/"+conversation+"/workspace/files/"+file.ID.String()+"/content", nil, 403, ""); err != nil {
			return errors.New("old source-bound file download did not reject revoked application")
		}
	}
	next, err := a.realModelTask(bounded, env, conversation, "plan8-after-app-revocation", fmt.Sprintf("应用 %s 的访问权限刚被撤销。请重新核对当前权限，尝试读取它的当前记录和之前文件 %s，遇到拒绝就说明，不能把旧结果或对话记忆当成新授权。然后只读查询仍有权限的应用 %s 的当前应用记录。不要修改权限或配置。结尾用JSON代码块归纳 {\"revoked_application\":\"实际ID\",\"accessible\":false,\"other_application\":\"实际ID\",\"other_accessible\":true,\"limitations\":[\"实际拒绝及检查限制\"]}。", apps[0], files[0].Path, apps[1]), nil, nil)
	second = &next
	if err != nil {
		return err
	}
	if next.RunStatus != "succeeded" || !next.UsageComplete || !next.CachedUsageComplete || next.ModelCalls < 1 || first.ModelCalls+next.ModelCalls > 32 {
		return errors.New("revocation model final state or combined usage budget incomplete")
	}
	reply, err := a.planV2ModelReply(bounded, env, next.RunID)
	if err != nil {
		return err
	}
	start := strings.LastIndex(reply, "```json")
	if start < 0 {
		return errors.New("revocation model omitted structured conclusion")
	}
	body := reply[start+7:]
	end := strings.Index(body, "```")
	if end < 0 {
		return errors.New("unfinished revocation conclusion")
	}
	var result struct {
		Revoked         string   `json:"revoked_application"`
		Accessible      bool     `json:"accessible"`
		Other           string   `json:"other_application"`
		OtherAccessible bool     `json:"other_accessible"`
		Limitations     []string `json:"limitations"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(body[:end])), &result) != nil || result.Revoked != apps[0].String() || result.Accessible || result.Other != apps[1].String() || !result.OtherAccessible || len(result.Limitations) == 0 {
		return errors.New("model conclusion differs from actual revocation and retained access")
	}
	calls, err := a.postgresQuery(bounded, env, "SELECT coalesce(json_agg(json_build_object('tool',tool_id,'input',input,'status',status,'error_code',error_code) ORDER BY created_at,call_sequence),'[]') FROM tool_calls WHERE run_id='"+next.RunID+"'")
	if err != nil {
		return err
	}
	var invoked []struct {
		Tool, Status string
		Input        struct {
			Category, Name string
			Arguments      map[string]any
		}
	}
	if json.Unmarshal([]byte(calls), &invoked) != nil {
		return errors.New("revocation model tool facts unavailable")
	}
	retained, denied := false, false
	for _, call := range invoked {
		if call.Tool != "tool.invoke" {
			continue
		}
		arguments, _ := json.Marshal(call.Input.Arguments)
		wire := string(arguments)
		if call.Input.Category == "application" && strings.TrimPrefix(call.Input.Name, "application.") == "get" {
			if strings.Contains(wire, apps[0].String()) {
				if call.Status == "succeeded" {
					return errors.New("model read the revoked application")
				}
				denied = true
			}
			if strings.Contains(wire, apps[1].String()) && call.Status == "succeeded" {
				retained = true
			}
		}
	}
	if !retained || !denied {
		return errors.New("model did not independently observe denied and retained application access")
	}
	pending, err := a.postgresQuery(bounded, env, "SELECT count(*) FROM pending_actions WHERE run_id IN ('"+first.RunID+"','"+next.RunID+"')")
	if err != nil || strings.TrimSpace(pending) != "0" {
		return errors.New("read-only revocation model prepared a change")
	}
	first.Passed = true
	next.Passed = true
	second = &next
	proof, _ := json.MarshalIndent(map[string]any{"passed": true, "scope": "real_model_same_conversation_real_pvc_source_bound_files_http_download_revocation_and_other_application_read", "conversation_id": conversation, "before": first, "after": next, "combined_provider_calls": first.ModelCalls + next.ModelCalls, "file_count": len(files), "denied_application": apps[0], "retained_application": apps[1], "conclusion": result}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan8-model-application-revocation.json"), proof)
}
