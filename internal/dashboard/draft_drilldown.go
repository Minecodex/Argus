package dashboard

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

type DraftDrilldownInput struct {
	ExpectedVersion int64 `json:"expected_version"`
	DrilldownInput
}
type DraftDrilldownExecution struct {
	DraftID      uuid.UUID          `json:"draft_id"`
	DraftVersion int64              `json:"draft_version"`
	Execution    DrilldownExecution `json:"execution"`
}

// Owned-draft navigation shares the exact row proof, source scopes, trace
// expansion, budget and cancellation implementation of published navigation.
func (runtime Runtime) DrilldownDraft(ctx context.Context, actor Actor, id uuid.UUID, input DraftDrilldownInput) (output DraftDrilldownExecution, err error) {
	service := Service{Store: runtime.Store}
	draft, err := service.Draft(ctx, actor, id)
	if err != nil {
		return output, err
	}
	if draft.DraftVersion != input.ExpectedVersion {
		return output, ErrConflict
	}
	if err = runtime.checkPreviewLifecycle(ctx, actor, draft); err != nil {
		return output, err
	}
	frozen, err := runtime.readContext(actor, input.ContextToken)
	if err != nil {
		return output, err
	}
	if frozen.Draft == nil || frozen.Draft.ID != id || frozen.Scope.RevisionID != uuid.Nil {
		return output, ErrDenied
	}
	ids := []string{}
	for _, p := range frozen.Scope.Panels {
		ids = append(ids, p.ID)
	}
	check := func(ctx context.Context) error {
		current, e := service.Draft(ctx, actor, id)
		if e != nil {
			return e
		}
		if e = runtime.checkPreviewLifecycle(ctx, actor, current); e != nil {
			return e
		}
		spec, e := DecodeSpec(current.Spec)
		if e != nil {
			return e
		}
		hash, e := draftDefinitionHash(spec, ids)
		if e != nil {
			return e
		}
		if hash != frozen.Draft.DefinitionHash {
			return ErrConflict
		}
		return nil
	}
	if err = check(ctx); err != nil {
		return output, err
	}
	spec, err := DecodeSpec(draft.Spec)
	if err != nil {
		return output, err
	}
	frozen.Draft.Version = draft.DraftVersion
	output = DraftDrilldownExecution{DraftID: id, DraftVersion: draft.DraftVersion}
	output.Execution, err = runtime.runDrilldown(ctx, actor, spec, frozen, input.DrilldownInput, check)
	result := Execution{DashboardID: output.Execution.DashboardID, ExecutionID: output.Execution.ParentExecutionID, ExecutionHash: output.Execution.ExecutionHash, From: output.Execution.From, To: output.Execution.To, Resources: output.Execution.Resources, Panels: []PanelExecution{{ID: input.PanelID, Status: output.Execution.Result.Status, Sources: output.Execution.Sources, Targets: []TargetExecution{output.Execution.Result}}}}
	err = errors.Join(err, runtime.recordExecution(ctx, actor, "dashboard.draft.drilldown.executed", draft.Name, result, ExecutionInput{}, err, map[string]any{"draft_id": id, "draft_version": draft.DraftVersion, "drilldown_id": input.DrilldownID}))
	return output, err
}
