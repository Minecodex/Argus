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
	return result, nil
}
