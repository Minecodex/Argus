package httpapi

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/gen/openapi/hostapi"
	"net/http"
)

func (handler HostHandler) PreviewRetryHost(ctx context.Context, request hostapi.PreviewRetryHostRequestObject) (hostapi.PreviewRetryHostResponseObject, error) {
	p, apiError := handler.auth(ctx, true, request.Params.XCSRFToken, "host.manage")
	if apiError != nil {
		return hostapi.PreviewRetryHostdefaultJSONResponse{Body: *apiError, StatusCode: http.StatusForbidden}, nil
	}
	if request.Body == nil {
		return hostapi.PreviewRetryHostdefaultJSONResponse{Body: hostError(ctx, errors.New("invalid request")), StatusCode: http.StatusBadRequest}, nil
	}
	action, err := handler.Service.PreviewRetryHost(ctx, resourceSubject(p), p.EnterpriseIDValue(), uuid.UUID(request.Id), hostCreateInput(*request.Body), request.Params.IdempotencyKey)
	if err != nil {
		return hostapi.PreviewRetryHostdefaultJSONResponse{Body: hostError(ctx, err), StatusCode: resourceStatus(err)}, nil
	}
	return hostapi.PreviewRetryHost201JSONResponse(pendingForHost(action)), nil
}
