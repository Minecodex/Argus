package dashboard

import (
	"context"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"slices"
)

type AnalysisCandidateInput struct {
	DashboardID uuid.UUID `json:"dashboard_id"`
	ContextRef  uuid.UUID `json:"context_ref"`
	Name        string    `json:"name"`
	PanelID     string    `json:"panel_id,omitempty"`
}

// A candidate lookup executes a published filter and its dependencies only.
// It cannot accept a free-form catalog query, execute panels, or change the
// persistent conditions. Actual reconciliation is committed with query enqueue.
func (runtime Runtime) AnalysisCandidates(ctx context.Context, actor Actor, conversation uuid.UUID, input AnalysisCandidateInput) (map[string]any, error) {
	q := runtime.Store.Queries
	if err := analysisScope(ctx, q, actor, conversation, input.DashboardID, true); err != nil {
		return nil, err
	}
	row, err := q.GetDashboardAnalysisContext(ctx, db.GetDashboardAnalysisContextParams{ID: input.ContextRef, EnterpriseID: actor.EnterpriseID, ConversationID: conversation, RunID: actor.RunID, OwnerUserID: actor.SubjectID})
	if err != nil || row.DashboardID != input.DashboardID {
		return nil, ErrContextExpired
	}
	current, err := q.GetDashboardRunParameters(ctx, db.GetDashboardRunParametersParams{RunID: actor.RunID, DashboardID: input.DashboardID, EnterpriseID: actor.EnterpriseID, OwnerUserID: actor.SubjectID})
	if err != nil || current.Version != row.ConditionVersion {
		return nil, ErrContextExpired
	}
	_, revision, err := (Service{Store: runtime.Store}).Get(ctx, actor, input.DashboardID)
	if err != nil {
		return nil, err
	}
	if revision.ID != row.RevisionID {
		return nil, ErrContextExpired
	}
	spec, err := DecodeSpec(revision.Spec)
	if err != nil {
		return nil, err
	}
	if err = runtimeSupported(spec); err != nil {
		return nil, err
	}
	view, err := analysisView(row)
	if err != nil {
		return nil, err
	}
	reduced, err := candidateSpec(spec, input.PanelID, input.Name)
	if err != nil {
		return nil, err
	}
	execution := Execution{}
	execution.From, execution.To, err = executionTime(spec, view.Parameters)
	if err != nil {
		return nil, err
	}
	execution.Resources, err = resolveResources(ctx, q, actor, view.Parameters.ResourceIDs)
	if err != nil {
		return nil, err
	}
	if err = prepareParameters(spec, view.Parameters, &execution); err != nil {
		return nil, err
	}
	if input.PanelID != "" {
		execution.Panels = []PanelExecution{{ID: input.PanelID}}
	}
	work, err := runtime.freezeCandidates(ctx, actor, reduced, execution)
	if err != nil {
		return nil, err
	}
	if err = runtime.resolveCandidates(ctx, actor, spec, work, &execution, newExecutionBudget(len(work))); err != nil {
		return nil, err
	}
	state := execution.VariableCandidates[input.Name]
	effective := execution.Variables[input.Name]
	if input.PanelID != "" {
		state = execution.LocalCandidates[input.PanelID][input.Name]
		effective = execution.LocalValues[input.PanelID][input.Name]
	}
	// Values too large to be valid selections cannot be sent into model context.
	// Mark incomplete rather than treating a shortened list as a complete set.
	values := []string{}
	truncated := false
	valueBytes := 0
	for _, value := range state.Values {
		if len(value) > 4096 || valueBytes+len(value) > 64<<10 {
			truncated = true
			continue
		}
		valueBytes += len(value)
		values = append(values, value)
	}
	return map[string]any{"context_ref": row.ID, "revision_id": row.RevisionID, "name": input.Name, "panel_id": input.PanelID, "status": state.Status, "code": state.Code, "values": values, "complete": state.Complete && !truncated, "values_truncated": truncated, "selected_exists": state.SelectedExists, "effective_selection": effective, "would_reset": state.Reset, "source_count": len(state.Sources), "preview_only": true}, nil
}
func candidateSpec(spec Spec, panelID, name string) (Spec, error) {
	global := map[string]Variable{}
	for _, v := range spec.Variables {
		global[v.Name] = v
	}
	local := map[string]LocalFilter{}
	panel := Panel{}
	if panelID != "" {
		i := slices.IndexFunc(spec.Panels, func(p Panel) bool { return p.ID == panelID })
		if i < 0 {
			return spec, ErrInvalid
		}
		panel = spec.Panels[i]
		for _, f := range panel.LocalFilters {
			local[f.ID] = f
		}
	}
	globals, locals := map[string]bool{}, map[string]bool{}
	var visitGlobal func(string) error
	visitGlobal = func(name string) error {
		if globals[name] {
			return nil
		}
		v, ok := global[name]
		if !ok {
			return ErrInvalid
		}
		globals[name] = true
		for _, f := range v.Query.Filters {
			if f.Variable != "" {
				if err := visitGlobal(f.Variable); err != nil {
					return err
				}
			}
		}
		return nil
	}
	var visitLocal func(string) error
	visitLocal = func(name string) error {
		if locals[name] {
			return nil
		}
		v, ok := local[name]
		if !ok || v.Query == nil {
			return ErrInvalid
		}
		locals[name] = true
		for _, f := range v.Query.Filters {
			if f.Variable != "" {
				if err := visitGlobal(f.Variable); err != nil {
					return err
				}
			}
			if f.LocalParameter != "" {
				if err := visitLocal(f.LocalParameter); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if name == "" {
		return spec, ErrInvalid
	}
	if panelID == "" {
		if err := visitGlobal(name); err != nil {
			return spec, err
		}
	} else {
		if err := visitLocal(name); err != nil {
			return spec, err
		}
	}
	reduced := spec
	reduced.Variables = []Variable{}
	reduced.Panels = []Panel{}
	for _, v := range spec.Variables {
		if globals[v.Name] {
			reduced.Variables = append(reduced.Variables, v)
		}
	}
	if panelID != "" {
		panel.LocalFilters = []LocalFilter{}
		for _, f := range spec.Panels[slices.IndexFunc(spec.Panels, func(p Panel) bool { return p.ID == panelID })].LocalFilters {
			if locals[f.ID] {
				panel.LocalFilters = append(panel.LocalFilters, f)
			}
		}
		reduced.Panels = []Panel{panel}
	}
	return reduced, nil
}
