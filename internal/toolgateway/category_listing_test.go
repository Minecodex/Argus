package toolgateway

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestCategoryListingAcceptsOnlyCategoryAndAppliesCurrentScope(t *testing.T) {
	gateway, set := testGateway(t, 80)
	p := toolruntime.Principal{EnterpriseID: uuid.New(), UserID: uuid.New(), Permissions: []string{"host.read"}}
	call := toolruntime.Invocation{Principal: p, RunID: uuid.New(), Arguments: map[string]any{"category": "host"}}
	result, err := set.Invoke(t.Context(), "tool.search", call)
	if err != nil || len(result.Data["items"].([]map[string]any)) != 80 || !result.DataIsDiscovery || result.Data["complete"] != true {
		t.Fatal("category listing lost tools or completeness", err)
	}
	if _, exists := result.Data["next_cursor"]; exists {
		t.Fatal("category listing retained a paging cursor")
	}
	for _, arguments := range []map[string]any{
		{"category": "host", "query": ""}, {"category": "host", "query": "主机"},
		{"category": "host", "limit": float64(20)}, {"category": "host", "cursor": ""},
		{}, {"category": "unknown"},
	} {
		_, err := set.Invoke(t.Context(), "tool.search", toolruntime.Invocation{Principal: p, Arguments: arguments})
		var invalid toolruntime.Error
		if !errors.As(err, &invalid) || invalid.Kind != "TOOL_INPUT_INVALID" {
			t.Fatalf("obsolete or invalid input accepted: %v: %v", arguments, err)
		}
	}
	all := true
	gateway.Scope = func(context.Context, toolruntime.Invocation) (func(string) bool, error) {
		return func(id string) bool { return all || strings.HasSuffix(id, "0000") }, nil
	}
	if _, err := set.Invoke(t.Context(), "tool.search", call); err != nil {
		t.Fatal(err)
	}
	all = false
	current, err := set.Invoke(t.Context(), "tool.search", call)
	if err != nil || len(current.Data["items"].([]map[string]any)) != 1 {
		t.Fatal("same-Run scope change reused a broader listing", err)
	}
}

func TestCategoryNavigationReachesSystemPromptAndSearchDescription(t *testing.T) {
	_, set := testGateway(t, 1)
	search, _ := set.Lookup("tool.search")
	for _, category := range toolruntime.NativeCategoryNames() {
		for _, text := range []string{toolruntime.SystemInstructions, search.Model.Description} {
			if !strings.Contains(text, "- "+category+": ") {
				t.Fatalf("missing category guidance: %s", category)
			}
		}
	}
	properties := search.Model.Schema["properties"].(map[string]any)
	if len(properties) != 1 || properties["category"] == nil {
		t.Fatal("search schema does not accept exactly one category parameter")
	}
}
