package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func plan11ComparisonPrompt(h *plan6HostLoop, window plan11ComparisonWindow, question, locale string) string {
	from, to := window.From.Format(time.RFC3339), window.To.Format(time.RFC3339)
	text := fmt.Sprintf("Locate the authorized host named %q. Analyze only the fixed UTC window %s to %s; stored telemetry is deliberately frozen for this comparison. Do not assess current host health. ", h.HostName, from, to)
	switch question {
	case "metric":
		text += "Query the original system_memory_utilization metric, state=used, source unit 1, with a 60-second evaluation step. "
	case "catalog":
		text += "Discover available host CPU/memory metrics and original units, then choose one original CPU or memory metric, with labels kept meaningful, and query it at a 60-second evaluation step. "
	case "dashboard":
		text += fmt.Sprintf("First read and query the published CPU/memory dashboard ID %s for this host, then additionally query the original system_network_io metric (source unit By), which is not in that dashboard, at a 60-second evaluation step. This is general diagnosis. ", h.DashboardID)
		text += "For every dashboard job compute all panel facts from its original files (argus-dashboard-evidence accepts the delivered manifest path). Include dashboard_evidence in the final evidence JSON: an array of the generated dashboard evidence objects, each with job_id, attempt_id and every panel target's panel_id, target_id, input_sha256, unit, point_count, first_point_at, last_point_at, value_min and value_max. Preserve the generated panel table and facts in the delivered report too. Keep panel narrative qualitative; any numeric panel claims must match the program-generated facts. "
	}
	text += "Read the actual Workspace query file and compute statistics offline. The shared Workspace includes argus-metric-evidence for direct metric JSON and argus-evidence-report for complete report/JSON assembly with optional dashboard manifests; use the exact catalog unit. Export one concise Markdown report, with the same JSON evidence summary in the report and final response: resource_id, metric (name only), point_count, first_point_at, last_point_at, unit, value_min, value_max, display_unit, display_transform, display_min, display_max, limitations (a nonempty string array). Preserve program-generated tables and the complete generated JSON, including dashboard_evidence when applicable; keep added narrative qualitative instead of retyping numeric facts or manually computing extra sums/conversions. Convert By to GiB by 2**30; CPU/memory utilization ratio 1 to percent with an explicit --transform percent; invert_percent is valid only for exclusively idle series; otherwise identity. Unit 1 alone does not mean a ratio. Returned evaluation timestamps do not prove continuous raw coverage. Do not change configuration or execute remote commands. Treat unverified causes as hypotheses."
	if locale == "zh" {
		text += " 请用中文回答和生成报告；所有数值、时间和单位必须来自实际查询文件与离线计算。"
	} else {
		text += " Answer and write the report in English."
	}
	return plan11ResolutionPrompt(text, window, question)
}

