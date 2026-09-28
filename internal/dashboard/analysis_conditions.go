package dashboard

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboardparams"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"reflect"
	"slices"
	"strings"
	"time"
)

type AnalysisResolveInput struct {
	DashboardID     uuid.UUID                  `json:"dashboard_id"`
	ExpectedVersion int64                      `json:"expected_version"`
	Changes         dashboardparams.Patch      `json:"changes"`
	Evidence        []dashboardparams.Evidence `json:"evidence"`
}

func conditionPath(parts ...string) string {
	for i, p := range parts {
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(p, "~", "~0"), "/", "~1")
	}
	return "/" + strings.Join(parts, "/")
}
func conditionContracts(spec Spec) map[string]string {
	result := map[string]string{}
	queries := map[string]string{}
	for _, panel := range spec.Panels {
		for _, target := range append(slices.Clone(panel.Targets), panel.DetailQueryTargets...) {
			queries[panel.ID+"\x00"+target.ID] = analysisQueryContract(target)
		}
	}
	for _, v := range spec.Variables {
		consumers := []any{}
		for _, p := range spec.Panels {
			for _, t := range append(slices.Clone(p.Targets), p.DetailQueryTargets...) {
				for _, b := range analysisTargetBindings(t) {
					if b.Variable == v.Name {
						consumers = append(consumers, []any{p.ID, p.Signal, p.SourceBinding, t.ID, t.Signal, t.SourceBinding, b, queries[p.ID+"\x00"+t.ID]})
					}
				}
			}
		}
		data, _ := json.Marshal([]any{v.ID, v.Name, v.Multiple, v.IncludeAll, v.Query, consumers})
		result[conditionPath("variables", v.Name)] = queryHash(data)
	}
	// Dependency fingerprints ensure a changed preceding variable cannot silently
	// change the meaning of an inherited dependent selection.
	base := map[string]string{}
	for k, v := range result {
		base[k] = v
	}
	var dependencies func(string, map[string]bool) map[string]string
	dependencies = func(name string, seen map[string]bool) map[string]string {
		deps := map[string]string{}
		if seen[name] {
			return deps
		}
		seen[name] = true
		for _, v := range spec.Variables {
			if v.Name == name {
				for _, f := range v.Query.Filters {
					if f.Variable != "" {
						path := conditionPath("variables", f.Variable)
						deps[path] = base[path]
						for k, h := range dependencies(f.Variable, seen) {
							deps[k] = h
						}
					}
				}
			}
		}
		return deps
	}
	for _, v := range spec.Variables {
		path := conditionPath("variables", v.Name)
		raw, _ := json.Marshal([]any{base[path], dependencies(v.Name, map[string]bool{})})
		result[path] = queryHash(raw)
	}

	for _, p := range spec.Panels {
		localBase := map[string]string{}
		localDefinitions := map[string]LocalFilter{}
		for _, local := range p.LocalFilters {
			consumers := []any{}
			for _, t := range append(slices.Clone(p.Targets), p.DetailQueryTargets...) {
				for _, b := range analysisTargetBindings(t) {
					if b.LocalParameter == local.ID {
						consumers = append(consumers, []any{t.ID, t.Signal, t.SourceBinding, b, queries[p.ID+"\x00"+t.ID]})
					}
				}
			}
			data, _ := json.Marshal([]any{p.Signal, p.SourceBinding, p.ApplicableResourceTypes, local.ID, local.Kind, local.Multiple, local.Required, local.Query, consumers})
			localBase[local.ID] = queryHash(data)
			localDefinitions[local.ID] = local
		}
		var localDependencies func(string, map[string]bool) map[string]string
		localDependencies = func(id string, seen map[string]bool) map[string]string {
			deps := map[string]string{}
			if seen[id] {
				return deps
			}
			seen[id] = true
			if query := localDefinitions[id].Query; query != nil {
				for _, f := range query.Filters {
					if f.Variable != "" {
						path := conditionPath("variables", f.Variable)
						deps[path] = result[path]
					}
					if f.LocalParameter != "" {
						path := conditionPath("local_values", p.ID, f.LocalParameter)
						deps[path] = localBase[f.LocalParameter]
						for k, v := range localDependencies(f.LocalParameter, seen) {
							deps[k] = v
						}
					}
				}
			}
			return deps
		}
		for _, local := range p.LocalFilters {
			data, _ := json.Marshal([]any{localBase[local.ID], localDependencies(local.ID, map[string]bool{})})
			result[conditionPath("local_values", p.ID, local.ID)] = queryHash(data)
		}
	}

	return result
}
func inheritedConflicts(state dashboardparams.State, contracts map[string]string) []string {
	result := []string{}
	for name := range state.Overrides.Variables {
		path := conditionPath("variables", name)
		if contracts[path] == "" || contracts[path] != state.Contracts[path] {
			result = append(result, path)
		}
	}
	for panel, locals := range state.Overrides.LocalValues {
		for name := range locals {
			path := conditionPath("local_values", panel, name)
			if contracts[path] == "" || contracts[path] != state.Contracts[path] {
				result = append(result, path)
			}
		}
	}
	slices.Sort(result)
	return result
}
func decodeConditions(raw []byte) (dashboardparams.State, error) {
	result := dashboardparams.Empty()
	if len(raw) > 64<<10 || json.Unmarshal(raw, &result) != nil || result.Schema != "argus.dashboard_conditions/v1" || result.Overrides.Variables == nil || result.Overrides.LocalValues == nil || result.Contracts == nil || result.Evidence == nil {
		return result, ErrInvalid
	}
	return result, nil
}
func mergeConditions(old dashboardparams.State, patch dashboardparams.Patch, evidence []dashboardparams.Evidence, eventID uuid.UUID, message string, contracts map[string]string) (dashboardparams.State, error) {
	raw, _ := json.Marshal(old)
	state, err := decodeConditions(raw)
	if err != nil {
		return state, err
	}
	paths := []string{}
	if patch.ResetAll {
		if patch.Time != nil || patch.Resources != nil || patch.ResetTime || patch.ResetResources || len(patch.Variables) > 0 || len(patch.LocalValues) > 0 {
			return state, ErrInvalid
		}
		state = dashboardparams.Empty()
		paths = append(paths, "/")
	}
	if patch.Time != nil && patch.ResetTime || patch.Resources != nil && patch.ResetResources {
		return state, ErrInvalid
	}
	if patch.Time != nil || patch.ResetTime {
		state.Overrides.Time = patch.Time
		paths = append(paths, "/time")
	}
	if patch.Resources != nil || patch.ResetResources {
		state.Overrides.Resources = patch.Resources
		paths = append(paths, "/resources")
	}
	for name, value := range patch.Variables {
		path := conditionPath("variables", name)
		if contracts[path] == "" && (value != nil || state.Contracts[path] == "") {
			return state, ErrInvalid
		}
		paths = append(paths, path)
		if value == nil {
			delete(state.Overrides.Variables, name)
			delete(state.Contracts, path)
		} else {
			state.Overrides.Variables[name] = *value
			state.Contracts[path] = contracts[path]
		}
	}
	for panel, locals := range patch.LocalValues {
		for name, value := range locals {
			path := conditionPath("local_values", panel, name)
			if contracts[path] == "" && (value != nil || state.Contracts[path] == "") {
				return state, ErrInvalid
			}
			paths = append(paths, path)
			if value == nil {
				delete(state.Overrides.LocalValues[panel], name)
				delete(state.Contracts, path)
			} else {
				if state.Overrides.LocalValues[panel] == nil {
					state.Overrides.LocalValues[panel] = map[string]Selection{}
				}
				state.Overrides.LocalValues[panel][name] = *value
				state.Contracts[path] = contracts[path]
			}
			if len(state.Overrides.LocalValues[panel]) == 0 {
				delete(state.Overrides.LocalValues, panel)
			}
		}
	}
	if len(paths) > 128 || len(evidence) != len(paths) {
		return state, toolruntime.Error{Kind: "DASHBOARD_CONDITION_EVIDENCE_REQUIRED"}
	}
	seen := map[string]bool{}
	for _, ref := range evidence {
		if !slices.Contains(paths, ref.Path) || seen[ref.Path] || strings.TrimSpace(ref.Quote) == "" || len(ref.Quote) > 2048 || !strings.Contains(message, ref.Quote) {
			return state, toolruntime.Error{Kind: "DASHBOARD_CONDITION_EVIDENCE_REQUIRED"}
		}
		seen[ref.Path] = true
		// This records an auditable model interpretation of a current user message,
		// not a claim that lexical containment proves semantic correctness.
		state.Evidence[ref.Path] = dashboardparams.Reference{EventID: eventID, Quote: ref.Quote, Origin: "message_interpretation"}
	}
	if conflicts := inheritedConflicts(state, contracts); len(conflicts) > 0 {
		return state, toolruntime.Error{Kind: "DASHBOARD_CONDITIONS_INCOMPATIBLE", Message: "Published parameter meanings changed; ask the user to replace or reset the listed overrides.", Details: map[string]any{"paths": strings.Join(conflicts, ",")}}
	}
	raw, _ = json.Marshal(state)
	if len(raw) > 64<<10 {
		return state, ErrInvalid
	}
	return state, nil
}
func conditionParameters(spec Spec, state dashboardparams.State, at time.Time) (ExecutionInput, error) {
	input := ExecutionInput{Variables: state.Overrides.Variables, LocalValues: state.Overrides.LocalValues, ResourceIDs: []uuid.UUID{}, PanelIDs: []string{}}
	chosen := spec.DefaultTimeRange
	if state.Overrides.Time != nil {
		chosen = *state.Overrides.Time
	}
	switch chosen.Kind {
	case "relative":
		if chosen.Seconds < 1 || chosen.Seconds > 604800 || chosen.From != nil || chosen.To != nil {
			return input, ErrInvalid
		}
		from := at.Add(-time.Duration(chosen.Seconds) * time.Second)
		input.From, input.To = &from, &at
	case "absolute":
		if chosen.From == nil || chosen.To == nil || chosen.Seconds != 0 || !chosen.To.After(*chosen.From) || chosen.To.Sub(*chosen.From) > 7*24*time.Hour {
			return input, ErrInvalid
		}
		input.From, input.To = chosen.From, chosen.To
	default:
		return input, ErrInvalid
	}
	if r := state.Overrides.Resources; r != nil {
		if r.All && len(r.IDs) > 0 || !r.All && len(r.IDs) == 0 || len(r.IDs) > 1000 {
			return input, ErrInvalid
		}
		seen := map[uuid.UUID]bool{}
		for _, id := range r.IDs {
			if id == uuid.Nil || seen[id] {
				return input, ErrInvalid
			}
			seen[id] = true
		}
		input.ResourceIDs = append([]uuid.UUID{}, r.IDs...)
	}
	probe := Execution{}
	if err := prepareParameters(spec, input, &probe); err != nil {
		return input, fmt.Errorf("%w: conditions: %v", ErrInvalid, err)
	}
	return input, nil
}
func reconcileExplicit(state dashboardparams.State, execution Execution) (dashboardparams.State, bool) {
	changed := false
	for name, old := range state.Overrides.Variables {
		if value, ok := execution.Variables[name]; ok && !reflect.DeepEqual(value, old) {
			state.Overrides.Variables[name] = value
			path := conditionPath("variables", name)
			ref := state.Evidence[path]
			ref.Origin = "candidate_reset"
			state.Evidence[path] = ref
			changed = true
		}
	}
	for panel, locals := range state.Overrides.LocalValues {
		for name, old := range locals {
			if value, ok := execution.LocalValues[panel][name]; ok && !reflect.DeepEqual(value, old) {
				locals[name] = value
				path := conditionPath("local_values", panel, name)
				ref := state.Evidence[path]
				ref.Origin = "candidate_reset"
				state.Evidence[path] = ref
				changed = true
			}
		}
	}
	return state, changed
}
