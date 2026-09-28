package dashboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboardparams"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

type RuntimeBackend interface {
	ExecuteEngineQuery(context.Context, queryengine.Request) (queryengine.Result, error)
	telemetry.DataCatalogBackend
}
type Runtime struct {
	Store                         *postgres.Store
	Backend                       RuntimeBackend
	ContextKey                    []byte
	cacheDashboard, cacheRevision uuid.UUID
}

var errNoResources = errors.New("no authorized resources are available")

type ExecutionInput = dashboardparams.Input
type TargetExecution struct {
	ID          string                `json:"id"`
	Status      string                `json:"status"`
	Code        string                `json:"code,omitempty"`
	QueryHash   string                `json:"query_hash"`
	ResultType  string                `json:"result_type"`
	StepSeconds int                   `json:"step_seconds,omitempty"`
	Data        any                   `json:"data"`
	Meta        queryengine.QueryMeta `json:"meta"`
}
type PanelExecution struct {
	ID            string                      `json:"id"`
	Status        string                      `json:"status"`
	Sources       []ResolvedSource            `json:"sources"`
	Targets       []TargetExecution           `json:"targets"`
	DetailSources map[string][]ResolvedSource `json:"detail_sources"`
}
type Execution struct {
	ExecutionID        uuid.UUID                            `json:"execution_id"`
	ContextToken       string                               `json:"context_token,omitempty"`
	ContextExpiresAt   *time.Time                           `json:"context_expires_at,omitempty"`
	DashboardID        uuid.UUID                            `json:"dashboard_id"`
	RevisionID         uuid.UUID                            `json:"revision_id"`
	From               time.Time                            `json:"from"`
	To                 time.Time                            `json:"to"`
	Resources          []ResourceScope                      `json:"resources"`
	Panels             []PanelExecution                     `json:"panels"`
	Partial            bool                                 `json:"partial"`
	ExecutionHash      string                               `json:"execution_hash"`
	Variables          map[string]Selection                 `json:"variables"`
	LocalValues        map[string]map[string]Selection      `json:"local_values"`
	VariableCandidates map[string]CandidateState            `json:"variable_candidates"`
	LocalCandidates    map[string]map[string]CandidateState `json:"local_candidates"`
}

type DraftSample struct {
	DraftID      uuid.UUID        `json:"draft_id"`
	DraftVersion int64            `json:"draft_version"`
	Validation   ValidationReport `json:"validation"`
	Sample       json.RawMessage  `json:"sample"`
}

func (runtime Runtime) SampleDraft(ctx context.Context, actor Actor, id uuid.UUID, version int64) (DraftSample, error) {
	draft, err := (Service{Store: runtime.Store}).Draft(ctx, actor, id)
	if err != nil {
		return DraftSample{}, err
	}
	if draft.Status != "editing" || draft.DraftVersion != version {
		return DraftSample{}, ErrConflict
	}
	spec, err := DecodeSpec(draft.Spec)
	if err != nil {
		return DraftSample{}, err
	}
	validation, sample, err := runtime.Verify(ctx, actor, spec)
	result := DraftSample{DraftID: id, DraftVersion: version, Validation: validation, Sample: sample}
	// Invalid configuration is the result of a validation request, not a lost
	// transport error. Publication still treats the same Verify error as a gate.
	if errors.Is(err, ErrInvalid) && !validation.Valid {
		if len(result.Sample) == 0 {
			result.Sample = json.RawMessage(`{"status":"not_executed","reason":"configuration_invalid"}`)
		}
		return result, nil
	}
	return result, err
}

func (runtime Runtime) Execute(ctx context.Context, actor Actor, id uuid.UUID, input ExecutionInput) (result Execution, err error) {
	name := ""
	defer func() {
		result.DashboardID = id
		err = errors.Join(err, runtime.recordExecution(ctx, actor, "dashboard.query.executed", name, result, input, err))
	}()
	service := Service{Store: runtime.Store}
	item, revision, err := service.Get(ctx, actor, id)
	if err != nil {
		return Execution{}, err
	}
	if item.Lifecycle != "active" {
		return Execution{}, ErrArchived
	}
	name = revision.Name
	spec, err := DecodeSpec(revision.Spec)
	if err != nil {
		return Execution{}, err
	}
	runtime.cacheDashboard, runtime.cacheRevision = id, revision.ID
	result, err = runtime.executeSpec(ctx, actor, spec, input)
	if errors.Is(err, errNoResources) {
		err = ErrDenied
	}
	if err == nil {
		current, _, checkErr := service.Get(ctx, actor, id)
		if checkErr != nil {
			return Execution{}, checkErr
		}
		if current.Lifecycle != "active" {
			return Execution{}, ErrArchived
		}
		ids := []uuid.UUID{}
		for _, resource := range result.Resources {
			ids = append(ids, resource.ID)
		}
		if _, checkErr := resolveResources(ctx, runtime.Store.Queries, actor, ids); checkErr != nil {
			return Execution{}, checkErr
		}
	}
	result.DashboardID = id
	result.RevisionID = revision.ID
	identity, _ := json.Marshal(struct {
		Dashboard, Revision  uuid.UUID
		Input                ExecutionInput
		Scope                Execution
		Subject              uuid.UUID
		AuthorizationVersion int64
		Compiler             string
	}{id, revision.ID, input, executionIdentity(result), actor.SubjectID, actor.AuthorizationVersion, CompilerVersion})
	hash := sha256.Sum256(identity)
	result.ExecutionHash = hex.EncodeToString(hash[:])
	result.ExecutionID = uuid.New()
	if err == nil && len(runtime.ContextKey) >= 32 {
		err = runtime.attachExecutionContext(actor, &result)
	}
	return result, err
}

