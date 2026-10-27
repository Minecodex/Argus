package httpapi

import (
	"context"
	api "github.com/kakj-go/Argus/internal/gen/openapi/monitoringapi"
	"github.com/kakj-go/Argus/internal/monitoring"
)

func ingestPageInput(cursor *string, limit *int) monitoring.ListInput {
	in := monitoring.ListInput{}
	if cursor != nil {
		in.Cursor = *cursor
	}
	if limit != nil {
		in.Limit = *limit
	}
	return in
}
func (h MonitoringHandler) ListApplicationIngestEndpoints(ctx context.Context, r api.ListApplicationIngestEndpointsRequestObject) (api.ListApplicationIngestEndpointsResponseObject, error) {
	a, f := h.auth(ctx, false, "", "application.read")
	if f != nil {
		return api.ListApplicationIngestEndpointsdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	result, err := h.Service.IngestEndpoints(ctx, a, r.Id, ingestPageInput(r.Params.Cursor, r.Params.Limit))
	if err != nil {
		return api.ListApplicationIngestEndpointsdefaultJSONResponse{Body: monitoringError(ctx, err), StatusCode: monitoringStatus(err)}, nil
	}
	return api.ListApplicationIngestEndpoints200JSONResponse(dashboardConvert[api.MonitoringIngestEndpointPage](result)), nil
}
func (h MonitoringHandler) ListApplicationIngestKeys(ctx context.Context, r api.ListApplicationIngestKeysRequestObject) (api.ListApplicationIngestKeysResponseObject, error) {
	a, f := h.auth(ctx, false, "", "application.read")
	if f != nil {
		return api.ListApplicationIngestKeysdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	result, err := h.Service.IngestKeys(ctx, a, r.Id, ingestPageInput(r.Params.Cursor, r.Params.Limit))
	if err != nil {
		return api.ListApplicationIngestKeysdefaultJSONResponse{Body: monitoringError(ctx, err), StatusCode: monitoringStatus(err)}, nil
	}
	return api.ListApplicationIngestKeys200JSONResponse(dashboardConvert[api.MonitoringIngestKeyPage](result)), nil
}
func (h MonitoringHandler) RevealApplicationIngestKey(ctx context.Context, r api.RevealApplicationIngestKeyRequestObject) (api.RevealApplicationIngestKeyResponseObject, error) {
	a, f := h.auth(ctx, true, r.Params.XCSRFToken, "application.manage")
	if f != nil {
		return api.RevealApplicationIngestKeydefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	value, err := h.Service.RevealIngestKey(ctx, a, r.Id)
	if err != nil {
		return api.RevealApplicationIngestKeydefaultJSONResponse{Body: monitoringError(ctx, err), StatusCode: monitoringStatus(err)}, nil
	}
	return api.RevealApplicationIngestKey200JSONResponse{Body: api.MonitoringIngestKeyReveal{Key: value}, Headers: api.RevealApplicationIngestKey200ResponseHeaders{CacheControl: "no-store"}}, nil
}
