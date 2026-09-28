// Package dashboard owns published telemetry dashboards and personal drafts.
// Rendering, HTTP and AI tools all consume this domain's configuration contract.
package dashboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboardaccess"
	"github.com/kakj-go/Argus/internal/dashboardparams"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

const SchemaVersion = "argus.telemetry_dashboard/v1"
const CompilerVersion = "argus.dashboard_compiler/v3"

var (
	ErrInvalid        = errors.New("DASHBOARD_INVALID")
	ErrDenied         = dashboardaccess.ErrDenied
	ErrConflict       = errors.New("DASHBOARD_VERSION_CONFLICT")
	ErrArchived       = errors.New("DASHBOARD_ARCHIVED")
	ErrNotFound       = errors.New("DASHBOARD_NOT_FOUND")
	ErrUnavailable    = errors.New("DASHBOARD_UNAVAILABLE")
	ErrSelectionStale = errors.New("DASHBOARD_SELECTION_STALE")
	ErrContextExpired = errors.New("DASHBOARD_CONTEXT_EXPIRED")
)

type Spec struct {
	SchemaVersion         string     `json:"schema_version"`
	DefaultTimeRange      TimeRange  `json:"default_time_range"`
	DefaultRefreshSeconds int        `json:"default_refresh_seconds"`
	Variables             []Variable `json:"variables"`
	Panels                []Panel    `json:"panels"`
	Layout                Grid       `json:"layout"`
}
type TimeRange = dashboardparams.TimeRange
type Grid struct {
	Columns   int `json:"columns"`
	RowHeight int `json:"row_height"`
}
type Rectangle struct {
	X    int `json:"x"`
	Y    int `json:"y"`
	W    int `json:"w"`
	H    int `json:"h"`
	MinW int `json:"min_w"`
	MinH int `json:"min_h"`
}
type SourceBinding struct {
	SourceType        string `json:"source_type"`
	CapabilityVersion string `json:"capability_version"`
}
type Variable struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Label      string         `json:"label"`
	Multiple   bool           `json:"multiple"`
	IncludeAll bool           `json:"include_all"`
	Default    Selection      `json:"default"`
	Query      CandidateQuery `json:"query"`
}
type Selection = dashboardparams.Value
type CandidateQuery struct {
	Signal            string             `json:"signal"`
	SourceBinding     SourceBinding      `json:"source_binding"`
	Metric            string             `json:"metric,omitempty"`
	Field             string             `json:"field"`
	Filters           []Filter           `json:"filters"`
	ParameterBindings []ParameterBinding `json:"parameter_bindings,omitempty"`
}
type Filter struct {
	Field          string   `json:"field"`
	Operator       string   `json:"operator"`
	Value          string   `json:"value,omitempty"`
	Values         []string `json:"values,omitempty"`
	Variable       string   `json:"variable,omitempty"`
	LocalParameter string   `json:"local_parameter,omitempty"`
	DrilldownInput string   `json:"drilldown_input,omitempty"`
}
type LocalFilter struct {
	ID       string          `json:"id"`
	Label    string          `json:"label"`
	Kind     string          `json:"kind"`
	Multiple bool            `json:"multiple"`
	Required bool            `json:"required"`
	Default  Selection       `json:"default"`
	Query    *CandidateQuery `json:"query,omitempty"`
}
type Panel struct {
	ID                      string          `json:"id"`
	Title                   string          `json:"title"`
	Description             string          `json:"description"`
	Type                    string          `json:"type"`
	Signal                  string          `json:"signal"`
	AuthoringMode           string          `json:"authoring_mode"`
	ApplicableResourceTypes []string        `json:"applicable_resource_types"`
	SourceBinding           SourceBinding   `json:"source_binding"`
	LocalFilters            []LocalFilter   `json:"local_filters"`
	Targets                 []Target        `json:"targets"`
	DetailQueryTargets      []Target        `json:"detail_query_targets"`
	Drilldowns              []Drilldown     `json:"drilldowns"`
	Layout                  Rectangle       `json:"layout"`
	Unit                    string          `json:"unit"`
	Decimals                int             `json:"decimals"`
	Legend                  bool            `json:"legend"`
	Thresholds              []Threshold     `json:"thresholds"`
	Display                 *DisplayOptions `json:"display,omitempty"`
}

