package toolgateway

import (
	"context"
	"time"

	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/mcp"
)

// A job's immutable execution manifest is not a live quota snapshot. Attach
// current Run accounting separately so a failure cannot leave the model with
// only the budget it saw before acquisition. HTTP/file contracts stay frozen.
func (tools DashboardTools) queryResult(ctx context.Context, c dashboardCall, job dashboard.QueryJobView, err error) (mcp.Result, error) {
	result, err := dashboardToolResult(job, err)
	if err != nil {
		return result, err
	}
	budget, err := tools.Service.RunBudget(ctx, c.actor)
	if err != nil {
		return mcp.Result{}, err
	}
	encoded, err := dashboardToolResult(budget, nil)
	if err != nil {
		return mcp.Result{}, err
	}
	result.Structured["run_budget"] = encoded.Structured
	result.Structured["run_budget_observed_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	result.ContextFacts = map[string]any{"kind": "query_job", "id": job.ID.String(), "dashboard_id": job.DashboardID.String(), "status": job.Status, "version": job.Version, "files": job.Files}
	summary := map[string]any{}
	for name, value := range result.Structured {
		if name != "manifest" {
			summary[name] = value
		}
	}
	if job.Manifest != nil {
		summary["manifest_summary"] = map[string]any{"schema_version": job.Manifest.Schema, "complete": job.Manifest.Complete, "analysis_status": job.Manifest.AnalysisStatus, "requested_parameters": job.Manifest.Requested,
			"details_location": "Full immutable manifest and definitions are in the delivered manifest.json and original result artifact; use those exact references when needed."}
	}
	result.Private = map[string]any{"model_summary": summary}
	return result, nil
}
