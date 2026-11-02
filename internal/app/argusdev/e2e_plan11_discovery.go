package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Plan11 extends the real deployed foundation: real model -> persisted Agent
// loop -> native queries -> actual Workspace reads and downloadable report.
// Prompts specify business deliverables, never a hard-coded tool sequence.
func (a *App) plan11DeployedDiscovery(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop) error {
	if _, err := a.plan7Model(ctx, env); err != nil {
		return fmt.Errorf("Plan11 model prerequisite: %w", err)
	}
	cases := []map[string]any{}
	var failures []error
	conversation := ""
	for _, locale := range []string{"zh", "en"} {
		for repeat := 1; repeat <= 3; repeat++ {
			name := fmt.Sprintf("plan11-discovery-%s-%d", locale, repeat)
			evidence, err := a.plan11DiscoveryCase(ctx, env, h, name, locale)
			cases = append(cases, evidence)
			if conversation == "" && evidence["run_status"] == "succeeded" {
				conversation, _ = evidence["conversation_id"].(string)
			}
			if err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", name, err))
				if run := env.State.Values["plan8_last_model_run"]; run != "" {
					if stopErr := a.plan8StopFailedModelTask(ctx, env, run); stopErr != nil {
						return errors.Join(errors.Join(failures...), stopErr)
					}
				}
			}
		}
	}
	if err := a.plan11TaskScopeCases(ctx, env, h); err != nil {
		failures = append(failures, err)
	}
	browserErr := errors.New("no completed real model conversation available for Chatbox verification")
	if conversation != "" {
		browserErr = a.plan11DeployedChatbox(ctx, env, conversation, h)
	}
	if browserErr != nil {
		failures = append(failures, browserErr)
	}
	raw, _ := json.MarshalIndent(map[string]any{"schema_version": "argus.plan11_deployed_acceptance/v1", "replay": false, "model": env.Options.RealModel.ModelID, "cases": cases, "passed": len(failures) == 0, "chatbox_passed": browserErr == nil, "chatbox_conversation_id": conversation, "scope": "six_independent_bilingual_business_discovery_workspace_delivery_runs_and_actual_chatbox_on_host_telemetry"}, "", "  ")
	return errors.Join(errors.Join(failures...), writePrivate(filepath.Join(env.Options.Artifacts, "plan11-business-discovery.json"), raw))
}

