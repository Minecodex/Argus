package dashboard

import (
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
)

func rowInputNames(target Target) []string {
	names := []string{}
	add := func(name string) {
		if name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	for _, binding := range target.ParameterBindings {
		add(binding.DrilldownInput)
	}
	if b := target.SourceDefinition.Builder; b != nil {
		for _, f := range b.Filters {
			add(f.DrilldownInput)
		}
	}
	return names
}

func probeRowInputs(target Target) map[string]Selection {
	result := map[string]Selection{}
	for _, name := range rowInputNames(target) {
		result[name] = Selection{Values: []string{"0"}}
	}
	if target.Language == queryengine.LanguageTrace {
		for name := range result {
			result[name] = Selection{Values: []string{uuid.Nil.String()}}
		}
		if query := target.SourceDefinition.DSL; query != nil {
			types, _ := graphQLParameterTypes(*query)
			for _, b := range target.ParameterBindings {
				if b.DrilldownInput == "" {
					continue
				}
				switch graphQLScalarName(types[b.Parameter]) {
				case "Boolean":
					result[b.DrilldownInput] = Selection{Values: []string{"false"}}
				case "Float", "Int":
					result[b.DrilldownInput] = Selection{Values: []string{"1"}}
				}
			}
		}
	}
	if b := target.SourceDefinition.Builder; b != nil {
		for _, f := range b.Filters {
			if f.DrilldownInput != "" && numericFilter(f) {
				result[f.DrilldownInput] = Selection{Values: []string{"0"}}
			}
		}
	}
	return result
}

func validateDrilldowns(spec Spec, issue func(string, string)) {
	values := map[string]Selection{}
	for _, v := range spec.Variables {
		values[v.Name] = v.Default
	}
	for _, panel := range spec.Panels {
		path := "panels." + panel.ID + ".drilldowns"
		if len(panel.Drilldowns) > 64 {
			issue(path, "drilldown count exceeds budget")
		}
		targets := map[string]Target{}
		details := map[string]bool{}
		locals := map[string]Selection{}
		for _, f := range panel.LocalFilters {
			locals[f.ID] = f.Default
		}
		for _, target := range panel.Targets {
			targets[target.ID] = target
			if len(rowInputNames(target)) > 0 {
				issue(path, "display query cannot depend on a clicked row")
			}
		}
		for _, target := range panel.DetailQueryTargets {
			targets[target.ID] = target
			details[target.ID] = true
			if target.SourceBinding == nil || target.SourceBinding.CapabilityVersion != "v1" || !slices.Contains(sourceSignals[target.SourceBinding.SourceType], target.Signal) {
				issue(path, "detail source capability unsupported")
				continue
			}
			ctxPanel := panel
			ctxPanel.SourceBinding, ctxPanel.Signal = *target.SourceBinding, target.Signal
			bound, err := bindTargetInputs(spec, ctxPanel, target, values, locals, probeRowInputs(target))
			if err == nil {
				_, err = compileConcreteTarget(bound, false)
			}
			if err != nil {
				issue(path+"."+target.ID, err.Error())
			}
			for _, name := range rowInputNames(target) {
				if !identifier.MatchString(name) {
					issue(path, "invalid drilldown input name")
				}
			}
		}
		ids, used := map[string]bool{}, map[string]bool{}
		for _, drill := range panel.Drilldowns {
			if !identifier.MatchString(drill.ID) || ids[drill.ID] || len(drill.Title) > 240 {
				issue(path, "invalid or duplicate drilldown id")
			}
			ids[drill.ID] = true
			origin, ok := targets[drill.OriginQueryRef]
			if !ok {
				issue(path, "origin query is undefined")
			}
			detail, ok := targets[drill.DetailQueryRef]
			if !ok || !details[detail.ID] {
				issue(path, "detail query is undefined")
				continue
			}
			used[detail.ID] = true
			required := rowInputNames(detail)
			if drill.TimeWindow != nil {
				if (drill.TimeWindow.Seconds == 0) == (drill.TimeWindow.DurationInput == "") || drill.TimeWindow.Seconds < 0 || drill.TimeWindow.Seconds > 7*86400 || !identifier.MatchString(drill.TimeWindow.Input) || drill.TimeWindow.DurationInput != "" && !identifier.MatchString(drill.TimeWindow.DurationInput) {
					issue(path, "invalid drilldown time window")
				}
				if !slices.Contains(required, drill.TimeWindow.Input) {
					required = append(required, drill.TimeWindow.Input)
				}
				if drill.TimeWindow.DurationInput != "" && !slices.Contains(required, drill.TimeWindow.DurationInput) {
					required = append(required, drill.TimeWindow.DurationInput)
				}
			}
			if len(drill.Inputs) > 16 || len(drill.Inputs) != len(required) {
				issue(path, "drilldown inputs do not match detail parameters")
			}
			for name, pointer := range drill.Inputs {
				if !slices.Contains(required, name) || !validRowPointer(pointer) {
					issue(path, "invalid row input mapping")
				}
			}
			for _, name := range required {
				if _, ok := drill.Inputs[name]; !ok {
					issue(path, "missing row input mapping")
				}
			}
			if drill.ScopePolicy == "authorized_trace" {
				originPanel := targetPanel(panel, origin)
				if originPanel.Signal != "traces" || detail.Signal != "traces" || detail.SourceBinding == nil || *detail.SourceBinding != originPanel.SourceBinding {
					issue(path, "full trace expansion must keep the originating trace source type")
				}
				for _, name := range []string{"trace_id", "source_id", "resource_id"} {
					if _, ok := drill.Inputs[name]; !ok {
						issue(path, "full trace expansion requires anchor identity inputs")
					}
				}
				bound, err := bindTargetInputs(spec, targetPanel(panel, detail), detail, values, locals, probeRowInputs(detail))
				if err == nil {
					compiled, e := compileConcreteTarget(bound, false)
					err = e
					if err == nil {
						_, err = skywalking.GraphIdentityFromQuery(compiled.Query.Expression, compiled.Query.Operation, compiled.Query.Variables)
					}
				}
				if err != nil {
					issue(path, err.Error())
				}
				compiled, err := CompileTarget(origin)
				if err == nil && compiled.ResultType != "traces" && compiled.ResultType != "trace_graph" {
					issue(path, "full trace expansion must start from a trace result")
				}
			}
		}
		for id := range details {
			if !used[id] {
				issue(path, fmt.Sprintf("detail query %s is not referenced", id))
			}
		}
	}
}

func validRowPointer(value string) bool {
	if value == "" || len(value) > 512 || !strings.HasPrefix(value, "/") {
		return false
	}
	return len(strings.Split(value[1:], "/")) <= 8
}
