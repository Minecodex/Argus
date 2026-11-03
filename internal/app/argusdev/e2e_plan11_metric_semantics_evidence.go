package argusdev

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/sourcekey"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

func (a *App) plan11VerifyMetricSemantics(ctx context.Context, env *E2EEnvironment, name, conversation, application string, from, to time.Time, sample *p5BenchmarkSample, proof map[string]any) error {
	if sample.RunStatus != "succeeded" {
		return errors.New("metric semantics task did not complete autonomously")
	}
	reply, err := a.planV2ModelReply(ctx, env, sample.RunID)
	if err != nil {
		return err
	}
	raw, err := a.postgresQuery(ctx, env, plan11MetricSemanticsQueriesSQL(sample.RunID))
	if err != nil {
		return err
	}
	var facts []struct {
		ID, Query, Hash string
		Bytes           int
		File            string
	}
	if json.Unmarshal([]byte(raw), &facts) != nil || len(facts) == 0 {
		return errors.New("metric semantics omitted actual sealed queries")
	}
	trace, err := a.postgresQuery(ctx, env, plan11InteractionTraceSQL(sample.RunID))
	if err != nil {
		return err
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	observed := map[string]float64{}
	var originals []map[string]any
	var unverifiedConsumption []string
	bodies := map[string][]byte{}
	for _, fact := range facts {
		if uuid.Validate(fact.ID) != nil || uuid.Validate(fact.File) != nil {
			return errors.New("metric original has no actual workspace file identity")
		}
		body, _, err := p5FileHTTP(ctx, client, env, http.MethodGet, "/conversations/"+conversation+"/workspace/files/"+fact.File+"/content", nil, 200, "")
		if err != nil {
			return err
		}
		if len(body) != fact.Bytes || fmt.Sprintf("%x", sha256.Sum256(body)) != fact.Hash {
			return errors.New("metric semantics original differs from its immutable artifact")
		}
		if err := writePrivate(filepath.Join(env.Options.Artifacts, name+"-original-"+fact.ID+".json"), body); err != nil {
			return err
		}
		bodies[fact.ID] = body
	}
	// Capture every original before checking the model conclusion or semantics.
	// A failed business assertion must not discard the evidence needed to audit it.
	conclusion, err := plan11ParseMetricSemanticsConclusion(reply)
	if err != nil {
		return err
	}
	if conclusion.Application != application || conclusion.CounterRate == nil || *conclusion.CounterRate || conclusion.GaugeSum == nil || *conclusion.GaugeSum || conclusion.MeanP95 == nil || *conclusion.MeanP95 {
		return errors.New("metric semantics conclusion changed identity or cumulative/gauge/mean interpretation")
	}
	variants := map[string]bool{}
	for _, variant := range conclusion.Variants {
		variants[variant.Type+"/"+variant.Unit] = true
	}
	if len(conclusion.Variants) != 3 || len(variants) != 3 || !variants["counter/By"] || !variants["gauge/By"] || !variants["gauge/s"] {
		return errors.New("same-name metric type/unit variants were merged or invented")
	}
	catalog, err := a.postgresQuery(ctx, env, plan11MetricSemanticsCatalogSQL(sample.RunID, conclusion.Source))
	if err != nil || strings.TrimSpace(catalog) != "6" {
		return errors.Join(err, errors.New("actual source-bound metric kinds and same-name variants were not discovered"))
	}
	for _, fact := range facts {
		body := bodies[fact.ID]
		values, err := plan11MetricSemanticValues(body, fact.Query, application, conclusion.Source, from, to)
		if err != nil {
			return fmt.Errorf("metric semantics original %s: %w", fact.ID, err)
		}
		consumed := plan11MetricOriginalConsumed(trace, "/workspace/"+toolruntime.NativeQueryFile("telemetry.promql.query", uuid.MustParse(fact.ID)), fact.Hash, fact.Bytes)
		inline := plan11MetricInlineOriginal(trace, fact.ID, fact.Hash, body)
		if inline {
			// Result availability alone does not prove exposure to a later model.
			visible, e := a.postgresQuery(ctx, env, plan11InlineModelExposureSQL(sample.RunID, fact.ID))
			if e != nil {
				return e
			}
			inline = strings.TrimSpace(visible) == "1"
		}
		consumed = consumed || inline
		if len(values) > 0 && !consumed {
			unverifiedConsumption = append(unverifiedConsumption, fact.ID)
		}
		for kind, value := range values {
			if old, exists := observed[kind]; exists && !plan11MetricClose(old, value) {
				return errors.New("repeated same-window metric query produced conflicting facts")
			}
			observed[kind] = value
		}
		originals = append(originals, map[string]any{"call_id": fact.ID, "query": fact.Query, "sha256": fact.Hash, "bytes": fact.Bytes, "semantic_values": values, "consumption_required": len(values) > 0, "consumption_verified_by_checker": consumed, "complete_inline_original_exposed": inline})
	}
	if _, exists := observed["histogram_mean"]; !exists {
		if sum, found := observed["histogram_sum"]; found && observed["histogram_count"] > 0 {
			observed["histogram_mean"] = sum / observed["histogram_count"]
		}
	}
	for kind, expected := range map[string]float64{"counter": 5, "gauge": 7, "histogram_count": 3, "histogram_mean": .2} {
		if actual, exists := observed[kind]; !exists || !plan11MetricClose(actual, expected) {
			return fmt.Errorf("actual SDK %s value absent or wrong: %g", kind, actual)
		}
	}
	if value, exists := observed["counter_rate"]; !exists || value <= 0 {
		return errors.New("reset-aware counter rate has no actual positive observation")
	}
	for _, claim := range []struct {
		kind  string
		value *float64
	}{{"counter", conclusion.Counter}, {"gauge", conclusion.Gauge}, {"histogram_count", conclusion.Count}, {"histogram_mean", conclusion.Mean}, {"counter_rate", conclusion.Rate}} {
		if claim.value == nil || !plan11MetricClose(*claim.value, observed[claim.kind]) {
			return fmt.Errorf("final %s claim disagrees with independently downloaded original", claim.kind)
		}
	}
	proof["conclusion"], proof["original_queries"], proof["analysis_evidence_version"] = conclusion, originals, "v2"
	if len(unverifiedConsumption) > 0 {
		proof["unverified_consumption"] = unverifiedConsumption
		return errors.New("contributing query original consumption could not be verified by this checker; review the actual process receipts")
	}
	return nil
}

func plan11MetricSemanticsCatalogSQL(run, source string) string {
	_, _, _, valid := sourcekey.Parse(source)
	if uuid.Validate(run) != nil || !valid {
		return "SELECT 0"
	}
	return `SELECT count(DISTINCT (metric->>'name',metric->>'type',metric->>'unit')) FROM tool_calls t JOIN tool_results r ON r.tool_call_id=t.id AND r.enterprise_id=t.enterprise_id
CROSS JOIN LATERAL jsonb_array_elements(coalesce(r.projection->'summary'->'metrics','[]'::jsonb)) metric
WHERE t.run_id='` + run + `' AND t.source='argus' AND t.status='succeeded' AND t.tool_id='tool.invoke'
 AND t.input->>'category'='observe' AND t.input->>'name'='catalog.query' AND metric->>'observed'='true'
 AND metric->'source_keys' @> jsonb_build_array('` + source + `'::text)
 AND ((metric->>'name'='argus_plan11_sdk_completed' AND metric->>'type'='counter' AND metric->>'unit'='{request}')
 OR (metric->>'name'='argus_plan11_sdk_queue' AND metric->>'type'='gauge' AND metric->>'unit'='{request}')
 OR (metric->>'name'='argus_plan11_sdk_latency' AND metric->>'type'='histogram' AND metric->>'unit'='s')
 OR (metric->>'name'='argus_plan11_sdk_variant' AND ((metric->>'type'='counter' AND metric->>'unit'='By') OR (metric->>'type'='gauge' AND metric->>'unit' IN ('By','s')))))`
}

func plan11MetricSemanticsQueriesSQL(run string) string {
	return `SELECT coalesce(json_agg(json_build_object('id',t.id,'query',t.input->'arguments'->>'query','hash',encode(a.content_hash,'hex'),'bytes',a.byte_size,'file',f.id) ORDER BY t.created_at,t.call_sequence),'[]')
FROM tool_calls t JOIN tool_results r ON r.tool_call_id=t.id AND r.enterprise_id=t.enterprise_id JOIN artifacts a ON a.id=r.artifact_id
LEFT JOIN workspace_files f ON f.enterprise_id=t.enterprise_id AND f.conversation_id=a.conversation_id AND f.deleted_at IS NULL
 AND f.path='queries/metrics/'||t.id::text||'.json' AND a.result_ref=ANY(f.source_result_refs)
WHERE t.run_id='` + run + `' AND t.source='argus' AND t.tool_id='tool.invoke' AND t.status='succeeded'
 AND t.input->>'category'='metric' AND t.input->>'name'='query'`
}

func plan11MetricExpressionKind(expression string) string {
	expr, err := parser.NewParser(parser.Options{}).ParseExpr(expression)
	if err != nil {
		return ""
	}
	var unwrap func(parser.Expr) parser.Expr
	unwrap = func(expr parser.Expr) parser.Expr {
		switch e := expr.(type) {
		case *parser.ParenExpr:
			return unwrap(e.Expr)
		case *parser.AggregateExpr:
			if e.Op == parser.SUM || e.Op == parser.AVG || e.Op == parser.MAX || e.Op == parser.MIN {
				return unwrap(e.Expr)
			}
		}
		return expr
	}
	selector := func(expr parser.Expr, name string) bool {
		v, ok := unwrap(expr).(*parser.VectorSelector)
		if !ok || v.OriginalOffset != 0 || v.Timestamp != nil {
			return false
		}
		metricName := v.Name
		if metricName == "" {
			for _, matcher := range v.LabelMatchers {
				if matcher.Name == "__name__" && matcher.Type == labels.MatchEqual {
					metricName = matcher.Value
				}
			}
		}
		return metricName == "argus_plan11_sdk_"+name
	}
	expr = unwrap(expr)
	if selector(expr, "completed") {
		return "counter"
	}
	if selector(expr, "queue") {
		return "gauge"
	}
	if selector(expr, "latency_count") {
		return "histogram_count"
	}
	if selector(expr, "latency_sum") {
		return "histogram_sum"
	}
	if binary, ok := expr.(*parser.BinaryExpr); ok && binary.Op == parser.DIV && selector(binary.LHS, "latency_sum") && selector(binary.RHS, "latency_count") {
		return "histogram_mean"
	}
	if binary, ok := expr.(*parser.BinaryExpr); ok && binary.Op == parser.DIV {
		call, callOK := unwrap(binary.LHS).(*parser.Call)
		seconds, secondsOK := unwrap(binary.RHS).(*parser.NumberLiteral)
		if callOK && secondsOK && call.Func.Name == "increase" && len(call.Args) == 1 {
			if matrix, ok := call.Args[0].(*parser.MatrixSelector); ok && selector(matrix.VectorSelector, "completed") && seconds.Val > 0 && seconds.Val == matrix.Range.Seconds() {
				return "counter_rate"
			}
		}
	}
	if call, ok := expr.(*parser.Call); ok && (call.Func.Name == "rate" || call.Func.Name == "irate") && len(call.Args) == 1 {
		if matrix, ok := call.Args[0].(*parser.MatrixSelector); ok && selector(matrix.VectorSelector, "completed") {
			return "counter_rate"
		}
	}
	return ""
}

func plan11MetricClose(a, b float64) bool {
	return !math.IsNaN(a) && !math.IsInf(a, 0) && !math.IsNaN(b) && !math.IsInf(b, 0) && math.Abs(a-b) <= math.Max(1e-9, math.Abs(b)*1e-5)
}