func executionIdentity(value Execution) Execution {
	copy := value
	copy.Panels = append([]PanelExecution{}, value.Panels...)
	for i := range copy.Panels {
		copy.Panels[i].Targets = nil
		copy.Panels[i].Status = ""
	}
	return copy
}

func executionTime(spec Spec, input ExecutionInput) (time.Time, time.Time, error) {
	if (input.From == nil) != (input.To == nil) {
		return time.Time{}, time.Time{}, ErrInvalid
	}
	if input.From != nil {
		if !input.To.After(*input.From) || input.To.Sub(*input.From) > 7*24*time.Hour {
			return time.Time{}, time.Time{}, ErrInvalid
		}
		return input.From.UTC(), input.To.UTC(), nil
	}
	if spec.DefaultTimeRange.Kind == "absolute" {
		return executionTime(spec, ExecutionInput{From: spec.DefaultTimeRange.From, To: spec.DefaultTimeRange.To})
	}
	if spec.DefaultTimeRange.Seconds < 1 || spec.DefaultTimeRange.Seconds > 7*86400 {
		return time.Time{}, time.Time{}, ErrInvalid
	}
	to := time.Now().UTC()
	return to.Add(-time.Duration(spec.DefaultTimeRange.Seconds) * time.Second), to, nil
}

func runtimeSupported(spec Spec) error {
	return nil
}

func (runtime Runtime) executeSpec(ctx context.Context, actor Actor, spec Spec, input ExecutionInput) (Execution, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	result, budget, err := runtime.prepareSpec(ctx, actor, spec, input)
	if err != nil {
		return result, err
	}
	return runtime.executePrepared(ctx, actor, spec, result, budget)
}

func (runtime Runtime) executePrepared(ctx context.Context, actor Actor, spec Spec, result Execution, budget *executionBudget) (Execution, error) {
	runtime = runtime.metered(actor)
	if result.DashboardID != uuid.Nil && result.RevisionID != uuid.Nil {
		runtime.cacheDashboard, runtime.cacheRevision = result.DashboardID, result.RevisionID
	}
	for i := range result.Panels {
		entry := &result.Panels[i]
		panel := spec.Panels[slices.IndexFunc(spec.Panels, func(p Panel) bool { return p.ID == entry.ID })]
		resources := []ResourceScope{}
		for _, r := range result.Resources {
			if slices.Contains(panel.ApplicableResourceTypes, r.Type) {
				resources = append(resources, r)
			}
		}
		for _, target := range panel.Targets {
			if entry.Status == "not_applicable" {
				_, _ = budget.next(ctx)
				entry.Targets = append(entry.Targets, TargetExecution{ID: target.ID, Status: "not_applicable"})
				continue
			}
			targetResult, e := runtime.executeTarget(ctx, actor, spec, panel, target, result.Variables, result.LocalValues[panel.ID], nil, queryScope{result.From, result.To, resources, entry.Sources}, budget)
			if e != nil {
				return result, e
			}
			if targetResult.Status == "partial" || targetResult.Status == "error" || targetResult.Status == "skipped_budget" {
				result.Partial = true
			}
			entry.Targets = append(entry.Targets, targetResult)
			if err := afterQueryTarget(ctx); err != nil {
				return result, err
			}
		}

		entry.Status = panelStatus(entry.Targets)
	}
	if err := authorize(ctx, runtime.Store.Queries, actor, "telemetry.dashboard.read"); err != nil {
		return Execution{}, err
	}
	ids := []uuid.UUID{}
	for _, r := range result.Resources {
		ids = append(ids, r.ID)
	}
	if _, err := resolveResources(ctx, runtime.Store.Queries, actor, ids); err != nil {
		return Execution{}, err
	}
	return result, nil
}