// DisplayOptions changes presentation only; it never rewrites a target or its query hash.
type DisplayOptions struct {
	Reducer   string   `json:"reducer,omitempty"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
	DrawStyle string   `json:"draw_style,omitempty"`
	Stack     bool     `json:"stack,omitempty"`
	Smooth    bool     `json:"smooth,omitempty"`
}
type Threshold struct {
	Value float64 `json:"value"`
	Tone  string  `json:"tone"`
}
type Target struct {
	ID                string               `json:"id"`
	Language          queryengine.Language `json:"language"`
	Signal            string               `json:"signal,omitempty"`
	SourceBinding     *SourceBinding       `json:"source_binding,omitempty"`
	SourceDefinition  Definition           `json:"source_definition"`
	QueryMode         string               `json:"query_mode"`
	RangeStepPolicy   StepPolicy           `json:"range_step_policy"`
	ParameterBindings []ParameterBinding   `json:"parameter_bindings"`
}
type Definition struct {
	Builder *Builder `json:"builder,omitempty"`
	DSL     *DSL     `json:"dsl,omitempty"`
}
type DSL struct {
	Expression string         `json:"expression"`
	Pipeline   string         `json:"pipeline,omitempty"`
	Operation  string         `json:"operation,omitempty"`
	Variables  map[string]any `json:"variables,omitempty"`
}
type Builder struct {
	Operation     string   `json:"operation"`
	Metric        string   `json:"metric,omitempty"`
	MetricType    string   `json:"metric_type,omitempty"`
	Filters       []Filter `json:"filters"`
	ErrorFilters  []Filter `json:"error_filters,omitempty"`
	GroupBy       []string `json:"group_by"`
	WindowSeconds int      `json:"window_seconds,omitempty"`
	BucketSeconds int      `json:"bucket_seconds,omitempty"`
	TopN          int      `json:"top_n,omitempty"`
	Limit         int      `json:"limit,omitempty"`
	TraceID       string   `json:"trace_id,omitempty"`
	ContextBefore int      `json:"context_before,omitempty"`
	ContextAfter  int      `json:"context_after,omitempty"`
}
type ParameterBinding struct {
	Parameter       string            `json:"parameter"`
	Variable        string            `json:"variable,omitempty"`
	LocalParameter  string            `json:"local_parameter,omitempty"`
	DrilldownInput  string            `json:"drilldown_input,omitempty"`
	IdentityMapping bool              `json:"identity_mapping,omitempty"`
	ValueMap        map[string]string `json:"value_map,omitempty"`
}
type StepPolicy struct {
	Kind           string `json:"kind"`
	Seconds        int    `json:"seconds,omitempty"`
	TargetPoints   int    `json:"target_points,omitempty"`
	MinStepSeconds int    `json:"min_step_seconds,omitempty"`
}
type Drilldown struct {
	ID             string               `json:"id"`
	Title          string               `json:"title"`
	DetailQueryRef string               `json:"detail_query_ref"`
	ScopePolicy    string               `json:"scope_policy"`
	Inputs         map[string]string    `json:"inputs"`
	OriginQueryRef string               `json:"origin_query_ref"`
	Kind           string               `json:"kind,omitempty"`
	TimeWindow     *DrilldownTimeWindow `json:"time_window,omitempty"`
}

type DrilldownTimeWindow struct {
	Input         string `json:"input"`
	Seconds       int    `json:"seconds"`
	DurationInput string `json:"duration_input,omitempty"`
}
type Binding struct {
	ResourceType string    `json:"resource_type"`
	ResourceID   uuid.UUID `json:"resource_id"`
}
type Issue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
type ValidationReport struct {
	Valid           bool    `json:"valid"`
	Issues          []Issue `json:"issues"`
	CompilerVersion string  `json:"compiler_version"`
}

func DecodeSpec(raw []byte) (Spec, error) {
	var spec Spec
	if len(raw) > 2<<20 {
		return spec, fmt.Errorf("%w: spec exceeds 2 MiB", ErrInvalid)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&spec); err != nil {
		return spec, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return spec, fmt.Errorf("%w: trailing JSON", ErrInvalid)
	}
	return spec, nil
}

func EmptySpec() Spec {
	return Spec{SchemaVersion: SchemaVersion, DefaultTimeRange: TimeRange{Kind: "relative", Seconds: 3600}, Variables: []Variable{}, Panels: []Panel{}, Layout: Grid{Columns: 12, RowHeight: 8}}
}

func (policy StepPolicy) Resolve(from, to time.Time) (time.Duration, error) {
	if !to.After(from) {
		return 0, ErrInvalid
	}
	switch policy.Kind {
	case "fixed":
		if policy.Seconds < 1 || policy.Seconds > 86400 {
			return 0, ErrInvalid
		}
		return time.Duration(policy.Seconds) * time.Second, nil
	case "auto":
		if policy.TargetPoints < 1 || policy.TargetPoints > 10000 || policy.MinStepSeconds < 1 {
			return 0, ErrInvalid
		}
		seconds := (int64(to.Sub(from)/time.Second) + int64(policy.TargetPoints) - 1) / int64(policy.TargetPoints)
		return time.Duration(max(seconds, int64(policy.MinStepSeconds))) * time.Second, nil
	default:
		return 0, ErrInvalid
	}
}
