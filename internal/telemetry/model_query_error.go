package telemetry

import (
	"errors"

	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// ModelQueryError preserves domain sentinels while exposing only authored
// recovery facts at the native tool boundary. It never refunds/reset budgets.
func ModelQueryError(err error) error {
	if err == nil {
		return nil
	}
	var already toolruntime.Error
	if errors.As(err, &already) {
		return err
	}
	if errors.Is(err, ErrQueryBudget) {
		return errors.Join(err, toolruntime.Error{Kind: "TOOL_QUERY_BUDGET_EXCEEDED", Message: "The shared query/catalog budget for this Run is exhausted or unavailable. Stop data-query retries in this Run. Use the evidence already collected and explicitly report unfinished work. No result was delivered.", Details: map[string]any{"query_budget": true, "budget_scope": "run", "retryable": false, "stop_query_retries": true}})
	}
	if errors.Is(err, queryengine.ErrBudget) {
		return errors.Join(err, toolruntime.Error{Kind: "TOOL_QUERY_BUDGET_EXCEEDED", Message: "The query exceeded its execution budget. Check the current server Run budget before retrying. Preserve the requested resources and fixed time window. Discover unknown metric names through the authorized catalog; narrow the selector only if budget remains. No result was delivered.", Details: map[string]any{"query_budget": true, "budget_scope": "query", "retry_condition": "run_budget_remaining", "next_tool": map[string]string{"category": "observe", "name": "catalog.query"}, "requires_search_and_describe": true}})
	}
	return err
}