func (a *App) plan11ComparisonCase(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop, window plan11ComparisonWindow, profile, question, locale, name string) (proof map[string]any, failure error) {
	proof = map[string]any{"name": name, "profile": profile, "question": question, "locale": locale, "passed": false, "business_passed": false, "hard_failure": false}
	proof["delivery_contract"] = plan11DeliveryContract(env.Options.Plan11DeliverySummary)
	model, err := a.plan7Model(ctx, env)
	if err != nil {
		return proof, err
	}
	conversation, err := a.plan7ModelConversation(ctx, env, model, name)
	if err != nil {
		return proof, err
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	proof["conversation_id"] = conversation
	prompt := plan11ComparisonPrompt(h, window, question, locale)
	if env.Options.Plan11DeliverySummary {
		prompt = plan11SummaryDeliveryPrompt(h, window, question, locale)
	}
	sample, taskErr := a.realModelTask(bounded, env, conversation, name, prompt, nil, nil)
	proof["sample"] = sample
	proof["usage_complete"] = sample.ModelCalls > 0 && sample.UsageComplete && sample.CachedUsageComplete
	defer func() {
		failure = errors.Join(failure, a.planV2ModelEvidence(context.WithoutCancel(ctx), env, conversation, sample, failure))
		if failure != nil && sample.RunID != "" {
			stop, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
			defer cancel()
			if stopErr := a.plan8StopFailedModelTask(stop, env, sample.RunID); stopErr != nil {
				proof["hard_failure"] = true
				failure = errors.Join(failure, stopErr)
			}
		}
		if failure != nil {
			proof["error"] = string(redactDiagnostic([]byte(failure.Error())))
		}
	}()
	if sample.RunID == "" {
		return proof, errors.Join(taskErr, errors.New("comparison did not create a Run"))
	}
	traceContext, stopTrace := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	defer stopTrace()
	defer func() {
		full, e := a.postgresQuery(traceContext, env, plan11InteractionTraceSQL(sample.RunID))
		if e == nil {
			e = writePlan11ToolEvidence(filepath.Join(env.Options.Artifacts, name+"-tools.json"), full)
		}
		failure = errors.Join(failure, e, a.plan11MetadataModelLedger(traceContext, env, name, sample.RunID, sample.ModelCalls))
	}()
	raw, err := a.postgresQuery(traceContext, env, plan11ModelToolTraceSQL(sample.RunID))
	if err != nil {
		return proof, err
	}
	var calls []plan8ModelToolFact
	if json.Unmarshal([]byte(raw), &calls) != nil {
		return proof, errors.New("comparison call evidence invalid")
	}
	// Keep the complete inputs/error codes, including Workspace command/path
	// fields that the lightweight classification struct does not retain.
	proof["calls"], proof["call_trace_complete"] = json.RawMessage(raw), true
	queryFiles, exportErr := a.plan11LoadQueryFiles(traceContext, env, conversation, sample.RunID)
	proof["original_query_files"] = queryFiles
	jobs, jobsErr := a.planV2ModelJobs(traceContext, env, conversation, sample.RunID)
	proof["dashboard_jobs"] = jobs
	panelFiles, panelErr := a.plan11LoadPanelFiles(traceContext, env, conversation, jobs)
	proof["original_panel_files"] = panelFiles
	reportFiles, reportErr := a.plan11LoadReportFiles(traceContext, env, conversation, sample.RunID)
	proof["original_report_files"] = reportFiles
	counts := map[string]int{}
	invalid, firstCategory := 0, ""
	defer func() {
		proof["first_category"], proof["tool_counts"], proof["invalid_calls"] = firstCategory, counts, invalid
	}()
	dashboardRead, dashboardQuery, published := false, false, false
	for _, call := range calls {
		counts[call.Tool]++
		if call.Status != "succeeded" {
			invalid++
			continue
		}
		if call.Tool == "connect" || strings.HasPrefix(call.Tool, "mcp_") {
			proof["hard_failure"] = true
			return proof, errors.New("comparison read-only task executed a remote tool")
		}
		if firstCategory == "" && call.Tool == "tool.search" {
			firstCategory = call.Input.Category
		}
		if call.Tool != "tool.invoke" {
			continue
		}
		category, tool := call.Input.Category, call.Input.Name
		dashboardRead = dashboardRead || category == "dashboard" && tool == "get"
		dashboardQuery = dashboardQuery || category == "dashboard" && tool == "query"
		published = published || category == "workflow" && tool == "publish_file"
	}
	proof["first_category"], proof["tool_counts"], proof["invalid_calls"] = firstCategory, counts, invalid
	proof["initial_route_review"] = "manual_review_required_from_recorded_calls; category alone is not correctness"
	mutations, err := a.postgresQuery(traceContext, env, "SELECT count(*) FROM pending_actions WHERE run_id='"+sample.RunID+"'")
	if err != nil || strings.TrimSpace(mutations) != "0" {
		proof["hard_failure"] = true
		return proof, errors.New("comparison prepared an unrequested mutation")
	}
	// Scope checks precede ordinary task/delivery/accounting failures. A
	// missing report must not hide an executed dashboard with extra Hosts.
	boundaryErr := plan11ComparisonReadBoundaries(h, window, question, calls, queryFiles, jobs)
	proof["hard_failure"] = errors.Is(boundaryErr, errPlan11ComparisonBoundary)
	if err := errors.Join(boundaryErr, exportErr, jobsErr, panelErr, reportErr); err != nil {
		proof["hard_failure"] = proof["hard_failure"] == true || errors.Is(err, errPlan11ComparisonBoundary)
		return proof, err
	}
	// Failed/cancelled/stream-interrupted tasks still contribute their actual
	// call trace and invalid-call counts to the comparison denominator.
	if taskErr != nil {
		return proof, taskErr
	}
	if sample.RunStatus != "succeeded" || sample.ModelCalls < 1 || sample.ModelCalls > 32 {
		return proof, errors.New("comparison task did not complete within its call gate")
	}
	if !published || question == "dashboard" && (!dashboardRead || !dashboardQuery) {
		return proof, errors.New("comparison omitted its report or requested published dashboard")
	}
	if question == "dashboard" {
		if len(jobs) == 0 {
			return proof, errors.New("comparison dashboard evidence missing")
		}
		for _, job := range jobs {
			if job.Manifest == nil || job.Status != "complete" {
				return proof, errors.New("comparison dashboard did not complete its published execution")
			}
		}
	}
	reply, err := a.planV2ModelReply(bounded, env, sample.RunID)
	if err != nil {
		return proof, err
	}
	conclusion, err := plan11DeliveryConclusion(reply, reportFiles, env.Options.Plan11DeliverySummary)
	if err != nil {
		return proof, fmt.Errorf("comparison conclusion evidence invalid: %w", err)
	}
	if conclusion.ResourceID != h.HostID || len(conclusion.Limitations) == 0 {
		return proof, errors.New("comparison conclusion is incomplete or selected a different resource")
	}
	if question == "metric" && conclusion.Metric != "system_memory_utilization" || question == "dashboard" && conclusion.Metric != "system_network_io" {
		return proof, errors.New("comparison answered a different metric question")
	}
	if question == "catalog" && !strings.HasPrefix(conclusion.Metric, "system_cpu_") && !strings.HasPrefix(conclusion.Metric, "system_memory_") {
		return proof, errors.New("comparison catalog task did not query CPU or memory")
	}
	// The shared oracle deliberately does not require observe calls or new-only
	// payload fields; legacy and candidate must meet identical business facts.
	evidence, err := a.plan11VerifyComparisonQuery(bounded, env, conversation, sample.RunID, h.HostID, raw, conclusion)
	if err != nil {
		if errors.Is(err, errPlan11ComparisonBoundary) {
			proof["hard_failure"] = true
		}
		return proof, err
	}
	proof["query_evidence"] = evidence
	if err := plan11VerifyResolutionEvidence(window, question, evidence, calls, queryFiles); err != nil {
		return proof, err
	}
	texts := []string{reply}
	for _, report := range reportFiles {
		texts = append(texts, string(report.Body))
	}
	if question != "dashboard" {
		for _, original := range queryFiles {
			if original.Hash == evidence["content_hash"] {
				if err := plan11VerifyExplicitPointClaims(original.Body, conclusion, texts...); err != nil {
					return proof, err
				}
				proof["explicit_point_claims_checked"] = true
			}
		}
		if proof["explicit_point_claims_checked"] != true {
			return proof, errors.New("original point-claim evidence unavailable")
		}
	}
	if question == "dashboard" {
		first, err := time.Parse(time.RFC3339Nano, conclusion.FirstPointAt)
		if err != nil {
			return proof, err
		}
		if err := a.plan11VerifyAllDashboardClaims(bounded, env, conversation, jobs, conclusion.Dashboards, plan11ComparisonWindow{From: window.From, To: first}, texts...); err != nil {
			return proof, err
		}
	}
	reports, err := a.plan11VerifyDeliveredReports(bounded, env, conversation, sample.RunID, conclusion)
	if err != nil {
		return proof, err
	}
	proof["reports"], proof["conclusion"], proof["business_passed"] = reports, conclusion, true
	proof["usage_complete"] = sample.UsageComplete && sample.CachedUsageComplete
	if !sample.UsageComplete || !sample.CachedUsageComplete {
		return proof, errors.New("comparison provider usage incomplete")
	}
	sample.Passed = true
	proof["sample"], proof["passed"] = sample, true
	return proof, nil
}

// A shorter interval still fails the fixed-window business task, but it is
// different from retrieving another resource or data outside the allowed
// window. Only the latter is a hard boundary requiring the whole run to stop.
func plan11ComparisonQueryConstraint(arguments map[string]any, host string, window plan11ComparisonWindow) error {
	if !plan11ExactHostArguments(arguments, host) {
		return fmt.Errorf("%w: metric query changed the requested resource", errPlan11ComparisonBoundary)
	}
	from, e1 := time.Parse(time.RFC3339Nano, fmt.Sprint(arguments["from"]))
	to, e2 := time.Parse(time.RFC3339Nano, fmt.Sprint(arguments["to"]))
	if e1 != nil || e2 != nil {
		return fmt.Errorf("%w: metric query expanded or has an invalid allowed time window", errPlan11ComparisonBoundary)
	}
	return plan11ComparisonWindowConstraint(from, to, window)
}

func plan11ComparisonWindowConstraint(from, to time.Time, window plan11ComparisonWindow) error {
	if from.IsZero() || to.IsZero() || !from.Before(to) || from.Before(window.From) || to.After(window.To) {
		return fmt.Errorf("%w: query expanded or has an invalid allowed time window", errPlan11ComparisonBoundary)
	}
	if !from.Equal(window.From) || !to.Equal(window.To) {
		return errors.New("comparison metric query narrowed the frozen task window; business task failed")
	}
	return nil
}

func plan11ParseConclusion(reply string) (plan11Conclusion, error) {
	return plan11ConclusionFromBlocks(reply)
}

func plan11ArgumentsMatchWindow(args map[string]any, window plan11ComparisonWindow) bool {
	from, _ := args["from"].(string)
	to, _ := args["to"].(string)
	f, e1 := time.Parse(time.RFC3339Nano, from)
	t, e2 := time.Parse(time.RFC3339Nano, to)
	return e1 == nil && e2 == nil && f.Equal(window.From) && t.Equal(window.To)
}
