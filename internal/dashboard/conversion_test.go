package dashboard

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

func TestBuilderStatementRoundTrip(t *testing.T) {
	for _, test := range []struct {
		name     string
		language queryengine.Language
		builder  Builder
	}{
		{"value", queryengine.LanguagePromQL, Builder{Operation: "value", Metric: "cpu", Filters: []Filter{{Field: "host", Operator: "=", Value: "quote\" and | pipe"}}}},
		{"sum", queryengine.LanguagePromQL, Builder{Operation: "sum", Metric: "cpu", GroupBy: []string{"host"}}},
		{"rate", queryengine.LanguagePromQL, Builder{Operation: "rate", Metric: "requests_total", MetricType: "counter", WindowSeconds: 300}},
		{"error rate", queryengine.LanguagePromQL, Builder{Operation: "error_rate", Metric: "requests_total", MetricType: "counter", WindowSeconds: 300, GroupBy: []string{"service"}, ErrorFilters: []Filter{{Field: "status", Operator: "=~", Value: "5.."}}}},
		{"topk", queryengine.LanguagePromQL, Builder{Operation: "topk", Metric: "cpu", TopN: 5}},
		{"p95", queryengine.LanguagePromQL, Builder{Operation: "p95", Metric: "latency", MetricType: "histogram", WindowSeconds: 60, GroupBy: []string{"service"}}},
		{"logs", queryengine.LanguageKQL, Builder{Operation: "records", Limit: 123, Filters: []Filter{{Field: "body", Operator: ":", Value: "a|b \" c"}, {Field: "severity_number", Operator: ">=", Value: "9"}}}},
		{"log set", queryengine.LanguageKQL, Builder{Operation: "records", Filters: []Filter{{Field: "service_name", Operator: "=", Values: []string{"a", "b"}}}}},
		{"log count", queryengine.LanguageKQL, Builder{Operation: "count"}},
		{"log group", queryengine.LanguageKQL, Builder{Operation: "count_by", GroupBy: []string{"service_name"}}},
		{"log buckets", queryengine.LanguageKQL, Builder{Operation: "count_over_time", BucketSeconds: 60, GroupBy: []string{"service_name"}}},
		{"log context", queryengine.LanguageKQL, Builder{Operation: "log_context", ContextBefore: 20, ContextAfter: 30, Filters: []Filter{{Field: "event_id", Operator: "=", Value: "event-1"}}}},
		{"trace list", queryengine.LanguageTrace, Builder{Operation: "list", Limit: 7, Filters: []Filter{{Field: "durationMin", Operator: "=", Value: "5.5"}}}},
		{"trace detail", queryengine.LanguageTrace, Builder{Operation: "detail", TraceID: "trace-1"}},
		{"trace graph", queryengine.LanguageTrace, Builder{Operation: "trace_graph", Filters: []Filter{{Field: "traceId", Operator: "=", Value: "trace-1"}, {Field: "sourceId", Operator: "=", Value: "11111111-1111-4111-8111-111111111111"}, {Field: "resourceId", Operator: "=", Value: "22222222-2222-4222-8222-222222222222"}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			panel := conversionPanel(test.language, test.builder)
			first := ConvertPanel(ConvertPanelInput{Panel: panel, Mode: "dsl"})
			if !first.Converted {
				t.Fatal(first.Issues)
			}
			second := ConvertPanel(ConvertPanelInput{Panel: first.Panel, Mode: "builder"})
			if !second.Converted {
				t.Fatalf("%v\nquery: %+v", second.Issues, *first.Panel.Targets[0].SourceDefinition.DSL)
			}
			before, _ := compileBuilder(test.language, test.builder)
			after, _ := compileBuilder(test.language, *second.Panel.Targets[0].SourceDefinition.Builder)
			equal, err := equivalentDSL(test.language, before, after)
			if err != nil || !equal {
				t.Fatalf("round trip changed semantics: %+v / %+v / %v", before, after, err)
			}
		})
	}
	for _, operation := range []string{"apm_services", "apm_instances", "apm_endpoints", "apm_red", "apm_topology"} {
		t.Run(operation, func(t *testing.T) {
			b := Builder{Operation: operation, Limit: 10, Filters: []Filter{{Field: "serviceName", Operator: "=", Value: "checkout"}, {Field: "resource_attributes.env", Operator: "!=", Values: []string{"test", "staging"}}}}
			if operation == "apm_red" {
				b.BucketSeconds = 60
			}
			p := conversionPanel(queryengine.LanguageTrace, b)
			forward := ConvertPanel(ConvertPanelInput{Panel: p, Mode: "dsl"})
			if !forward.Converted {
				t.Fatal(forward.Issues)
			}
			back := ConvertPanel(ConvertPanelInput{Panel: forward.Panel, Mode: "builder"})
			if !back.Converted {
				t.Fatal(back.Issues)
			}
		})
	}
}

func TestConversionPreservesRuntimeParametersAndAll(t *testing.T) {
	for _, language := range []queryengine.Language{queryengine.LanguagePromQL, queryengine.LanguageKQL, queryengine.LanguageTrace} {
		t.Run(string(language), func(t *testing.T) {
			field, op := "service", "value"
			if language == queryengine.LanguageKQL {
				field, op = "service_name", "records"
			}
			if language == queryengine.LanguageTrace {
				field, op = "resource_attributes.service.name", "apm_services"
			}
			panel := conversionPanel(language, Builder{Operation: op, Metric: "", Filters: []Filter{{Field: field, Operator: "=", Variable: "service"}}})
			if language == queryengine.LanguagePromQL {
				panel.Targets[0].SourceDefinition.Builder.Metric = "cpu"
			}
			panel.Targets[0].ParameterBindings = []ParameterBinding{{Parameter: "app", Variable: "service", IdentityMapping: true}}
			converted := ConvertPanel(ConvertPanelInput{Panel: panel, Mode: "dsl"})
			if !converted.Converted {
				t.Fatal(converted.Issues)
			}
			if !reflect.DeepEqual(converted.Panel.Targets[0].ParameterBindings, panel.Targets[0].ParameterBindings) {
				t.Fatal("mapping changed")
			}
			back := ConvertPanel(ConvertPanelInput{Panel: converted.Panel, Mode: "builder"})
			if !back.Converted {
				t.Fatal(back.Issues)
			}
			if back.Panel.Targets[0].SourceDefinition.Builder.Filters[0].Variable != "service" {
				t.Fatal("parameter became a fixed literal")
			}
			for _, selection := range []Selection{{All: true}, {Values: []string{"a.b", "x|y"}}} {
				query, err := bindDSL(language, *converted.Panel.Targets[0].SourceDefinition.DSL, map[string]Selection{"app": selection})
				if err != nil {
					t.Fatal(err)
				}
				target := converted.Panel.Targets[0]
				target.ParameterBindings = nil
				target.SourceDefinition = Definition{DSL: &query}
				if _, err := compileConcreteTarget(target, false); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestConversionRejectsLossAndIsAtomic(t *testing.T) {
	for _, test := range []struct {
		language queryengine.Language
		query    DSL
	}{
		{queryengine.LanguagePromQL, DSL{Expression: `cpu offset 1h`}},
		{queryengine.LanguagePromQL, DSL{Expression: `sum without(host)(cpu)`}},
		{queryengine.LanguagePromQL, DSL{Expression: `rate(requests_total[1m]) + cpu`}},
		{queryengine.LanguageKQL, DSL{Expression: `service_name="a" OR body="b"`}},
		{queryengine.LanguageKQL, DSL{Expression: `json.count=12 | stats count()`}},
		{queryengine.LanguageKQL, DSL{Expression: `* | sort timestamp asc | limit 10`}},
		{queryengine.LanguageTrace, DSL{Expression: `query { queryTraces(pageSize:5) {total traces{traceId}} }`}},
	} {
		t.Run(test.query.Expression, func(t *testing.T) {
			panel := conversionPanel(test.language, Builder{})
			panel.AuthoringMode = "dsl"
			panel.Targets[0].SourceDefinition = Definition{DSL: &test.query}
			encoded, _ := json.Marshal(panel)
			result := ConvertPanel(ConvertPanelInput{Panel: panel, Mode: "builder"})
			if result.Converted || len(result.Issues) == 0 {
				t.Fatal("lossy conversion was accepted")
			}
			actual, _ := json.Marshal(result.Panel)
			if string(encoded) != string(actual) {
				t.Fatal("failed conversion changed input")
			}
		})
	}
	good := conversionPanel(queryengine.LanguagePromQL, Builder{Operation: "value", Metric: "cpu"})
	converted := ConvertPanel(ConvertPanelInput{Panel: good, Mode: "dsl"})
	panel := converted.Panel
	extra := panel.Targets[0]
	extra.ID = "second"
	extra.SourceDefinition = Definition{DSL: &DSL{Expression: "cpu + memory"}}
	panel.Targets = append(panel.Targets, extra)
	before, _ := json.Marshal(panel)
	result := ConvertPanel(ConvertPanelInput{Panel: panel, Mode: "builder"})
	after, _ := json.Marshal(panel)
	if result.Converted || string(before) != string(after) || result.Panel.Targets[0].SourceDefinition.DSL == nil {
		t.Fatal("partial conversion escaped")
	}
}

func TestConversionKeepsSharedInputAcrossDifferentGraphQLTypes(t *testing.T) {
	panel := conversionPanel(queryengine.LanguageTrace, Builder{Operation: "list", Filters: []Filter{
		{Field: "serviceName", Operator: "=", Variable: "a"},
		{Field: "durationMin", Operator: "=", Variable: "b"},
		{Field: "resource_attributes.service.name", Operator: "=", Variable: "a"},
		{Field: "durationMax", Operator: "=", Variable: "a"},
	}})
	panel.Targets[0].ParameterBindings = []ParameterBinding{{Parameter: "p1", Variable: "a"}, {Parameter: "p1_2", Variable: "b"}}
	result := ConvertPanel(ConvertPanelInput{Panel: panel, Mode: "dsl"})
	if !result.Converted {
		t.Fatal(result.Issues)
	}
	for _, binding := range result.Panel.Targets[0].ParameterBindings {
		if binding.Parameter == "p1_2" && binding.Variable != "b" {
			t.Fatal("typed alias captured another input")
		}
	}
	back := ConvertPanel(ConvertPanelInput{Panel: result.Panel, Mode: "builder"})
	if !back.Converted {
		t.Fatal(back.Issues)
	}
	for _, original := range panel.Targets[0].SourceDefinition.Builder.Filters {
		found := false
		for _, actual := range back.Panel.Targets[0].SourceDefinition.Builder.Filters {
			if reflect.DeepEqual(original, actual) {
				found = true
			}
		}
		if !found {
			t.Fatal("input identity changed", original)
		}
	}
	if len(back.Panel.Targets[0].ParameterBindings) != 2 {
		t.Fatal("aliases were retained as duplicate builder bindings")
	}
}
func conversionPanel(language queryengine.Language, builder Builder) Panel {
	signal := "metrics"
	if language == queryengine.LanguageKQL {
		signal = "logs"
	}
	if language == queryengine.LanguageTrace {
		signal = "traces"
	}
	return Panel{ID: "p", Signal: signal, AuthoringMode: "builder", Targets: []Target{{ID: "q", Language: language, QueryMode: "instant", SourceDefinition: Definition{Builder: &builder}}}}
}
