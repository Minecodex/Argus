package argusdev

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"slices"
)

func TestPlan8ModelOracleUsesPublicDiscoveryCategoriesAndExplicitScopes(t *testing.T) {
	apps := []uuid.UUID{uuid.New(), uuid.New()}
	shared := uuid.New()
	for _, category := range []string{"metric", "log", "trace", "k8s"} {
		if !slices.Contains(toolruntime.NativeCategoryNames(), category) {
			t.Fatal("oracle category missing from shared guide", category)
		}
	}
	calls := []map[string]any{}
	add := func(category string, id uuid.UUID) {
		arguments := map[string]any{"resource_ids": []string{id.String()}, "query": "actual-query"}
		if category == "trace" {
			delete(arguments, "query")
			arguments["document"] = "query { queryAPMServices { rows { sampleCount } } }"
		}
		calls = append(calls, map[string]any{"tool": "tool.search", "status": "succeeded", "input": map[string]any{"category": category}}, map[string]any{"tool": "tool.describe", "status": "succeeded", "input": map[string]any{"category": category, "name": "query"}}, map[string]any{"tool": "tool.invoke", "status": "succeeded", "input": map[string]any{"category": category, "name": "query", "arguments": arguments}})
	}
	add("trace", apps[0])
	add("trace", apps[1])
	add("log", apps[0])
	add("metric", shared)
	encode := func() string { raw, _ := json.Marshal(calls); return string(raw) }
	if err := plan8VerifyApplicationModelQueries(encode(), apps, shared); err != nil {
		t.Fatal(err)
	}
	for _, call := range calls {
		input := call["input"].(map[string]any)
		if call["tool"] == "tool.invoke" && input["category"] == "trace" {
			arguments := input["arguments"].(map[string]any)
			delete(arguments, "document")
			arguments["application_view"] = "services"
		}
	}
	if err := plan8VerifyApplicationModelQueries(encode(), apps, shared); err != nil {
		t.Fatal("typed application view was not counted with its explicit scope", err)
	}
	if err := plan8VerifyModelMetricTarget(encode(), shared); err != nil {
		t.Fatal(err)
	}
	if plan8VerifyModelMetricTarget(encode(), uuid.New()) == nil {
		t.Fatal("another target satisfied actual collection read")
	}
	call := calls[len(calls)-1]
	input := call["input"].(map[string]any)
	arguments := input["arguments"].(map[string]any)
	arguments["resource_ids"] = []string{}
	arguments["query"] = shared.String()
	if plan8VerifyApplicationModelQueries(encode(), apps, shared) == nil {
		t.Fatal("ID in query text substituted for authorized target selection")
	}
	arguments["resource_ids"] = []string{shared.String()}
	call["status"] = "failed"
	if plan8VerifyApplicationModelQueries(encode(), apps, shared) == nil {
		t.Fatal("failed invocation counted as actual query")
	}
}
