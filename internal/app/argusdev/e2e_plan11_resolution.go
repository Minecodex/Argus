package argusdev

import (
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

func plan11ValidateHighResolution(enabled, summary bool, suite, phase string, profiles []string) error {
	if enabled && (!summary || suite != "plan8" || phase != "plan11-compare" || len(profiles) != 1 || profiles[0] != "candidate") {
		return errors.New("high-resolution evidence acceptance requires candidate-only standalone delivery_summary_v2")
	}
	return nil
}

func plan11NativeStep(window plan11ComparisonWindow) int {
	if window.HighResolution {
		return 1
	}
	return 60
}

func plan11ResolutionPrompt(text string, window plan11ComparisonWindow, question string) string {
	if !window.HighResolution {
		return text
	}
	text = strings.ReplaceAll(text, "60-second evaluation step", "1-second evaluation step")
	if question == "catalog" {
		text = strings.ReplaceAll(text, "choose one original CPU or memory metric, with labels kept meaningful", "choose the original system_cpu_utilization metric and preserve all actually returned state labels")
	}
	return text + " Query originals may exceed the inline projection budget: analyze the full sealed file with the offline helper, preserve every returned series, and do not calculate from a truncated preview."
}

// Validate the exact original selected by the report, not an arbitrary unused
// large query or the number of tool calls. Default historical gates are intact.
func plan11VerifyResolutionEvidence(window plan11ComparisonWindow, question string, evidence map[string]any, calls []plan8ModelToolFact, files []plan11QueryFile) error {
	if !window.HighResolution {
		return nil
	}
	var identity uuid.UUID
	switch value := evidence["query_call_id"].(type) {
	case uuid.UUID:
		identity = value
	case string:
		var err error
		identity, err = uuid.Parse(value)
		if err != nil {
			return errors.New("selected evidence has an invalid query identity")
		}
	default:
		return errors.New("selected evidence query identity is unavailable")
	}
	if identity == uuid.Nil {
		return errors.New("selected evidence has an empty query identity")
	}
	id := identity.String()
	matched := false
	for _, call := range calls {
		if call.ID == id && call.Tool == "tool.invoke" && call.Input.Category == "metric" && call.Status == "succeeded" {
			step, ok := call.Input.Arguments["step_seconds"].(float64)
			matched = ok && step == 1
		}
	}
	if !matched {
		return errors.New("selected evidence original did not use requested one-second resolution")
	}
	if question == "catalog" {
		for _, file := range files {
			if file.ID == id && plan11UnfilteredCPUSelector(file.Query) && len(file.Body) > toolruntime.MaxInlineQueryOriginalBytes {
				return nil
			}
		}
		return errors.New("large-original CPU acceptance did not retain an actual >32KiB unfiltered CPU original")
	}
	return nil
}

func plan11UnfilteredCPUSelector(expression string) bool {
	node, err := parser.NewParser(parser.Options{}).ParseExpr(expression)
	if err != nil {
		return false
	}
	for {
		if p, ok := node.(*parser.ParenExpr); ok {
			node = p.Expr
		} else {
			break
		}
	}
	selector, ok := node.(*parser.VectorSelector)
	if !ok || selector.OriginalOffset != 0 || selector.Timestamp != nil || selector.StartOrEnd != 0 {
		return false
	}
	name := selector.Name == "system_cpu_utilization"
	for _, matcher := range selector.LabelMatchers {
		if matcher.Name == "state" {
			if matcher.Type != labels.MatchRegexp || (matcher.Value != ".*" && matcher.Value != "^.*$") {
				return false
			}
		}
		if matcher.Name == labels.MetricName && matcher.Type == labels.MatchEqual && matcher.Value == "system_cpu_utilization" {
			name = true
		}
	}
	return name
}