func (runtime Runtime) prepareSpec(ctx context.Context, actor Actor, spec Spec, input ExecutionInput) (Execution, *executionBudget, error) {
	runtime = runtime.metered(actor)
	result := Execution{Panels: []PanelExecution{}}
	if runtime.Backend == nil {
		return result, nil, ErrUnavailable
	}
	if err := runtimeSupported(spec); err != nil {
		return result, nil, err
	}
	var err error
	result.From, result.To, err = executionTime(spec, input)
	if err != nil {
		return result, nil, err
	}
	result.Resources, err = resolveResources(ctx, runtime.Store.Queries, actor, input.ResourceIDs)
	if err != nil {
		return result, nil, err
	}
	if len(result.Resources) == 0 && len(spec.Panels) > 0 {
		return result, nil, errNoResources
	}
	for _, id := range input.PanelIDs {
		if !slices.ContainsFunc(spec.Panels, func(panel Panel) bool { return panel.ID == id }) {
			return result, nil, ErrInvalid
		}
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := prepareParameters(spec, input, &result); err != nil {
		return result, nil, err
	}
	count := 0
	for _, panel := range spec.Panels {
		if !input.CandidatesOnly && (len(input.PanelIDs) == 0 || slices.Contains(input.PanelIDs, panel.ID)) {
			count += len(panel.Targets)
		}
	}
	// Freeze every source set before executing the first query.
	for _, panel := range spec.Panels {
		if input.CandidatesOnly || (len(input.PanelIDs) > 0 && !slices.Contains(input.PanelIDs, panel.ID)) {
			continue
		}
		ids := []uuid.UUID{}
		for _, resource := range result.Resources {
			if slices.Contains(panel.ApplicableResourceTypes, resource.Type) {
				ids = append(ids, resource.ID)
			}
		}
		entry := PanelExecution{ID: panel.ID, Status: "success", Targets: []TargetExecution{}, Sources: []ResolvedSource{}, DetailSources: map[string][]ResolvedSource{}}
		if len(ids) == 0 {
			entry.Status = "not_applicable"
		} else {
			entry.Sources, err = resolveSources(ctx, runtime.Store.Queries, actor, panel.SourceBinding, panel.Signal, ids)
			if err != nil {
				return result, nil, err
			}
			if len(entry.Sources) == 0 {
				entry.Status = "no_data"
			}
		}
		result.Panels = append(result.Panels, entry)
		for _, detail := range panel.DetailQueryTargets {
			if detail.SourceBinding == nil {
				return result, nil, ErrInvalid
			}
			sources, e := resolveSources(ctx, runtime.Store.Queries, actor, *detail.SourceBinding, detail.Signal, ids)
			if e != nil {
				return result, nil, e
			}
			entry.DetailSources[detail.ID] = sources
		}
		total := 0
		for _, p := range result.Panels {
			total += len(p.Sources)
			for _, sources := range p.DetailSources {
				total += len(sources)
			}
		}
		if total > 10000 {
			return result, nil, telemetry.ErrQueryBudget
		}
	}
	work, err := runtime.freezeCandidates(ctx, actor, spec, result)
	if err != nil {
		return result, nil, err
	}
	budget := newExecutionBudget(count + len(work))
	if err := runtime.resolveCandidates(ctx, actor, spec, work, &result, budget); err != nil {
		return result, nil, err
	}
	return result, budget, nil
}

func emptyResult(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
		return v.Len() == 0
	}
	if values, ok := value.(map[string]any); ok {
		for key, item := range values {
			if key == "total" {
				if n, ok := item.(float64); ok && n > 0 {
					return false
				}
				if number, ok := item.(json.Number); ok {
					if n, err := number.Float64(); err == nil && n > 0 {
						return false
					}
				}
				continue
			}
			if !emptyResult(item) {
				return false
			}
		}
		return true
	}
	return false
}
func panelStatus(targets []TargetExecution) string {
	status := "no_data"
	for _, target := range targets {
		if target.Status == "error" || target.Status == "partial" || target.Status == "skipped_budget" {
			return "partial"
		}
		if target.Status == "success" {
			status = "success"
		}
		if target.Status == "not_applicable" {
			status = "not_applicable"
		}
	}
	return status
}

