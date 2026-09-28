package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboard"
)

// Structured conclusions are authored by the real model in Chat. The harness
// computes expected facts independently from immutable files, not from its text.
const planV2ConclusionPrompt = `实际取数，等待任务完成后用 read/bash 读取并分析本轮文件及清单。不得修改已发布查询。说明结论、各图的时间/资源范围、样本口径与未检查部分，不推断全量。在回答末尾用一个 JSON 代码块汇总，结构为 {"overall":"observed_issues 或 no_observed_issues 或 unknown","panels":[{"dashboard_id":"仪表盘ID","panel_id":"图ID","status":"清单中的状态；未能执行时为 not_checked","records":实际记录数,"metric_sum":指标 instant vector 的 value 数值总和或 null,"error_logs":日志 severity_text 为 ERROR 的记录数或 null}],"limitations":["实际限制说明"]}。每张本轮应检查的图恰好一项；未拿到数据时 records 为 0，metric_sum/error_logs 为 null。不要把未查询或未读取的数据说成已检查。`

type planV2ConclusionPanel struct {
	DashboardID    string   `json:"dashboard_id"`
	PanelID        string   `json:"panel_id"`
	Status         string   `json:"status"`
	AnalysisStatus string   `json:"analysis_status,omitempty"`
	QueryStatus    string   `json:"query_status,omitempty"`
	DeliveryStatus string   `json:"delivery_status,omitempty"`
	Records        int      `json:"records"`
	MetricSum      *float64 `json:"metric_sum"`
	ErrorLogs      *int     `json:"error_logs"`
}
type planV2Conclusion struct {
	Overall     string                  `json:"overall"`
	Panels      []planV2ConclusionPanel `json:"panels"`
	Limitations []string                `json:"limitations"`
	BudgetAfter *planV2BudgetFact       `json:"budget_after,omitempty"`
}

type planV2BudgetFact struct {
	Scan    int64 `json:"scan_bytes"`
	Bytes   int64 `json:"result_bytes"`
	Rows    int64 `json:"rows"`
	Samples int64 `json:"samples"`
	Calls   int64 `json:"calls"`
}
type planV2ExpectedQuery struct {
	Board     string
	Revision  string
	Seconds   int
	Pool      string
	Resources []uuid.UUID
	Panels    []string
	Explicit  bool
}

func (a *App) planV2ModelReply(ctx context.Context, env *E2EEnvironment, run string) (string, error) {
	if uuid.Validate(run) != nil {
		return "", fmt.Errorf("invalid model Run")
	}
	raw, err := a.postgresQuery(ctx, env, "SELECT coalesce(json_agg(payload->>'content' ORDER BY sequence),'[]') FROM conversation_events WHERE run_id='"+run+"' AND event_type='assistant_message' AND coalesce(payload->>'content','')<>'';")
	if err != nil {
		return "", err
	}
	var messages []string
	if err = json.Unmarshal([]byte(raw), &messages); err != nil {
		return "", err
	}
	if len(messages) == 0 {
		return "", fmt.Errorf("model returned no Chat conclusion")
	}
	return messages[len(messages)-1], nil
}

