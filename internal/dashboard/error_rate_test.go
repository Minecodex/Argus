package dashboard

import (
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

func TestErrorRateSubsetAndLosslessConversion(t *testing.T) {
	b := Builder{Operation: "error_rate", Metric: "requests_total", MetricType: "counter", WindowSeconds: 300,
		GroupBy: []string{"service"}, Filters: []Filter{{Field: "env", Operator: "=", Variable: "env"}},
		ErrorFilters: []Filter{{Field: "status", Operator: "=~", Value: "5.."}}}
	panel := conversionPanel(queryengine.LanguagePromQL, b)
	panel.Targets[0].ParameterBindings = []ParameterBinding{{Parameter: "environment", Variable: "env", IdentityMapping: true}}
	forward := ConvertPanel(ConvertPanelInput{Panel: panel, Mode: "dsl"})
	if !forward.Converted {
		t.Fatal(forward.Issues)
	}
	back := ConvertPanel(ConvertPanelInput{Panel: forward.Panel, Mode: "builder"})
	if !back.Converted {
		t.Fatal(back.Issues)
	}
	if back.Panel.Targets[0].SourceDefinition.Builder.Filters[0].Variable != "env" {
		t.Fatal("common dynamic scope lost")
	}
	for _, selection := range []Selection{{All: true}, {Values: []string{"prod", "staging"}}} {
		query, err := bindDSL(queryengine.LanguagePromQL, *forward.Panel.Targets[0].SourceDefinition.DSL, map[string]Selection{"environment": selection})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(query.Expression, "$environment") {
			t.Fatal("unbound scope")
		}
	}
	for _, mutation := range []func(string) string{
		func(q string) string { return strings.Replace(q, "> 0", ">= 0", 1) },
		func(q string) string { return strings.Replace(q, "* 0", "* 1", 1) },
		func(q string) string { return strings.Replace(q, "requests_total", "different_total", 1) },
		func(q string) string { return strings.Replace(q, " / ", " / ignoring(service) ", 1) },
	} {
		changed := forward.Panel
		changed.Targets = append([]Target{}, forward.Panel.Targets...)
		changed.Targets[0].SourceDefinition = Definition{DSL: &DSL{Expression: mutation(forward.Panel.Targets[0].SourceDefinition.DSL.Expression)}}
		if result := ConvertPanel(ConvertPanelInput{Panel: changed, Mode: "builder"}); result.Converted {
			t.Fatal("lossy ratio conversion accepted")
		}
	}
}

func TestErrorRateRejectsInvalidClassifications(t *testing.T) {
	base := Builder{Operation: "error_rate", Metric: "requests_total", MetricType: "counter", WindowSeconds: 300, ErrorFilters: []Filter{{Field: "status", Operator: "=", Value: "500"}}}
	for _, change := range []func(*Builder){
		func(b *Builder) { b.MetricType = "gauge" }, func(b *Builder) { b.ErrorFilters = nil },
		func(b *Builder) { b.ErrorFilters = []Filter{{Field: "status", Operator: "=", Variable: "v"}} },
		func(b *Builder) { b.ErrorFilters = []Filter{{Field: "__name__", Operator: "=", Value: "other_total"}} },
		func(b *Builder) { b.ErrorFilters = []Filter{{Field: "status", Operator: ">", Value: "500"}} },
		func(b *Builder) { b.Operation = "sum" },
	} {
		b := base
		change(&b)
		if _, err := compileBuilder(queryengine.LanguagePromQL, b); err == nil {
			t.Fatalf("invalid builder accepted: %+v", b)
		}
	}
}

func TestConversionDoesNotCaptureLiteralAsDynamicScope(t *testing.T) {
	for _, language := range []queryengine.Language{queryengine.LanguagePromQL, queryengine.LanguageKQL} {
		b := Builder{Operation: "value", Metric: "requests_total", Filters: []Filter{{Field: "env", Operator: "=", Variable: "env"}, {Field: "code", Operator: "=", Value: "$environment"}}}
		if language == queryengine.LanguageKQL {
			b.Operation = "records"
			b.Filters[0].Field = "service_name"
			b.Filters[1].Field = "body"
		}
		panel := conversionPanel(language, b)
		panel.Targets[0].ParameterBindings = []ParameterBinding{{Parameter: "environment", Variable: "env", IdentityMapping: true}}
		if _, err := CompileTarget(panel.Targets[0]); err != nil {
			t.Fatal(err)
		}
		if result := ConvertPanel(ConvertPanelInput{Panel: panel, Mode: "dsl"}); result.Converted {
			t.Fatal("literal was captured by dynamic parameter")
		}
	}
	b := Builder{Operation: "error_rate", Metric: "requests_total", MetricType: "counter", WindowSeconds: 300, ErrorFilters: []Filter{{Field: "status", Operator: "=", Value: "$code"}}}
	if result := ConvertPanel(ConvertPanelInput{Panel: conversionPanel(queryengine.LanguagePromQL, b), Mode: "dsl"}); result.Converted {
		t.Fatal("error classification became dynamic")
	}
}
