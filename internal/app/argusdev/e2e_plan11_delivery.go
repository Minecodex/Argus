package argusdev

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

func plan11DeliveryContract(summary bool) string {
	if summary {
		return "delivery_summary_v2"
	}
	return "strict_json_v1"
}

func plan11ValidateDeliverySummary(enabled bool, suite, phase string, profiles []string) error {
	if enabled && (suite != "plan8" || phase != "plan11-compare" || len(profiles) != 1 || profiles[0] != "candidate") {
		return errors.New("delivery_summary_v2 requires explicit candidate-only standalone acceptance; legacy/comparative or other phases retain their original output contract")
	}
	return nil
}

func plan11SummaryDeliveryPrompt(h *plan6HostLoop, window plan11ComparisonWindow, question, locale string) string {
	text := fmt.Sprintf("Locate the authorized host named %q. Analyze only the fixed UTC window %s to %s; stored telemetry is deliberately frozen. Do not assess current host health. ", h.HostName, window.From.Format(time.RFC3339), window.To.Format(time.RFC3339))
	switch question {
	case "metric":
		text += "Query the original system_memory_utilization metric, state=used, source unit 1, with a 60-second evaluation step. "
	case "catalog":
		text += "Discover available host CPU/memory metrics and original units, then choose one original CPU or memory metric, with labels kept meaningful, and query it at a 60-second evaluation step. "
	case "dashboard":
		text += fmt.Sprintf("First read and query the published CPU/memory dashboard ID %s for this host, then additionally query the original system_network_io metric (source unit By), which is not in that dashboard, at a 60-second evaluation step. This is general diagnosis. Preserve every executed dashboard job and all panel targets in the delivered report's complete dashboard_evidence JSON and tables. ", h.DashboardID)
	}
	text += "Read the original Workspace files and compute statistics offline. Prefer argus-evidence-report with the original metric file, its workspace_file.sha256 and every delivered dashboard manifest. Type/unit come from original authorized query metadata; caller hints cannot certify them. Export and publish one concise Markdown report containing the complete program-generated evidence JSON and all tables: exact resource identity, metric name, point count, first/last observation times, raw unit/min/max, display unit/transform/min/max, nonempty limitations, and every dashboard job/attempt/panel with its input SHA256 and facts. Convert By to GiB by 2**30; confirmed CPU/memory utilization ratios use explicit --transform percent; invert_percent only for exclusively idle series, otherwise identity. Unit 1 alone is not a ratio. In your final response give a concise qualitative business summary, observations and limitations plus the actual delivered report reference. The complete JSON belongs in that report and need not be duplicated in the final response; if you do include evidence JSON, it must be complete and agree with the report. Do not retype opaque IDs or numerical tables. Restrict trends and stability to returned observations; requested time bounds and evaluation timestamps do not prove continuous raw coverage or ingestion completeness. Cumulative counter magnitude does not establish interval activity, volume or throughput without reset-aware calculation. Do not change configuration or execute remote commands. Treat unverified causes as hypotheses."
	if locale == "zh" {
		return plan11ResolutionPrompt(text+" 请用中文回答和生成报告。", window, question)
	}
	return plan11ResolutionPrompt(text+" Answer and write the report in English.", window, question)
}

// Select only the original sealed delivery. The caller still independently
// checks exact scope, raw arithmetic, display policy and every dashboard panel.
// This helper cannot make a report correct merely because it was published.
func plan11DeliveryConclusion(reply string, reports []plan11ReportFile, summary bool) (plan11Conclusion, error) {
	if !summary {
		return plan11ParseConclusion(reply)
	}
	var empty plan11Conclusion
	if len(reports) != 1 || strings.TrimSpace(reply) == "" {
		return empty, errors.New("summary delivery requires exactly one actual report and a final answer")
	}
	report := reports[0]
	if uuid.Validate(report.ID) != nil {
		return empty, errors.New("summary delivery has no valid immutable delivery identifier")
	}
	if report.Bytes != len(report.Body) || fmt.Sprintf("%x", sha256.Sum256(report.Body)) != report.Hash {
		return empty, errors.New("summary delivery report original/hash is unavailable")
	}
	if !strings.Contains(reply, report.ID) && (report.Name == "" || !strings.Contains(reply, report.Name)) {
		return empty, errors.New("final summary does not reference its actual delivered report")
	}
	conclusion, err := plan11ParseConclusion(string(report.Body))
	if err != nil {
		return empty, fmt.Errorf("delivered report lacks valid machine evidence: %w", err)
	}
	if err := plan11MatchReportConclusion(report.Body, conclusion); err != nil {
		return empty, fmt.Errorf("delivered report lacks complete machine evidence: %w", err)
	}
	// Optional final JSON remains binding: incomplete/conflicting evidence is
	// not excused by the presence of a correct report.
	includesJSON := strings.Contains(strings.ToLower(reply), "```json") || strings.HasPrefix(strings.TrimSpace(reply), "{")
	for _, field := range []string{`"resource_id"`, `"metric_evidence"`, `"dashboard_evidence"`, `"point_count"`, `"display_transform"`} {
		includesJSON = includesJSON || strings.Contains(reply, field)
	}
	if includesJSON {
		final, err := plan11ParseConclusion(reply)
		if err != nil || plan11MatchReportConclusion(report.Body, final) != nil {
			return empty, errors.New("optional final evidence JSON is incomplete or conflicts with its report")
		}
	}
	return conclusion, nil
}
