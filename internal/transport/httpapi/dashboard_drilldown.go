package httpapi

import (
	"context"

	"github.com/kakj-go/Argus/internal/dashboard"
	api "github.com/kakj-go/Argus/internal/gen/openapi/dashboardapi"
)

func (handler DashboardHandler) ExecuteDashboardDrilldown(ctx context.Context, r api.ExecuteDashboardDrilldownRequestObject) (api.ExecuteDashboardDrilldownResponseObject, error) {
	actor, failure := handler.auth(ctx, false, "")
	if failure != nil {
		return api.ExecuteDashboardDrilldowndefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	if handler.Runtime == nil {
		return api.ExecuteDashboardDrilldowndefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrUnavailable), StatusCode: 503}, nil
	}
	if r.Body == nil {
		return api.ExecuteDashboardDrilldowndefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrInvalid), StatusCode: 400}, nil
	}
	result, err := handler.Runtime.Drilldown(ctx, actor, r.Id, dashboardConvert[dashboard.DrilldownInput](r.Body))
	if err != nil {
		return api.ExecuteDashboardDrilldowndefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	return api.ExecuteDashboardDrilldown200JSONResponse(dashboardConvert[api.DashboardDrilldownExecution](result)), nil
}

func (handler DashboardHandler) GenerateDashboardDrilldowns(ctx context.Context, r api.GenerateDashboardDrilldownsRequestObject) (api.GenerateDashboardDrilldownsResponseObject, error) {
	actor, failure := handler.auth(ctx, true, r.Params.XCSRFToken)
	if failure != nil {
		return api.GenerateDashboardDrilldownsdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	if r.Body == nil {
		return api.GenerateDashboardDrilldownsdefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrInvalid), StatusCode: 400}, nil
	}
	result, err := handler.Service.GenerateDrilldowns(ctx, actor, r.Id, dashboardConvert[dashboard.GenerateDrilldownsInput](r.Body))
	if err != nil {
		return api.GenerateDashboardDrilldownsdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	return api.GenerateDashboardDrilldowns200JSONResponse{Draft: dashboardDraftView(result.Draft), Added: result.Added, Issues: dashboardConvert[[]api.DashboardIssue](result.Issues)}, nil
}
