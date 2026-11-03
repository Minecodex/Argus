package telemetry

import (
	"github.com/kakj-go/Argus/internal/toolruntime"
	"testing"
)

func TestNativeQueryScopeContract(t *testing.T) {
	for _, language := range []string{"promql", "kql", "skywalking_graphql"} {
		schema, err := toolruntime.CompileInputSchema(dslQueryInputSchema(language, "metrics"))
		if err != nil {
			t.Fatal(err)
		}
		valid := func() map[string]any {
			v := map[string]any{"from": "2026-10-29T00:00:00Z", "to": "2026-10-29T01:00:00Z", "scope": map[string]any{"mode": "explicit", "resources": []any{map[string]any{"type": "monitoring_target", "id": "10000000-0000-4000-8000-000000000001"}}, "source_type": "mysql", "source_keys": []any{"source"}}}
			if language == "skywalking_graphql" {
				v["application_view"] = "services"
			} else {
				v["query"] = "metric"
			}
			return v
		}
		if err := schema.Validate(valid()); err != nil {
			t.Fatalf("%s valid scope: %v", language, err)
		}
		for _, mutate := range []func(map[string]any){
			func(v map[string]any) { v["resource_ids"] = []any{"10000000-0000-4000-8000-000000000001"} },
			func(v map[string]any) { v["scope"].(map[string]any)["mode"] = "all_authorized" },
			func(v map[string]any) { v["scope"].(map[string]any)["resources"] = []any{} },
			func(v map[string]any) { v["scope"].(map[string]any)["source_keys"] = []any{} },
			func(v map[string]any) { delete(v, "scope") },
		} {
			input := valid()
			mutate(input)
			if schema.Validate(input) == nil {
				t.Fatalf("%s accepted invalid scope: %v", language, input)
			}
		}
	}
}
