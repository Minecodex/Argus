package httpapi

import (
	"context"

	"github.com/kakj-go/Argus/internal/dashboard"
	api "github.com/kakj-go/Argus/internal/gen/openapi/dashboardapi"
)

func (h DashboardHandler) ListDashboardScopeResources(ctx context.Context, r api.ListDashboardScopeResourcesRequestObject) (api.ListDashboardScopeResourcesResponseObject, error) {
	a, failure := h.auth(ctx, false, "")
	if failure != nil {
		return api.ListDashboardScopeResourcesdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	in := dashboard.ScopeDirectoryInput{Kind: string(r.Params.Kind)}
	if r.Params.Search != nil {
		in.Search = *r.Params.Search
	}
	if r.Params.Cursor != nil {
		in.Cursor = *r.Params.Cursor
	}
	if r.Params.Limit != nil {
		in.Limit = *r.Params.Limit
	}
	page, err := h.Service.ScopeDirectory(ctx, a, in)
	if err != nil {
		return api.ListDashboardScopeResourcesdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	return api.ListDashboardScopeResources200JSONResponse(dashboardConvert[api.DashboardScopeDirectoryPage](page)), nil
}
