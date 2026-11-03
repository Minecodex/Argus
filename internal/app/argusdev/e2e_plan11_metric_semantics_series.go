package argusdev

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/kakj-go/Argus/internal/resourceref"
	"github.com/kakj-go/Argus/internal/telemetry/metricmeta"
	"github.com/prometheus/prometheus/promql/parser"
)

// One query can return several typed metrics and classic histogram family
// members. Evidence is counted by actual series, never by tool invocation count.
func plan11MetricSemanticValues(body []byte, expression, application, source string, from, to time.Time) (map[string]float64, error) {
	var out struct {
		Status  string
		Context struct {
			Resources []resourceref.Ref
			Sources   []string `json:"source_keys"`
			From, To  time.Time
		} `json:"query_context"`
		Meta struct {
			Partial  bool
			Input    []metricmeta.Descriptor `json:"input_metrics"`
			Complete bool                    `json:"input_metrics_complete"`
			Origin   string                  `json:"input_metrics_origin"`
		} `json:"argus_meta"`
		Data struct {
			Type   string `json:"resultType"`
			Result []struct {
				Metric map[string]string
				Value  []json.RawMessage
				Values [][]json.RawMessage
			}
		}
	}
	if json.Unmarshal(body, &out) != nil || out.Status != "success" || out.Meta.Partial || len(out.Context.Resources) != 1 || out.Context.Resources[0].Type != "application" || out.Context.Resources[0].ID.String() != application || len(out.Context.Sources) != 1 || out.Context.Sources[0] != source || !out.Context.From.Equal(from) || !out.Context.To.Equal(to) {
		return nil, errors.New("metric result escaped application/source/window or was partial")
	}
	if out.Data.Type != "vector" && out.Data.Type != "matrix" {
		return nil, errors.New("metric result has no complete typed input or actual returned series")
	}
	if len(out.Data.Result) == 0 {
		// A bounded exploratory empty query contributes no numeric fact. Keep
		// the original; later valid evidence must still establish every value.
		return map[string]float64{}, nil
	}
	if !out.Meta.Complete || out.Meta.Origin != metricmeta.Origin || len(out.Meta.Input) == 0 {
		return nil, errors.New("metric result has no complete typed input metadata")
	}
	inputs := map[string]metricmeta.Descriptor{}
	for _, input := range out.Meta.Input {
		if input.SourceKey != source {
			return nil, errors.New("metric input metadata changed source version")
		}
		if old, exists := inputs[input.Name]; exists && (old.Type != input.Type || old.Unit != input.Unit) {
			return nil, errors.New("metric query combined incompatible same-name input variants")
		}
		inputs[input.Name] = input
	}
	derived := plan11MetricExpressionKind(expression)
	node, err := parser.NewParser(parser.Options{}).ParseExpr(expression)
	if err != nil {
		return nil, err
	}
	for {
		if p, ok := node.(*parser.ParenExpr); ok {
			node = p.Expr
		} else {
			break
		}
	}
	batch := plan11RawMetricBatch(node)
	if !batch && derived == "" {
		// An unsupported exploration certifies zero values, including constant
		// or value-preserving set expressions. It cannot replace any required
		// fact. Every final value still needs a recognized independent original.
		return map[string]float64{}, nil
	}
	values := map[string]float64{}
	for _, row := range out.Data.Result {
		kind, inputName, wantType, wantUnit := derived, "argus_plan11_sdk_completed", "counter", "{request}"
		if batch {
			switch row.Metric["__name__"] {
			case "argus_plan11_sdk_completed":
				kind = "counter"
			case "argus_plan11_sdk_queue":
				kind = "gauge"
			case "argus_plan11_sdk_latency_count":
				kind = "histogram_count"
			case "argus_plan11_sdk_latency_sum":
				kind = "histogram_sum"
			case "argus_plan11_sdk_latency_bucket":
				// Bucket family members can arrive with a valid raw selector.
				// They are not ordinary samples for a mean or percentile.
				input, ok := inputs["argus_plan11_sdk_latency"]
				if !ok || input.Type != "histogram" || input.Unit != "s" {
					return nil, errors.New("histogram bucket lacks its actual original kind/unit")
				}
				continue
			default:
				return nil, errors.New("batch metric returned an unrelated or conflicting series")
			}
		}
		if kind == "gauge" {
			inputName, wantType = "argus_plan11_sdk_queue", "gauge"
		} else if strings.HasPrefix(kind, "histogram_") {
			inputName, wantType, wantUnit = "argus_plan11_sdk_latency", "histogram", "s"
		}
		input, exists := inputs[inputName]
		if !exists || input.Type != wantType || input.Unit != wantUnit {
			return nil, errors.New("actual returned metric kind/unit conflicts with original input metadata")
		}
		points := row.Values
		if out.Data.Type == "vector" {
			points = [][]json.RawMessage{row.Value}
		}
		found := false
		value := 0.0
		for _, point := range points {
			if len(point) != 2 {
				return nil, errors.New("invalid returned metric point")
			}
			var timestamp float64
			var text string
			if json.Unmarshal(point[0], &timestamp) != nil || json.Unmarshal(point[1], &text) != nil || math.IsNaN(timestamp) || math.IsInf(timestamp, 0) {
				return nil, errors.New("invalid metric point timestamp/value")
			}
			actual, err := strconv.ParseFloat(text, 64)
			if err != nil || math.IsNaN(actual) || math.IsInf(actual, 0) {
				return nil, errors.New("non-finite metric point")
			}
			if time.UnixMilli(int64(math.Round(timestamp*1000))).UnixMilli() == to.UnixMilli() {
				value, found = actual, true
			}
		}
		if !found {
			return nil, errors.New("metric query has no returned evaluation at the requested window end")
		}
		if _, duplicate := values[kind]; duplicate {
			return nil, errors.New("multiple series for one semantic value require explicit disambiguation")
		}
		values[kind] = value
	}
	if len(values) == 0 {
		return nil, errors.New("query returned no requested semantic values")
	}
	return values, nil
}

func plan11RawMetricBatch(node parser.Expr) bool {
	switch expr := node.(type) {
	case *parser.ParenExpr:
		return plan11RawMetricBatch(expr.Expr)
	case *parser.VectorSelector:
		return expr.OriginalOffset == 0 && expr.Timestamp == nil
	case *parser.BinaryExpr:
		return expr.Op == parser.LOR && plan11RawMetricBatch(expr.LHS) && plan11RawMetricBatch(expr.RHS)
	}
	return false
}
