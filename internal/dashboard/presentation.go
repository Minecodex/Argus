package dashboard

import (
	"context"
	_ "embed"
	"encoding/json"
	"io"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

//go:embed templates/query.html
var queryTemplate string

func QueryTemplate() *toolruntime.TemplateAsset {
	return presentation.ToolAsset("telemetry.dashboard.query", queryTemplate)
}

// PresentQuery projects the frozen execution, never model-supplied labels or
// current page filters. Chart previews use only hash-verified sealed files;
// full data remains in Workspace for model analysis and download.
func (jobs QueryJobs) PresentQuery(ctx context.Context, actor Actor, conversation, id uuid.UUID) (*toolruntime.Presentation, error) {
	job, err := jobs.loadJob(ctx, actor, conversation, id)
	if err != nil {
		return nil, err
	}
	var plan queryPlan
	if json.Unmarshal(job.FrozenPlan, &plan) != nil {
		return nil, ErrInvalid
	}
	scope := plan.Scope
	revision, err := jobs.Runtime.Store.Queries.GetDashboardRevision(ctx, db.GetDashboardRevisionParams{ID: job.RevisionID, EnterpriseID: actor.EnterpriseID})
	if err != nil {
		return nil, err
	}
	spec, err := DecodeSpec(revision.Spec)
	if err != nil {
		return nil, err
	}
	if len(job.Manifest) > 0 {
		var manifest QueryManifest
		if json.Unmarshal(job.Manifest, &manifest) != nil || manifest.JobID != job.ID || manifest.Execution.RevisionID != job.RevisionID {
			return nil, ErrInvalid
		}
		scope, spec = manifest.Execution, manifest.Definition
	}
	ids := []uuid.UUID{}
	for _, resource := range scope.Resources {
		ids = append(ids, resource.ID)
	}
	resources, refs, err := presentation.Resources(ctx, jobs.Runtime.Store, actor.EnterpriseID, ids)
	if err != nil {
		return nil, err
	}
	refs = append(refs, toolruntime.ResourceRef{Type: "dashboard", ID: job.DashboardID.String()})
	panels := queryPresentationPanels(spec, scope)
	if len(job.Manifest) > 0 && job.AttemptID.Valid {
		files, e := jobs.Runtime.Store.Queries.ListDashboardQueryFiles(ctx, db.ListDashboardQueryFilesParams{AttemptID: job.AttemptID.UUID, EnterpriseID: actor.EnterpriseID})
		if e != nil {
			return nil, e
		}
		remaining := int64(12 << 20)
		for _, file := range files {
			if file.FileKind != "data" {
				continue
			}
			for _, panel := range panels {
				if panel["id"] != file.PanelID {
					continue
				}
				for _, target := range panel["targets"].([]map[string]any) {
					if target["id"] != file.TargetID {
						continue
					}
					if file.ByteSize > 8<<20 || file.ByteSize > remaining {
						target["preview_omitted"] = true
						continue
					}
					guard := func() error { _, e := jobs.loadJob(ctx, actor, conversation, id); return e }
					reader, e := newQueryFileReader(ctx, jobs.Objects, file, guard)
					if e != nil {
						return nil, e
					}
					encoded, e := io.ReadAll(reader)
					if e != nil {
						return nil, e
					}
					remaining -= file.ByteSize
					var data any
					if json.Unmarshal(encoded, &data) != nil {
						return nil, ErrInvalid
					}
					target["data"] = data
				}
			}
		}
	}
	enterprise, err := jobs.Runtime.Store.Queries.GetEnterprise(ctx, actor.EnterpriseID)
	if err != nil {
		return nil, err
	}
	authorization, err := presentation.Scope(ctx, jobs.Runtime.Store, actor.EnterpriseID, actor.SubjectID)
	if err != nil {
		return nil, err
	}
	if err := jobs.checkJob(ctx, actor, conversation, job); err != nil {
		return nil, err
	}
	asset := QueryTemplate()
	data := map[string]any{"view": "dashboard-result", "job_id": job.ID, "dashboard_id": job.DashboardID, "attempt_id": job.AttemptID.UUID, "name": revision.Name, "revision_number": revision.RevisionNumber, "status": job.Status, "error_code": job.ErrorCode, "from": scope.From, "to": scope.To, "timezone": enterprise.Timezone, "resources": resources, "variables": scope.Variables, "local_values": scope.LocalValues, "panels": panels, "layout": spec.Layout, "partial": scope.Partial}
	public := presentation.PublicSnapshot(data)
	presentation.DashboardObservationContext(public, resources)
	data = public
	p := presentation.Bound(asset, data, authorization, refs)
	p.Binding["kind"], p.Binding["job_id"], p.Binding["attempt_id"] = "dashboard_query", job.ID.String(), ""
	if job.AttemptID.Valid {
		p.Binding["attempt_id"] = job.AttemptID.UUID.String()
	}
	return p, nil
}

func queryPresentationPanels(spec Spec, execution Execution) []map[string]any {
	result := []map[string]any{}
	for _, panel := range execution.Panels {
		name, signal, kind := panel.ID, "", ""
		fields := map[string]any{}
		for _, definition := range spec.Panels {
			if definition.ID == panel.ID {
				name, signal, kind = definition.Title, definition.Signal, definition.Type
				fields = map[string]any{"description": definition.Description, "unit": definition.Unit, "decimals": definition.Decimals, "legend": definition.Legend, "thresholds": definition.Thresholds, "display": definition.Display, "layout": definition.Layout}
				break
			}
		}
		targets := []map[string]any{}
		for _, target := range panel.Targets {
			targets = append(targets, map[string]any{"id": target.ID, "status": target.Status, "code": target.Code, "result_type": target.ResultType, "step_seconds": target.StepSeconds})
		}
		fields["id"], fields["name"], fields["signal"], fields["type"], fields["status"], fields["targets"] = panel.ID, name, signal, kind, panel.Status, targets
		result = append(result, fields)
	}
	return result
}
