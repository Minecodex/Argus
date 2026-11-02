package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

func (a *App) plan11DeployedMetadataSemantics(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) (failure error) {
	if env.Options.Plan8Phase != "plan11-metadata" || env.Options.RealModel == nil {
		return errors.New("metadata semantics requires the explicit selected-provider phase")
	}
	proof := map[string]any{"schema_version": "argus.plan11_metadata_semantics/v1", "passed": false,
		"planned_tasks": plan8SelectedModelNames("plan11-metadata"), "provider_call_acceptance_limit_total": 384,
		"provider_probe_calls_separate": true, "scope": "actual SDK pipeline setup plus metadata-only provider tasks, no sample query by these model tasks", "whole_plan_complete": false}
	var cases []map[string]any
	defer func() {
		proof["passed"], proof["cases"] = failure == nil, cases
		if failure != nil {
			proof["error"] = string(redactDiagnostic([]byte(failure.Error())))
		}
		raw, err := json.MarshalIndent(proof, "", "  ")
		failure = errors.Join(failure, err, writePrivate(filepath.Join(env.Options.Artifacts, "plan11-metadata-semantics.json"), raw))
	}()
	application, endpoint := env.State.Values["p11_metadata_application_id"], env.State.Values["p11_metadata_endpoint_id"]
	if uuid.Validate(application) != nil || uuid.Validate(endpoint) != nil {
		return errors.New("metadata semantics has no actual SDK application/endpoint prerequisite")
	}
	version, err := a.postgresQuery(ctx, env, "SELECT version FROM monitoring_ingest_endpoints WHERE id='"+endpoint+"' AND enterprise_id='"+env.State.Values["enterprise_id"]+"'")
	if err != nil {
		return err
	}
	current, err := strconv.ParseInt(strings.TrimSpace(version), 10, 64)
	if err != nil {
		return errors.New("SDK endpoint has no actual version for normal re-enable preview")
	}
	if _, err = a.plan8ServiceCommit(ctx, env, "metadata-sdk-enable", map[string]any{"kind": "endpoint", "operation": "set_enabled", "id": endpoint, "expected_version": current, "endpoint": map[string]any{"enabled": true}}); err != nil {
		return err
	}
	if _, err = a.waitP4HostCollector(ctx, env, host.HostID, "converged"); err != nil {
		return err
	}
	empty, err := a.postgresQuery(ctx, env, "SELECT count(*) FROM monitoring_collections WHERE enterprise_id='"+env.State.Values["enterprise_id"]+"' AND application_id='"+application+"'")
	if err != nil || strings.TrimSpace(empty) != "0" {
		return errors.Join(err, errors.New("SDK-only counterexample unexpectedly has collection instances"))
	}
	collection, err := a.plan11NavigationFixture(ctx, env, host, "Plan11 metadata-only database", "metadata-activity.invalid:3306", "metadata-activity")
	if err != nil {
		return err
	}
	proof["fixtures"] = map[string]any{"application_id": application, "endpoint_id": endpoint, "sdk_endpoint_reenabled_from_version": current,
		"SDK_application_collection_instances": 0, "disabled_collection": collection, "database_reachability_claimed": false}
	for _, locale := range []string{"zh", "en"} {
		for repeat := 1; repeat <= 3; repeat++ {
			name := fmt.Sprintf("plan11-sdk-coverage-%s-%d", locale, repeat)
			row, err := a.plan11PreparedMetadataTask(ctx, env, name, plan11SDKCoveragePrompt(locale, application), nil, plan11MetadataTraceAllowed,
				func(ctx context.Context, conversation string, sample *p5BenchmarkSample, row map[string]any) error {
					return a.plan11VerifySDKMetadata(ctx, env, sample, application, endpoint, row)
				})
			cases = append(cases, row)
			if err != nil {
				return err
			}
			name = fmt.Sprintf("plan11-request-activity-%s-%d", locale, repeat)
			row, err = a.plan11PreparedMetadataTask(ctx, env, name, plan11CollectionActivityPrompt(locale, collection.ID.String()), nil, plan11MetadataTraceAllowed,
				func(ctx context.Context, conversation string, sample *p5BenchmarkSample, row map[string]any) error {
					return a.plan11VerifyCollectionActivity(ctx, env, sample, collection.ID, row)
				})
			cases = append(cases, row)
			if err != nil {
				return err
			}
		}
	}
	return a.plan11WaitModelAccounting(ctx, env)
}

func plan11SDKCoveragePrompt(locale, application string) string {
	if locale == "zh" {
		return "请只读核对应用 " + application + " 当前已配置的监控接入类型、信号与启用状态。我看到它的采集实例列表为空，这能说明应用没有监控吗？请实际查证，并明确已核对范围和仍未知的历史数据情况。不查询遥测样本、不探测、不修改、不发布文件、不取Key原值。用中文业务摘要回答。"
	}
	return "Read-only inspect the currently configured monitoring ingress types, signals and enabled state of application " + application + ". Its collection instance list appears empty: does that establish that the application has no monitoring? Check actual authorized metadata and distinguish verified coverage from historical data that remains unknown. Do not query telemetry samples, probe, modify, publish files or retrieve raw Keys. Answer with an English business summary."
}

