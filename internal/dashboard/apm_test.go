package dashboard

import (
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

func TestAPMSharedAttributeVariableSupportsMultipleAndAll(t *testing.T) {
	spec := EmptySpec()
	source := SourceBinding{SourceType: "otlp", CapabilityVersion: "v1"}
	spec.Variables = []Variable{{ID: "env", Name: "env", Multiple: true, IncludeAll: true, Default: Selection{All: true}, Query: CandidateQuery{Signal: "traces", SourceBinding: source, Field: "resource_attributes.deployment.environment.name"}}}
	panel := validPanel()
	panel.Type, panel.Signal, panel.SourceBinding = "apm_services", "traces", source
	panel.Targets = []Target{{ID: "a", Language: queryengine.LanguageTrace, SourceDefinition: Definition{Builder: &Builder{Operation: "apm_services", Filters: []Filter{{Field: "resource_attributes.deployment.environment.name", Operator: "=", Variable: "env"}}}}}}
	spec.Panels = []Panel{panel}
	if report := Validate(spec); !report.Valid {
		t.Fatal(report.Issues)
	}
	bound, err := bindTarget(spec, panel, panel.Targets[0], map[string]Selection{"env": {Values: []string{"prod", "stage"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := compileConcreteTarget(bound, false)
	if err != nil || !strings.Contains(compiled.Query.Expression, `values:["prod","stage"]`) {
		t.Fatalf("shared environment filter lost: %v %+v", err, compiled)
	}
	bound, err = bindTarget(spec, panel, panel.Targets[0], map[string]Selection{"env": {All: true}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err = compileConcreteTarget(bound, false)
	if err != nil || strings.Contains(compiled.Query.Expression, "filters:") {
		t.Fatal("All failed to remove only the attribute condition")
	}
}

func TestAPMBuilderProducesTypedSampleQueries(t *testing.T) {
	for _, kind := range []string{"apm_services", "apm_instances", "apm_endpoints", "apm_red", "apm_topology"} {
		t.Run(kind, func(t *testing.T) {
			panel := validPanel()
			panel.Type, panel.Signal, panel.SourceBinding = kind, "traces", SourceBinding{SourceType: "otlp", CapabilityVersion: "v1"}
			target := Target{ID: "a", Language: queryengine.LanguageTrace, SourceDefinition: Definition{Builder: &Builder{Operation: kind, Filters: []Filter{{Field: "serviceName", Operator: "=", Value: `api"quoted`}}}}}
			if kind == "apm_red" {
				target.SourceDefinition.Builder.BucketSeconds = 30
			}
			panel.Targets = []Target{target}
			spec := EmptySpec()
			spec.Panels = []Panel{panel}
			if report := Validate(spec); !report.Valid {
				t.Fatal(report.Issues)
			}
			if err := runtimeSupported(spec); err != nil {
				t.Fatal(err)
			}
			compiled, err := CompileTarget(target)
			if err != nil || compiled.ResultType != kind || !compatibleResult(kind, "traces", compiled.ResultType) {
				t.Fatalf("APM shape lost: %+v %v", compiled, err)
			}
			if compatibleResult("trace_list", "traces", compiled.ResultType) {
				t.Fatal("APM query accepted as a trace list")
			}
		})
	}
}

func TestAPMProjectionAndInputGates(t *testing.T) {
	for _, expression := range []string{
		`query {queryAPMServices {rows{serviceName}}}`,
		`query {queryAPMRED(bucketSeconds:0){basis status coverage{observedSpanCount} rows{serviceName}}}`,
	} {
		if _, err := CompileTarget(Target{ID: "a", Language: queryengine.LanguageTrace, SourceDefinition: Definition{DSL: &DSL{Expression: expression}}}); err == nil {
			t.Fatal("missing projection or invalid bucket accepted")
		}
	}
	query, err := compileAPMBuilder(Builder{Operation: "apm_services"})
	if err != nil {
		t.Fatal(err)
	}
	conditional := DSL{Expression: `query($hide:Boolean!){...Root @skip(if:$hide)} fragment Root on Query{queryAPMServices{` + apmProjection + `}}`, Variables: map[string]any{"hide": false}}
	if _, err := CompileTarget(Target{ID: "a", Language: queryengine.LanguageTrace, SourceDefinition: Definition{DSL: &conditional}}); err == nil {
		t.Fatal("conditional fragment bypassed APM projection checks")
	}
	query.Expression = `query($origin:String){queryAPMServices(sourceId:$origin){` + apmProjection + `}}`
	target := Target{ID: "a", Language: queryengine.LanguageTrace, SourceDefinition: Definition{DSL: &query}, ParameterBindings: []ParameterBinding{{Parameter: "origin", LocalParameter: "origin"}}}
	if compiled, err := CompileTarget(target); err != nil || !compiled.Deferred {
		t.Fatalf("UUID parameter probe rejected a valid definition: %v", err)
	}
	if !emptyTypedResult("apm_services", map[string]any{"result": map[string]any{"basis": "received_entry_spans", "status": "no_data", "rows": []any{}}}) {
		t.Fatal("APM metadata made empty data look populated")
	}
}
