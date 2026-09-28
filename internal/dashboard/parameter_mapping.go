package dashboard

import (
	"fmt"
	"slices"
)

type parameterContext struct {
	variables        map[string]Variable
	values           map[string]Selection
	locals           map[string]Selection
	panel            Panel
	localDefinitions map[string]LocalFilter
	rowInputs        map[string]Selection
}

func newParameterContext(spec Spec, panel Panel, values, locals map[string]Selection) parameterContext {
	ctx := parameterContext{variables: map[string]Variable{}, values: values, locals: locals, panel: panel, localDefinitions: map[string]LocalFilter{}}
	for _, variable := range spec.Variables {
		ctx.variables[variable.Name] = variable
	}
	for _, local := range panel.LocalFilters {
		ctx.localDefinitions[local.ID] = local
	}
	return ctx
}

func (ctx parameterContext) resolve(binding ParameterBinding) (Selection, error) {
	var selected Selection
	if bindingSourceCount(binding) != 1 {
		return selected, fmt.Errorf("%w: parameter requires exactly one input", ErrInvalid)
	}
	if binding.DrilldownInput != "" {
		var ok bool
		selected, ok = ctx.rowInputs[binding.DrilldownInput]
		if !ok || selected.All || len(selected.Values) != 1 || binding.IdentityMapping || len(binding.ValueMap) > 0 {
			return selected, fmt.Errorf("%w: missing or invalid drilldown input", ErrInvalid)
		}
	} else if binding.Variable != "" {
		variable, ok := ctx.variables[binding.Variable]
		if !ok {
			return selected, fmt.Errorf("%w: undefined variable %s", ErrInvalid, binding.Variable)
		}
		selected, ok = ctx.values[binding.Variable]
		if !ok {
			return selected, fmt.Errorf("%w: unresolved variable %s", ErrInvalid, binding.Variable)
		}
		if (variable.Query.Signal != ctx.panel.Signal || variable.Query.SourceBinding != ctx.panel.SourceBinding) && !binding.IdentityMapping && len(binding.ValueMap) == 0 {
			return selected, fmt.Errorf("%w: cross-source variable requires explicit mapping", ErrInvalid)
		}
	} else {
		if _, ok := ctx.localDefinitions[binding.LocalParameter]; !ok {
			return selected, fmt.Errorf("%w: undefined local parameter", ErrInvalid)
		}
		var ok bool
		selected, ok = ctx.locals[binding.LocalParameter]
		if !ok {
			return selected, fmt.Errorf("%w: unresolved local parameter", ErrInvalid)
		}
	}
	if binding.IdentityMapping && len(binding.ValueMap) > 0 || len(binding.ValueMap) > 1000 {
		return selected, ErrInvalid
	}
	for from, to := range binding.ValueMap {
		if len(from) > 4096 || len(to) > 4096 {
			return selected, ErrInvalid
		}
	}
	if selected.All {
		return Selection{All: true, Values: []string{}}, nil
	}
	selected.Values = slices.Clone(selected.Values)
	if len(binding.ValueMap) > 0 {
		for i, value := range selected.Values {
			mapped, ok := binding.ValueMap[value]
			if !ok {
				return selected, fmt.Errorf("%w: selected value has no source mapping", ErrInvalid)
			}
			selected.Values[i] = mapped
		}
	}
	return selected, nil
}

func bindTarget(spec Spec, panel Panel, target Target, values, locals map[string]Selection) (Target, error) {
	return bindTargetInputs(spec, panel, target, values, locals, nil)
}

func bindTargetInputs(spec Spec, panel Panel, target Target, values, locals, inputs map[string]Selection) (Target, error) {
	ctx := newParameterContext(spec, panel, values, locals)
	ctx.rowInputs = inputs
	parameters := map[string]Selection{}
	for _, binding := range target.ParameterBindings {
		if !identifier.MatchString(binding.Parameter) {
			return target, ErrInvalid
		}
		if _, duplicate := parameters[binding.Parameter]; duplicate {
			return target, ErrInvalid
		}
		value, err := ctx.resolve(binding)
		if err != nil {
			return target, err
		}
		parameters[binding.Parameter] = value
	}
	if target.SourceDefinition.Builder != nil {
		for i, b := range target.ParameterBindings {
			if !slices.ContainsFunc(target.SourceDefinition.Builder.Filters, func(f Filter) bool {
				return f.Variable == b.Variable && f.LocalParameter == b.LocalParameter && f.DrilldownInput == b.DrilldownInput
			}) {
				return target, fmt.Errorf("%w: unused builder parameter", ErrInvalid)
			}
			for _, previous := range target.ParameterBindings[:i] {
				if previous.Variable == b.Variable && previous.LocalParameter == b.LocalParameter && previous.DrilldownInput == b.DrilldownInput {
					return target, fmt.Errorf("%w: duplicate builder parameter source", ErrInvalid)
				}
			}
		}
		builder := *target.SourceDefinition.Builder
		builder.Filters = nil
		for _, filter := range target.SourceDefinition.Builder.Filters {
			if len(filter.Values) > 200 || len(filter.Value) > 4096 || filter.Value != "" && len(filter.Values) > 0 || (filter.Variable != "" || filter.LocalParameter != "" || filter.DrilldownInput != "") && (filter.Value != "" || len(filter.Values) > 0) {
				return target, ErrInvalid
			}
			if filter.Variable == "" && filter.LocalParameter == "" && filter.DrilldownInput == "" {
				builder.Filters = append(builder.Filters, filter)
				continue
			}
			binding := ParameterBinding{Variable: filter.Variable, LocalParameter: filter.LocalParameter, DrilldownInput: filter.DrilldownInput}
			for _, explicit := range target.ParameterBindings {
				if explicit.Variable == filter.Variable && explicit.LocalParameter == filter.LocalParameter && explicit.DrilldownInput == filter.DrilldownInput {
					binding = explicit
					break
				}
			}
			value, err := ctx.resolve(binding)
			if err != nil {
				return target, err
			}
			if value.All {
				continue
			}
			filter.Variable = ""
			filter.LocalParameter = ""
			filter.DrilldownInput = ""
			filter.Value = ""
			filter.Values = slices.Clone(value.Values)
			builder.Filters = append(builder.Filters, filter)
		}
		target.SourceDefinition = Definition{Builder: &builder}
	} else if target.SourceDefinition.DSL != nil {
		if err := validateQueryReferences(target, *target.SourceDefinition.DSL); err != nil {
			return target, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		query, err := bindDSL(target.Language, *target.SourceDefinition.DSL, parameters)
		if err != nil {
			return target, err
		}
		target.SourceDefinition = Definition{DSL: &query}
	}
	target.ParameterBindings = nil
	return target, nil
}

func bindingSourceCount(binding ParameterBinding) int {
	count := 0
	for _, name := range []string{binding.Variable, binding.LocalParameter, binding.DrilldownInput} {
		if name != "" {
			count++
		}
	}
	return count
}