func plan11CollectionActivityPrompt(locale, collection string) string {
	if locale == "zh" {
		return "仅只读查看采集实例 " + collection + " 的当前配置与状态，明确区分本次是否查询过遥测样本、目标历史数据是否已核对。不要把启停、健康或freshness当成历史/本次采样证明。不查询遥测、不探测、不修改、不发布文件。简要说明来源，并用一个小JSON代码块给出 collection_id、collection_enabled、sampling_performed、historical_data_checked。请用中文回答。"
	}
	return "Read only the current configuration/state of collection instance " + collection + ". Distinguish whether this request queried telemetry samples from whether the target's historical data was checked; enablement, health and freshness alone prove neither query activity nor historical absence. Do not query telemetry, probe, modify or publish files. Briefly explain the source and provide one small JSON block with collection_id, collection_enabled, sampling_performed, historical_data_checked. Answer in English."
}

func plan11MetadataTraceAllowed(raw string) error {
	var calls []struct {
		Tool  string
		Input struct{ Category, Name string }
	}
	if json.Unmarshal([]byte(raw), &calls) != nil || len(calls) == 0 {
		return errors.New("metadata semantics trace missing")
	}
	for _, call := range calls {
		if call.Tool == "tool.invoke" && call.Input.Category == "application" && (call.Input.Name == "endpoint.list" || call.Input.Name == "service.list" || call.Input.Name == "endpoint.connection" || call.Input.Name == "key.list") {
			continue
		}
		one, _ := json.Marshal([]any{call})
		if err := plan11IdentityTraceAllowed(string(one)); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) plan11VerifySDKMetadata(ctx context.Context, env *E2EEnvironment, sample *p5BenchmarkSample, application, endpoint string, proof map[string]any) error {
	if sample.RunStatus != "succeeded" {
		return errors.New("exact SDK application metadata task did not complete autonomously")
	}
	observed, err := a.postgresQuery(ctx, env, plan11SDKEndpointReadSQL(sample.RunID, application, endpoint))
	if err != nil || strings.TrimSpace(observed) == "0" {
		return errors.Join(err, errors.New("SDK-only conclusion did not observe the actual authorized enabled OTLP endpoint"))
	}
	reply, err := a.planV2ModelReply(ctx, env, sample.RunID)
	if err != nil || !strings.Contains(strings.ToLower(reply), "otlp") {
		return errors.Join(err, errors.New("SDK-only answer omitted the actually configured ingress protocol"))
	}
	proof["application_id"], proof["endpoint_id"] = application, endpoint
	return nil
}

func (a *App) plan11VerifyCollectionActivity(ctx context.Context, env *E2EEnvironment, sample *p5BenchmarkSample, collection uuid.UUID, proof map[string]any) error {
	if sample.RunStatus != "succeeded" {
		return errors.New("exact collection metadata task did not complete autonomously")
	}
	reply, err := a.planV2ModelReply(ctx, env, sample.RunID)
	if err != nil {
		return err
	}
	var result struct {
		Collection string `json:"collection_id"`
		Enabled    *bool  `json:"collection_enabled"`
		Sampled    *bool  `json:"sampling_performed"`
		Historical *bool  `json:"historical_data_checked"`
	}
	start := strings.LastIndex(reply, "```json")
	if start < 0 {
		return errors.New("collection activity answer omitted its small structured conclusion")
	}
	text := reply[start+7:]
	end := strings.Index(text, "```")
	if end < 0 || json.Unmarshal([]byte(strings.TrimSpace(text[:end])), &result) != nil || result.Collection != collection.String() || result.Enabled == nil || *result.Enabled || result.Sampled == nil || *result.Sampled || result.Historical == nil || *result.Historical {
		return errors.New("collection configuration/request activity/history distinction differs from actual metadata-only task")
	}
	observed, err := a.postgresQuery(ctx, env, plan11CollectionActivityReadSQL(sample.RunID, collection))
	if err != nil || strings.TrimSpace(observed) == "0" {
		return errors.Join(err, errors.New("collection activity result did not actually read the exact instance and explicit activity fact"))
	}
	proof["conclusion"] = result
	return nil
}

func plan11SDKEndpointReadSQL(run, application, endpoint string) string {
	return fmt.Sprintf(`SELECT count(*) FROM tool_calls t JOIN tool_results r ON r.tool_call_id=t.id AND r.enterprise_id=t.enterprise_id WHERE t.run_id='%s' AND t.status='succeeded' AND t.source='argus' AND t.tool_id='tool.invoke' AND t.input->>'category'='application' AND t.input->>'name'='endpoint.list' AND t.input->'arguments'->>'application_id'='%s' AND EXISTS(SELECT 1 FROM jsonb_array_elements(coalesce(r.projection->'summary'->'items','[]'::jsonb)) item WHERE item->>'id'='%s' AND item->>'protocol'='otlp' AND item->>'enabled'='true')`, run, application, endpoint)
}

func plan11CollectionActivityReadSQL(run string, collection uuid.UUID) string {
	return fmt.Sprintf(`SELECT count(*) FROM tool_calls t JOIN tool_results r ON r.tool_call_id=t.id AND r.enterprise_id=t.enterprise_id WHERE t.run_id='%s' AND t.status='succeeded' AND t.source='argus' AND t.tool_id='tool.invoke' AND t.input->>'category'='monitoring' AND t.input->>'name' IN ('collection.list','collection.get') AND r.projection->'summary'->>'sampling_performed'='false' AND r.projection->'summary'->>'data_state'='not_checked' AND ((r.projection->'summary'->>'id'='%s' AND r.projection->'summary'->>'desired_enabled'='false') OR EXISTS(SELECT 1 FROM jsonb_array_elements(coalesce(r.projection->'summary'->'items','[]'::jsonb)) item WHERE item->>'id'='%s' AND item->>'desired_enabled'='false'))`, run, collection, collection)
}
