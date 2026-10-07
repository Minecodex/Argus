package dashboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
)

type DrilldownInput struct {
	ContextToken              string            `json:"context_token"`
	PanelID                   string            `json:"panel_id"`
	DrilldownID               string            `json:"drilldown_id"`
	Values                    map[string]string `json:"values"`
	ExpandAuthorizedResources bool              `json:"expand_authorized_resources"`
}
type DrilldownExecution struct {
	DashboardID       uuid.UUID        `json:"dashboard_id"`
	RevisionID        uuid.UUID        `json:"revision_id"`
	ParentExecutionID uuid.UUID        `json:"parent_execution_id"`
	PanelID           string           `json:"panel_id"`
	DrilldownID       string           `json:"drilldown_id"`
	ScopePolicy       string           `json:"scope_policy"`
	From              time.Time        `json:"from"`
	To                time.Time        `json:"to"`
	Resources         []ResourceScope  `json:"resources"`
	Sources           []ResolvedSource `json:"sources"`
	Result            TargetExecution  `json:"result"`
	ContextToken      string           `json:"context_token"`
	ContextExpiresAt  time.Time        `json:"context_expires_at"`
	ExecutionHash     string           `json:"execution_hash"`
}

func (runtime Runtime) Drilldown(ctx context.Context, actor Actor, id uuid.UUID, input DrilldownInput) (output DrilldownExecution, err error) {
	name := ""
	defer func() {
		conditions, _ := json.Marshal(struct {
			Values map[string]string
			Expand bool
		}{input.Values, input.ExpandAuthorizedResources})
		result := Execution{DashboardID: id, RevisionID: output.RevisionID, ExecutionID: output.ParentExecutionID, ExecutionHash: output.ExecutionHash, From: output.From, To: output.To, Resources: output.Resources, Panels: []PanelExecution{{ID: input.PanelID, Status: output.Result.Status, Sources: output.Sources, Targets: []TargetExecution{output.Result}}}, Partial: output.Result.Status == "partial" || output.Result.Status == "error" || output.Result.Status == "skipped_budget"}
		err = errors.Join(err, runtime.recordExecution(ctx, actor, "dashboard.drilldown.executed", name, result, ExecutionInput{}, err, map[string]any{"drilldown_id": input.DrilldownID, "scope_policy": output.ScopePolicy, "parameters_hash": queryHash(conditions)}))
	}()
	if runtime.Backend == nil || runtime.Store == nil {
		return output, ErrUnavailable
	}
	frozen, err := runtime.readContext(actor, input.ContextToken)
	if err != nil {
		return output, err
	}
	if frozen.Draft != nil || frozen.Scope.DashboardID != id {
		return output, ErrDenied
	}
	service := Service{Store: runtime.Store}
	item, _, err := service.Get(ctx, actor, id)
	if err != nil {
		return output, err
	}
	if item.Lifecycle != "active" {
		return output, ErrArchived
	}
	revision, err := runtime.Store.Queries.GetDashboardRevision(ctx, db.GetDashboardRevisionParams{ID: frozen.Scope.RevisionID, EnterpriseID: actor.EnterpriseID})
	if err != nil || revision.DashboardID != id {
		return output, ErrDenied
	}
	name = revision.Name
	spec, err := DecodeSpec(revision.Spec)
	if err != nil {
		return output, err
	}
	return runtime.runDrilldown(ctx, actor, spec, frozen, input, func(ctx context.Context) error {
		current, _, err := service.Get(ctx, actor, id)
		if err != nil {
			return err
		}
		if current.Lifecycle != "active" {
			return ErrArchived
		}
		return nil
	})
}

func (runtime Runtime) runDrilldown(ctx context.Context, actor Actor, spec Spec, frozen executionContext, input DrilldownInput, recheck func(context.Context) error) (output DrilldownExecution, err error) {
	runtime.cacheDashboard, runtime.cacheRevision = frozen.Scope.DashboardID, frozen.Scope.RevisionID
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	prepared, err := runtime.prepareDrilldown(ctx, actor, spec, frozen, input, func(ctx context.Context, drill Drilldown, origin Target, base queryScope, originInputs map[string]string, ledger *executionBudget) (any, error) {
		panel := spec.Panels[slices.IndexFunc(spec.Panels, func(p Panel) bool { return p.ID == input.PanelID })]
		proof, e := runtime.executeTarget(ctx, actor, spec, targetPanel(panel, origin), origin, frozen.Scope.Variables, frozen.Scope.LocalValues[panel.ID], selectionValues(originInputs), base, ledger)
		if e != nil {
			return nil, e
		}
		if e = targetFailure(proof); e != nil {
			return nil, e
		}
		return proof.Data, nil
	})
	if err != nil {
		return output, err
	}
	panel, drill, detail, scope, detailSources, ledger := prepared.Panel, prepared.Drill, prepared.Detail, prepared.Scope, prepared.DetailSources, prepared.Budget
	variables, locals := frozen.Scope.Variables, frozen.Scope.LocalValues[panel.ID]
	result, err := runtime.executeTarget(ctx, actor, spec, targetPanel(panel, detail), detail, variables, locals, selectionValues(input.Values), scope, ledger)
	if err != nil {
		return output, err
	}
	if err := recheck(ctx); err != nil {
		return output, err
	}
	if err := runtime.checkQueryScope(ctx, actor, scope); err != nil {
		return output, err
	}
	frozen.Depth++
	frozen.Leaf = &detailContext{PanelID: panel.ID, TargetID: detail.ID, Inputs: input.Values, From: scope.From, To: scope.To, Resources: scope.Resources, Sources: scope.Sources, DetailSources: detailSources}
	token, err := runtime.signContext(frozen)
	if err != nil {
		return output, err
	}
	output = DrilldownExecution{DashboardID: frozen.Scope.DashboardID, RevisionID: frozen.Scope.RevisionID, ParentExecutionID: frozen.Scope.ExecutionID, PanelID: panel.ID, DrilldownID: drill.ID, ScopePolicy: drill.ScopePolicy, From: scope.From, To: scope.To, Resources: scope.Resources, Sources: scope.Sources, Result: result, ContextToken: token, ContextExpiresAt: frozen.ExpiresAt}
	identity, _ := json.Marshal(struct {
		Context       executionContext
		Drill         string
		Authorization int64
	}{frozen, drill.ID, actor.AuthorizationVersion})
	hash := sha256.Sum256(identity)
	output.ExecutionHash = hex.EncodeToString(hash[:])
	return output, nil
}

