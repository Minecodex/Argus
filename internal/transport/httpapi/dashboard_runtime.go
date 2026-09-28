package httpapi

import (
	"context"

	"github.com/kakj-go/Argus/internal/dashboard"
	api "github.com/kakj-go/Argus/internal/gen/openapi/dashboardapi"
)

func (handler DashboardHandler) ExecuteDashboard(ctx context.Context, r api.ExecuteDashboardRequestObject) (api.ExecuteDashboardResponseObject, error) {
	actor, failure := handler.auth(ctx, false, "")
	if failure != nil {
		return api.ExecuteDashboarddefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	if handler.Runtime == nil {
		return api.ExecuteDashboarddefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrUnavailable), StatusCode: 503}, nil
	}
	if r.Body == nil {
		return api.ExecuteDashboarddefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrInvalid), StatusCode: 400}, nil
	}
	result, err := handler.Runtime.Execute(ctx, actor, r.Id, dashboardConvert[dashboard.ExecutionInput](r.Body))
	if err != nil {
		return api.ExecuteDashboarddefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	return api.ExecuteDashboard200JSONResponse(dashboardConvert[api.DashboardExecution](result)), nil
}

func (handler DashboardHandler) QueryDashboardCatalog(ctx context.Context, r api.QueryDashboardCatalogRequestObject) (api.QueryDashboardCatalogResponseObject, error) {
	actor, failure := handler.auth(ctx, false, "")
	if failure != nil {
		return api.QueryDashboardCatalogdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	if handler.Runtime == nil {
		return api.QueryDashboardCatalogdefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrUnavailable), StatusCode: 503}, nil
	}
	if r.Body == nil {
		return api.QueryDashboardCatalogdefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrInvalid), StatusCode: 400}, nil
	}
	result, err := handler.Runtime.Catalog(ctx, actor, dashboardConvert[dashboard.CatalogInput](r.Body))
	if err != nil {
		return api.QueryDashboardCatalogdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	return api.QueryDashboardCatalog200JSONResponse(dashboardConvert[api.DashboardCatalogResult](result)), nil
}

func (handler DashboardHandler) SampleDashboardDraft(ctx context.Context, r api.SampleDashboardDraftRequestObject) (api.SampleDashboardDraftResponseObject, error) {
	actor, failure := handler.auth(ctx, false, "")
	if failure != nil {
		return api.SampleDashboardDraftdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	if handler.Runtime == nil {
		return api.SampleDashboardDraftdefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrUnavailable), StatusCode: 503}, nil
	}
	if r.Body == nil {
		return api.SampleDashboardDraftdefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrInvalid), StatusCode: 400}, nil
	}
	result, err := handler.Runtime.SampleDraft(ctx, actor, r.Id, r.Body.ExpectedVersion)
	if err != nil {
		return api.SampleDashboardDraftdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	return api.SampleDashboardDraft200JSONResponse(dashboardConvert[api.DashboardDraftSample](result)), nil
}
