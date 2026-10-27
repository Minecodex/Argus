package httpapi

import (
	"context"

	api "github.com/kakj-go/Argus/internal/gen/openapi/monitoringapi"
	"github.com/kakj-go/Argus/internal/monitoring"
)

func (h MonitoringHandler) ListMonitoringBuiltins(ctx context.Context, r api.ListMonitoringBuiltinsRequestObject) (api.ListMonitoringBuiltinsResponseObject, error) {
	a, f := h.auth(ctx, false, "", "telemetry.collector.read")
	if f != nil {
		return api.ListMonitoringBuiltinsdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	items, err := h.Service.Builtins(ctx, a, monitoring.ResourceRef{Kind: string(r.Params.TargetType), ID: r.Params.TargetId})
	if err != nil {
		return api.ListMonitoringBuiltinsdefaultJSONResponse{Body: monitoringError(ctx, err), StatusCode: monitoringStatus(err)}, nil
	}
	return api.ListMonitoringBuiltins200JSONResponse(dashboardConvert[[]api.MonitoringBuiltin](items)), nil
}

func (h MonitoringHandler) PreviewMonitoringBuiltin(ctx context.Context, r api.PreviewMonitoringBuiltinRequestObject) (api.PreviewMonitoringBuiltinResponseObject, error) {
	a, f := h.auth(ctx, true, r.Params.XCSRFToken, "telemetry.collector.manage")
	if f != nil {
		return api.PreviewMonitoringBuiltindefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	if r.Body == nil {
		return api.PreviewMonitoringBuiltindefaultJSONResponse{Body: monitoringError(ctx, monitoring.ErrInvalid), StatusCode: 400}, nil
	}
	result, err := h.Service.PreviewBuiltin(ctx, a, dashboardConvert[monitoring.BuiltinInput](r.Body), r.Params.IdempotencyKey)
	if err != nil {
		return api.PreviewMonitoringBuiltindefaultJSONResponse{Body: monitoringError(ctx, err), StatusCode: monitoringStatus(err)}, nil
	}
	return api.PreviewMonitoringBuiltin201JSONResponse(convertPending[api.PendingActionPublicSchema](result)), nil
}
