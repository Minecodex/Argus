package dashboard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
)

type GeneratedDrilldowns struct {
	Spec   Spec     `json:"spec"`
	Added  []string `json:"added"`
	Issues []Issue  `json:"issues"`
}

func GenerateStandardDrilldowns(spec Spec, panelID string, routes map[string]SourceBinding) (GeneratedDrilldowns, error) {
	for signal, binding := range routes {
		if !slices.Contains([]string{"metrics", "logs", "traces"}, signal) || binding.CapabilityVersion != "v1" || !slices.Contains(sourceSignals[binding.SourceType], signal) {
			return GeneratedDrilldowns{}, ErrInvalid
		}
	}
	encoded, _ := json.Marshal(spec)
	copy, err := DecodeSpec(encoded)
	if err != nil {
		return GeneratedDrilldowns{}, err
	}
	result := GeneratedDrilldowns{Spec: copy, Added: []string{}, Issues: []Issue{}}
	index := slices.IndexFunc(copy.Panels, func(p Panel) bool { return p.ID == panelID })
	if index < 0 {
		return result, ErrInvalid
	}
	panel := &result.Spec.Panels[index]
	add := func(origin Target, target Target, kind, policy string, inputs map[string]string) *Target {
		hash := sha256.Sum256([]byte(origin.ID + "/" + kind))
		suffix := hex.EncodeToString(hash[:6])
		target.ID = "detail_" + suffix
		if _, exists := findTarget(*panel, target.ID); exists {
			return nil
		}
		if panel.AuthoringMode == "dsl" {
			converted, err := targetToDSL(target)
			if err != nil {
				result.Issues = append(result.Issues, Issue{Path: target.ID, Code: "DRILLDOWN_CONVERSION_UNSUPPORTED", Message: err.Error()})
				return nil
			}
			target = converted
		}
		normalizeTargetCollections(&target)
		panel.DetailQueryTargets = append(panel.DetailQueryTargets, target)
		drill := Drilldown{ID: "open_" + suffix, Kind: kind, OriginQueryRef: origin.ID, DetailQueryRef: target.ID, ScopePolicy: policy, Inputs: inputs}
		panel.Drilldowns = append(panel.Drilldowns, drill)
		result.Added = append(result.Added, drill.ID)
		return &panel.DetailQueryTargets[len(panel.DetailQueryTargets)-1]
	}

	addLogContext := func(origin Target, binding SourceBinding) {
		target := Target{Language: queryengine.LanguageKQL, Signal: "logs", SourceBinding: &binding, SourceDefinition: Definition{Builder: &Builder{Operation: "log_context", ContextBefore: 20, ContextAfter: 20, Filters: []Filter{{Field: "event_id", Operator: "=", DrilldownInput: "event"}}}}}
		if b := origin.SourceDefinition.Builder; b != nil {
			for _, f := range b.Filters {
				if f.Variable != "" || f.LocalParameter != "" {
					target.SourceDefinition.Builder.Filters = append(target.SourceDefinition.Builder.Filters, f)
				}
			}
			for _, binding := range origin.ParameterBindings {
				if binding.DrilldownInput == "" {
					target.ParameterBindings = append(target.ParameterBindings, binding)
				}
			}
		}
		add(origin, target, "log_context", "inherit", map[string]string{"event": "/event_id"})
	}
	var addTraceActions func(Target, SourceBinding)
	addTraceActions = func(origin Target, binding SourceBinding) {
		graph := Target{Language: queryengine.LanguageTrace, Signal: "traces", SourceBinding: &binding, SourceDefinition: Definition{Builder: &Builder{Operation: "trace_graph", Filters: []Filter{{Field: "traceId", Operator: "=", DrilldownInput: "trace_id"}, {Field: "sourceId", Operator: "=", DrilldownInput: "source_id"}, {Field: "resourceId", Operator: "=", DrilldownInput: "resource_id"}}}}}
		// Keep whole-dataset attribute constraints when the origin uses them.
		if b := origin.SourceDefinition.Builder; b != nil {
			for _, f := range b.Filters {
				if apmAttributeField(f.Field) {
					graph.SourceDefinition.Builder.Filters = append(graph.SourceDefinition.Builder.Filters, f)
				}
			}
			graph.ParameterBindings = attributeBindings(origin)
		}
		if origin.SourceDefinition.DSL != nil {
			derived, e := deriveTarget(origin, "queryTraceGraph", traceGraphProjection, map[string]string{"traceId": "trace_id", "sourceId": "source_id", "resourceId": "resource_id"})
			if e != nil {
				result.Issues = append(result.Issues, Issue{Path: origin.ID, Code: "DRILLDOWN_DERIVATION_UNSUPPORTED", Message: e.Error()})
				return
			}
			graph.SourceDefinition, graph.ParameterBindings = derived.SourceDefinition, derived.ParameterBindings
		}
		mapping := map[string]string{"trace_id": "/traceId", "source_id": "/sourceId", "resource_id": "/resourceId"}
		if t := add(origin, graph, "trace_details", "inherit", mapping); t != nil {
			full := *t
			fullTarget := add(*t, full, "full_trace", "authorized_trace", mapping)
			if logSource, ok := detailRoute(binding, "logs", routes); ok {
				logs := Target{Language: queryengine.LanguageKQL, Signal: "logs", SourceBinding: &logSource, SourceDefinition: Definition{Builder: &Builder{Operation: "records", Filters: []Filter{{Field: "trace_id", Operator: "=", DrilldownInput: "trace_id"}, {Field: "span_id", Operator: "=", DrilldownInput: "span_id"}, {Field: "resource_id", Operator: "=", DrilldownInput: "resource_id"}}}}}
				inputs := map[string]string{"trace_id": "/traceId", "span_id": "/spanId", "resource_id": "/resourceId"}
				// A shared multi-signal source (OTLP) has one installation identity.
				// Keep it when correlating, so reinstallations cannot mix logs.
				// Explicit cross-source routes have distinct identities and keep
				// their authored binding instead of equating unrelated source IDs.
				if logSource == binding {
					logs.SourceDefinition.Builder.Filters = append(logs.SourceDefinition.Builder.Filters, Filter{Field: "source_id", Operator: "=", DrilldownInput: "source_id"})
					inputs["source_id"] = "/sourceId"
				}
				if logTarget := add(*t, logs, "span_logs", "inherit", inputs); logTarget != nil {
					addLogContext(*logTarget, logSource)
				}
				if fullTarget != nil {
					if logTarget := add(*fullTarget, logs, "span_logs", "inherit", inputs); logTarget != nil {
						addLogContext(*logTarget, logSource)
					}
				}
			}
		}
	}
	for _, origin := range slices.Clone(panel.Targets) {
		inspect := origin
		inspect.Signal = panel.Signal
		binding := panel.SourceBinding
		inspect.SourceBinding = &binding
		add(origin, inspect, "inspect", "inherit", map[string]string{})
		compiled, e := CompileTarget(origin)
		if e != nil {
			return result, e
		}
		if strings.HasPrefix(compiled.ResultType, "apm_") {
			overrides := []string{"serviceName", "sourceId", "resourceId"}
			if compiled.ResultType == "apm_instances" {
				overrides = append(overrides, "serviceInstanceName")
			}
			if compiled.ResultType == "apm_endpoints" {
				overrides = append(overrides, "operationName")
			}
			inputs := map[string]string{"service": "/serviceName", "source": "/sourceId", "resource": "/resourceId"}
			rowFields := map[string]string{"serviceName": "service", "sourceId": "source", "resourceId": "resource"}
			if compiled.ResultType == "apm_instances" {
				inputs["instance"] = "/instanceId"
				rowFields["serviceInstanceName"] = "instance"
			}
			if compiled.ResultType == "apm_endpoints" {
				inputs["operation"] = "/operationName"
				rowFields["operationName"] = "operation"
			}
			target := Target{Language: queryengine.LanguageTrace, Signal: "traces", SourceBinding: &binding}
			if origin.SourceDefinition.Builder != nil {
				filters, bindings, e := traceBusinessFilters(origin, overrides)
				if e != nil {
					return result, e
				}
				for field, name := range rowFields {
					filters = append(filters, Filter{Field: field, Operator: "=", DrilldownInput: name})
				}
				target.SourceDefinition = Definition{Builder: &Builder{Operation: "list", Filters: filters, Limit: 100}}
				target.ParameterBindings = bindings
			} else {
				derived, e := deriveTarget(origin, "queryTraces", traceListProjection, rowFields)
				if e != nil {
					result.Issues = append(result.Issues, Issue{Path: origin.ID, Code: "DRILLDOWN_DERIVATION_UNSUPPORTED", Message: e.Error()})
					continue
				}
				target.SourceDefinition, target.ParameterBindings = derived.SourceDefinition, derived.ParameterBindings
			}

			if t := add(origin, target, "service_traces", "inherit", inputs); t != nil {
				if compiled.ResultType == "apm_red" {
					d := &panel.Drilldowns[len(panel.Drilldowns)-1]
					d.Inputs["bucket_start"] = "/timestamp"
					d.Inputs["bucket_seconds"] = "/intervalSeconds"
					d.TimeWindow = &DrilldownTimeWindow{Input: "bucket_start", DurationInput: "bucket_seconds"}
				}
				addTraceActions(*t, binding)
			}
		} else if panel.Signal == "traces" {
			if origin.SourceDefinition.Builder == nil && !skywalking.HasTraceIdentity(origin.SourceDefinition.DSL.Expression, origin.SourceDefinition.DSL.Operation) {
				result.Issues = append(result.Issues, Issue{Path: origin.ID, Code: "DRILLDOWN_IDENTITY_REQUIRED", Message: "Trace rows must project traceId, sourceId and resourceId"})
				continue
			}
			addTraceActions(origin, binding)
		} else if panel.Signal == "logs" && compiled.ResultType == "log_entries" {
			addLogContext(origin, binding)
			if traceSource, ok := detailRoute(binding, "traces", routes); ok {
				target := Target{Language: queryengine.LanguageTrace, Signal: "traces", SourceBinding: &traceSource, SourceDefinition: Definition{Builder: &Builder{Operation: "list", Filters: []Filter{{Field: "traceId", Operator: "=", DrilldownInput: "trace_id"}, {Field: "resourceId", Operator: "=", DrilldownInput: "resource_id"}}}}}
				inputs := map[string]string{"trace_id": "/trace_id", "resource_id": "/resource_id"}
				if traceSource == binding {
					target.SourceDefinition.Builder.Filters = append(target.SourceDefinition.Builder.Filters, Filter{Field: "sourceId", Operator: "=", DrilldownInput: "source_id"})
					inputs["source_id"] = "/source_id"
				}
				if t := add(origin, target, "log_traces", "inherit", inputs); t != nil {
					addTraceActions(*t, traceSource)
				}
			} else {
				result.Issues = append(result.Issues, Issue{Path: origin.ID, Code: "DRILLDOWN_SOURCE_REQUIRED", Message: "Choose a trace source to publish log-to-trace navigation"})
			}
		}
	}
	if report := Validate(result.Spec); !report.Valid {
		return result, fmt.Errorf("%w: %s", ErrInvalid, report.Issues[0].Message)
	}
	return result, nil
}

