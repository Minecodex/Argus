package httpapi

import (
	"context"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/gen/openapi/hostapi"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"net/http"
)

func (handler HostHandler) RefreshHostInventory(ctx context.Context, request hostapi.RefreshHostInventoryRequestObject) (hostapi.RefreshHostInventoryResponseObject, error) {
	p, apiError := handler.auth(ctx, true, request.Params.XCSRFToken, "host.read")
	if apiError != nil {
		return hostapi.RefreshHostInventorydefaultJSONResponse{Body: *apiError, StatusCode: http.StatusForbidden}, nil
	}
	id := uuid.UUID(request.Id)
	if err := handler.Service.RefreshHostInventory(ctx, resourceSubject(p), p.EnterpriseIDValue(), id, request.Params.IdempotencyKey); err != nil {
		return hostapi.RefreshHostInventorydefaultJSONResponse{Body: hostError(ctx, err), StatusCode: resourceStatus(err)}, nil
	}
	host, err := handler.Service.GetHost(ctx, p.EnterpriseIDValue(), id, p.AuthorizedResourceIDs)
	if err != nil {
		return hostapi.RefreshHostInventorydefaultJSONResponse{Body: hostError(ctx, err), StatusCode: resourceStatus(err)}, nil
	}
	runtime, err := handler.Queries.GetHostRuntimeObservation(ctx, db.GetHostRuntimeObservationParams{EnterpriseID: p.EnterpriseIDValue(), HostID: id})
	if err != nil {
		return hostapi.RefreshHostInventorydefaultJSONResponse{Body: hostError(ctx, err), StatusCode: resourceStatus(err)}, nil
	}
	onboarding := loadHostOnboarding(ctx, handler.Queries, p.EnterpriseIDValue(), []db.Host{host})
	return hostapi.RefreshHostInventory200JSONResponse(toHost(host, runtime, onboarding[id])), nil
}
