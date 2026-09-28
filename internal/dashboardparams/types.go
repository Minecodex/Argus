// Package dashboardparams contains the shared, engine-independent parameter
// contract used by published queries and persistent Chat conditions.
package dashboardparams

import (
	"github.com/google/uuid"
	"time"
)

type Value struct {
	All    bool     `json:"all"`
	Values []string `json:"values"`
}
type TimeRange struct {
	Kind    string     `json:"kind"`
	Seconds int        `json:"seconds,omitempty"`
	From    *time.Time `json:"from,omitempty"`
	To      *time.Time `json:"to,omitempty"`
}
type Input struct {
	From        *time.Time  `json:"from,omitempty"`
	To          *time.Time  `json:"to,omitempty"`
	ResourceIDs []uuid.UUID `json:"resource_ids"`
	PanelIDs    []string    `json:"panel_ids"`
	// CandidatesOnly reconciles selections without executing panel queries.
	CandidatesOnly bool                        `json:"candidates_only,omitempty"`
	Variables      map[string]Value            `json:"variables"`
	LocalValues    map[string]map[string]Value `json:"local_values"`
}
type Resources struct {
	All bool        `json:"all"`
	IDs []uuid.UUID `json:"ids"`
}
type Overrides struct {
	Time        *TimeRange                  `json:"time,omitempty"`
	Resources   *Resources                  `json:"resources,omitempty"`
	Variables   map[string]Value            `json:"variables"`
	LocalValues map[string]map[string]Value `json:"local_values"`
}
type Patch struct {
	ResetAll       bool       `json:"reset_all,omitempty"`
	ResetTime      bool       `json:"reset_time,omitempty"`
	ResetResources bool       `json:"reset_resources,omitempty"`
	Time           *TimeRange `json:"time,omitempty"`
	Resources      *Resources `json:"resources,omitempty"`
	// null resets one explicit override to the current published default.
	Variables   map[string]*Value            `json:"variables,omitempty"`
	LocalValues map[string]map[string]*Value `json:"local_values,omitempty"`
}
type Evidence struct {
	Path  string `json:"path"`
	Quote string `json:"quote"`
}
type Reference struct {
	EventID uuid.UUID `json:"event_id"`
	Quote   string    `json:"quote,omitempty"`
	Origin  string    `json:"origin"`
}
type State struct {
	Schema    string               `json:"schema_version"`
	Overrides Overrides            `json:"overrides"`
	Contracts map[string]string    `json:"contracts"`
	Evidence  map[string]Reference `json:"evidence"`
}

func Empty() State {
	return State{Schema: "argus.dashboard_conditions/v1", Overrides: Overrides{Variables: map[string]Value{}, LocalValues: map[string]map[string]Value{}}, Contracts: map[string]string{}, Evidence: map[string]Reference{}}
}