func planV2ParseConclusion(reply string) (planV2Conclusion, error) {
	var result planV2Conclusion
	blocks := regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)```").FindAllStringSubmatch(reply, -1)
	if len(blocks) == 0 {
		return result, fmt.Errorf("missing structured Chat conclusion")
	}
	err := json.Unmarshal([]byte(strings.TrimSpace(blocks[len(blocks)-1][1])), &result)
	if err == nil && !slices.Contains([]string{"unknown", "observed_issues", "no_observed_issues"}, result.Overall) {
		err = fmt.Errorf("invalid overall conclusion")
	}
	return result, err
}

func (a *App) planV2ModelJobs(ctx context.Context, env *E2EEnvironment, convo, run string) ([]dashboard.QueryJobView, error) {
	if uuid.Validate(run) != nil {
		return nil, fmt.Errorf("invalid model Run")
	}
	raw, err := a.postgresQuery(ctx, env, "SELECT id::text FROM dashboard_query_jobs WHERE run_id='"+run+"' ORDER BY created_at;")
	if err != nil {
		return nil, err
	}
	jobs := []dashboard.QueryJobView{}
	for _, id := range strings.Fields(raw) {
		job, err := a.planV2Job(ctx, env, "/conversations/"+convo+"/dashboard-queries/"+id)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func (a *App) planV2VerifyModelQueries(ctx context.Context, env *E2EEnvironment, convo, run string, want []planV2ExpectedQuery) error {
	jobs, err := a.planV2ModelJobs(ctx, env, convo, run)
	if err != nil {
		return err
	}
	if len(jobs) != len(want) {
		return fmt.Errorf("expected %d queries, got %d", len(want), len(jobs))
	}
	expected := map[string]planV2ExpectedQuery{}
	for _, w := range want {
		expected[w.Board] = w
	}
	for _, job := range jobs {
		w, ok := expected[job.DashboardID.String()]
		if !ok || job.Status != "complete" || job.Manifest == nil {
			return fmt.Errorf("unexpected/incomplete query %s: %s", job.DashboardID, job.Status)
		}
		delete(expected, job.DashboardID.String())
		e := job.Manifest.Execution
		if e.RevisionID.String() != w.Revision || int(e.To.Sub(e.From)/time.Second) != w.Seconds || len(e.Panels) != len(w.Panels) {
			return fmt.Errorf("revision/time/panel scope mismatch for %s", w.Board)
		}
		for _, p := range e.Panels {
			if !slices.Contains(w.Panels, p.ID) {
				return fmt.Errorf("unexpected panel %s", p.ID)
			}
		}
		if w.Pool != "" && (e.Variables["pool"].All || !slices.Equal(e.Variables["pool"].Values, []string{w.Pool})) {
			return fmt.Errorf("variable inheritance mismatch")
		}
		if len(w.Resources) > 0 {
			if len(e.Resources) != len(w.Resources) {
				return fmt.Errorf("resource inheritance expanded")
			}
			for _, r := range e.Resources {
				if !slices.Contains(w.Resources, r.ID) {
					return fmt.Errorf("unexpected resource")
				}
			}
		}
		var state struct {
			Overrides map[string]json.RawMessage `json:"overrides"`
		}
		raw, err := a.postgresQuery(ctx, env, "SELECT state FROM dashboard_analysis_contexts WHERE id='"+job.Manifest.AnalysisContextID.String()+"';")
		if err != nil {
			return err
		}
		if err = json.Unmarshal([]byte(raw), &state); err != nil {
			return err
		}
		if !w.Explicit {
			for key, value := range state.Overrides {
				if key == "time" || key == "resources" || string(value) != "{}" {
					return fmt.Errorf("implicit defaults became explicit conditions: %s", key)
				}
			}
		}
	}
	return a.planV2VerifyConclusion(ctx, env, convo, run, jobs)
}

func (a *App) planV2VerifyConclusion(ctx context.Context, env *E2EEnvironment, convo, run string, jobs []dashboard.QueryJobView) error {
	reply, err := a.planV2ModelReply(ctx, env, run)
	if err != nil {
		return err
	}
	conclusion, err := planV2ParseConclusion(reply)
	if err != nil {
		return err
	}
	want := map[string]planV2ConclusionPanel{}
	client, _ := scenarioHTTP(env)
	issues, unknown := false, false
	for _, job := range jobs {
		if job.Manifest == nil {
			return fmt.Errorf("missing query manifest")
		}
		for _, p := range job.Manifest.Execution.Panels {
			w := planV2ConclusionPanel{DashboardID: job.DashboardID.String(), PanelID: p.ID, Status: p.Status}
			if p.Status != "success" {
				unknown = true
			}
			for _, f := range job.Files {
				if f.Kind != "data" || f.PanelID != p.ID {
					continue
				}
				if f.WorkspaceFileID == nil {
					return fmt.Errorf("undelivered data file")
				}
				body, _, err := p5FileHTTP(ctx, client, env, http.MethodGet, "/conversations/"+convo+"/workspace/files/"+f.WorkspaceFileID.String()+"/content", nil, 200, "")
				if err != nil {
					return err
				}
				var data any
				if err = json.Unmarshal(body, &data); err != nil {
					return err
				}
				w.Records += planV2RecordCount(data)
				for _, def := range job.Manifest.Definition.Panels {
					if def.ID != p.ID {
						continue
					}
					if def.Signal == "metrics" {
						sum, ok := planV2MetricSum(data)
						if !ok && w.Records > 0 {
							return fmt.Errorf("unsupported numeric fixture shape")
						}
						w.MetricSum = &sum
					}
					if def.Signal == "logs" {
						count := planV2ErrorLogCount(data)
						w.ErrorLogs = &count
						issues = issues || count > 0
					}
				}
			}
			want[w.DashboardID+"/"+w.PanelID] = w
		}
	}
	if len(conclusion.Panels) != len(want) {
		return fmt.Errorf("Chat omitted or invented panel coverage")
	}
	for _, got := range conclusion.Panels {
		key := got.DashboardID + "/" + got.PanelID
		w, ok := want[key]
		if !ok || got.Status != w.Status || got.Records != w.Records || !planV2OptionalNumber(got.MetricSum, w.MetricSum) || !planV2OptionalInt(got.ErrorLogs, w.ErrorLogs) {
			return fmt.Errorf("Chat facts disagree with immutable data for %s: got=%+v want=%+v", key, got, w)
		}
		delete(want, key)
	}
	if issues && conclusion.Overall != "observed_issues" {
		return fmt.Errorf("received ERROR logs omitted from conclusion")
	}
	if !issues && unknown && conclusion.Overall != "unknown" {
		return fmt.Errorf("unknown coverage claimed healthy")
	}
	if !issues && !unknown && conclusion.Overall != "no_observed_issues" {
		return fmt.Errorf("invented issue in known fixture")
	}
	if unknown && len(conclusion.Limitations) == 0 {
		return fmt.Errorf("unknown coverage has no explanation")
	}
	reads, err := a.postgresQuery(ctx, env, "SELECT count(*) FROM tool_calls WHERE run_id='"+run+"' AND status='succeeded' AND tool_id IN ('read','bash');")
	if err != nil {
		return err
	}
	if strings.TrimSpace(reads) == "0" {
		return fmt.Errorf("conclusion without actual file analysis")
	}
	return nil
}

func planV2OptionalNumber(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return math.Abs(*a-*b) < 1e-8
}
func planV2OptionalInt(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
func planV2MetricSum(value any) (float64, bool) {
	var sum float64
	found := false
	switch v := value.(type) {
	case []any:
		for _, x := range v {
			n, ok := planV2MetricSum(x)
			sum += n
			found = found || ok
		}
	case map[string]any:
		if pair, ok := v["value"].([]any); ok && len(pair) == 2 {
			n, err := strconv.ParseFloat(fmt.Sprint(pair[1]), 64)
			return n, err == nil
		}
		for _, x := range v {
			n, ok := planV2MetricSum(x)
			sum += n
			found = found || ok
		}
	}
	return sum, found
}
func planV2ErrorLogCount(value any) int {
	n := 0
	switch v := value.(type) {
	case []any:
		for _, x := range v {
			n += planV2ErrorLogCount(x)
		}
	case map[string]any:
		if v["severity_text"] == "ERROR" {
			return 1
		}
		for _, x := range v {
			n += planV2ErrorLogCount(x)
		}
	}
	return n
}

func (a *App) planV2ModelEvidence(ctx context.Context, env *E2EEnvironment, convo string, sample p5BenchmarkSample, oracleErr error) error {
	reply, replyErr := a.planV2ModelReply(ctx, env, sample.RunID)
	jobs, jobErr := a.planV2ModelJobs(ctx, env, convo, sample.RunID)
	message := ""
	if oracleErr != nil {
		message = oracleErr.Error()
	}
	body, _ := json.MarshalIndent(map[string]any{"sample": sample, "reply": reply, "jobs": jobs, "oracle_error": message, "reply_available": replyErr == nil, "jobs_available": jobErr == nil}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, sample.ID+"-evidence.json"), body)
}
