package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/modelmetrics"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

type interactiveKey struct{}

// Interactive is set only by the explicit HTTP directory/view/execution routes.
// Agent, export and Workspace paths never acquire this capability.
func Interactive(ctx context.Context) context.Context {
	return context.WithValue(ctx, interactiveKey{}, true)
}
func interactive(ctx context.Context) bool { return ctx.Value(interactiveKey{}) == true }

func modelGovernanceTemplate() TemplateDefinition {
	spec := EmptySpec()
	spec.DefaultRefreshSeconds = 0
	spec.Variables = []Variable{
		{ID: "model", Name: "model", Label: "模型", Multiple: true, IncludeAll: true, Default: Selection{All: true, Values: []string{}}, Query: CandidateQuery{Signal: "metrics", SourceBinding: SourceBinding{SourceType: modelmetrics.SourceType, CapabilityVersion: "v1"}, Field: "model_id"}},
		{ID: "month", Name: "month", Label: "月份", Multiple: false, IncludeAll: false, Default: Selection{Values: []string{}}, Query: CandidateQuery{Signal: "metrics", SourceBinding: SourceBinding{SourceType: modelmetrics.SourceType, CapabilityVersion: "v1"}, Field: "month"}},
	}
	for i, p := range modelmetrics.Panels() {
		mode := "instant"
		if p.Trend {
			mode = "range"
		}
		spec.Panels = append(spec.Panels, Panel{ID: p.ID, Title: p.Title, Type: p.Chart, Signal: "metrics", AuthoringMode: "dsl", ApplicableResourceTypes: []string{"internal_model"}, SourceBinding: SourceBinding{SourceType: modelmetrics.SourceType, CapabilityVersion: "v1"}, Layout: Rectangle{X: i % 2 * 6, Y: i / 2 * 36, W: 6, H: 36, MinW: 2, MinH: 20}, Unit: p.Unit, Decimals: 2, Legend: true, LocalFilters: []LocalFilter{}, DetailQueryTargets: []Target{}, Drilldowns: []Drilldown{}, Thresholds: []Threshold{}, Targets: []Target{{ID: "a", Language: queryengine.LanguagePromQL, SourceDefinition: Definition{DSL: &DSL{Expression: p.Expression}}, QueryMode: mode, RangeStepPolicy: StepPolicy{Kind: "auto", TargetPoints: 31, MinStepSeconds: 1}, ParameterBindings: []ParameterBinding{}}}})
	}
	for i := range spec.Variables {
		spec.Variables[i].Query.Filters = []Filter{}
		spec.Variables[i].Query.ParameterBindings = []ParameterBinding{}
	}
	for i := range spec.Panels {
		if spec.Panels[i].Type == "timeseries" {
			spec.Panels[i].Display = &DisplayOptions{DrawStyle: "bar"}
		}
		if slices.Contains([]string{"requests", "tokens", "cached", "errors", "cancelled", "compaction", "requests_trend", "tokens_trend", "latency", "p95"}, spec.Panels[i].ID) {
			spec.Panels[i].Decimals = 0
		}
		if slices.Contains([]string{"amount", "amount_trend", "models", "departments", "users"}, spec.Panels[i].ID) {
			spec.Panels[i].Decimals = 6
		}
		if i < 9 {
			spec.Panels[i].Layout = Rectangle{X: i % 3 * 4, Y: i / 3 * 20, W: 4, H: 20, MinW: 2, MinH: 16}
		} else {
			spec.Panels[i].Layout = Rectangle{X: (i - 9) % 2 * 6, Y: 60 + (i-9)/2*36, W: 6, H: 36, MinW: 2, MinH: 20}
		}
	}
	return TemplateDefinition{Key: modelmetrics.TemplateKey, Version: modelmetrics.TemplateVersion, Name: "模型治理", Description: "内部模型调用、用量、结算金额与当前额度；自然月统计，按企业和部门授权。", IntegrationKeys: []string{}, Spec: spec, References: []TemplateReference{}}
}

func (s Service) listInteractive(ctx context.Context, actor Actor) ([]db.Dashboard, error) {
	result := []db.Dashboard{}
	canRead := authorize(ctx, s.Store.Queries, actor, "telemetry.dashboard.read") == nil
	if canRead {
		rows, err := s.listResources(ctx, actor)
		if err != nil {
			return nil, err
		}
		result = rows
	}
	if _, err := modelmetrics.Authorize(ctx, s.Store, actor); err == nil {
		row, err := s.ApplyManagedTemplate(ctx, actor.EnterpriseID, modelmetrics.TemplateKey)
		if err != nil {
			return nil, err
		}
		result = append(result, row)
	} else if !canRead {
		return nil, errors.Join(ErrDenied, err)
	}
	return result, nil
}
func (s Service) getModelGovernance(ctx context.Context, actor Actor) (db.Dashboard, db.DashboardRevision, error) {
	if !interactive(ctx) {
		return db.Dashboard{}, db.DashboardRevision{}, ErrDenied
	}
	if _, err := modelmetrics.Authorize(ctx, s.Store, actor); err != nil {
		return db.Dashboard{}, db.DashboardRevision{}, errors.Join(ErrDenied, err)
	}
	item, err := s.ApplyManagedTemplate(ctx, actor.EnterpriseID, modelmetrics.TemplateKey)
	if err != nil {
		return item, db.DashboardRevision{}, err
	}
	revision, err := s.Store.Queries.GetDashboardRevision(ctx, db.GetDashboardRevisionParams{ID: item.ActiveRevisionID.UUID, EnterpriseID: actor.EnterpriseID})
	return item, revision, err
}

