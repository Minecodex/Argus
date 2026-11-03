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

func (a *App) plan11DeployedMetricSemantics(ctx context.Context, env *E2EEnvironment) (failure error) {
	if env.Options.Plan8Phase != "plan11-metrics" || env.Options.RealModel == nil {
		return errors.New("metric semantics requires the explicit selected-provider phase")
	}
	application := env.State.Values["p11_metric_application_id"]
	if uuid.Validate(application) != nil {
		return errors.New("metric semantics has no actual SDK application prerequisite")
	}
	to := time.Now().UTC().Truncate(time.Second)
	from := to.Add(-10 * time.Minute)
	proof := map[string]any{"schema_version": "argus.plan11_metric_semantics/v1", "passed": false,
		"application_id": application, "from": from, "to": to, "planned_tasks": plan8SelectedModelNames("plan11-metrics"),
		"provider_call_acceptance_limit_total": 192, "provider_probe_calls_separate": true,
		"scope":               "actual SDK cumulative counter/gauge/explicit histogram -> Collector -> Ingest -> Kafka -> Writer -> ClickHouse -> native query -> sealed Workspace originals; fixture expected values never in model prompts",
		"whole_plan_complete": false}
	var cases []map[string]any
	defer func() {
		proof["passed"], proof["cases"] = failure == nil, cases
		if failure != nil {
			proof["error"] = string(redactDiagnostic([]byte(failure.Error())))
		}
		raw, err := json.MarshalIndent(proof, "", "  ")
		failure = errors.Join(failure, err, writePrivate(filepath.Join(env.Options.Artifacts, "plan11-metric-semantics.json"), raw))
	}()
	for _, locale := range []string{"zh", "en"} {
		for repeat := 1; repeat <= 3; repeat++ {
			name := fmt.Sprintf("plan11-metric-semantics-%s-%d", locale, repeat)
			row, err := a.plan11PreparedMetadataTask(ctx, env, name, plan11MetricSemanticsPrompt(locale, application, from, to), nil, plan11MetricSemanticsTraceAllowed,
				func(ctx context.Context, conversation string, sample *p5BenchmarkSample, row map[string]any) error {
					return a.plan11VerifyMetricSemantics(ctx, env, name, conversation, application, from, to, sample, row)
				})
			cases = append(cases, row)
			if err != nil {
				failure = errors.Join(failure, err)
				if plan11MetricCaseMustStop(row, err) || ctx.Err() != nil {
					return failure
				}
			}
		}
	}
	return failure
}

func plan11MetricCaseMustStop(row map[string]any, err error) bool {
	return err != nil && (errors.Is(err, errPlan11NavigationBoundary) || row["actual_model_evaluated"] != true)
}

