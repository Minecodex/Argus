package httpapi

import (
	"context"
	api "github.com/kakj-go/Argus/internal/gen/openapi/monitoringapi"
)

func (h MonitoringHandler) GetMonitoringIngestConnection(ctx context.Context, r api.GetMonitoringIngestConnectionRequestObject) (api.GetMonitoringIngestConnectionResponseObject, error) {
	actor, failure := h.auth(ctx, false, "", "application.read")
	if failure != nil {
		return api.GetMonitoringIngestConnectiondefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	result, err := h.Service.IngestConnection(ctx, actor, r.Id)
	if err != nil {
		return api.GetMonitoringIngestConnectiondefaultJSONResponse{Body: monitoringError(ctx, err), StatusCode: monitoringStatus(err)}, nil
	}
	return api.GetMonitoringIngestConnection200JSONResponse(dashboardConvert[api.MonitoringIngestConnection](result)), nil
}
