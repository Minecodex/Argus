package dashboard

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

var chartSignals = map[string][]string{
	"timeseries": {"metrics", "logs"}, "stat": {"metrics", "logs"}, "gauge": {"metrics"}, "bar_gauge": {"metrics"},
	"bar": {"metrics", "logs"}, "pie": {"metrics", "logs"}, "histogram": {"metrics"}, "heatmap": {"metrics"},
	"state_timeline": {"metrics"}, "scatter": {"metrics"}, "table": {"metrics", "logs"}, "logs": {"logs"},
	"trace_list": {"traces"}, "trace_detail": {"traces"}, "apm_services": {"traces"}, "apm_instances": {"traces"},
	"apm_endpoints": {"traces"}, "apm_red": {"traces"}, "apm_topology": {"traces"},
}

func Validate(spec Spec) ValidationReport {
	report := ValidationReport{Valid: true, Issues: []Issue{}, CompilerVersion: CompilerVersion}
	issue := func(path, message string) {
		report.Valid = false
		report.Issues = append(report.Issues, Issue{Path: path, Code: "DASHBOARD_INVALID", Message: message})
	}
	if spec.SchemaVersion != SchemaVersion {
		issue("schema_version", "unsupported schema version")
	}
	if spec.Layout.Columns != 12 || spec.Layout.RowHeight < 1 || spec.Layout.RowHeight > 64 {
		issue("layout", "expected twelve columns and a valid row height")
	}
	if spec.DefaultRefreshSeconds != 0 && (spec.DefaultRefreshSeconds < 5 || spec.DefaultRefreshSeconds > 86400) {
		issue("default_refresh_seconds", "refresh must be disabled or between 5 and 86400 seconds")
	}
	if r := spec.DefaultTimeRange; r.Kind == "relative" {
		if r.Seconds < 1 || r.Seconds > 7*86400 {
			issue("default_time_range", "relative time outside supported range")
		}
	} else if r.Kind != "absolute" || r.From == nil || r.To == nil || !r.To.After(*r.From) || r.To.Sub(*r.From).Hours() > 7*24 {
		issue("default_time_range", "invalid absolute time range")
	}
	if len(spec.Panels) > 64 || len(spec.Variables) > 32 {
		issue("spec", "panel or variable budget exceeded")
	}
	variables := map[string]Variable{}
	ids := map[string]bool{}
	for i, v := range spec.Variables {
		path := fmt.Sprintf("variables[%d]", i)
		if !identifier.MatchString(v.Name) || variables[v.Name].Name != "" {
			issue(path, "invalid or duplicate variable name")
		}
		if v.ID == "" || ids[v.ID] {
			issue(path, "invalid or duplicate variable id")
		}
		ids[v.ID] = true
		variables[v.Name] = v
		if v.Query.Field == "" || v.Query.SourceBinding.SourceType == "" || !slices.Contains([]string{"metrics", "logs", "traces"}, v.Query.Signal) {
			issue(path, "candidate query needs signal, source and field")
		}
		if err := validateSelection(v.Default, v.Multiple, v.IncludeAll); err != nil {
			issue(path, err.Error())
		}
	}
	visiting, visited := map[string]bool{}, map[string]bool{}
	var visit func(string)
	visit = func(name string) {
		if visiting[name] {
			issue("variables", "cyclic variable dependency: "+name)
			return
		}
		if visited[name] {
			return
		}
		visiting[name] = true
		for _, filter := range variables[name].Query.Filters {
			if filter.LocalParameter != "" {
				issue("variables", "candidate query cannot depend on a panel local parameter")
			}
			if filter.Variable != "" {
				if _, ok := variables[filter.Variable]; !ok {
					issue("variables", "undefined variable: "+filter.Variable)
				} else {
					visit(filter.Variable)
				}
			}
		}
		visiting[name] = false
		visited[name] = true
	}
	for name := range variables {
		visit(name)
	}
	ids = map[string]bool{}
	for i, panel := range spec.Panels {
		path := fmt.Sprintf("panels[%d]", i)
		if panel.ID == "" || ids[panel.ID] {
			issue(path, "invalid or duplicate panel id")
		}
		ids[panel.ID] = true
		if strings.TrimSpace(panel.Title) == "" || len(panel.Title) > 240 {
			issue(path, "panel title required, at most 240 bytes")
		}
		if !slices.Contains(chartSignals[panel.Type], panel.Signal) {
			issue(path, "chart and signal are incompatible")
		}
		validateDisplay(panel, path, issue)
		if panel.AuthoringMode != "builder" && panel.AuthoringMode != "dsl" {
			issue(path, "invalid authoring mode")
		}
		if panel.SourceBinding.SourceType == "" || panel.SourceBinding.CapabilityVersion == "" {
			issue(path, "source type and capability version required")
		}
		if len(panel.ApplicableResourceTypes) == 0 {
			issue(path, "resource applicability is required")
		}
		for _, kind := range panel.ApplicableResourceTypes {
			if kind != "host" && kind != "kubernetes_cluster" {
				issue(path, "unsupported resource type")
			}
		}
		r := panel.Layout
		if r.X < 0 || r.Y < 0 || r.Y > 100000 || r.MinW < 1 || r.MinH < 1 || r.W < r.MinW || r.H < r.MinH || r.X+r.W > 12 || r.H > 1000 {
			issue(path+".layout", "invalid grid rectangle")
		}
		for j := 0; j < i; j++ {
			o := spec.Panels[j].Layout
			if r.X < o.X+o.W && o.X < r.X+r.W && r.Y < o.Y+o.H && o.Y < r.Y+r.H {
				issue(path+".layout", "panels overlap")
			}
		}
		locals := map[string]bool{}
		for _, local := range panel.LocalFilters {
			if !identifier.MatchString(local.ID) || locals[local.ID] {
				issue(path, "invalid or duplicate local filter")
			}
			locals[local.ID] = true
		}
		if len(panel.Targets) == 0 || len(panel.Targets) > 8 || len(panel.DetailQueryTargets) > 16 {
			issue(path, "query target count outside budget")
		}
		targets := map[string]bool{}
		details := map[string]bool{}
		all := append(slices.Clone(panel.Targets), panel.DetailQueryTargets...)
		for index, target := range all {
			if target.ID == "" || targets[target.ID] {
				issue(path, "invalid or duplicate target id")
			}
			targets[target.ID] = true
			detail := index >= len(panel.Targets)
			if detail {
				details[target.ID] = true
			}
			signal := panel.Signal
			if detail {
				signal = target.Signal
				if target.SourceBinding == nil || target.SourceBinding.SourceType == "" {
					issue(path, "detail target needs its own source binding")
				}
			} else if target.Signal != "" && target.Signal != panel.Signal || target.SourceBinding != nil {
				issue(path, "display target must inherit panel signal and source")
			}
			if languageForSignal(signal) != target.Language {
				issue(path, "query language and signal are incompatible")
			}
			if panel.AuthoringMode == "builder" && (target.SourceDefinition.Builder == nil || target.SourceDefinition.DSL != nil) || panel.AuthoringMode == "dsl" && (target.SourceDefinition.DSL == nil || target.SourceDefinition.Builder != nil) {
				issue(path, "query must have exactly one editable source")
				continue
			}
			for _, binding := range target.ParameterBindings {
				if binding.Parameter == "" || bindingSourceCount(binding) != 1 {
					issue(path, "parameter binding needs one source")
				}
				if binding.Variable != "" {
					if _, ok := variables[binding.Variable]; !ok {
						issue(path, "undefined variable: "+binding.Variable)
					}
				}
				if binding.LocalParameter != "" && !locals[binding.LocalParameter] {
					issue(path, "undefined local parameter: "+binding.LocalParameter)
				}
			}
			if _, err := CompileTarget(target); err != nil {
				issue(path+".targets."+target.ID, err.Error())
			}
		}
		for _, drill := range panel.Drilldowns {
			if !details[drill.DetailQueryRef] {
				issue(path, "drilldown must reference a published detail query")
			}
			if drill.ScopePolicy != "inherit" && drill.ScopePolicy != "authorized_trace" {
				issue(path, "invalid drilldown resource scope")
			}
		}
	}
	validateParameters(spec, issue)
	validateDrilldowns(spec, issue)
	return report
}

func validateSelection(value Selection, multiple, includeAll bool) error {
	if value.All {
		if !includeAll || len(value.Values) != 0 {
			return fmt.Errorf("invalid All selection")
		}
		return nil
	}
	if !multiple && len(value.Values) > 1 || len(value.Values) > 200 {
		return fmt.Errorf("selection exceeds cardinality")
	}
	if len(value.Values) == 0 {
		return fmt.Errorf("choose All or at least one value")
	}
	for i, v := range value.Values {
		if len(v) > 4096 || slices.Contains(value.Values[:i], v) {
			return fmt.Errorf("invalid or duplicate selection value")
		}
	}
	return nil
}

// ReconcileSelection distinguishes proven absence from an incomplete page or
// unavailable discovery. Only proven absence may broaden a custom selection.
func ReconcileSelection(current Selection, candidates []string, complete bool, membership map[string]bool) (Selection, bool) {
	if current.All {
		return current, false
	}
	for _, value := range current.Values {
		present, checked := membership[value]
		if checked && !present || !checked && complete && !slices.Contains(candidates, value) {
			return Selection{All: true, Values: []string{}}, true
		}
	}
	return current, false
}