func (runtime Runtime) Verify(ctx context.Context, actor Actor, spec Spec) (ValidationReport, json.RawMessage, error) {
	report := Validate(spec)
	if !report.Valid {
		return report, nil, ErrInvalid
	}
	if err := runtimeSupported(spec); err != nil {
		return report, nil, err
	}
	for _, panel := range spec.Panels {
		if panel.SourceBinding.CapabilityVersion != "v1" || !slices.Contains(sourceSignals[panel.SourceBinding.SourceType], panel.Signal) {
			report.Valid = false
			report.Issues = append(report.Issues, Issue{Path: "panels." + panel.ID, Code: "SOURCE_UNSUPPORTED", Message: "source capability is not supported"})
		}
		for _, target := range panel.Targets {
			compiled, err := CompileTarget(target)
			if err != nil {
				return report, nil, ErrInvalid
			}
			if !compatibleResult(panel.Type, panel.Signal, compiled.ResultType) {
				report.Valid = false
				report.Issues = append(report.Issues, Issue{Path: "panels." + panel.ID, Code: "QUERY_TYPE_ERROR", Message: "query result is incompatible with chart"})
			}
		}
	}
	if !report.Valid {
		return report, nil, ErrInvalid
	}
	sample, err := runtime.executeSpec(ctx, actor, spec, ExecutionInput{})
	if errors.Is(err, errNoResources) {
		return report, json.RawMessage(`{"status":"unavailable","reason":"no_authorized_resources"}`), nil
	}
	if err != nil {
		return report, nil, err
	}
	for name, state := range sample.VariableCandidates {
		if state.Code == "QUERY_BUDGET_EXCEEDED" {
			report.Valid = false
			report.Issues = append(report.Issues, Issue{Path: "variables." + name, Code: state.Code, Message: "candidate query exceeded execution budget"})
		}
	}
	for id, states := range sample.LocalCandidates {
		for name, state := range states {
			if state.Code == "QUERY_BUDGET_EXCEEDED" {
				report.Valid = false
				report.Issues = append(report.Issues, Issue{Path: "panels." + id + ".local_filters." + name, Code: state.Code, Message: "candidate query exceeded execution budget"})
			}
		}
	}
	for _, panel := range sample.Panels {
		for _, target := range panel.Targets {
			if target.Meta.Partial {
				report.Valid = false
				report.Issues = append(report.Issues, Issue{Path: "panels." + panel.ID + ".targets." + target.ID, Code: "QUERY_BUDGET_EXCEEDED", Message: "sample execution exceeded its result budget"})
			}
			if target.Code == "QUERY_INVALID" || target.Code == "QUERY_BUDGET_EXCEEDED" || target.Code == "QUERY_TYPE_ERROR" {
				report.Valid = false
				report.Issues = append(report.Issues, Issue{Path: "panels." + panel.ID + ".targets." + target.ID, Code: target.Code, Message: "query failed a hard execution gate"})
			}
		}
	}
	state := "no_data"
	for _, panel := range sample.Panels {
		if panel.Status == "success" {
			state = "success"
		}
	}
	if sample.Partial {
		state = "unavailable"
	} else {
		for _, panel := range sample.Panels {
			for _, target := range panel.Targets {
				for _, warning := range target.Meta.Warnings {
					if strings.HasPrefix(warning, "APM_") {
						state = "warning"
					}
				}
			}
		}
	}
	details := map[string]string{}
	for _, panel := range spec.Panels {
		for _, target := range panel.DetailQueryTargets {
			details[panel.ID+"/"+target.ID] = "configuration_valid_sample_requires_selection"
		}
	}
	if len(details) > 0 && state == "success" {
		state = "warning"
	}
	encoded, _ := json.Marshal(map[string]any{"status": state, "execution": sample, "details_validation": details})
	if !report.Valid {
		return report, encoded, ErrInvalid
	}
	return report, encoded, nil
}

func compatibleResult(chart, signal, result string) bool {
	if strings.HasPrefix(chart, "apm_") {
		return chart == result
	}
	if signal == "traces" {
		return result == "traces" || chart == "trace_detail" && result == "trace_graph"
	}
	if signal == "logs" {
		if chart == "logs" {
			return result == "log_entries"
		}
		if chart == "timeseries" {
			return result == "timeseries"
		}
		return result == "table" || result == "timeseries"
	}
	if chart == "timeseries" || chart == "state_timeline" || chart == "scatter" {
		return result == "matrix"
	}
	return result == "vector" || result == "scalar" || result == "matrix"
}

func emptyTypedResult(kind string, data any) bool {
	if !strings.HasPrefix(kind, "apm_") {
		return emptyResult(data)
	}
	object, ok := data.(map[string]any)
	if !ok {
		return true
	}
	for _, value := range object {
		field := "rows"
		if kind == "apm_topology" {
			field = "nodes"
		}
		if row, ok := value.(map[string]any); ok && !emptyResult(row[field]) {
			return false
		}
	}
	return true
}