func drillOrigin(spec Spec, panel Panel, drill Drilldown, frozen executionContext) (Target, queryScope, map[string]string, map[string][]ResolvedSource, error) {
	origin, ok := findTarget(panel, drill.OriginQueryRef)
	if !ok {
		return origin, queryScope{}, nil, nil, ErrInvalid
	}
	if leaf := frozen.Leaf; leaf != nil {
		if leaf.PanelID != panel.ID || leaf.TargetID != origin.ID {
			return origin, queryScope{}, nil, nil, ErrInvalid
		}
		return origin, queryScope{leaf.From, leaf.To, leaf.Resources, leaf.Sources}, leaf.Inputs, leaf.DetailSources, nil
	}
	if !slices.ContainsFunc(panel.Targets, func(t Target) bool { return t.ID == origin.ID }) {
		return origin, queryScope{}, nil, nil, ErrInvalid
	}
	index := slices.IndexFunc(frozen.Scope.Panels, func(p PanelExecution) bool { return p.ID == panel.ID })
	if index < 0 {
		return origin, queryScope{}, nil, nil, ErrInvalid
	}
	entry := frozen.Scope.Panels[index]
	resources := []ResourceScope{}
	for _, r := range frozen.Scope.Resources {
		if slices.Contains(panel.ApplicableResourceTypes, r.Type) {
			resources = append(resources, r)
		}
	}
	return origin, queryScope{frozen.Scope.From, frozen.Scope.To, resources, entry.Sources}, nil, entry.DetailSources, nil
}

func findTarget(panel Panel, id string) (Target, bool) {
	for _, t := range panel.Targets {
		if t.ID == id {
			return t, true
		}
	}
	for _, t := range panel.DetailQueryTargets {
		if t.ID == id {
			return t, true
		}
	}
	return Target{}, false
}
func targetPanel(panel Panel, target Target) Panel {
	if target.SourceBinding != nil {
		panel.SourceBinding = *target.SourceBinding
		panel.Signal = target.Signal
	}
	return panel
}

func (runtime Runtime) freezeDetailScopes(ctx context.Context, actor Actor, panel Panel, resources []ResourceScope) (map[string][]ResolvedSource, error) {
	ids := []uuid.UUID{}
	for _, r := range resources {
		ids = append(ids, r.ID)
	}
	result := map[string][]ResolvedSource{}
	count := 0
	for _, t := range panel.DetailQueryTargets {
		if t.SourceBinding == nil {
			return nil, ErrInvalid
		}
		sources, err := resolveSources(ctx, runtime.Store.Queries, actor, *t.SourceBinding, t.Signal, ids)
		if err != nil {
			return nil, err
		}
		count += len(sources)
		if count > 10000 {
			return nil, ErrInvalid
		}
		result[t.ID] = sources
	}
	return result, nil
}

func (runtime Runtime) checkExpansionAnchor(ctx context.Context, actor Actor, spec Spec, panel Panel, detail Target, variables, locals map[string]Selection, selected map[string]string, base queryScope, ledger *executionBudget) (skywalking.GraphIdentity, error) {
	var empty skywalking.GraphIdentity
	bound, err := bindTargetInputs(spec, targetPanel(panel, detail), detail, variables, locals, selectionValues(selected))
	if err != nil {
		return empty, err
	}
	compiled, err := compileConcreteTarget(bound, false)
	if err != nil {
		return empty, ErrInvalid
	}
	anchor, err := skywalking.GraphIdentityFromQuery(compiled.Query.Expression, compiled.Query.Operation, compiled.Query.Variables)
	if err != nil {
		return empty, ErrInvalid
	}
	if anchor.TraceID != selected["trace_id"] || anchor.SourceID != selected["source_id"] || anchor.ResourceID != selected["resource_id"] {
		return empty, ErrInvalid
	}
	if !slices.ContainsFunc(base.Sources, func(s ResolvedSource) bool {
		return s.ID.String() == anchor.SourceID && s.ResourceID.String() == anchor.ResourceID
	}) {
		return empty, ErrDenied
	}
	guard := Target{ID: "trace_anchor", Language: queryengine.LanguageTrace, SourceDefinition: Definition{DSL: &DSL{Expression: "query {queryTrace(traceId:" + quoteGraphQL(anchor.TraceID) + ",sourceId:" + quoteGraphQL(anchor.SourceID) + ",resourceId:" + quoteGraphQL(anchor.ResourceID) + "){traceId}}"}}}
	proof, err := runtime.executeTarget(ctx, actor, EmptySpec(), targetPanel(panel, detail), guard, nil, nil, nil, base, ledger)
	if err != nil {
		return empty, err
	}
	if err = targetFailure(proof); err != nil {
		return empty, err
	}
	if proof.Status == "no_data" {
		return empty, ErrSelectionStale
	}
	return anchor, nil
}
