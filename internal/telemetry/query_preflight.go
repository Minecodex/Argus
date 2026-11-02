package telemetry

import (
	"context"
	"errors"

	promqlparser "github.com/prometheus/prometheus/promql/parser"

	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	kqlengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/kql"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// Validate with the execution compiler before entering the metered backend.
// The placeholder table is never executed; real scope/authorization is already
// resolved by the caller and is still enforced again at the actual query RPC.
func executeValidatedModelQuery(ctx context.Context, backend EngineQueryBackend, request queryengine.Request) (queryengine.Result, error) {
	if request.Start.IsZero() || request.End.IsZero() || !request.Start.Before(request.End) {
		return queryengine.Result{}, toolruntime.Error{Kind: "TOOL_QUERY_INVALID", Message: "The query time window is invalid: from must be earlier than to. Preserve the user's original positive absolute window; an instant metric query evaluates at to and still requires that original from/to window. Do not collapse both bounds to the evaluation timestamp. No query was executed or Run query budget reserved.", Details: map[string]any{"query_validation": true, "path": "/arguments/from", "budget_reserved": false}}
	}
	if request.Language == queryengine.LanguagePromQL {
		expression, err := promqlparser.NewParser(promqlparser.Options{}).ParseExpr(request.Expression)
		if err != nil || !request.Instant && expression.Type() != promqlparser.ValueTypeVector && expression.Type() != promqlparser.ValueTypeScalar {
			return queryengine.Result{}, toolruntime.Error{Kind: "TOOL_QUERY_INVALID", Message: "The PromQL document is invalid or its top-level result type cannot be evaluated as a range query. Use a supported expression from the authorized metric catalog and preserve the requested resource/time scope. A range query must return an instant vector or scalar at each step. No query was executed or Run query budget reserved.", Details: map[string]any{"query_validation": true, "path": "/arguments/query", "language": "promql", "budget_reserved": false, "next_tool": map[string]string{"category": "observe", "name": "catalog.query"}, "requires_search_and_describe": true}}
		}
	}
	if request.Language == queryengine.LanguageKQL {
		_, err := kqlengine.CompileQuery("argus_log_validation", kqlengine.Request{
			Expression: request.Expression, Pipeline: request.Pipeline, Start: request.Start, End: request.End,
			Scope:  kqlengine.Scope{EnterpriseID: request.Scope.EnterpriseID, ResourceIDs: request.Scope.ResourceIDs, SourceKeys: request.Scope.SourceKeys},
			Budget: kqlengine.Budget{MaxRows: request.Budget.MaxRows, MaxScanBytes: request.Budget.MaxScanBytes, Timeout: request.Budget.Timeout},
		})
		if err != nil {
			if errors.Is(err, kqlengine.ErrBudget) {
				return queryengine.Result{}, ModelQueryError(queryengine.ErrBudget)
			}
			return queryengine.Result{}, toolruntime.Error{Kind: "TOOL_QUERY_INVALID", Message: modelLogSyntaxGuidance + " This document is outside the supported Argus log DSL. Start with * or a supported predicate; use parse/where/unwrap/stats/sort/limit/context stages. For example: * | limit 100, or * | stats count() by severity_text. Syslog table names, summarize, SeverityLevel and arbitrary upstream KQL are not supported. Discover actual fields through the authorized observation catalog before rewriting. Keep the requested resource/time scope. No query was executed or Run query budget reserved.", Details: map[string]any{"query_validation": true, "path": "/arguments/query", "language": "argus-kql", "budget_reserved": false, "next_tool": map[string]string{"category": "observe", "name": "catalog.query"}, "requires_search_and_describe": true}}
		}
	}
	return backend.ExecuteEngineQuery(ctx, request)
}
