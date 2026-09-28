package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

type CandidateState struct {
	Status         string           `json:"status"`
	Values         []string         `json:"values"`
	Complete       bool             `json:"complete"`
	SelectedExists map[string]bool  `json:"selected_exists"`
	Reset          bool             `json:"reset"`
	Sources        []ResolvedSource `json:"sources"`
	Code           string           `json:"code,omitempty"`
}

type candidateWork struct {
	name      string
	panel     Panel
	query     CandidateQuery
	resources []uuid.UUID
	sources   []ResolvedSource
}

func prepareParameters(spec Spec, input ExecutionInput, result *Execution) error {
	result.Variables, result.LocalValues = map[string]Selection{}, map[string]map[string]Selection{}
	result.VariableCandidates, result.LocalCandidates = map[string]CandidateState{}, map[string]map[string]CandidateState{}
	for _, v := range spec.Variables {
		value, ok := input.Variables[v.Name]
		if !ok {
			value = v.Default
		}
		if err := validateSelection(value, v.Multiple, true); err != nil {
			return fmt.Errorf("%w: %s: %v", ErrInvalid, v.Name, err)
		}
		result.Variables[v.Name] = value
	}
	for name := range input.Variables {
		if _, ok := result.Variables[name]; !ok {
			return ErrInvalid
		}
	}
	for _, p := range spec.Panels {
		locals := map[string]Selection{}
		for _, f := range p.LocalFilters {
			value, ok := input.LocalValues[p.ID][f.ID]
			if !ok {
				value = f.Default
			}
			if err := validateLocalSelection(f, value); err != nil {
				return fmt.Errorf("%w: %s.%s: %v", ErrInvalid, p.ID, f.ID, err)
			}
			locals[f.ID] = value
		}
		for name := range input.LocalValues[p.ID] {
			if _, ok := locals[name]; !ok {
				return ErrInvalid
			}
		}
		result.LocalValues[p.ID], result.LocalCandidates[p.ID] = locals, map[string]CandidateState{}
	}
	for id := range input.LocalValues {
		if _, ok := result.LocalValues[id]; !ok {
			return ErrInvalid
		}
	}
	return nil
}

