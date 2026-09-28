package httpapi

import (
	"context"
	"strings"

	"github.com/kakj-go/Argus/internal/dashboard"
	api "github.com/kakj-go/Argus/internal/gen/openapi/dashboardapi"
)

func (handler DashboardHandler) ListDashboardFolders(ctx context.Context, r api.ListDashboardFoldersRequestObject) (api.ListDashboardFoldersResponseObject, error) {
	actor, failure := handler.auth(ctx, false, "")
	if failure != nil {
		return api.ListDashboardFoldersdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	items, err := handler.Service.Folders(ctx, actor)
	if err != nil {
		return api.ListDashboardFoldersdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	result := []api.DashboardFolder{}
	for _, item := range items {
		result = append(result, api.DashboardFolder{Id: item.ID, Name: item.Name, Description: item.Description, SortOrder: item.SortOrder, Status: api.DashboardFolderStatus(item.Status), Version: item.Version})
	}
	return api.ListDashboardFolders200JSONResponse(result), nil
}

func (handler DashboardHandler) PreviewDashboardFolder(ctx context.Context, r api.PreviewDashboardFolderRequestObject) (api.PreviewDashboardFolderResponseObject, error) {
	actor, failure := handler.auth(ctx, true, r.Params.XCSRFToken)
	if failure != nil {
		return api.PreviewDashboardFolderdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	if r.Body == nil || !strings.HasPrefix(string(r.Body.Operation), "folder.") {
		return api.PreviewDashboardFolderdefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrInvalid), StatusCode: 400}, nil
	}
	action, err := handler.Service.PreviewLifecycle(ctx, actor, dashboardConvert[dashboard.LifecycleInput](r.Body), r.Params.IdempotencyKey)
	if err != nil {
		return api.PreviewDashboardFolderdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	return api.PreviewDashboardFolder201JSONResponse(convertPending[api.PendingActionPublicSchema](action)), nil
}

func (handler DashboardHandler) PreviewDashboardLifecycle(ctx context.Context, r api.PreviewDashboardLifecycleRequestObject) (api.PreviewDashboardLifecycleResponseObject, error) {
	actor, failure := handler.auth(ctx, true, r.Params.XCSRFToken)
	if failure != nil {
		return api.PreviewDashboardLifecycledefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	if r.Body == nil || (r.Body.Operation != "archive" && r.Body.Operation != "restore") || r.Body.Id != nil && *r.Body.Id != r.Id {
		return api.PreviewDashboardLifecycledefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrInvalid), StatusCode: 400}, nil
	}
	input := dashboardConvert[dashboard.LifecycleInput](r.Body)
	input.ID = r.Id
	action, err := handler.Service.PreviewLifecycle(ctx, actor, input, r.Params.IdempotencyKey)
	if err != nil {
		return api.PreviewDashboardLifecycledefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	return api.PreviewDashboardLifecycle201JSONResponse(convertPending[api.PendingActionPublicSchema](action)), nil
}
