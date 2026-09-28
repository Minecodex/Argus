package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Audit only bounded identities, version facts, costs and condition hashes.
// Query text, variable values, data rows and execution tokens never enter it.
func appendDashboardAudit(ctx context.Context, q *db.Queries, actor Actor, event, kind string, id uuid.UUID, name, result string, details map[string]any) error {
	actorType := actor.SubjectType
	if actorType == "user" {
		actorType = "enterprise_user"
	}
	details["authorization_version"] = actor.AuthorizationVersion
	if name != "" {
		details["resource_name"] = name
	}
	_, err := audit.Append(ctx, q, audit.Entry{Domain: "enterprise", EnterpriseID: nullID(actor.EnterpriseID), ActorType: actorType, ActorID: actor.SubjectID.String(), Action: event, ResourceType: kind, ResourceID: id.String(), Result: result, Details: details})
	return err
}

func draftAuditDetails(draft db.DashboardDraft) map[string]any {
	details := map[string]any{"draft_id": draft.ID, "draft_version": draft.DraftVersion, "base_object_version": draft.BaseObjectVersion, "spec_hash": draftHash(draft)}
	for key, id := range map[string]uuid.NullUUID{"dashboard_id": draft.DashboardID, "base_revision_id": draft.BaseRevisionID, "revision_id": draft.PublishedRevisionID, "folder_id": draft.FolderID} {
		if id.Valid {
			details[key] = id.UUID
		}
	}
	return details
}

func executionAuditDetails(result Execution, input ExecutionInput) map[string]any {
	conditions, _ := json.Marshal(struct {
		Requested ExecutionInput
		Variables map[string]Selection
		Local     map[string]map[string]Selection
	}{input, result.Variables, result.LocalValues})
	details := map[string]any{"dashboard_id": result.DashboardID, "revision_id": result.RevisionID, "execution_id": result.ExecutionID, "execution_hash": result.ExecutionHash, "parameters_hash": queryHash(conditions), "partial": result.Partial, "from": result.From, "to": result.To}
	resources := []map[string]any{}
	for _, r := range result.Resources {
		resources = append(resources, map[string]any{"resource_id": r.ID, "resource_type": r.Type})
	}
	details["resources"], details["resource_count"] = resources, len(resources)
	variables := []map[string]any{}
	for name, selection := range result.Variables {
		value, _ := json.Marshal(selection)
		variables = append(variables, map[string]any{"variable_id": name, "all": selection.All, "value_count": len(selection.Values), "selection_hash": queryHash(value)})
	}
	details["variables"] = variables
	locals := []map[string]any{}
	for panel, filters := range result.LocalValues {
		for name, selection := range filters {
			value, _ := json.Marshal(selection)
			locals = append(locals, map[string]any{"panel_id": panel, "variable_id": name, "all": selection.All, "value_count": len(selection.Values), "selection_hash": queryHash(value)})
		}
	}
	details["local_filters"] = locals
	panels := []map[string]any{}
	for _, p := range result.Panels {
		sources := []map[string]any{}
		for _, s := range p.Sources {
			sources = append(sources, map[string]any{"source_id": s.ID, "source_revision": s.Revision, "source_type": s.Type, "generation": s.Generation, "resource_id": s.ResourceID})
		}
		targets := []map[string]any{}
		for _, t := range p.Targets {
			targets = append(targets, map[string]any{"target_id": t.ID, "status": t.Status, "reason_code": t.Code, "query_hash": t.QueryHash, "result_type": t.ResultType, "step_seconds": t.StepSeconds, "scanned_bytes": t.Meta.ScannedBytes, "scanned_rows": t.Meta.ScannedRows, "returned_rows": t.Meta.ReturnedRows, "loaded_samples": t.Meta.LoadedSamples, "cache_hit": t.Meta.CacheHit})
		}
		panels = append(panels, map[string]any{"panel_id": p.ID, "status": p.Status, "sources": sources, "targets": targets})
	}
	details["panels"] = panels
	for key, id := range map[string]uuid.UUID{"dashboard_id": result.DashboardID, "revision_id": result.RevisionID, "execution_id": result.ExecutionID} {
		if id == uuid.Nil {
			delete(details, key)
		}
	}
	if result.From.IsZero() {
		delete(details, "from")
	}
	if result.To.IsZero() {
		delete(details, "to")
	}
	return details
}

