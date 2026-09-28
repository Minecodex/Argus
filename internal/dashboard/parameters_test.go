package dashboard

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/kql"
	"github.com/prometheus/prometheus/promql/parser"
)

func TestParameterBindingUsesLanguageStructure(t *testing.T) {
	injected := `a"} OR service_name=* | limit 999`
	expression, err := bindPromQL(`label_replace(up{service="$service", env="prod"}, "copy", "$name", "source", "(?P<name>.*)")`, map[string]Selection{"service": {Values: []string{injected, "api.v1"}}})
	if err != nil {
		t.Fatal(err)
	}
	expr, err := parser.NewParser(parser.Options{}).ParseExpr(expression)
	if err != nil {
		t.Fatal(err)
	}
	selectors := parser.ExtractSelectors(expr)
	for _, m := range selectors[0] {
		if m.Name == "service" && (!m.Matches(injected) || !m.Matches("api.v1") || m.Matches("apiXv1")) {
			t.Fatalf("parameter changed literal matching: %s", m)
		}
	}
	if !strings.Contains(expression, `"$name"`) {
		t.Fatal("native replacement capture modified")
	}
	all, err := bindPromQL(`up{env="prod",service="$service"}`, map[string]Selection{"service": {All: true}})
	if err != nil || strings.Contains(all, "service=") || !strings.Contains(all, `env="prod"`) {
		t.Fatalf("All removed unrelated constraint: %s %v", all, err)
	}
	query, err := bindKQL(DSL{Expression: `service_name = "$service" AND severity_text = "ERROR"`, Pipeline: `limit 5`}, map[string]Selection{"service": {Values: []string{injected}}})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := kql.CompileQuery("logs", kql.Request{Expression: query.Expression, Pipeline: query.Pipeline, Start: time.Unix(1, 0), End: time.Unix(2, 0), Scope: kql.Scope{ResourceIDs: []uuid.UUID{uuid.New()}}, Budget: kql.Budget{MaxRows: 100}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plan.SQL, injected) || !strings.Contains(query.Pipeline, "limit 5") {
		t.Fatal("parameter escaped into SQL or changed limit")
	}
	if _, err := bindKQL(DSL{Expression: `service_name = "$service" OR severity_text = "ERROR"`}, map[string]Selection{"service": {All: true}}); err == nil {
		t.Fatal("ambiguous OR All accepted")
	}
	if _, err := bindKQL(DSL{Expression: `severity_number >= "$level"`}, map[string]Selection{"level": {Values: []string{"invalid"}}}); err == nil {
		t.Fatal("non-numeric comparison accepted")
	}
	bound, err := bindDSL(queryengine.LanguageTrace, DSL{Expression: `query($service:String,$duration:Float){queryTraces(serviceName:$service,durationMin:$duration){total}}`}, map[string]Selection{"service": {Values: []string{injected}}, "duration": {Values: []string{"12.5"}}})
	if err != nil || bound.Variables["service"] != injected || bound.Variables["duration"] != 12.5 {
		t.Fatalf("GraphQL inputs lost types: %+v %v", bound, err)
	}
	if _, err := bindDSL(queryengine.LanguageTrace, bound, map[string]Selection{"service": {Values: []string{"a", "b"}}}); err == nil {
		t.Fatal("multi value coerced to GraphQL scalar")
	}
}

func TestParametersRequireExplicitCrossSourceMapping(t *testing.T) {
	spec := EmptySpec()
	panel := validPanel()
	spec.Variables = []Variable{{ID: "service", Name: "service", IncludeAll: true, Default: Selection{All: true}, Query: CandidateQuery{Signal: "metrics", Field: "service", SourceBinding: SourceBinding{SourceType: "prometheus", CapabilityVersion: "v1"}}}}
	target := panel.Targets[0]
	target.SourceDefinition.Builder.Filters = []Filter{{Field: "service", Operator: "=", Variable: "service"}}
	values := map[string]Selection{"service": {Values: []string{"frontend"}}}
	if _, err := bindTarget(spec, panel, target, values, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("implicit cross-source merge: %v", err)
	}
	target.ParameterBindings = []ParameterBinding{{Parameter: "service", Variable: "service", ValueMap: map[string]string{"frontend": "gateway"}}}
	bound, err := bindTarget(spec, panel, target, values, nil)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := compileConcreteTarget(bound, false)
	if err != nil || !strings.Contains(compiled.Query.Expression, `service="gateway"`) {
		t.Fatalf("mapping not applied: %+v %v", compiled, err)
	}
	if target.SourceDefinition.Builder.Filters[0].Variable != "service" {
		t.Fatal("binding mutated published source")
	}
	if _, err := bindTarget(spec, panel, target, map[string]Selection{"service": {Values: []string{"unmapped"}}}, nil); err == nil {
		t.Fatal("missing mapping broadened query")
	}
	compiled, err = CompileTarget(target)
	if err != nil || !compiled.Deferred || compiled.Query.Expression != "" {
		t.Fatal("validation probe exposed as executable query")
	}
	spec.Panels = []Panel{panel}
	spec.Panels[0].Targets = []Target{target}
	if report := Validate(spec); !report.Valid {
		t.Fatal(report.Issues)
	}
	spec.Panels[0].Targets[0].ParameterBindings = nil
	if Validate(spec).Valid {
		t.Fatal("publish accepted missing cross-source mapping")
	}
}

type candidateBackend struct {
	requests []telemetry.DataCatalogRequest
	respond  func(telemetry.DataCatalogRequest) (telemetry.DataCatalogResult, error)
}

func (b *candidateBackend) DiscoverData(_ context.Context, r telemetry.DataCatalogRequest) (telemetry.DataCatalogResult, error) {
	b.requests = append(b.requests, r)
	return b.respond(r)
}
func (*candidateBackend) ExecuteEngineQuery(context.Context, queryengine.Request) (queryengine.Result, error) {
	panic("candidate test must not execute targets")
}

func TestCandidateCascadeAbsenceAndUnavailableAreDifferent(t *testing.T) {
	source := SourceBinding{SourceType: "otlp", CapabilityVersion: "v1"}
	spec := EmptySpec()
	spec.Variables = []Variable{
		{ID: "env", Name: "env", IncludeAll: true, Multiple: true, Default: Selection{Values: []string{"prod", "gone"}}, Query: CandidateQuery{Signal: "logs", SourceBinding: source, Field: "resource_attributes.env"}},
		{ID: "service", Name: "service", IncludeAll: true, Default: Selection{Values: []string{"api"}}, Query: CandidateQuery{Signal: "logs", SourceBinding: source, Field: "service_name", Filters: []Filter{{Field: "resource_attributes.env", Operator: "=", Variable: "env"}}}},
	}
	resource, sourceID := uuid.New(), uuid.New()
	work := []candidateWork{}
	for _, v := range spec.Variables {
		work = append(work, candidateWork{name: v.Name, query: v.Query, resources: []uuid.UUID{resource}, sources: []ResolvedSource{{ID: sourceID, Revision: 1, ResourceID: resource}}})
	}
	for _, mode := range []string{"absent", "incomplete", "unavailable"} {
		t.Run(mode, func(t *testing.T) {
			result := Execution{From: time.Unix(10, 0), To: time.Unix(20, 0)}
			if err := prepareParameters(spec, ExecutionInput{}, &result); err != nil {
				t.Fatal(err)
			}
			backend := &candidateBackend{respond: func(r telemetry.DataCatalogRequest) (telemetry.DataCatalogResult, error) {
				if r.ResourceIDs[0] != resource || len(r.SourceKeys) != 1 || r.SourceKeys[0] != sourceID.String()+":1" {
					t.Fatal("candidate lost frozen scope")
				}
				if r.Field == "resource_attributes.env" {
					if mode == "unavailable" {
						return telemetry.DataCatalogResult{}, errors.New("temporary outage")
					}
					proof := map[string]bool{}
					if mode == "absent" {
						proof = map[string]bool{"prod": true, "gone": false}
					}
					return telemetry.DataCatalogResult{Values: []string{"prod"}, Complete: false, Membership: proof}, nil
				}
				if (len(r.Filters) == 0) != (mode == "absent") {
					t.Fatalf("downstream saw stale selection: %+v", r.Filters)
				}
				return telemetry.DataCatalogResult{Values: []string{}, Complete: false, Membership: map[string]bool{"api": true}}, nil
			}}
			if err := (Runtime{Backend: backend}).resolveCandidates(context.Background(), Actor{}, spec, work, &result, newExecutionBudget(3)); err != nil {
				t.Fatal(err)
			}
			if result.Variables["env"].All != (mode == "absent") || result.VariableCandidates["env"].Reset != (mode == "absent") {
				t.Fatalf("wrong fallback: %+v", result)
			}
			if result.Variables["service"].All || spec.Variables[0].Default.All {
				t.Fatal("retained selected value or saved default changed")
			}
		})
	}
}

func TestLocalFiltersAndCandidateBudget(t *testing.T) {
	panel := validPanel()
	panel.LocalFilters = []LocalFilter{{ID: "term", Kind: "text", Default: Selection{Values: []string{"never seen"}}}, {ID: "other", Kind: "query", Default: Selection{All: true}, Query: &CandidateQuery{Signal: "metrics", SourceBinding: panel.SourceBinding, Field: "service"}}}
	spec := EmptySpec()
	spec.Panels = []Panel{panel}
	result := Execution{}
	if err := prepareParameters(spec, ExecutionInput{LocalValues: map[string]map[string]Selection{"cpu": {"term": {Values: []string{"missing"}}}}}, &result); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.LocalValues["cpu"]["term"].Values, []string{"missing"}) {
		t.Fatal("manual condition reset")
	}
	if err := prepareParameters(spec, ExecutionInput{LocalValues: map[string]map[string]Selection{"unknown": {}}}, &result); !errors.Is(err, ErrInvalid) {
		t.Fatal("unknown panel accepted")
	}
	panel.LocalFilters[1].Query.Filters = []Filter{{Field: "service", Operator: "=", LocalParameter: "other"}}
	if _, err := localOrder(panel); err == nil {
		t.Fatal("local dependency cycle accepted")
	}
	budget := newExecutionBudget(2)
	allocation, err := budget.next(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	budget.failed(allocation)
	next, err := budget.next(context.Background())
	if err != nil || allocation.MaxScanBytes+next.MaxScanBytes > telemetry.DefaultMaxScanBytes {
		t.Fatal("failed candidate budget was recycled")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := budget.next(canceled); err == nil {
		t.Fatal("canceled work allocated")
	}
}
