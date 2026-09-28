package dashboard

import (
	"context"
	"encoding/json"
	"io"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
)

type QueryDrilldownInput struct {
	ParentJobID               uuid.UUID         `json:"parent_job_id"`
	PanelID                   string            `json:"panel_id"`
	DrilldownID               string            `json:"drilldown_id"`
	Values                    map[string]string `json:"values"`
	ExpandAuthorizedResources bool              `json:"expand_authorized_resources"`
}

// Scope and remaining budget live in the immutable query plan. This public
// lineage describes which published definition and immutable row evidence led
// to the child, without exposing private object keys or an editable query.
type QueryDrilldownContext struct {
	ParentJobID       uuid.UUID         `json:"parent_job_id"`
	ParentAttemptID   uuid.UUID         `json:"parent_attempt_id"`
	ParentExecutionID uuid.UUID         `json:"parent_execution_id"`
	PanelID           string            `json:"panel_id"`
	DrilldownID       string            `json:"drilldown_id"`
	OriginQueryRef    string            `json:"origin_query_ref"`
	TargetID          string            `json:"target_id"`
	ScopePolicy       string            `json:"scope_policy"`
	Inputs            map[string]string `json:"inputs"`
	Depth             int               `json:"depth"`
}

func hasQueryOverrides(input ExecutionInput) bool {
	return input.CandidatesOnly || input.From != nil || input.To != nil || len(input.ResourceIDs) > 0 || len(input.PanelIDs) > 0 || len(input.Variables) > 0 || len(input.LocalValues) > 0
}
func fileExecutionContext(parent db.DashboardQueryJob, manifest QueryManifest) (executionContext, error) {
	if manifest.Schema != "argus.dashboard_query_manifest/v1" || !parent.AttemptID.Valid || manifest.JobID != parent.ID || manifest.AttemptID != parent.AttemptID.UUID || manifest.Execution.DashboardID != parent.DashboardID || manifest.Execution.RevisionID != parent.RevisionID || manifest.Compiler != CompilerVersion {
		return executionContext{}, ErrInvalid
	}
	frozen := executionContext{Scope: manifest.Execution}
	if detail := manifest.Drilldown; detail != nil {
		if detail.Depth < 1 || detail.Depth > 16 || len(frozen.Scope.Panels) != 1 || frozen.Scope.Panels[0].ID != detail.PanelID {
			return frozen, ErrInvalid
		}
		panel := frozen.Scope.Panels[0]
		if len(panel.Targets) != 1 || panel.Targets[0].ID != detail.TargetID {
			return frozen, ErrInvalid
		}
		frozen.Depth = detail.Depth
		frozen.Leaf = &detailContext{PanelID: detail.PanelID, TargetID: detail.TargetID, Inputs: detail.Inputs, From: frozen.Scope.From, To: frozen.Scope.To, Resources: frozen.Scope.Resources, Sources: panel.Sources, DetailSources: panel.DetailSources}
	}
	return frozen, nil
}
func (jobs QueryJobs) prepareFileDrilldown(ctx context.Context, actor Actor, conversation uuid.UUID, input QueryJobInput) (queryPlan, error) {
	var empty queryPlan
	request := input.Drilldown
	if request == nil || request.ParentJobID == uuid.Nil || hasQueryOverrides(input.Parameters) {
		return empty, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	parent, err := jobs.loadJob(ctx, actor, conversation, request.ParentJobID)
	if err != nil {
		return empty, err
	}
	if parent.DashboardID != input.DashboardID {
		return empty, ErrDenied
	}
	// A later Chat Run is a new request: it must acquire the latest published
	// definition first. Only navigation within the same Run may continue a
	// previous result's immutable revision. Human API jobs use the nil Run.
	if parent.RunID != nullID(input.RunID) {
		return empty, ErrContextExpired
	}
	if len(parent.Manifest) == 0 || !parent.AttemptID.Valid {
		return empty, ErrSelectionStale
	}
	var manifest QueryManifest
	if json.Unmarshal(parent.Manifest, &manifest) != nil {
		return empty, ErrInvalid
	}
	frozen, err := fileExecutionContext(parent, manifest)
	if err != nil {
		return empty, err
	}
	revision, err := jobs.Runtime.Store.Queries.GetDashboardRevision(ctx, db.GetDashboardRevisionParams{ID: parent.RevisionID, EnterpriseID: actor.EnterpriseID})
	if err != nil || revision.DashboardID != parent.DashboardID {
		return empty, ErrDenied
	}
	spec, err := DecodeSpec(revision.Spec)
	if err != nil {
		return empty, err
	}
	in := DrilldownInput{PanelID: request.PanelID, DrilldownID: request.DrilldownID, Values: request.Values, ExpandAuthorizedResources: request.ExpandAuthorizedResources}
	prepared, err := jobs.Runtime.prepareDrilldown(ctx, actor, spec, frozen, in, func(ctx context.Context, _ Drilldown, origin Target, _ queryScope, _ map[string]string, ledger *executionBudget) (any, error) {
		return jobs.readFileRowEvidence(ctx, actor, conversation, parent, manifest, request.PanelID, origin.ID, ledger)
	})
	if err != nil {
		return empty, err
	}
	scope := prepared.Scope
	detail := &QueryDrilldownContext{ParentJobID: parent.ID, ParentAttemptID: parent.AttemptID.UUID, ParentExecutionID: manifest.Execution.ExecutionID, PanelID: prepared.Panel.ID, DrilldownID: prepared.Drill.ID, OriginQueryRef: prepared.Drill.OriginQueryRef, TargetID: prepared.Detail.ID, ScopePolicy: prepared.Drill.ScopePolicy, Inputs: request.Values, Depth: frozen.Depth + 1}
	result := Execution{DashboardID: parent.DashboardID, RevisionID: parent.RevisionID, From: scope.From, To: scope.To, Resources: scope.Resources, Variables: frozen.Scope.Variables, LocalValues: frozen.Scope.LocalValues, VariableCandidates: frozen.Scope.VariableCandidates, LocalCandidates: frozen.Scope.LocalCandidates,
		Panels: []PanelExecution{{ID: prepared.Panel.ID, Sources: scope.Sources, Targets: []TargetExecution{}, DetailSources: prepared.DetailSources}}}
	if err := jobs.checkJob(ctx, actor, conversation, parent); err != nil {
		return empty, err
	}
	if err := jobs.Runtime.checkQueryScope(ctx, actor, scope); err != nil {
		return empty, err
	}
	return queryPlan{AnalysisContextID: manifest.AnalysisContextID, Compiler: CompilerVersion, Scope: result, Budget: freezeBudget(prepared.Budget), Requested: manifest.Requested, Drilldown: detail}, nil
}
func (jobs QueryJobs) readFileRowEvidence(ctx context.Context, actor Actor, conversation uuid.UUID, parent db.DashboardQueryJob, manifest QueryManifest, panelID, targetID string, ledger *executionBudget) (any, error) {
	pi := slices.IndexFunc(manifest.Execution.Panels, func(p PanelExecution) bool { return p.ID == panelID })
	if pi < 0 {
		return nil, ErrInvalid
	}
	targets := manifest.Execution.Panels[pi].Targets
	ti := slices.IndexFunc(targets, func(t TargetExecution) bool { return t.ID == targetID })
	if ti < 0 {
		return nil, ErrInvalid
	}
	fi := slices.IndexFunc(manifest.Files, func(f QueryFileView) bool { return f.PanelID == panelID && f.TargetID == targetID && f.Kind == "data" })
	if fi < 0 {
		return nil, ErrSelectionStale
	}
	expected := manifest.Files[fi]
	file, err := jobs.Runtime.Store.Queries.GetDashboardQueryFile(ctx, db.GetDashboardQueryFileParams{ID: expected.ID, EnterpriseID: actor.EnterpriseID})
	if err != nil {
		return nil, err
	}
	if file.JobID != parent.ID || file.AttemptID != parent.AttemptID.UUID || file.PanelID != panelID || file.TargetID != targetID || file.FileKind != "data" || file.ContentHash != expected.Hash || file.ByteSize != expected.Bytes {
		return nil, ErrInvalid
	}
	allocation, err := ledger.next(ctx)
	if err != nil {
		return nil, err
	}
	rows := targets[ti].Meta.ReturnedRows
	if file.ByteSize > allocation.MaxResultBytes || rows > int64(allocation.MaxRows) || rows < 0 {
		return nil, telemetry.ErrQueryBudget
	}
	if err := chargeRunEvidence(ctx, jobs.Runtime.Store, actor, file.ByteSize, rows); err != nil {
		return nil, err
	}
	reader, err := newQueryFileReader(ctx, jobs.Objects, file, func() error { return jobs.checkJob(ctx, actor, conversation, parent) })
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(reader)
	decoder.UseNumber()
	var data any
	if err = decoder.Decode(&data); err != nil {
		return nil, err
	}
	// Force EOF to verify every fragment and the entire file hash. Never accept
	// a matching prefix while later fragments are corrupt or unavailable.
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, err
		}
		return nil, ErrInvalid
	}
	ledger.bytes -= file.ByteSize
	ledger.rows -= int(rows)
	return data, nil
}
func (runtime Runtime) executeFileDrilldown(ctx context.Context, actor Actor, spec Spec, plan queryPlan) (Execution, error) {
	runtime = runtime.metered(actor)
	result := plan.Scope
	d := plan.Drilldown
	if d == nil || d.Depth < 1 || d.Depth > 16 || len(result.Panels) != 1 || result.Panels[0].ID != d.PanelID {
		return result, ErrInvalid
	}
	index := slices.IndexFunc(spec.Panels, func(p Panel) bool { return p.ID == d.PanelID })
	if index < 0 {
		return result, ErrInvalid
	}
	panel := spec.Panels[index]
	if !slices.ContainsFunc(panel.Drilldowns, func(drill Drilldown) bool {
		return drill.ID == d.DrilldownID && drill.OriginQueryRef == d.OriginQueryRef && drill.DetailQueryRef == d.TargetID && drill.ScopePolicy == d.ScopePolicy
	}) {
		return result, ErrInvalid
	}
	target, ok := findTarget(panel, d.TargetID)
	if !ok || target.SourceBinding == nil {
		return result, ErrInvalid
	}
	scope := queryScope{result.From, result.To, result.Resources, result.Panels[0].Sources}
	runtime.cacheDashboard, runtime.cacheRevision = result.DashboardID, result.RevisionID
	targetResult, err := runtime.executeTarget(ctx, actor, spec, targetPanel(panel, target), target, result.Variables, result.LocalValues[panel.ID], selectionValues(d.Inputs), scope, plan.Budget.ledger())
	if err != nil {
		return result, err
	}
	if err = runtime.checkQueryScope(ctx, actor, scope); err != nil {
		return result, err
	}
	result.Panels[0].Targets = []TargetExecution{targetResult}
	result.Panels[0].Status = panelStatus(result.Panels[0].Targets)
	result.Partial = targetResult.Status == "error" || targetResult.Status == "partial" || targetResult.Status == "skipped_budget"
	return result, nil
}