func plan11MetricSemanticsPrompt(locale, application string, from, to time.Time) string {
	window := from.Format(time.RFC3339Nano) + " / " + to.Format(time.RFC3339Nano)
	variants := " Also enumerate the actually observed type/unit variants of the same-named argus_plan11_sdk_variant on that selected source version. Explain why incompatible variants cannot be merged; their numeric values are not requested. Add conflict_variants to the small JSON as an array of objects with type and unit."
	if locale == "zh" {
		variants = " 另外逐项列出所选来源版本上同名 argus_plan11_sdk_variant 的已观测类型/单位变体，说明为何不能混合；无需查询这些冲突项的数值。在小JSON中增加 conflict_variants 数组，每项只含 type 和 unit。"
	}
	if locale == "zh" {
		return "只读分析应用 " + application + " 在绝对窗口 " + window + " 中名称以 argus_plan11_sdk_ 开头的实际已观测指标。请自行发现指标与真实类型/单位，选一个确有观测的来源版本，并在全部查询保持同一来源版本和窗口。给出窗口结束时请求累计值、队列当前值、延迟直方图累计观测数与平均延迟（秒），另外用适当的可处理重置的查询计算请求每秒速率。按当前框架证据分析完整小结果；大结果、截断或完整性不明时读取原件。以简洁的中文业务摘要回答，不重抄已有证据卡片或内部校验字段，不增加未请求的换算或教程；用三句短句说明累计量与速率、Gauge与累计量、均值与P95的区别，不声称未查询的分位数。不要修改配置、探测远端或读取凭据；不必发布报告。最后附一个小JSON便于核对，字段为 application_id、source_key、counter_value、gauge_value、histogram_count、histogram_mean_seconds、counter_rate_per_second、counter_is_rate、gauge_is_cumulative、histogram_mean_is_p95。" + variants
	}
	return "Read-only analyze actual observed metrics prefixed argus_plan11_sdk_ for application " + application + " in the absolute window " + window + ". Discover their real types/units, select one actually observed source version, and keep that same source version and window for every query. Give the request cumulative value, current queue value, histogram cumulative observation count and mean latency in seconds at the window end, plus a reset-aware per-second request rate query. Analyze complete small results certified by current framework evidence; read originals for large, truncated or uncertain results. Answer in concise English, without recreating existing evidence cards, exposing internal framework flags or adding unsolicited conversions/tutorials. In three short statements distinguish cumulative value from rate, gauge from cumulative quantity, and mean from P95; do not claim an unqueried percentile. Do not modify configuration, probe remote systems or read credentials; no published report is required. End with one small JSON for checking, with fields application_id, source_key, counter_value, gauge_value, histogram_count, histogram_mean_seconds, counter_rate_per_second, counter_is_rate, gauge_is_cumulative, histogram_mean_is_p95." + variants
}

func plan11MetricSemanticsTraceAllowed(raw string) error {
	var calls []struct {
		Tool  string
		Input struct{ Category, Name string }
	}
	if json.Unmarshal([]byte(raw), &calls) != nil || len(calls) == 0 {
		return errors.New("metric semantics trace missing")
	}
	for _, call := range calls {
		switch call.Tool {
		case "tool.search", "tool.describe", "read", "write", "edit", "bash":
		case "tool.invoke":
			allowed := call.Input.Category == "observe" && (call.Input.Name == "resources.list" || call.Input.Name == "capabilities.get" || call.Input.Name == "catalog.query")
			allowed = allowed || call.Input.Category == "metric" && call.Input.Name == "query"
			allowed = allowed || call.Input.Category == "application" && (call.Input.Name == "get" || call.Input.Name == "list" || call.Input.Name == "endpoint.list")
			if !allowed {
				return errPlan11NavigationBoundary
			}
		default:
			return errPlan11NavigationBoundary
		}
	}
	return nil
}

type plan11MetricSemanticsConclusion struct {
	Application string                `json:"application_id"`
	Source      string                `json:"source_key"`
	Counter     *float64              `json:"counter_value"`
	Gauge       *float64              `json:"gauge_value"`
	Count       *float64              `json:"histogram_count"`
	Mean        *float64              `json:"histogram_mean_seconds"`
	Rate        *float64              `json:"counter_rate_per_second"`
	CounterRate *bool                 `json:"counter_is_rate"`
	GaugeSum    *bool                 `json:"gauge_is_cumulative"`
	MeanP95     *bool                 `json:"histogram_mean_is_p95"`
	Variants    []plan11MetricVariant `json:"conflict_variants"`
}

type plan11MetricVariant struct {
	Type string `json:"type"`
	Unit string `json:"unit"`
}

func plan11ParseMetricSemanticsConclusion(reply string) (plan11MetricSemanticsConclusion, error) {
	var out plan11MetricSemanticsConclusion
	start := strings.LastIndex(reply, "```json")
	if start < 0 {
		return out, errors.New("metric semantics reply omitted its small structured conclusion")
	}
	text := reply[start+7:]
	end := strings.Index(text, "```")
	if end < 0 || json.Unmarshal([]byte(strings.TrimSpace(text[:end])), &out) != nil {
		return out, errors.New("invalid metric semantics conclusion JSON")
	}
	return out, nil
}