func (a *App) plan11DiscoveryCase(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop, name, locale string) (proof map[string]any, failure error) {
	proof = map[string]any{"name": name, "locale": locale, "passed": false}
	model, err := a.plan7Model(ctx, env)
	if err != nil {
		return proof, err
	}
	conversation, err := a.plan7ModelConversation(ctx, env, model, name)
	if err != nil {
		return proof, err
	}
	proof["conversation_id"] = conversation
	prompt := fmt.Sprintf("请在有权访问的资源中定位名称为 %q 的主机，说明它当前的采集能力、实际数据来源，以及最近30分钟可查询的主机指标和单位。请从实际目录里选择一项主机 CPU 或内存指标执行查询，读取 Workspace 中的实际结果文件核对后，生成一份简短 Markdown 报告并提供下载。只有来源登记、没有样本时必须明确说明，不能编造健康结论。不要修改配置或执行远程命令。最后用 JSON 代码块总结 {\"resource_id\":\"实际主机ID\",\"metric\":\"实际查询的指标名\",\"limitations\":[\"实际覆盖限制\"]}。", h.HostName)
	if locale == "en" {
		prompt = fmt.Sprintf("Find the authorized host named %q. Explain its current collection capabilities, actual sources, and available host metrics and units in the last 30 minutes. Choose one CPU or memory metric from the actual catalog, query it, read the real Workspace result files, then create a short Markdown report and provide its download. Distinguish registered sources from received samples; never invent a healthy conclusion. Do not change configuration or run remote commands. End with a JSON code block {\"resource_id\":\"actual host ID\",\"metric\":\"actual queried metric name\",\"limitations\":[\"actual coverage limits\"]}. The response and report should be in English.", h.HostName)
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Minute)
	prompt += "\n在最终 JSON 中补充整数 point_count、RFC3339 first_point_at 和 last_point_at。它们必须来自实际返回点的统计；metric 只填写指标名，不含标签选择器。区分请求窗口、返回点评估时间和原始采样覆盖，不能按点数或请求起点猜测覆盖位置。 Also include the actual integer point_count and RFC3339 first_point_at/last_point_at from the result file. Metric is the catalog name without selectors. Returned evaluation times do not prove continuous raw measurement coverage."
	prompt += "\n下载的 Markdown 报告中也必须包含同一份 JSON 证据摘要，报告正文与摘要保持一致。Include the same JSON evidence summary inside the downloadable Markdown report, keeping its prose consistent with those facts."
	prompt += "\n为核对单位，请直接查询一个原始 CPU 或内存指标，不在查询表达式中缩放或聚合。必须用离线脚本读取实际查询文件，计算所有返回数值的 min/max，不得把首点当最小值或靠阅读数列猜统计值。在最终和报告 JSON 中同时填写 unit（目录原单位）、value_min、value_max、display_unit、display_transform、display_min、display_max，保留计算精度。原单位 By 时以 GiB 展示，display_transform=bytes_to_gib，除以 2**30；仅原始 system_cpu_utilization 或 system_memory_utilization 的单位 1 用百分比，display_transform=percent（乘100），或对 idle 使用 invert_percent（(1-x)*100，注意交换最小最大）；其他指标（包括单位 1 的数量或负载）用 identity。正文只能使用脚本计算的统计和换算结果。未验证的趋势原因须标为假设。 Query one direct original CPU/memory metric without arithmetic or aggregation in the query. Use an offline script on the actual result file to compute min/max across returned numeric values. Include unit, value_min/value_max and display_unit/display_transform/display_min/display_max in both JSON summaries. Preserve precision. Convert By to GiB using bytes_to_gib and 2**30; convert only system_cpu_utilization/system_memory_utilization with ratio unit 1 to percent, or CPU idle to invert_percent; use identity for other metrics, including dimensionless counts/load. Base prose statistics and conversions on that script. Label unverified causal explanations as hypotheses."
	defer cancel()
	sample, err := a.realModelTask(bounded, env, conversation, name, prompt, nil, nil)
	env.State.Values["plan8_last_model_run"] = sample.RunID
	proof["sample"] = sample
	proof["run_status"] = sample.RunStatus
	defer func() { failure = errors.Join(failure, a.planV2ModelEvidence(ctx, env, conversation, sample, failure)) }()
	if err != nil {
		return proof, err
	}
	if uuid.Validate(sample.RunID) != nil || sample.RunStatus != "succeeded" || sample.ModelCalls < 1 || sample.ModelCalls > 32 {
		return proof, errors.New("model completion or usage gate failed")
	}
	var usageFailure error
	if !sample.UsageComplete || !sample.CachedUsageComplete {
		usageFailure = errors.New("provider usage gate failed")
	}
	defer func() { failure = errors.Join(failure, usageFailure) }()
	raw, err := a.postgresQuery(bounded, env, plan11ModelToolTraceSQL(sample.RunID))
	if err != nil {
		return proof, err
	}
	var calls []plan8ModelToolFact
	if json.Unmarshal([]byte(raw), &calls) != nil {
		return proof, errors.New("model tool evidence invalid")
	}
	discovered, capability, catalog, read, published := false, false, false, false, false
	for _, call := range calls {
		if call.Status != "succeeded" {
			continue
		}
		read = read || call.Tool == "read" || call.Tool == "bash"
		if call.Tool != "tool.invoke" {
			continue
		}
		category, n := call.Input.Category, call.Input.Name
		discovered = discovered || (category == "observe" && n == "resources.list") || (category == "host" && n == "list")
		capability = capability || category == "observe" && n == "capabilities.get"
		catalog = catalog || category == "observe" && n == "catalog.query"
		published = published || category == "workflow" && n == "publish_file"
	}
	if !discovered || !capability || !catalog || !read || !published {
		return proof, errors.New("business discovery, actual Workspace reading or file delivery incomplete")
	}
	if err = verifyTypedToolDiscoveryOrder(raw, "observe"); err != nil {
		return proof, err
	}
	target, err := uuid.Parse(h.HostID)
	if err != nil {
		return proof, err
	}
	if err = plan8VerifyModelMetricTarget(raw, target); err != nil {
		return proof, err
	}
	for _, call := range calls {
		if call.Tool == "tool.invoke" && call.Status == "succeeded" && call.Input.Category == "metric" && call.Input.Name == "query" && !plan11ExactHostArguments(call.Input.Arguments, h.HostID) {
			return proof, errors.New("metric input included an unrequested resource")
		}
	}
	mutations, err := a.postgresQuery(bounded, env, "SELECT count(*) FROM pending_actions WHERE run_id='"+sample.RunID+"'")
	if err != nil || strings.TrimSpace(mutations) != "0" {
		return proof, errors.New("read-only discovery prepared a mutation")
	}
	reply, err := a.planV2ModelReply(bounded, env, sample.RunID)
	if err != nil {
		return proof, err
	}
	start := strings.LastIndex(reply, "```json")
	if start < 0 {
		return proof, errors.New("missing observed conclusion")
	}
	text := reply[start+7:]
	end := strings.Index(text, "```")
	if end < 0 {
		return proof, errors.New("incomplete observed conclusion")
	}
	var conclusion plan11Conclusion
	if json.Unmarshal([]byte(strings.TrimSpace(text[:end])), &conclusion) != nil || conclusion.ResourceID != h.HostID || conclusion.Metric == "" || len(conclusion.Limitations) == 0 {
		return proof, errors.New("conclusion lost resource, metric or limitations")
	}
	matchedMetric := false
	for _, call := range calls {
		if call.Tool == "tool.invoke" && call.Status == "succeeded" && call.Input.Category == "metric" && call.Input.Name == "query" {
			query, _ := call.Input.Arguments["query"].(string)
			matchedMetric = matchedMetric || strings.Contains(query, conclusion.Metric)
		}
	}
	if !matchedMetric {
		return proof, errors.New("conclusion metric was not actually queried")
	}
	queryEvidence, err := a.plan11VerifyQueryConclusion(bounded, env, conversation, sample.RunID, h.HostID, raw, conclusion)
	if err != nil {
		return proof, err
	}
	proof["query_evidence"] = queryEvidence
	reports, err := a.plan11VerifyDeliveredReports(bounded, env, conversation, sample.RunID, conclusion)
	if err != nil {
		return proof, err
	}
	proof["reports"] = reports
	proof["business_passed"] = true
	sample.Passed = usageFailure == nil
	proof["sample"] = sample
	proof["conclusion"] = conclusion
	proof["passed"] = usageFailure == nil
	return proof, nil
}
