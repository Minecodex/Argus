package dashboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func preparationFixture(t *testing.T) DraftInput {
	t.Helper()
	spec := EmptySpec()
	spec.Panels = []Panel{validPanel()}
	raw, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	return DraftInput{Name: "Prepared", Description: "Keep this", Spec: raw, ProposedBindings: []Binding{}}
}

func TestDraftPreparationIsExplicitAtomicAndUsesSharedCompilers(t *testing.T) {
	input := preparationFixture(t)
	initial := bytes.Clone(input.Spec)
	unchanged, err := prepareDraftInput(input)
	if err != nil || !bytes.Equal(unchanged.Spec, initial) {
		t.Fatal("omitted preparation changed the supplied spec", err)
	}
	spec, _ := DecodeSpec(input.Spec)
	id := spec.Panels[0].ID
	input.Preparation = &DraftPreparation{
		Conversions: []DraftPanelConversion{{PanelID: id, Mode: "dsl"}},
		Drilldowns:  []DraftPanelDrilldowns{{PanelID: id, SignalSources: map[string]SourceBinding{}}},
	}
	prepared, err := prepareDraftInput(input)
	if err != nil {
		t.Fatal(err)
	}
	result, _ := DecodeSpec(prepared.Spec)
	if result.Panels[0].AuthoringMode != "dsl" || len(result.Panels[0].Drilldowns) == 0 || prepared.Description != input.Description || !bytes.Equal(input.Spec, initial) {
		t.Fatal("preparation failed to retain metadata, generate drilldowns, or preserve its caller's input")
	}
	for _, invalid := range []*DraftPreparation{
		{Conversions: []DraftPanelConversion{{PanelID: "missing", Mode: "dsl"}}},
		{Conversions: []DraftPanelConversion{{PanelID: id, Mode: "unsupported"}}},
		{Conversions: []DraftPanelConversion{{PanelID: id, Mode: "dsl"}, {PanelID: id, Mode: "builder"}}},
		{Conversions: []DraftPanelConversion{{PanelID: id, Mode: "dsl"}}, Drilldowns: []DraftPanelDrilldowns{{PanelID: "missing"}}},
	} {
		input.Preparation = invalid
		failed, err := prepareDraftInput(input)
		if !errors.Is(err, ErrInvalid) || !bytes.Equal(failed.Spec, initial) {
			t.Fatal("invalid preparation partially changed the supplied spec", err)
		}
	}
}

func TestPostgresDraftPreparationPersistsOneVersionAndRollsBackOnFailure(t *testing.T) {
	f := newAnalysisFixture(t)
	input := preparationFixture(t)
	draft, err := f.service.CreateDraft(t.Context(), f.actor, input)
	if err != nil {
		t.Fatal(err)
	}
	spec, _ := DecodeSpec(input.Spec)
	input.ExpectedVersion = draft.DraftVersion
	input.Preparation = &DraftPreparation{Conversions: []DraftPanelConversion{{PanelID: spec.Panels[0].ID, Mode: "dsl"}}, Drilldowns: []DraftPanelDrilldowns{{PanelID: spec.Panels[0].ID}}}
	saved, err := f.service.SaveDraft(t.Context(), f.actor, draft.ID, input)
	if err != nil || saved.DraftVersion != draft.DraftVersion+1 {
		t.Fatal("preparation did not persist with one version-checked save", err)
	}
	prepared, _ := DecodeSpec(saved.Spec)
	if prepared.Panels[0].AuthoringMode != "dsl" || len(prepared.Panels[0].Drilldowns) == 0 {
		t.Fatal("prepared definition not persisted")
	}
	if _, err = f.service.SaveDraft(t.Context(), f.actor, draft.ID, input); !errors.Is(err, ErrConflict) {
		t.Fatal("stale preparation overwrote the draft", err)
	}
	input.ExpectedVersion = saved.DraftVersion
	input.Preparation.Drilldowns[0].PanelID = "missing"
	if _, err = f.service.SaveDraft(t.Context(), f.actor, draft.ID, input); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid preparation accepted", err)
	}
	current, err := f.service.Draft(t.Context(), f.actor, draft.ID)
	if err != nil || current.DraftVersion != saved.DraftVersion || !bytes.Equal(current.Spec, saved.Spec) {
		t.Fatal("failed preparation changed persisted content/version", err)
	}
}
