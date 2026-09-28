package dashboard

import (
	"encoding/json"
	"fmt"
	"github.com/graphql-go/graphql/language/printer"
	"reflect"
	"slices"

	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

type ConvertPanelInput struct {
	Panel Panel  `json:"panel"`
	Mode  string `json:"mode"`
}
type ConvertedPanel struct {
	Converted bool    `json:"converted"`
	Panel     Panel   `json:"panel"`
	Issues    []Issue `json:"issues"`
}

// Conversion is all-or-nothing across display and detail targets. There is no
// hidden builder backup: the returned definition is the sole editable source.
func ConvertPanel(input ConvertPanelInput) ConvertedPanel {
	result := ConvertedPanel{Panel: input.Panel, Issues: []Issue{}}
	fail := func(path string, err error) ConvertedPanel {
		result.Issues = append(result.Issues, Issue{Path: path, Code: "QUERY_CONVERSION_UNSUPPORTED", Message: err.Error()})
		return result
	}
	if input.Mode != "builder" && input.Mode != "dsl" {
		return fail("mode", fmt.Errorf("unknown editing source"))
	}
	if len(input.Panel.Targets) == 0 || len(input.Panel.Targets) > 8 || len(input.Panel.DetailQueryTargets) > 16 {
		return fail("targets", fmt.Errorf("query count outside conversion budget"))
	}
	encoded, err := json.Marshal(input.Panel)
	if err != nil || len(encoded) > 1024*1024 {
		return fail("panel", fmt.Errorf("configuration exceeds conversion budget"))
	}
	var panel Panel
	if err = json.Unmarshal(encoded, &panel); err != nil {
		return fail("panel", err)
	}
	for _, group := range []struct {
		path    string
		targets []Target
	}{{"targets", panel.Targets}, {"detail_query_targets", panel.DetailQueryTargets}} {
		for i, target := range group.targets {
			before, compileErr := CompileTarget(target)
			if compileErr != nil {
				return fail(group.path+"."+target.ID, compileErr)
			}
			if input.Mode == "dsl" {
				group.targets[i], err = targetToDSL(target)
			} else {
				group.targets[i], err = targetToBuilder(target)
			}
			if err != nil {
				return fail(group.path+"."+target.ID, err)
			}
			after, compileErr := CompileTarget(group.targets[i])
			if compileErr != nil {
				return fail(group.path+"."+target.ID, compileErr)
			}
			if before.ResultType != after.ResultType {
				return fail(group.path+"."+target.ID, fmt.Errorf("conversion would change the result type"))
			}
		}
	}
	panel.AuthoringMode = input.Mode
	normalizePanelCollections(&panel)
	result.Panel = panel
	result.Converted = true
	return result
}

func filterBinding(f Filter, bindings []ParameterBinding) (ParameterBinding, bool) {
	if f.Variable == "" && f.LocalParameter == "" && f.DrilldownInput == "" {
		return ParameterBinding{}, false
	}
	for _, b := range bindings {
		if b.Variable == f.Variable && b.LocalParameter == f.LocalParameter && b.DrilldownInput == f.DrilldownInput {
			return b, true
		}
	}
	return ParameterBinding{Variable: f.Variable, LocalParameter: f.LocalParameter, DrilldownInput: f.DrilldownInput}, true
}
func templateBindings(target Target) ([]ParameterBinding, map[int]string, error) {
	bindings := slices.Clone(target.ParameterBindings)
	names := map[int]string{}
	used := map[string]bool{}
	types := map[string]string{}
	aliases := map[string]string{}
	for _, binding := range bindings {
		if !identifier.MatchString(binding.Parameter) || used[binding.Parameter] || bindingSourceCount(binding) != 1 {
			return nil, nil, fmt.Errorf("invalid parameter binding")
		}
		used[binding.Parameter] = true
	}
	for i, f := range target.SourceDefinition.Builder.Filters {
		binding, dynamic := filterBinding(f, bindings)
		if !dynamic {
			continue
		}
		if bindingSourceCount(binding) != 1 || f.Value != "" || len(f.Values) > 0 {
			return nil, nil, fmt.Errorf("filter needs exactly one value source")
		}
		if binding.Parameter == "" {
			for n := 1; ; n++ {
				name := fmt.Sprintf("p%d", n)
				if !used[name] {
					binding.Parameter = name
					used[name] = true
					break
				}
			}
			bindings = append(bindings, binding)
		}
		names[i] = binding.Parameter
		if target.Language == queryengine.LanguageTrace {
			typ := fmt.Sprint(printer.Print(graphFilterType(*target.SourceDefinition.Builder, f)))
			if previous, ok := types[binding.Parameter]; ok && previous != typ {
				base := binding.Parameter
				aliasKey := base + "/" + typ
				if alias := aliases[aliasKey]; alias != "" {
					binding.Parameter = alias
				} else {
					for n := 2; ; n++ {
						alias := fmt.Sprintf("%s_%d", base, n)
						if len(alias) > 64 {
							alias = fmt.Sprintf("p%d", n)
						}
						if !used[alias] {
							binding.Parameter = alias
							used[alias] = true
							bindings = append(bindings, binding)
							aliases[aliasKey] = alias
							break
						}
					}
				}
			}
			types[binding.Parameter] = typ
			names[i] = binding.Parameter
		}
	}
	return bindings, names, nil
}
func targetToDSL(target Target) (Target, error) {
	if target.SourceDefinition.DSL != nil {
		return target, nil
	}
	if target.Language == queryengine.LanguagePromQL || target.Language == queryengine.LanguageKQL {
		builder := target.SourceDefinition.Builder
		for _, filters := range [][]Filter{builder.Filters, builder.ErrorFilters} {
			for _, f := range filters {
				if f.Variable != "" || f.LocalParameter != "" || f.DrilldownInput != "" {
					continue
				}
				for _, value := range append(slices.Clone(f.Values), f.Value) {
					if parameterName(value) != "" {
						return target, fmt.Errorf("literal resembles a query parameter; keep builder to preserve its meaning")
					}
				}
			}
		}
	}
	bindings, names, err := templateBindings(target)
	if err != nil {
		return target, err
	}
	var query DSL
	switch target.Language {
	case queryengine.LanguagePromQL:
		builder := *target.SourceDefinition.Builder
		builder.Filters = slices.Clone(builder.Filters)
		for i, name := range names {
			f := &builder.Filters[i]
			f.Variable, f.LocalParameter, f.DrilldownInput = "", "", ""
			f.Value = "$" + name
			f.Values = nil
		}
		query, err = compileBuilder(target.Language, builder)
	case queryengine.LanguageKQL:
		query, err = logBuilderTemplate(*target.SourceDefinition.Builder, names)
	case queryengine.LanguageTrace:
		query, err = traceBuilderTemplate(*target.SourceDefinition.Builder, names)
	default:
		err = fmt.Errorf("unsupported query language")
	}
	if err != nil {
		return target, err
	}
	target.SourceDefinition = Definition{DSL: &query}
	target.ParameterBindings = bindings
	return target, nil
}
func targetToBuilder(target Target) (Target, error) {
	if target.SourceDefinition.Builder != nil {
		return target, nil
	}
	original := target
	var builder Builder
	var err error
	switch target.Language {
	case queryengine.LanguagePromQL:
		builder, err = metricBuilderFromDSL(target)
	case queryengine.LanguageKQL:
		builder, err = logBuilderFromDSL(target)
	case queryengine.LanguageTrace:
		builder, err = traceBuilderFromDSL(target)
	default:
		err = fmt.Errorf("unsupported query language")
	}
	if err != nil {
		return original, err
	}
	target.SourceDefinition = Definition{Builder: &builder}
	target.ParameterBindings, err = collapseConversionBindings(target.ParameterBindings)
	if err != nil {
		return original, err
	}
	regenerated, err := targetToDSL(target)
	if err != nil {
		return original, err
	}
	equal, err := equivalentConversionTargets(original, regenerated)
	if err != nil || !equal {
		return original, fmt.Errorf("query contains semantics not expressible by this builder")
	}
	return target, nil
}

func collapseConversionBindings(bindings []ParameterBinding) ([]ParameterBinding, error) {
	result := slices.Clone(bindings[:0])
	for _, b := range bindings {
		found := false
		for _, previous := range result {
			if previous.Variable == b.Variable && previous.LocalParameter == b.LocalParameter && previous.DrilldownInput == b.DrilldownInput {
				a, c := previous, b
				a.Parameter, c.Parameter = "", ""
				if !reflect.DeepEqual(a, c) {
					return nil, fmt.Errorf("per-condition mappings cannot be represented by the builder")
				}
				found = true
				break
			}
		}
		if !found {
			result = append(result, b)
		}
	}
	return result, nil
}
func restoreFilterParameter(filter Filter, name string, bindings []ParameterBinding) (Filter, error) {
	for _, binding := range bindings {
		if binding.Parameter == name {
			filter.Value = ""
			filter.Values = nil
			filter.Variable = binding.Variable
			filter.LocalParameter = binding.LocalParameter
			filter.DrilldownInput = binding.DrilldownInput
			return filter, nil
		}
	}
	return filter, fmt.Errorf("missing parameter binding %s", name)
}
