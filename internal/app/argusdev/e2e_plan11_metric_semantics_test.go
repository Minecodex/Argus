package argusdev

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/telemetry/metricmeta"
)

func TestMetricSemanticsPhaseHasSixIndependentTasksAndOnlyActualSDKPrerequisites(t *testing.T) {
	if len(plan8SelectedModelNames("plan11-metrics")) != 6 || plan8SDKModelKeySelected("plan11-metrics", "otlp_http") {
		t.Fatal("metric semantics silently dispatches undeclared paid Key tasks")
	}
	for _, stage := range []string{"formal-onboarding-and-managed-collector", "actual-query-and-dashboard", "normal-production-backend", "sdk-application-protocols", "plan11-metric-semantics"} {
		if !plan8StageSelected("plan11-metrics", stage) {
			t.Fatal("metric semantics lost its real data prerequisites", stage)
		}
	}
	for _, stage := range []string{"plan11-metadata-semantics", "plan11-resource-identity", "plan11-customer-mcp", "plan11-chat-interactions", "real-model-application-revocation", "plan11-profile-comparison"} {
		if plan8StageSelected("plan11-metrics", stage) {
			t.Fatal("metric semantics gained an unrelated scenario", stage)
		}
	}
	for _, phase := range []string{"all", "plan11", "plan11-metadata", "plan11-compare"} {
		if plan8StageSelected(phase, "plan11-metric-semantics") {
			t.Fatal("existing phase silently gained metric semantics tasks", phase)
		}
	}
	for _, locale := range []string{"zh", "en"} {
		prompt := plan11MetricSemanticsPrompt(locale, "actual-application-id", time.Unix(100, 0), time.Unix(200, 0))
		if !strings.Contains(prompt, "actual-application-id") || strings.Contains(prompt, "0.2") || strings.Contains(prompt, "counter_value=5") {
			t.Fatal("metric semantics prompt lost identity or leaked fixture oracle")
		}
	}
}

func TestMetricSemanticsTraceRejectsMutationRemoteExecutionAndUnrelatedSignals(t *testing.T) {
	if err := plan11MetricSemanticsTraceAllowed(`[{"Tool":"tool.invoke","Input":{"Category":"observe","Name":"catalog.query"}},{"Tool":"tool.invoke","Input":{"Category":"metric","Name":"query"}},{"Tool":"read"},{"Tool":"bash"}]`); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`[{"Tool":"connect"}]`, `[{"Tool":"tool.invoke","Input":{"Category":"log","Name":"query"}}]`, `[{"Tool":"tool.invoke","Input":{"Category":"application","Name":"key.reveal"}}]`, `[{"Tool":"tool.invoke","Input":{"Category":"application","Name":"change.preview"}}]`} {
		if !errors.Is(plan11MetricSemanticsTraceAllowed(raw), errPlan11NavigationBoundary) {
			t.Fatal("read-only metric task crossed its domain boundary", raw)
		}
	}
}

func TestMetricSemanticExpressionsCannotSubstituteConstantsOrWrongKinds(t *testing.T) {
	for expression, expected := range map[string]string{
		"argus_plan11_sdk_completed": "counter", "sum(argus_plan11_sdk_queue)": "gauge",
		`argus_plan11_sdk_latency_count{service_name="api"}`:                                          "histogram_count",
		"sum(argus_plan11_sdk_latency_sum)/sum(argus_plan11_sdk_latency_count)":                       "histogram_mean",
		"sum(rate(argus_plan11_sdk_completed[5m]))":                                                   "counter_rate",
		"sum(increase(argus_plan11_sdk_completed[5m]))/300":                                           "counter_rate",
		`({__name__="argus_plan11_sdk_latency_sum"}) / ({__name__="argus_plan11_sdk_latency_count"})`: "histogram_mean",
		`rate({__name__="argus_plan11_sdk_completed"}[5m])`:                                           "counter_rate",
	} {
		if got := plan11MetricExpressionKind(expression); got != expected {
			t.Fatal("valid semantic expression rejected", expression, got)
		}
	}
	for _, expression := range []string{"vector(5)", "argus_plan11_sdk_queue*0+7", "rate(argus_plan11_sdk_queue[5m])", "argus_plan11_sdk_completed offset 1m", "argus_plan11_sdk_latency_count/argus_plan11_sdk_latency_sum", "increase(argus_plan11_sdk_completed[5m])/60"} {
		if plan11MetricExpressionKind(expression) != "" {
			t.Fatal("wrong metric semantics or manufactured value accepted", expression)
		}
	}
}

func TestMetricSemanticOriginalRejectsSourceWindowTypeAndUnitSubstitution(t *testing.T) {
	application, source := uuid.NewString(), uuid.NewString()+":3"
	from, to := time.Unix(100, 0).UTC(), time.Unix(200, 0).UTC()
	makeResult := func() map[string]any {
		return map[string]any{"status": "success", "query_context": map[string]any{"resources": []any{map[string]any{"type": "application", "id": application}}, "source_keys": []string{source}, "from": from, "to": to},
			"argus_meta": map[string]any{"partial": false, "input_metrics_origin": metricmeta.Origin, "input_metrics_complete": true, "input_metrics": []metricmeta.Descriptor{{Name: "argus_plan11_sdk_completed", Type: "counter", Unit: "{request}", SourceKey: source}}},
			"data":       map[string]any{"resultType": "vector", "result": []any{map[string]any{"metric": map[string]string{"__name__": "argus_plan11_sdk_completed"}, "value": []any{200, "5"}}}}}
	}
	for _, change := range []string{"none", "source", "window", "type", "unit", "partial", "nonfinite"} {
		t.Run(change, func(t *testing.T) {
			out := makeResult()
			context := out["query_context"].(map[string]any)
			meta := out["argus_meta"].(map[string]any)
			switch change {
			case "source":
				context["source_keys"] = []string{uuid.NewString() + ":3"}
			case "window":
				context["to"] = to.Add(time.Second)
			case "type":
				meta["input_metrics"].([]metricmeta.Descriptor)[0].Type = "gauge"
			case "unit":
				meta["input_metrics"].([]metricmeta.Descriptor)[0].Unit = "s"
			case "partial":
				meta["partial"] = true
			case "nonfinite":
				out["data"].(map[string]any)["result"].([]any)[0].(map[string]any)["value"] = []any{200, "NaN"}
			}
			raw, _ := json.Marshal(out)
			values, err := plan11MetricSemanticValues(raw, "argus_plan11_sdk_completed", application, source, from, to)
			if (err == nil) != (change == "none") || change == "none" && values["counter"] != 5 {
				t.Fatal("source-bound original acceptance changed", change, values, err)
			}
		})
	}
}

func TestPostgresMetricSemanticsQueriesUseCurrentSchema(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable migrated PostgreSQL")
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var count int
	if err := store.Pool.QueryRow(t.Context(), plan11MetricSemanticsCatalogSQL(uuid.NewString(), uuid.NewString()+":3")).Scan(&count); err != nil || count != 0 {
		t.Fatal("catalog semantics query failed or matched another Run", count, err)
	}
	var raw string
	if err := store.Pool.QueryRow(t.Context(), plan11InlineModelExposureSQL(uuid.NewString(), uuid.NewString())).Scan(&count); err != nil || count != 0 {
		t.Fatal("inline model exposure SQL failed or matched another Run", count, err)
	}
	if err := store.Pool.QueryRow(t.Context(), plan11MetricSemanticsQueriesSQL(uuid.NewString())).Scan(&raw); err != nil || raw != "[]" {
		t.Fatal("sealed metric semantics query failed or matched another Run", raw, err)
	}
}
