package dashboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// scopedPreviewSpec keeps only selected panels and the transitive variables they
// actually consume. Invalid sibling panels cannot prevent editing this panel.
func scopedPreviewSpec(spec Spec, input ExecutionInput) (Spec, ExecutionInput, error) {
	if input.CandidatesOnly || len(input.PanelIDs) > 64 {
		return spec, input, ErrInvalid
	}
	selected := map[string]bool{}
	for _, id := range input.PanelIDs {
		if selected[id] || !slices.ContainsFunc(spec.Panels, func(p Panel) bool { return p.ID == id }) {
			return spec, input, ErrInvalid
		}
		selected[id] = true
	}
	globals := map[string]Variable{}
	duplicates := map[string]bool{}
	for _, v := range spec.Variables {
		if _, exists := globals[v.Name]; exists {
			duplicates[v.Name] = true
		}
		globals[v.Name] = v
	}
	for name := range input.Variables {
		if _, ok := globals[name]; !ok {
			return spec, input, ErrInvalid
		}
	}
	for id, values := range input.LocalValues {
		i := slices.IndexFunc(spec.Panels, func(p Panel) bool { return p.ID == id })
		if i < 0 {
			return spec, input, ErrInvalid
		}
		for name := range values {
			if !slices.ContainsFunc(spec.Panels[i].LocalFilters, func(f LocalFilter) bool { return f.ID == name }) {
				return spec, input, ErrInvalid
			}
		}
	}
	if len(selected) == 0 {
		return spec, input, nil
	}
	used := map[string]bool{}
	var visit func(string) error
	visit = func(name string) error {
		if duplicates[name] {
			return ErrInvalid
		}
		if used[name] {
			return nil
		}
		v, ok := globals[name]
		if !ok {
			return ErrInvalid
		}
		used[name] = true
		for _, f := range v.Query.Filters {
			if f.Variable != "" {
				if err := visit(f.Variable); err != nil {
					return err
				}
			}
		}
		for _, b := range v.Query.ParameterBindings {
			if b.Variable != "" {
				if err := visit(b.Variable); err != nil {
					return err
				}
			}
		}
		return nil
	}
	mark := func(filters []Filter, bindings []ParameterBinding) error {
		for _, f := range filters {
			if f.Variable != "" {
				if err := visit(f.Variable); err != nil {
					return err
				}
			}
		}
		for _, b := range bindings {
			if b.Variable != "" {
				if err := visit(b.Variable); err != nil {
					return err
				}
			}
		}
		return nil
	}
	panels := []Panel{}
	for _, p := range spec.Panels {
		if !selected[p.ID] {
			continue
		}
		panels = append(panels, p)
		for _, target := range append(slices.Clone(p.Targets), p.DetailQueryTargets...) {
			filters := []Filter{}
			if b := target.SourceDefinition.Builder; b != nil {
				filters = append(slices.Clone(b.Filters), b.ErrorFilters...)
			}
			if err := mark(filters, target.ParameterBindings); err != nil {
				return spec, input, err
			}
		}
		for _, f := range p.LocalFilters {
			if f.Query != nil {
				if err := mark(f.Query.Filters, f.Query.ParameterBindings); err != nil {
					return spec, input, err
				}
			}
		}
	}
	spec.Panels = panels
	spec.Variables = []Variable{}
	for _, v := range globals {
		if used[v.Name] {
			spec.Variables = append(spec.Variables, v)
		}
	}
	slices.SortFunc(spec.Variables, func(a, b Variable) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	vars := map[string]Selection{}
	for name, v := range input.Variables {
		if used[name] {
			vars[name] = v
		}
	}
	input.Variables = vars
	locals := map[string]map[string]Selection{}
	for id, v := range input.LocalValues {
		if selected[id] {
			locals[id] = v
		}
	}
	input.LocalValues = locals
	return spec, input, nil
}

// Pure presentation changes do not invalidate data or cause a query to run.
// Query definitions, dependencies, applicability and down-navigation do.
func draftDefinitionHash(spec Spec, panels []string) (string, error) {
	copy, _, err := scopedPreviewSpec(spec, ExecutionInput{PanelIDs: panels})
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(copy)
	if err != nil {
		return "", err
	}
	if err = json.Unmarshal(raw, &copy); err != nil {
		return "", err
	}
	copy.DefaultTimeRange = TimeRange{}
	copy.DefaultRefreshSeconds = 0
	copy.Layout = Grid{}
	for i := range copy.Variables {
		copy.Variables[i].Label = ""
		copy.Variables[i].Default = Selection{}
	}
	for i := range copy.Panels {
		p := &copy.Panels[i]
		p.Title = ""
		p.Description = ""
		p.Type = ""
		p.Layout = Rectangle{}
		p.Unit = ""
		p.Decimals = 0
		p.Legend = false
		p.Thresholds = nil
		p.Display = nil
		for j := range p.LocalFilters {
			p.LocalFilters[j].Label = ""
			p.LocalFilters[j].Default = Selection{}
		}
		for j := range p.Drilldowns {
			p.Drilldowns[j].Title = ""
		}
	}
	raw, err = json.Marshal(copy)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func (runtime Runtime) checkPreviewLifecycle(ctx context.Context, actor Actor, draft db.DashboardDraft) error {
	if draft.Status != "editing" {
		return ErrConflict
	}
	if err := checkFolder(ctx, runtime.Store.Queries, actor.EnterpriseID, draft.FolderID, true); err != nil {
		return err
	}
	if draft.DashboardID.Valid {
		item, _, err := (Service{Store: runtime.Store}).Get(ctx, actor, draft.DashboardID.UUID)
		if err != nil {
			return err
		}
		if item.Lifecycle != "active" {
			return ErrArchived
		}
	}
	return nil
}

func (runtime Runtime) freezeDraftSample(ctx context.Context, actor Actor, draft db.DashboardDraft, spec Spec, sample *DraftSample) error {
	// Recheck ownership, permissions and lifecycle after all query work.
	current, err := (Service{Store: runtime.Store}).Draft(ctx, actor, draft.ID)
	if err != nil {
		return err
	}
	if err = runtime.checkPreviewLifecycle(ctx, actor, current); err != nil {
		return err
	}
	var payload struct {
		Execution *Execution `json:"execution"`
	}
	if err = json.Unmarshal(sample.Sample, &payload); err != nil {
		return err
	}
	if payload.Execution == nil {
		return nil
	}
	ids := []string{}
	for _, p := range spec.Panels {
		ids = append(ids, p.ID)
	}
	hash, err := draftDefinitionHash(spec, ids)
	if err != nil {
		return err
	}
	exec := payload.Execution
	exec.ExecutionID = uuid.New()
	exec.DashboardID = draft.DashboardID.UUID
	exec.RevisionID = uuid.Nil
	identity, _ := json.Marshal([]any{draft.ID, draft.DraftVersion, hash, actor.SubjectID, actor.AuthorizationVersion, executionIdentity(*exec)})
	digest := sha256.Sum256(identity)
	exec.ExecutionHash = hex.EncodeToString(digest[:])
	if len(runtime.ContextKey) >= 32 {
		scope := executionIdentity(*exec)
		scope.VariableCandidates = nil
		scope.LocalCandidates = nil
		frozen := executionContext{Version: contextVersion, Enterprise: actor.EnterpriseID, Subject: actor.SubjectID, SubjectType: actor.SubjectType, ExpiresAt: time.Now().UTC().Add(15 * time.Minute), Scope: scope, Draft: &draftExecutionContext{draft.ID, draft.DraftVersion, hash}}
		token, e := runtime.signContext(frozen)
		if e != nil {
			return e
		}
		exec.ContextToken = token
		exec.ContextExpiresAt = &frozen.ExpiresAt
	}
	var result map[string]any
	if err = json.Unmarshal(sample.Sample, &result); err != nil {
		return err
	}
	result["execution"] = exec
	result["definition_hash"] = hash
	sample.Sample, err = json.Marshal(result)
	return err
}