func privateSpec(spec Spec) bool {
	for _, v := range spec.Variables {
		if v.Query.SourceBinding.SourceType == modelmetrics.SourceType {
			return true
		}
	}
	for _, p := range spec.Panels {
		for _, target := range append(slices.Clone(p.Targets), p.DetailQueryTargets...) {
			if target.SourceBinding != nil && target.SourceBinding.SourceType == modelmetrics.SourceType {
				return true
			}
		}
	}
	return slices.ContainsFunc(spec.Panels, func(p Panel) bool {
		return p.SourceBinding.SourceType == modelmetrics.SourceType || slices.Contains(p.ApplicableResourceTypes, "internal_model")
	})
}
func internalTemplateSpec(spec Spec) bool {
	a, _ := json.Marshal(spec)
	b, _ := json.Marshal(modelGovernanceTemplate().Spec)
	return string(a) == string(b)
}

func (runtime Runtime) executeModelGovernance(ctx context.Context, actor Actor, item db.Dashboard, revision db.DashboardRevision, input ExecutionInput) (Execution, error) {
	backend, ok := runtime.Backend.(modelmetrics.Backend)
	if !ok || !interactive(ctx) {
		return Execution{}, ErrDenied
	}
	spec, err := DecodeSpec(revision.Spec)
	if err != nil || !internalTemplateSpec(spec) {
		return Execution{}, ErrInvalid
	}
	if input.From != nil || input.To != nil || len(input.ResourceIDs) > 0 || input.ResourceMode != "" || len(input.ResourceTypes) > 0 || input.ApplicationMode != "" || len(input.ApplicationIDs) > 0 || len(input.LocalValues) > 0 {
		return Execution{}, ErrInvalid
	}
	r := modelmetrics.Request{Actor: actor, TemplateHash: modelmetrics.ContractHash(), ModelMode: "all", PanelIDs: input.PanelIDs, CandidatesOnly: input.CandidatesOnly}
	for name, value := range input.Variables {
		switch name {
		case "model":
			if value.All {
				if len(value.Values) > 0 {
					return Execution{}, ErrInvalid
				}
			} else {
				r.ModelMode = "explicit"
				for _, text := range value.Values {
					id, e := uuid.Parse(text)
					if e != nil {
						return Execution{}, ErrInvalid
					}
					r.ModelIDs = append(r.ModelIDs, id)
				}
			}
		case "month":
			if value.All || len(value.Values) > 1 {
				return Execution{}, ErrInvalid
			}
			if len(value.Values) == 1 {
				r.Month = value.Values[0]
			}
		default:
			return Execution{}, ErrInvalid
		}
	}
	response, err := backend.QueryModelGovernance(ctx, r)
	if err != nil {
		switch {
		case errors.Is(err, modelmetrics.ErrDenied):
			return Execution{}, ErrDenied
		case errors.Is(err, modelmetrics.ErrInvalid):
			return Execution{}, ErrInvalid
		default:
			return Execution{}, err
		}
	}
	if _, err = modelmetrics.Authorize(ctx, runtime.Store, actor); err != nil {
		return Execution{}, ErrDenied
	}
	result := Execution{ExecutionID: uuid.New(), DashboardID: item.ID, RevisionID: revision.ID, From: response.From, To: response.To, Resources: []ResourceScope{}, Panels: []PanelExecution{}, Partial: response.Pending > 0, ScopeSignature: response.ScopeHash, ExecutionHash: response.ScopeHash, ScopeVersions: ScopeVersions{Resources: map[string]int64{}, Relations: map[string]int64{}}, Variables: map[string]Selection{"model": {All: r.ModelMode == "all", Values: []string{}}, "month": {Values: []string{response.From.In(mustLocation(response.Timezone)).Format("2006-01")}}}, LocalValues: map[string]map[string]Selection{}, VariableCandidates: map[string]CandidateState{}, LocalCandidates: map[string]map[string]CandidateState{}, ModelGovernance: &response}
	for _, id := range r.ModelIDs {
		v := result.Variables["model"]
		v.Values = append(v.Values, id.String())
		result.Variables["model"] = v
	}
	for _, p := range response.Panels {
		state := "success"
		code := ""
		if p.Unavailable {
			state = "error"
			code = "MODEL_GOVERNANCE_METRIC_UNAVAILABLE"
		}
		result.Panels = append(result.Panels, PanelExecution{ID: p.ID, Status: state, Sources: []ResolvedSource{}, DetailSources: map[string][]ResolvedSource{}, Targets: []TargetExecution{{ID: "a", Status: state, Code: code, QueryHash: modelmetrics.ContractHash(), ResultType: p.Result.ResultType, Data: p.Result.Data, Meta: p.Result.Meta}}})
	}
	// Statistics live in regular PanelExecution; metadata must not duplicate them.
	result.ModelGovernance.Panels = nil
	return result, nil
}
func mustLocation(name string) *time.Location {
	location, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return location
}
