package httpapi

import (
	"context"

	"github.com/kakj-go/Argus/internal/dashboard"
	api "github.com/kakj-go/Argus/internal/gen/openapi/dashboardapi"
)

func (handler DashboardHandler) StartDashboardQueryJob(ctx context.Context, r api.StartDashboardQueryJobRequestObject) (api.StartDashboardQueryJobResponseObject, error) {
	actor, failure := handler.authPermission(ctx, true, r.Params.XCSRFToken, "telemetry.dashboard.read")
	if failure != nil {
		return api.StartDashboardQueryJobdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	if handler.Queries == nil {
		return api.StartDashboardQueryJobdefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrUnavailable), StatusCode: 503}, nil
	}
	if r.Body == nil {
		return api.StartDashboardQueryJobdefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrInvalid), StatusCode: 400}, nil
	}
	result, err := handler.Queries.Start(ctx, actor, r.Id, dashboardConvert[dashboard.QueryJobInput](r.Body), r.Params.IdempotencyKey)
	if err != nil {
		return api.StartDashboardQueryJobdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	return api.StartDashboardQueryJob202JSONResponse(dashboardConvert[api.DashboardQueryJobView](result)), nil
}
func (handler DashboardHandler) GetDashboardQueryJob(ctx context.Context, r api.GetDashboardQueryJobRequestObject) (api.GetDashboardQueryJobResponseObject, error) {
	actor, failure := handler.auth(ctx, false, "")
	if failure != nil {
		return api.GetDashboardQueryJobdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	if handler.Queries == nil {
		return api.GetDashboardQueryJobdefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrUnavailable), StatusCode: 503}, nil
	}
	result, err := handler.Queries.Get(ctx, actor, r.Id, r.JobId)
	if err != nil {
		return api.GetDashboardQueryJobdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	return api.GetDashboardQueryJob200JSONResponse(dashboardConvert[api.DashboardQueryJobView](result)), nil
}
func (handler DashboardHandler) CancelDashboardQueryJob(ctx context.Context, r api.CancelDashboardQueryJobRequestObject) (api.CancelDashboardQueryJobResponseObject, error) {
	actor, failure := handler.authPermission(ctx, true, r.Params.XCSRFToken, "telemetry.dashboard.read")
	if failure != nil {
		return api.CancelDashboardQueryJobdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	if handler.Queries == nil {
		return api.CancelDashboardQueryJobdefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrUnavailable), StatusCode: 503}, nil
	}
	result, err := handler.Queries.Cancel(ctx, actor, r.Id, r.JobId)
	if err != nil {
		return api.CancelDashboardQueryJobdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	return api.CancelDashboardQueryJob200JSONResponse(dashboardConvert[api.DashboardQueryJobView](result)), nil
}
func (handler DashboardHandler) ResumeDashboardQueryJob(ctx context.Context, r api.ResumeDashboardQueryJobRequestObject) (api.ResumeDashboardQueryJobResponseObject, error) {
	actor, failure := handler.authPermission(ctx, true, r.Params.XCSRFToken, "telemetry.dashboard.read")
	if failure != nil {
		return api.ResumeDashboardQueryJobdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	if handler.Queries == nil {
		return api.ResumeDashboardQueryJobdefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrUnavailable), StatusCode: 503}, nil
	}
	if r.Body == nil {
		return api.ResumeDashboardQueryJobdefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrInvalid), StatusCode: 400}, nil
	}
	result, err := handler.Queries.Resume(ctx, actor, r.Id, r.JobId, r.Body.ExpectedVersion)
	if err != nil {
		return api.ResumeDashboardQueryJobdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	return api.ResumeDashboardQueryJob202JSONResponse(dashboardConvert[api.DashboardQueryJobView](result)), nil
}
