package dashboard

import (
	"encoding/json"
	"fmt"
	"slices"
)

// Preparation is opt-in and shares the editor's deterministic compilers.
// Only the prepared spec is persisted, in the draft's version-checked save.
type DraftPreparation struct {
	Conversions []DraftPanelConversion `json:"conversions,omitempty"`
	Drilldowns  []DraftPanelDrilldowns `json:"drilldowns,omitempty"`
}

type DraftPanelConversion struct {
	PanelID string `json:"panel_id"`
	Mode    string `json:"mode"`
}

type DraftPanelDrilldowns struct {
	PanelID       string                   `json:"panel_id"`
	SignalSources map[string]SourceBinding `json:"signal_sources"`
}

func prepareDraftInput(input DraftInput) (DraftInput, error) {
	if input.Preparation == nil {
		return input, nil
	}
	if len(input.Preparation.Conversions) > 64 || len(input.Preparation.Drilldowns) > 64 {
		return input, ErrInvalid
	}
	spec, err := DecodeSpec(input.Spec)
	if err != nil {
		return input, err
	}
	seen := map[string]bool{}
	for _, conversion := range input.Preparation.Conversions {
		index := slices.IndexFunc(spec.Panels, func(panel Panel) bool { return panel.ID == conversion.PanelID })
		if index < 0 || seen[conversion.PanelID] {
			return input, ErrInvalid
		}
		seen[conversion.PanelID] = true
		result := ConvertPanel(ConvertPanelInput{Panel: spec.Panels[index], Mode: conversion.Mode})
		if !result.Converted {
			return input, fmt.Errorf("%w: conversion for panel %s: %v", ErrInvalid, conversion.PanelID, result.Issues)
		}
		spec.Panels[index] = result.Panel
	}
	seen = map[string]bool{}
	for _, drilldowns := range input.Preparation.Drilldowns {
		if drilldowns.PanelID == "" || seen[drilldowns.PanelID] {
			return input, ErrInvalid
		}
		seen[drilldowns.PanelID] = true
		result, err := GenerateStandardDrilldowns(spec, drilldowns.PanelID, drilldowns.SignalSources)
		if err != nil {
			return input, err
		}
		if len(result.Issues) != 0 {
			return input, fmt.Errorf("%w: drilldowns for panel %s: %v", ErrInvalid, drilldowns.PanelID, result.Issues)
		}
		spec = result.Spec
	}
	encoded, err := json.Marshal(spec)
	if err != nil {
		return input, err
	}
	input.Spec = encoded
	return input, nil
}