// freezeCandidates resolves all candidate source sets before any data query.
func (runtime Runtime) freezeCandidates(ctx context.Context, actor Actor, spec Spec, result Execution) ([]candidateWork, error) {
	names, variables := []string{}, map[string]Variable{}
	for _, v := range spec.Variables {
		names = append(names, v.Name)
		variables[v.Name] = v
	}
	order, err := dependencyOrder(names, func(n string) []string {
		deps := []string{}
		for _, f := range variables[n].Query.Filters {
			if f.Variable != "" {
				deps = append(deps, f.Variable)
			}
		}
		return deps
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	work := []candidateWork{}
	for _, name := range order {
		work = append(work, candidateWork{name: name, query: variables[name].Query})
	}
	for _, panel := range spec.Panels {
		if !slices.ContainsFunc(result.Panels, func(p PanelExecution) bool { return p.ID == panel.ID }) {
			continue
		}
		order, err := localOrder(panel)
		if err != nil {
			return nil, err
		}
		for _, name := range order {
			local := panel.LocalFilters[slices.IndexFunc(panel.LocalFilters, func(f LocalFilter) bool { return f.ID == name })]
			if local.Query != nil {
				work = append(work, candidateWork{name: name, panel: panel, query: *local.Query})
			}
		}
	}
	total := 0
	for _, p := range result.Panels {
		total += len(p.Sources)
		for _, sources := range p.DetailSources {
			total += len(sources)
		}
	}
	for i := range work {
		item := &work[i]
		for _, r := range result.Resources {
			if item.panel.ID == "" || slices.Contains(item.panel.ApplicableResourceTypes, r.Type) {
				item.resources = append(item.resources, r.ID)
			}
		}
		item.sources, err = resolveSources(ctx, runtime.Store.Queries, actor, item.query.SourceBinding, item.query.Signal, item.resources)
		if err != nil {
			return nil, err
		}
		total += len(item.sources)
		if total > 10000 {
			return nil, telemetry.ErrQueryBudget
		}
	}
	return work, nil
}

func (runtime Runtime) resolveCandidates(ctx context.Context, actor Actor, spec Spec, work []candidateWork, result *Execution, budget *executionBudget) error {
	runtime = runtime.metered(actor)
	for _, item := range work {
		values, states := result.Variables, result.VariableCandidates
		if item.panel.ID != "" {
			values, states = result.LocalValues[item.panel.ID], result.LocalCandidates[item.panel.ID]
		}
		selected := values[item.name]
		state := CandidateState{Status: "success", Values: []string{}, SelectedExists: map[string]bool{}, Sources: item.sources}
		allocation, err := budget.next(ctx)
		if err != nil {
			state.Status, state.Code = "skipped_budget", "QUERY_BUDGET_EXCEEDED"
			states[item.name] = state
			result.Partial = true
			continue
		}
		filters, err := bindCandidate(spec, item.panel, item.query, result.Variables, result.LocalValues[item.panel.ID])
		if err != nil {
			return err
		}
		data := telemetry.DataCatalogResult{Complete: true, Values: []string{}, Membership: map[string]bool{}}
		if len(item.resources) > 0 && len(item.sources) > 0 {
			keys := []string{}
			for _, s := range item.sources {
				keys = append(keys, s.Key())
			}
			data, err = runtime.Backend.DiscoverData(ctx, telemetry.DataCatalogRequest{SubjectID: actor.SubjectID, SubjectType: actor.SubjectType, EnterpriseID: actor.EnterpriseID, AuthorizationVersion: actor.AuthorizationVersion, ResourceIDs: item.resources, SourceKeys: keys, Signal: item.query.Signal, Kind: "values", Metric: item.query.Metric, Field: item.query.Field, Filters: filters, SelectedValues: selected.Values, From: result.From, To: result.To, Limit: min(200, max(1, allocation.MaxRows-len(selected.Values))), Budget: allocation})
		}
		if err != nil {
			budget.failed(allocation)
			state.Status, state.Code = "unavailable", "CATALOG_UNAVAILABLE"
			if errors.Is(err, telemetry.ErrQueryInvalid) {
				return fmt.Errorf("%w: candidate query invalid", ErrInvalid)
			}
			if errors.Is(err, telemetry.ErrQueryBudget) || errors.Is(err, queryengine.ErrBudget) {
				state.Status, state.Code = "skipped_budget", "QUERY_BUDGET_EXCEEDED"
			}
			result.Partial = true
		} else {
			budget.record(data.Meta, data)
			budget.rows -= len(data.Membership)
			state.Values, state.Complete, state.SelectedExists = data.Values, data.Complete, data.Membership
			if !data.Complete {
				state.Status = "partial"
			}
			values[item.name], state.Reset = ReconcileSelection(selected, data.Values, data.Complete, data.Membership)
		}
		states[item.name] = state
	}
	return nil
}

// One ledger covers candidate discovery, metadata checks and display queries.
// Failed requests consume their reservation because scanned work is unknown.
type executionBudget struct {
	scan, bytes            int64
	rows, samples, pending int
}

func newExecutionBudget(count int) *executionBudget {
	return &executionBudget{scan: telemetry.DefaultMaxScanBytes, bytes: 8 << 20, rows: telemetry.DefaultMaxRows, samples: telemetry.DefaultMaxSamples, pending: count}
}
func (b *executionBudget) next(ctx context.Context) (queryengine.Budget, error) {
	count := max(b.pending, 1)
	b.pending--
	value := queryengine.Budget{MaxScanBytes: b.scan / int64(count), MaxResultBytes: b.bytes / int64(count), MaxRows: b.rows / count, MaxSamples: b.samples / count, MaxSeries: telemetry.DefaultMaxSeries, Timeout: telemetry.DefaultTimeout}
	if value.MaxScanBytes <= 0 || value.MaxResultBytes <= 0 || value.MaxRows <= 0 || value.MaxSamples <= 0 || ctx.Err() != nil {
		return value, telemetry.ErrQueryBudget
	}
	return value, nil
}
func (b *executionBudget) failed(v queryengine.Budget) {
	b.scan -= v.MaxScanBytes
	b.bytes -= v.MaxResultBytes
	b.rows -= v.MaxRows
	b.samples -= v.MaxSamples
}
func (b *executionBudget) record(meta queryengine.QueryMeta, data any) {
	encoded, _ := json.Marshal(data)
	b.scan -= meta.ScannedBytes
	b.bytes -= int64(len(encoded))
	b.rows -= int(meta.ReturnedRows)
	b.samples -= int(meta.LoadedSamples)
}