func detailRoute(origin SourceBinding, signal string, routes map[string]SourceBinding) (SourceBinding, bool) {
	if value, ok := routes[signal]; ok {
		return value, slices.Contains(sourceSignals[value.SourceType], signal) && value.CapabilityVersion == "v1"
	}
	return origin, slices.Contains(sourceSignals[origin.SourceType], signal)
}
func attributeBindings(target Target) []ParameterBinding {
	bindings := []ParameterBinding{}
	if target.SourceDefinition.Builder == nil {
		return bindings
	}
	for _, b := range target.ParameterBindings {
		if slices.ContainsFunc(target.SourceDefinition.Builder.Filters, func(f Filter) bool {
			return apmAttributeField(f.Field) && f.Variable == b.Variable && f.LocalParameter == b.LocalParameter && f.DrilldownInput == b.DrilldownInput
		}) {
			bindings = append(bindings, b)
		}
	}
	return bindings
}
func traceBusinessFilters(origin Target, overrides []string) ([]Filter, []ParameterBinding, error) {
	if origin.SourceDefinition.Builder == nil {
		return nil, nil, fmt.Errorf("native query constraints cannot be discarded by standard drilldown generation")
	}
	filters := []Filter{}
	for _, f := range origin.SourceDefinition.Builder.Filters {
		if !slices.Contains(overrides, f.Field) {
			filters = append(filters, f)
		}
	}
	bindings := []ParameterBinding{}
	for _, b := range origin.ParameterBindings {
		if slices.ContainsFunc(filters, func(f Filter) bool {
			return f.Variable == b.Variable && f.LocalParameter == b.LocalParameter && f.DrilldownInput == b.DrilldownInput
		}) {
			bindings = append(bindings, b)
		}
	}
	return filters, bindings, nil
}

func deriveTarget(origin Target, root, projection string, row map[string]string) (Target, error) {
	query := origin.SourceDefinition.DSL
	derived, err := skywalking.DeriveDrillQuery(query.Expression, query.Operation, root, projection, row)
	if err != nil {
		return Target{}, err
	}
	target := Target{SourceDefinition: Definition{DSL: &DSL{Expression: derived.Expression, Variables: map[string]any{}}}}
	for _, name := range derived.UsedVariables {
		if value, ok := query.Variables[name]; ok {
			target.SourceDefinition.DSL.Variables[name] = value
		}
	}
	for _, b := range origin.ParameterBindings {
		if slices.Contains(derived.UsedVariables, b.Parameter) {
			target.ParameterBindings = append(target.ParameterBindings, b)
		}
	}
	names := []string{}
	for input := range derived.RowVariables {
		names = append(names, input)
	}
	slices.Sort(names)
	for _, input := range names {
		target.ParameterBindings = append(target.ParameterBindings, ParameterBinding{Parameter: derived.RowVariables[input], DrilldownInput: input})
	}
	return target, nil
}
