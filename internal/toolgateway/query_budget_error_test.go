package toolgateway

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestNativeBudgetFailuresPreserveScopeAndSafeRecovery(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cause error
		scope string
	}{
		{"run", errors.Join(telemetry.ErrQueryBudget, errors.New("private database diagnostic")), "run"},
		{"query", errors.Join(queryengine.ErrBudget, errors.New("private SQL and credentials")), "query"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := mcp.NewRegistry()
			if err := registry.Register(mcp.Metadata{ID: "telemetry.promql.query", Discovery: fixtureDiscovery("budget contract"), Risk: "read", Visibility: mcp.Visible, InputVersion: "1", OutputVersion: "1", MaxResultBytes: 1024, InputSchema: emptyObjectSchema(), Execute: func(context.Context, mcp.Call) (mcp.Result, error) { return mcp.Result{}, tc.cause }}); err != nil {
				t.Fatal(err)
			}
			gateway, err := New(registry, "budget-feedback")
			if err != nil {
				t.Fatal(err)
			}
			set, err := toolruntime.NewSet(toolruntime.Snapshot{NativeCatalog: gateway.Revision}, gateway.CoreTools())
			if err != nil {
				t.Fatal(err)
			}
			_, err = set.Invoke(t.Context(), "tool.invoke", toolruntime.Invocation{Arguments: map[string]any{"category": "metric", "name": "query", "arguments": map[string]any{}}})
			var safe toolruntime.Error
			if !errors.Is(err, tc.cause) || !errors.As(err, &safe) || safe.Kind != "TOOL_QUERY_BUDGET_EXCEEDED" || safe.Details["budget_scope"] != tc.scope || safe.Details["query_budget"] != true {
				t.Fatal("native budget scope or typed cause lost")
			}
			if strings.Contains(safe.Message, "private") || strings.Contains(safe.Message, "credentials") {
				t.Fatal("private diagnostics entered model feedback")
			}
			if tc.scope == "run" && (safe.Details["retryable"] != false || safe.Details["stop_query_retries"] != true) {
				t.Fatal("exhausted Run invited another query")
			}
			if telemetry.ModelQueryError(err) != err {
				t.Fatal("budget normalization is not idempotent")
			}
		})
	}
	unknown := errors.New("private unknown error")
	if telemetry.ModelQueryError(nil) != nil || telemetry.ModelQueryError(unknown) != unknown {
		t.Fatal("unknown errors were relabeled as recoverable budgets")
	}
}
