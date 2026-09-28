package dashboard

import (
	"fmt"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"math"
	"slices"
	"strconv"

	"github.com/kakj-go/Argus/internal/telemetry"
)

func validateParameters(spec Spec, issue func(string, string)) {
	values := map[string]Selection{}
	for _, v := range spec.Variables {
		values[v.Name] = v.Default
	}
	for _, v := range spec.Variables {
		if err := validateCandidate(spec, Panel{}, v.Query); err != nil {
			issue("variables."+v.Name, err.Error())
		}
	}
	for _, panel := range spec.Panels {
		path := "panels." + panel.ID
		if len(panel.LocalFilters) > 32 {
			issue(path, "local filter budget exceeded")
		}
		locals := map[string]Selection{}
		for _, f := range panel.LocalFilters {
			locals[f.ID] = f.Default
			if err := validateLocalSelection(f, f.Default); err != nil {
				issue(path+".local_filters."+f.ID, err.Error())
			}
			if f.Kind == "query" {
				if f.Query == nil {
					issue(path, "query filter needs a candidate definition")
					continue
				}
				if f.Query.Signal != panel.Signal || f.Query.SourceBinding != panel.SourceBinding {
					issue(path, "local candidate must inherit panel source and signal")
				}
				if err := validateCandidate(spec, panel, *f.Query); err != nil {
					issue(path, err.Error())
				}
			} else if f.Query != nil {
				issue(path, "text and numeric filters cannot have candidate queries")
			}
		}
		if _, err := localOrder(panel); err != nil {
			issue(path, err.Error())
		}
		for _, target := range panel.Targets {
			if err := validateParameterCardinality(spec, panel, target); err != nil {
				issue(path+".targets."+target.ID, err.Error())
			}
			bound, err := bindTarget(spec, panel, target, values, locals)
			if err == nil {
				_, err = compileConcreteTarget(bound, false)
			}
			if err != nil {
				issue(path+".targets."+target.ID, err.Error())
			}
			// Every optional candidate can automatically become All. Validate that
			// transition before publishing, even when the saved default is concrete.
			for _, binding := range target.ParameterBindings {
				if target.SourceDefinition.DSL == nil {
					continue
				}
				if binding.LocalParameter != "" && slices.ContainsFunc(panel.LocalFilters, func(f LocalFilter) bool { return f.ID == binding.LocalParameter && f.Required }) {
					continue
				}
				allValues, allLocals := cloneSelections(values), cloneSelections(locals)
				if binding.Variable != "" {
					allValues[binding.Variable] = Selection{All: true}
				} else {
					allLocals[binding.LocalParameter] = Selection{All: true}
				}
				b, e := bindTarget(spec, panel, target, allValues, allLocals)
				if e == nil {
					_, e = compileConcreteTarget(b, false)
				}
				if e != nil {
					issue(path+".targets."+target.ID, "parameter cannot represent All: "+e.Error())
				}
			}
		}
	}
}

func validateParameterCardinality(spec Spec, panel Panel, target Target) error {
	multiple := func(variable, local string) bool {
		for _, v := range spec.Variables {
			if variable == v.Name {
				return v.Multiple
			}
		}
		for _, f := range panel.LocalFilters {
			if local == f.ID {
				return f.Multiple
			}
		}
		return false
	}
	if b := target.SourceDefinition.Builder; b != nil {
		for _, f := range b.Filters {
			traceScalar := target.Language == queryengine.LanguageTrace && !apmAttributeField(f.Field)
			if multiple(f.Variable, f.LocalParameter) && (traceScalar || f.Operator == ">" || f.Operator == ">=" || f.Operator == "<" || f.Operator == "<=") {
				return fmt.Errorf("multi-select parameter requires an equality/set filter")
			}
		}
		return nil
	}
	if target.SourceDefinition.DSL == nil {
		return nil
	}
	parameters := map[string]Selection{}
	for _, b := range target.ParameterBindings {
		parameters[b.Parameter] = Selection{Values: []string{"0"}}
		if multiple(b.Variable, b.LocalParameter) {
			parameters[b.Parameter] = Selection{Values: []string{"0", "1"}}
		}
	}
	if target.Language == queryengine.LanguageTrace {
		types, err := graphQLParameterTypes(*target.SourceDefinition.DSL)
		if err != nil {
			return err
		}
		for _, b := range target.ParameterBindings {
			if !multiple(b.Variable, b.LocalParameter) {
				continue
			}
			t := types[b.Parameter]
			if n, ok := t.(*ast.NonNull); ok {
				t = n.Type
			}
			if _, ok := t.(*ast.List); !ok {
				return fmt.Errorf("multi-select parameter cannot bind a GraphQL scalar")
			}
		}
		return nil
	}
	_, err := bindDSL(target.Language, *target.SourceDefinition.DSL, parameters)
	return err
}

