package dashboard

import (
	"context"
	"slices"
)

type drilldownProof func(context.Context, Drilldown, Target, queryScope, map[string]string, *executionBudget) (any, error)
type preparedDrilldown struct {
	Panel         Panel
	Drill         Drilldown
	Detail        Target
	Scope         queryScope
	DetailSources map[string][]ResolvedSource
	Budget        *executionBudget
}

// Both interactive and file-based navigation share published-definition, row,
// time-window and authorization rules. Only the row evidence provider differs.
func (runtime Runtime) prepareDrilldown(ctx context.Context, actor Actor, spec Spec, frozen executionContext, input DrilldownInput, proof drilldownProof) (preparedDrilldown, error) {
	runtime = runtime.metered(actor)
	if frozen.Depth < 0 || frozen.Depth >= 16 {
		return preparedDrilldown{}, ErrContextExpired
	}
	index := slices.IndexFunc(spec.Panels, func(p Panel) bool { return p.ID == input.PanelID })
	if index < 0 {
		return preparedDrilldown{}, ErrInvalid
	}
	panel := spec.Panels[index]
	index = slices.IndexFunc(panel.Drilldowns, func(d Drilldown) bool { return d.ID == input.DrilldownID })
	if index < 0 {
		return preparedDrilldown{}, ErrInvalid
	}
	drill := panel.Drilldowns[index]
	if (drill.ScopePolicy == "authorized_trace") != input.ExpandAuthorizedResources {
		return preparedDrilldown{}, ErrInvalid
	}
	if _, err := matchDrilldownRow(nil, drill, input.Values); err != nil {
		return preparedDrilldown{}, err
	}
	origin, base, originInputs, detailSources, err := drillOrigin(spec, panel, drill, frozen)
	if err != nil {
		return preparedDrilldown{}, err
	}
	if err := runtime.checkQueryScope(ctx, actor, base); err != nil {
		return preparedDrilldown{}, err
	}
	detail, ok := findTarget(panel, drill.DetailQueryRef)
	if !ok || detail.SourceBinding == nil {
		return preparedDrilldown{}, ErrInvalid
	}
	count := 1
	if len(drill.Inputs) > 0 {
		count++
	}
	if drill.ScopePolicy == "authorized_trace" {
		count++
	}
	ledger := newExecutionBudget(count)
	variables, locals := frozen.Scope.Variables, frozen.Scope.LocalValues[panel.ID]
	if len(drill.Inputs) > 0 {
		data, e := proof(ctx, drill, origin, base, originInputs, ledger)
		if e != nil {
			return preparedDrilldown{}, e
		}
		matches, e := matchDrilldownRow(data, drill, input.Values)
		if e != nil {
			return preparedDrilldown{}, e
		}
		if !matches {
			return preparedDrilldown{}, ErrSelectionStale
		}
	}
	scope := base
	scope.Sources = detailSources[detail.ID]
	if drill.ScopePolicy == "authorized_trace" {
		_, e := runtime.checkExpansionAnchor(ctx, actor, spec, panel, detail, variables, locals, input.Values, base, ledger)
		if e != nil {
			return preparedDrilldown{}, e
		}
		scope.Resources, e = resolveResources(ctx, runtime.Store.Queries, actor, nil)
		if e != nil {
			return preparedDrilldown{}, e
		}
		detailSources, e = runtime.freezeDetailScopes(ctx, actor, panel, scope.Resources)
		if e != nil {
			return preparedDrilldown{}, e
		}
		scope.Sources = detailSources[detail.ID]
	}
	scope, err = applyDrillWindow(scope, drill, input.Values)
	if err != nil {
		return preparedDrilldown{}, err
	}
	return preparedDrilldown{panel, drill, detail, scope, detailSources, ledger}, nil
}
