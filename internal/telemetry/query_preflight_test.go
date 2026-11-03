package telemetry

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type preflightBackend struct {
	calls   int
	request queryengine.Request
}

func TestInvalidQueryTimeWindowNeverEntersMeteredBackend(t *testing.T) {
	for _, language := range []queryengine.Language{queryengine.LanguagePromQL, queryengine.LanguageKQL, queryengine.LanguageTrace} {
		for _, instant := range []bool{false, true} {
			for _, window := range [][2]time.Time{{{}, {}}, {time.Unix(100, 0), time.Unix(100, 0)}, {time.Unix(200, 0), time.Unix(100, 0)}} {
				backend := &preflightBackend{}
				request := queryengine.Request{Language: language, Expression: "up", Instant: instant, Start: window[0], End: window[1]}
				if language == queryengine.LanguageKQL {
					request.Expression = "*"
				}
				_, err := executeValidatedModelQuery(t.Context(), backend, request)
				var safe toolruntime.Error
				if !errors.As(err, &safe) || safe.Kind != "TOOL_QUERY_INVALID" || safe.Details["budget_reserved"] != false || backend.calls != 0 {
					t.Fatalf("invalid time entered metered backend: language=%s instant=%v calls=%d err=%v", language, instant, backend.calls, err)
				}
			}
		}
	}
	backend := &preflightBackend{}
	request := queryengine.Request{Language: queryengine.LanguagePromQL, Expression: "up", Instant: true, Start: time.Unix(100, 0), End: time.Unix(200, 0)}
	if _, err := executeValidatedModelQuery(t.Context(), backend, request); err != nil || backend.calls != 1 || !backend.request.Instant || !backend.request.Start.Equal(request.Start) || !backend.request.End.Equal(request.End) {
		t.Fatal("valid instant query did not preserve the original positive window", err)
	}
}

func TestInvalidPromQLCannotConsumeRunBudgetAndCanBeCorrected(t *testing.T) {
	backend := &preflightBackend{}
	request := queryengine.Request{Language: queryengine.LanguagePromQL, Expression: "sum(private_fixture_value", Start: time.Unix(1, 0), End: time.Unix(61, 0), Step: time.Minute, Scope: queryengine.Scope{EnterpriseID: uuid.New(), ResourceIDs: []uuid.UUID{uuid.New()}, SourceKeys: []string{"exact-source"}}}
	for _, expression := range []string{"sum(private_fixture_value", "system_memory_usage[5m]", `"private_fixture_value"`, "sum(1)"} {
		request.Expression = expression
		_, err := executeValidatedModelQuery(t.Context(), backend, request)
		var safe toolruntime.Error
		if !errors.As(err, &safe) || safe.Kind != "TOOL_QUERY_INVALID" || safe.Details["budget_reserved"] != false || backend.calls != 0 || strings.Contains(safe.Message, "private_fixture_value") {
			t.Fatal("invalid PromQL entered the metered backend or exposed parser input", expression, err, backend.calls)
		}
	}
	request.Expression = `system_memory_utilization{state="used"}`
	if _, err := executeValidatedModelQuery(t.Context(), backend, request); err != nil || backend.calls != 1 {
		t.Fatal("valid corrected PromQL did not execute", err, backend.calls)
	}
	if backend.request.Scope.EnterpriseID != request.Scope.EnterpriseID || backend.request.Scope.ResourceIDs[0] != request.Scope.ResourceIDs[0] || backend.request.Scope.SourceKeys[0] != "exact-source" || backend.request.Step != time.Minute || !backend.request.Start.Equal(request.Start) || !backend.request.End.Equal(request.End) {
		t.Fatal("preflight changed scope/time/step")
	}
	request.Expression, request.Instant = "system_memory_usage[5m]", true
	if _, err := executeValidatedModelQuery(t.Context(), backend, request); err != nil || backend.calls != 2 {
		t.Fatal("an instant query's valid matrix was rejected", err, backend.calls)
	}
}

func (b *preflightBackend) ExecuteEngineQuery(_ context.Context, r queryengine.Request) (queryengine.Result, error) {
	b.calls++
	b.request = r
	return queryengine.Result{}, nil
}

func TestInvalidLogDocumentNeverEntersMeteredBackend(t *testing.T) {
	backend := &preflightBackend{}
	request := queryengine.Request{Language: queryengine.LanguageKQL, Expression: "Syslog | summarize count() by SeverityLevel", Start: time.Unix(1, 0), End: time.Unix(61, 0), Scope: queryengine.Scope{EnterpriseID: uuid.New(), ResourceIDs: []uuid.UUID{uuid.New()}}, Budget: queryengine.Budget{MaxRows: 100, MaxScanBytes: 1024, Timeout: time.Second}}
	_, err := executeValidatedModelQuery(t.Context(), backend, request)
	var safe toolruntime.Error
	if !errors.As(err, &safe) || safe.Kind != "TOOL_QUERY_INVALID" || safe.Details["budget_reserved"] != false || backend.calls != 0 {
		t.Fatal("invalid log DSL entered execution/budget backend", err, backend.calls)
	}
	request.Expression = "* | stats count() by severity_text"
	if _, err = executeValidatedModelQuery(t.Context(), backend, request); err != nil || backend.calls != 1 {
		t.Fatal("corrected supported grammar did not execute", err, backend.calls)
	}
	if backend.request.Scope.EnterpriseID != request.Scope.EnterpriseID || backend.request.Scope.ResourceIDs[0] != request.Scope.ResourceIDs[0] || !backend.request.Start.Equal(request.Start) || !backend.request.End.Equal(request.End) {
		t.Fatal("preflight changed authorized resources or time")
	}
}

func TestLogSubstringGuidanceUsesExecutableSyntaxAndRejectsRecordedMistakes(t *testing.T) {
	backend := &preflightBackend{}
	base := queryengine.Request{Language: queryengine.LanguageKQL, Start: time.Unix(1, 0), End: time.Unix(61, 0), Scope: queryengine.Scope{EnterpriseID: uuid.New(), ResourceIDs: []uuid.UUID{uuid.New()}}, Budget: queryengine.Budget{MaxRows: 100, MaxScanBytes: 1024, Timeout: time.Second}}
	for _, query := range []string{`* | parse body as note | where note contains "plan11" | limit 50`, `* | where body contains "plan11-operational-log" | limit 50`, `* | where body contains 'plan11-operational-log' | limit 50`} {
		base.Expression = query
		_, err := executeValidatedModelQuery(t.Context(), backend, base)
		var safe toolruntime.Error
		if !errors.As(err, &safe) || safe.Kind != "TOOL_QUERY_INVALID" || safe.Details["budget_reserved"] != false || backend.calls != 0 || !strings.Contains(safe.Message, `body : "text" | limit 100`) {
			t.Fatal("recorded invalid syntax lost exact safe correction", err)
		}
	}
	base.Expression = `body : "plan11-operational-log" | limit 100`
	if _, err := executeValidatedModelQuery(t.Context(), backend, base); err != nil || backend.calls != 1 {
		t.Fatal("advertised body substring predicate does not compile", err)
	}
	schema := dslQueryInputSchema("kql", "logs")
	query := schema["properties"].(map[string]any)["query"].(map[string]any)
	if query["description"] != modelLogSyntaxGuidance {
		t.Fatal("model input schema and error guidance diverged")
	}
}
