package dashboard

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type GenerateDrilldownsInput struct {
	ExpectedVersion int64                    `json:"expected_version"`
	PanelID         string                   `json:"panel_id"`
	SignalSources   map[string]SourceBinding `json:"signal_sources"`
}
type GeneratedDraftDrilldowns struct {
	Draft  db.DashboardDraft
	Added  []string
	Issues []Issue
}

func (service Service) GenerateDrilldowns(ctx context.Context, actor Actor, id uuid.UUID, input GenerateDrilldownsInput) (GeneratedDraftDrilldowns, error) {
	var result GeneratedDraftDrilldowns
	draft, err := service.Draft(ctx, actor, id)
	if err != nil {
		return result, err
	}
	if draft.DraftVersion != input.ExpectedVersion || draft.Status != "editing" {
		return result, ErrConflict
	}
	spec, err := DecodeSpec(draft.Spec)
	if err != nil {
		return result, err
	}
	generated, err := GenerateStandardDrilldowns(spec, input.PanelID, input.SignalSources)
	if err != nil {
		return result, err
	}
	result.Added, result.Issues = generated.Added, generated.Issues
	if len(generated.Added) == 0 {
		result.Draft = draft
		return result, nil
	}
	encoded, err := json.Marshal(generated.Spec)
	if err != nil {
		return result, err
	}
	bindings := []Binding{}
	if err := json.Unmarshal(draft.ProposedBindings, &bindings); err != nil {
		return result, err
	}
	result.Draft, err = service.SaveDraft(ctx, actor, id, DraftInput{ExpectedVersion: input.ExpectedVersion, Name: draft.Name, Description: draft.Description, FolderID: draft.FolderID.UUID, Spec: encoded, ProposedBindings: bindings})
	return result, err
}
