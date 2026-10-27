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

	"github.com/google/uuid"
)

// The human claims the value using the authenticated one-time HTTP endpoint.
// Only public endpoint metadata and the change preview enter the real model.
func (a *App) plan8ModelSDKKey(ctx context.Context, env *E2EEnvironment, app, endpoint uuid.UUID) (keyID uuid.UUID, key string, failure error) {
	bounded, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	model, err := a.plan7Model(bounded, env)
	if err != nil {
		return uuid.Nil, "", err
	}
	conversation, err := a.plan7ModelConversation(bounded, env, model, "Plan8 SDK private configuration delivery")
	if err != nil {
		return uuid.Nil, "", err
	}
	prompt := fmt.Sprintf("请核对应用 %s 的 SDK 接入 %s 的实际接入配置、固定环境、TLS和服务自动发现要求，并为这个已有接入签发一个有效期约1小时的新 Key。请只生成平台类型化预览等我确认，不创建服务或其他接入，不改变应用或现有Key；Key原值由我通过平台的一次性领取接口获取，不能进入你的工具输入、结果或回答。", app, endpoint)
	sample, err := a.realModelTask(bounded, env, conversation, "plan8-sdk-key-delivery", prompt, nil, nil)
	defer func() { failure = errors.Join(failure, a.planV2ModelEvidence(ctx, env, conversation, sample, failure)) }()
	if err != nil {
		return uuid.Nil, "", err
	}
	if uuid.Validate(sample.RunID) != nil || !sample.UsageComplete || !sample.CachedUsageComplete || sample.ModelCalls < 1 || sample.ModelCalls > 32 {
		return uuid.Nil, "", errors.New("SDK model preview usage incomplete")
	}
	raw, err := a.postgresQuery(bounded, env, "SELECT coalesce(json_agg(json_build_object('ref',action_ref,'type',action_type,'preview',preview)),'[]') FROM pending_actions WHERE run_id='"+sample.RunID+"' AND status='awaiting_confirmation'")
	if err != nil {
		return uuid.Nil, "", err
	}
	var actions []struct {
		Ref, Type string
		Preview   struct {
			Kind, Operation string
			IngestKey       struct {
				Endpoint uuid.UUID `json:"endpoint_id"`
				Expires  time.Time `json:"expires_at"`
			} `json:"ingest_key"`
		}
	}
	if json.Unmarshal([]byte(raw), &actions) != nil || len(actions) != 1 {
		return uuid.Nil, "", errors.New("SDK model did not prepare exactly one typed key action")
	}
	action := actions[0]
	if action.Type != "monitoring.ingest_key.create" || action.Preview.Kind != "ingest_key" || action.Preview.Operation != "create" || action.Preview.IngestKey.Endpoint != endpoint || time.Until(action.Preview.IngestKey.Expires) < 30*time.Minute || time.Until(action.Preview.IngestKey.Expires) > 2*time.Hour {
		return uuid.Nil, "", errors.New("SDK key preview exceeded the requested scope")
	}
	count, err := a.postgresQuery(bounded, env, "SELECT count(*) FROM monitoring_ingest_keys WHERE endpoint_id='"+endpoint.String()+"'")
	if err != nil || strings.TrimSpace(count) != "0" {
		return uuid.Nil, "", errors.New("SDK key existed before the human confirmation")
	}
	result, err := a.confirmPendingAction(bounded, env, "plan8-model-sdk-key", action.Ref)
	if err != nil {
		return uuid.Nil, "", err
	}
	id, err := stringField(result, "resource_ref", "resource_id")
	if err != nil {
		return uuid.Nil, "", err
	}
	keyID, err = uuid.Parse(id)
	if err != nil {
		return uuid.Nil, "", err
	}
	if err = a.waitRunStatus(bounded, env, sample.RunID, "succeeded", 5*time.Minute); err != nil {
		return uuid.Nil, "", err
	}
	sample, err = a.p5BenchmarkSample(bounded, env, sample.ID, sample.RunID)
	if err != nil || sample.RunStatus != "succeeded" || !sample.UsageComplete || !sample.CachedUsageComplete || sample.ModelCalls > 32 {
		return uuid.Nil, "", errors.Join(err, errors.New("SDK model terminal usage incomplete"))
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return uuid.Nil, "", err
	}
	revealPath := "/monitoring/ingest-keys/" + keyID.String() + "/reveal"
	value, err := client.JSON(bounded, "model-sdk-private-human-claim", "enterprise", http.MethodPost, revealPath, 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return uuid.Nil, "", err
	}
	key, err = stringField(value, "key")
	if err != nil || len(key) < 40 {
		return uuid.Nil, "", errors.New("SDK private claim returned no valid value")
	}
	if _, err = client.JSON(bounded, "model-sdk-claim-replay-denied", "enterprise", http.MethodPost, revealPath, 409, nil, enterpriseHeaders(env, "")); err != nil {
		return uuid.Nil, "", err
	}
	calls, err := a.postgresQuery(bounded, env, "SELECT coalesce(json_agg(json_build_object('tool',tool_id,'input',input,'status',status,'error_code',error_code) ORDER BY created_at,call_sequence),'[]') FROM tool_calls WHERE run_id='"+sample.RunID+"'")
	if err != nil {
		return uuid.Nil, "", err
	}
	if err = verifyTypedToolDiscoveryOrder(calls, "application"); err != nil {
		return uuid.Nil, "", err
	}
	var invoked []struct {
		Tool, Status string
		Input        struct {
			Category, Name string
			Arguments      map[string]any
		}
	}
	if json.Unmarshal([]byte(calls), &invoked) != nil {
		return uuid.Nil, "", errors.New("SDK model tool facts unavailable")
	}
	connectionRead := false
	for _, call := range invoked {
		if call.Tool == "tool.invoke" && call.Status == "succeeded" && call.Input.Category == "application" && strings.TrimPrefix(call.Input.Name, "application.") == "endpoint.connection" {
			data, _ := json.Marshal(call.Input.Arguments)
			connectionRead = connectionRead || strings.Contains(string(data), endpoint.String())
		}
	}
	if !connectionRead {
		return uuid.Nil, "", errors.New("SDK model did not inspect the actual endpoint configuration")
	}
	public, err := a.postgresQuery(bounded, env, "SELECT json_build_object('calls',(SELECT coalesce(json_agg(json_build_object('tool',tool_id,'input',input)),'[]') FROM tool_calls WHERE run_id='"+sample.RunID+"'),'events',(SELECT coalesce(json_agg(payload),'[]') FROM conversation_events WHERE run_id='"+sample.RunID+"'))")
	if err != nil || strings.Contains(public, key) {
		return uuid.Nil, "", errors.New("SDK private value reached public model records or scan failed")
	}
	confirmations, err := a.postgresQuery(bounded, env, "SELECT count(*) FROM user_confirmations WHERE pending_action_id=(SELECT id FROM pending_actions WHERE action_ref='"+action.Ref+"')")
	if err != nil || strings.TrimSpace(confirmations) != "1" {
		return uuid.Nil, "", errors.New("SDK key lacks exactly one confirmation")
	}
	sample.Passed = true
	proof, _ := json.MarshalIndent(map[string]any{"passed": true, "scope": "real_model_connection_guide_typed_key_preview_human_confirmation_and_one_time_authenticated_http_claim", "application_id": app, "endpoint_id": endpoint, "key_id": keyID, "sample": sample, "confirmation_count": 1, "claim_replay_denied": true, "canary_scope": "persisted_public_tools_and_events_not_provider_wire", "sdk_transport_validation": "performed_by_enclosing_actual_sdk_scenario"}, "", "  ")
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan8-model-sdk-key.json"), proof); err != nil {
		return uuid.Nil, "", err
	}
	return keyID, key, nil
}
