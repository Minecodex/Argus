package httpapi

import (
	"context"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboard"
	api "github.com/kakj-go/Argus/internal/gen/openapi/dashboardapi"
)

func dashboardBindingView(item dashboard.BindingEntry) api.DashboardBindingView {
	return api.DashboardBindingView{Id: item.Binding.ID, Version: item.Binding.Version, ResourceType: api.DashboardBindingViewResourceType(item.Binding.ResourceType), ResourceId: item.Binding.ResourceID, ResourceName: item.ResourceName, ResourceVersion: item.ResourceVersion, Dashboard: dashboardView(item.Dashboard)}
}
func (handler DashboardHandler) resourceBindings(ctx context.Context, kind string, id uuid.UUID) (api.ResourceDashboardBindings, *api.ApiError, int) {
	var result api.ResourceDashboardBindings
	actor, failure := handler.auth(ctx, false, "")
	if failure != nil {
		return result, failure, 403
	}
	rows, err := handler.Service.ResourceBindings(ctx, actor, kind, id)
	if err != nil {
		failure := dashboardError(ctx, err)
		return result, &failure, dashboardStatus(err)
	}
	result = api.ResourceDashboardBindings{ResourceType: api.ResourceDashboardBindingsResourceType(kind), ResourceId: id, ResourceName: rows.Name, ResourceVersion: rows.Version, Items: []api.DashboardBindingView{}}
	for _, row := range rows.Items {
		result.Items = append(result.Items, dashboardBindingView(row))
	}
	return result, nil, 200
}
func (handler DashboardHandler) ListHostDashboardBindings(ctx context.Context, r api.ListHostDashboardBindingsRequestObject) (api.ListHostDashboardBindingsResponseObject, error) {
	result, failure, status := handler.resourceBindings(ctx, "host", r.Id)
	if failure != nil {
		return api.ListHostDashboardBindingsdefaultJSONResponse{Body: *failure, StatusCode: status}, nil
	}
	return api.ListHostDashboardBindings200JSONResponse(result), nil
}
func (handler DashboardHandler) ListClusterDashboardBindings(ctx context.Context, r api.ListClusterDashboardBindingsRequestObject) (api.ListClusterDashboardBindingsResponseObject, error) {
	result, failure, status := handler.resourceBindings(ctx, "kubernetes_cluster", r.Id)
	if failure != nil {
		return api.ListClusterDashboardBindingsdefaultJSONResponse{Body: *failure, StatusCode: status}, nil
	}
	return api.ListClusterDashboardBindings200JSONResponse(result), nil
}
func (handler DashboardHandler) ListDashboardBindings(ctx context.Context, r api.ListDashboardBindingsRequestObject) (api.ListDashboardBindingsResponseObject, error) {
	actor, failure := handler.auth(ctx, false, "")
	if failure != nil {
		return api.ListDashboardBindingsdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	rows, err := handler.Service.DashboardBindings(ctx, actor, r.Id)
	if err != nil {
		return api.ListDashboardBindingsdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	result := []api.DashboardBindingView{}
	for _, row := range rows {
		result = append(result, dashboardBindingView(row))
	}
	return api.ListDashboardBindings200JSONResponse(result), nil
}
func (handler DashboardHandler) previewResourceBinding(ctx context.Context, kind string, id uuid.UUID, csrf, key string, input *api.DashboardBindingInput) (api.PendingActionPublicSchema, *api.ApiError, int) {
	var result api.PendingActionPublicSchema
	// Resource managers may bind a dashboard they can read without receiving dashboard editing rights.
	actor, failure := handler.authPermission(ctx, true, csrf, "telemetry.dashboard.read")
	if failure != nil {
		return result, failure, 403
	}
	if input == nil {
		failure := dashboardError(ctx, dashboard.ErrInvalid)
		return result, &failure, 400
	}
	action, err := handler.Service.PreviewBinding(ctx, actor, kind, id, dashboardConvert[dashboard.BindingInput](input), key)
	if err != nil {
		failure := dashboardError(ctx, err)
		return result, &failure, dashboardStatus(err)
	}
	return convertPending[api.PendingActionPublicSchema](action), nil, 201
}
func (handler DashboardHandler) PreviewHostDashboardBinding(ctx context.Context, r api.PreviewHostDashboardBindingRequestObject) (api.PreviewHostDashboardBindingResponseObject, error) {
	result, failure, status := handler.previewResourceBinding(ctx, "host", r.Id, r.Params.XCSRFToken, r.Params.IdempotencyKey, r.Body)
	if failure != nil {
		return api.PreviewHostDashboardBindingdefaultJSONResponse{Body: *failure, StatusCode: status}, nil
	}
	return api.PreviewHostDashboardBinding201JSONResponse(result), nil
}
func (handler DashboardHandler) PreviewClusterDashboardBinding(ctx context.Context, r api.PreviewClusterDashboardBindingRequestObject) (api.PreviewClusterDashboardBindingResponseObject, error) {
	result, failure, status := handler.previewResourceBinding(ctx, "kubernetes_cluster", r.Id, r.Params.XCSRFToken, r.Params.IdempotencyKey, r.Body)
	if failure != nil {
		return api.PreviewClusterDashboardBindingdefaultJSONResponse{Body: *failure, StatusCode: status}, nil
	}
	return api.PreviewClusterDashboardBinding201JSONResponse(result), nil
}
