package httpapi

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	hostapi "github.com/kakj-go/Argus/internal/gen/openapi/hostapi"
	"github.com/kakj-go/Argus/internal/hostremoval"
)

func (handler HostHandler) GetHostRemovalConnectionDefaults(ctx context.Context, request hostapi.GetHostRemovalConnectionDefaultsRequestObject) (hostapi.GetHostRemovalConnectionDefaultsResponseObject, error) {
	p, apiError := handler.auth(ctx, false, "", "host.manage")
	if apiError == nil {
		_, apiError = handler.auth(ctx, false, "", "secret.read")
	}
	if apiError != nil {
		return hostapi.GetHostRemovalConnectionDefaultsdefaultJSONResponse{Body: *apiError, StatusCode: http.StatusForbidden}, nil
	}
	value, err := handler.Removal.ConnectionDefaults(ctx, resourceSubject(p), p.EnterpriseIDValue(), hostremoval.PreviewInput{
		TargetType: string(request.Params.TargetType), TargetID: uuid.UUID(request.Params.TargetId), ExpectedVersion: request.Params.ExpectedVersion,
	})
	if err != nil {
		return hostapi.GetHostRemovalConnectionDefaultsdefaultJSONResponse{Body: hostError(ctx, err), StatusCode: hostRemovalStatus(err)}, nil
	}
	return hostapi.GetHostRemovalConnectionDefaults200JSONResponse{
		Username: value.Username, Status: hostapi.HostRemovalConnectionDefaultsStatus(value.Status), CredentialId: value.CredentialID, CredentialName: value.CredentialName,
	}, nil
}