func (runtime Runtime) recordExecution(ctx context.Context, actor Actor, event, name string, result Execution, input ExecutionInput, cause error, extra ...map[string]any) error {
	if runtime.Store == nil {
		return nil
	}
	details := executionAuditDetails(result, input)
	for _, fields := range extra {
		for key, value := range fields {
			details[key] = value
		}
	}
	status := "success"
	if cause != nil {
		status = "failure"
		if errors.Is(cause, ErrDenied) {
			status = "denied"
		}
		details["reason_code"] = "DASHBOARD_UNAVAILABLE"
		for _, known := range []error{ErrDenied, ErrNotFound, ErrArchived, ErrConflict, ErrInvalid, ErrContextExpired, ErrSelectionStale} {
			if errors.Is(cause, known) {
				details["reason_code"] = known.Error()
				break
			}
		}
	}
	// A cancelled request must still record its bounded outcome. This context
	// never authorizes another query or returns revoked data to the caller.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return runtime.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		return appendDashboardAudit(ctx, q, actor, event, "dashboard", result.DashboardID, name, status, details)
	})
}

func recordBinding(ctx context.Context, q *db.Queries, actor Actor, id, dashboard uuid.UUID, resourceType string, resourceID uuid.UUID, name, operation, actionRef string) error {
	event := "dashboard.binding.attached"
	if operation == "detach" {
		event = "dashboard.binding.detached"
	}
	return appendDashboardAudit(ctx, q, actor, event, "dashboard_binding", id, name, "success", map[string]any{"binding_id": id, "dashboard_id": dashboard, "bound_resource_type": resourceType, "bound_resource_id": resourceID, "action_ref": actionRef})
}

func recordQueryJob(ctx context.Context, q *db.Queries, actor Actor, job db.DashboardQueryJob, event string, causeCode string) error {
	if actor.SubjectID == uuid.Nil {
		actor = Actor{EnterpriseID: job.EnterpriseID, SubjectID: job.OwnerUserID, SubjectType: "user", AuthorizationVersion: job.AuthorizationVersion}
	}
	var plan queryPlan
	if err := json.Unmarshal(job.FrozenPlan, &plan); err != nil {
		return err
	}
	scope := plan.Scope
	files := []map[string]any{}
	var manifest QueryManifest
	if len(job.Manifest) > 0 {
		if err := json.Unmarshal(job.Manifest, &manifest); err != nil {
			return err
		}
		scope = manifest.Execution
		for _, f := range manifest.Files {
			files = append(files, map[string]any{"file_id": f.ID, "panel_id": f.PanelID, "target_id": f.TargetID, "content_hash": f.Hash, "byte_size": f.Bytes, "file_kind": f.Kind, "source_ref": f.SourceRef})
		}
	}
	details := executionAuditDetails(scope, plan.Requested)
	details["query_job_id"], details["conversation_id"], details["status"] = job.ID, job.ConversationID, job.Status
	if job.RunID.Valid {
		details["run_id"] = job.RunID.UUID
	}
	if job.AttemptID.Valid {
		details["attempt_id"] = job.AttemptID.UUID
	}
	details["files"] = files
	if len(job.Manifest) > 0 {
		details["complete"], details["analysis_status"] = manifest.Complete, manifest.AnalysisStatus
	}
	result := "success"
	if causeCode != "" {
		details["reason_code"] = causeCode
		result = "failure"
	}
	revision, err := q.GetDashboardRevision(ctx, db.GetDashboardRevisionParams{ID: job.RevisionID, EnterpriseID: job.EnterpriseID})
	if err != nil {
		return err
	}
	details["spec_hash"] = revision.SpecHash
	return appendDashboardAudit(ctx, q, actor, event, "dashboard_query_job", job.ID, revision.Name, result, details)
}