func validateLocalSelection(f LocalFilter, value Selection) error {
	if !slices.Contains([]string{"query", "text", "number"}, f.Kind) {
		return fmt.Errorf("invalid local filter kind")
	}
	if f.Kind == "query" && f.Required {
		return fmt.Errorf("candidate filters must support automatic All fallback")
	}
	if err := validateSelection(value, f.Multiple, !f.Required); err != nil {
		return err
	}
	if f.Kind == "number" && !value.All {
		for _, v := range value.Values {
			n, err := strconv.ParseFloat(v, 64)
			if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
				return fmt.Errorf("numeric filter needs finite numbers")
			}
		}
	}
	return nil
}

func validateCandidate(spec Spec, panel Panel, query CandidateQuery) error {
	for _, f := range query.Filters {
		if f.DrilldownInput != "" {
			return fmt.Errorf("candidate query cannot depend on clicked rows")
		}
	}
	if err := validateParameterCardinality(spec, panel, Target{Language: queryengine.LanguageKQL, SourceDefinition: Definition{Builder: &Builder{Filters: query.Filters}}}); err != nil {
		return err
	}
	if query.SourceBinding.CapabilityVersion != "v1" || !slices.Contains(sourceSignals[query.SourceBinding.SourceType], query.Signal) || len(query.Metric) > 256 {
		return fmt.Errorf("unsupported candidate source")
	}
	// Use All to verify references/mappings without executing discovery.
	values, locals := map[string]Selection{}, map[string]Selection{}
	for _, v := range spec.Variables {
		values[v.Name] = Selection{All: true}
	}
	for _, f := range panel.LocalFilters {
		locals[f.ID] = Selection{All: true}
	}
	if _, err := bindCandidate(spec, panel, query, values, locals); err != nil {
		return err
	}
	filters := []telemetry.CatalogFilter{}
	for _, f := range query.Filters {
		if (f.Variable != "" || f.LocalParameter != "") && (f.Value != "" || len(f.Values) > 0) || f.Variable != "" && f.LocalParameter != "" {
			return fmt.Errorf("filter must have one value source")
		}
		values := f.Values
		if len(values) == 0 {
			values = []string{f.Value}
		}
		if f.Variable != "" || f.LocalParameter != "" {
			values = []string{"0"}
		}
		filters = append(filters, telemetry.CatalogFilter{Field: f.Field, Operator: f.Operator, Values: values})
	}
	return telemetry.ValidateCatalogDefinition(query.Signal, query.Field, filters)
}

func bindCandidate(spec Spec, panel Panel, query CandidateQuery, values, locals map[string]Selection) ([]telemetry.CatalogFilter, error) {
	panel.Signal, panel.SourceBinding = query.Signal, query.SourceBinding
	target := Target{SourceDefinition: Definition{Builder: &Builder{Filters: query.Filters}}, ParameterBindings: query.ParameterBindings}
	bound, err := bindTarget(spec, panel, target, values, locals)
	if err != nil {
		return nil, err
	}
	filters := []telemetry.CatalogFilter{}
	for _, f := range bound.SourceDefinition.Builder.Filters {
		values := f.Values
		if len(values) == 0 {
			values = []string{f.Value}
		}
		filters = append(filters, telemetry.CatalogFilter{Field: f.Field, Operator: f.Operator, Values: values})
	}
	return filters, nil
}

func cloneSelections(values map[string]Selection) map[string]Selection {
	result := map[string]Selection{}
	for name, value := range values {
		value.Values = slices.Clone(value.Values)
		result[name] = value
	}
	return result
}

func dependencyOrder(names []string, dependencies func(string) []string) ([]string, error) {
	known, state := map[string]bool{}, map[string]int{}
	for _, n := range names {
		known[n] = true
	}
	result := []string{}
	var visit func(string) error
	visit = func(n string) error {
		if !known[n] {
			return fmt.Errorf("undefined parameter %s", n)
		}
		if state[n] == 1 {
			return fmt.Errorf("cyclic parameter dependency %s", n)
		}
		if state[n] == 2 {
			return nil
		}
		state[n] = 1
		for _, d := range dependencies(n) {
			if err := visit(d); err != nil {
				return err
			}
		}
		state[n] = 2
		result = append(result, n)
		return nil
	}
	for _, n := range names {
		if err := visit(n); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func localOrder(panel Panel) ([]string, error) {
	names, deps := []string{}, map[string][]string{}
	for _, f := range panel.LocalFilters {
		names = append(names, f.ID)
		if f.Query != nil {
			for _, filter := range f.Query.Filters {
				if filter.LocalParameter != "" {
					deps[f.ID] = append(deps[f.ID], filter.LocalParameter)
				}
			}
		}
	}
	return dependencyOrder(names, func(n string) []string { return deps[n] })
}
