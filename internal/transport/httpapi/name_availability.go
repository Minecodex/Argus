package httpapi

import (
	"context"
	"errors"
	"net/http"

	connectorapi "github.com/kakj-go/Argus/internal/gen/openapi/connectorapi"
	hostapi "github.com/kakj-go/Argus/internal/gen/openapi/hostapi"
	"github.com/kakj-go/Argus/internal/resource"
)

func (handler HostHandler) CheckHostNameAvailability(ctx context.Context, request hostapi.CheckHostNameAvailabilityRequestObject) (hostapi.CheckHostNameAvailabilityResponseObject, error) {
	p, apiError := handler.auth(ctx, false, "", "host.manage")
	if apiError != nil {
		return hostapi.CheckHostNameAvailabilitydefaultJSONResponse{Body: *apiError, StatusCode: http.StatusForbidden}, nil
	}
	available, err := handler.Service.HostNameAvailable(ctx, p.EnterpriseIDValue(), request.Params.Name)
	if err != nil {
		return hostapi.CheckHostNameAvailabilitydefaultJSONResponse{Body: hostError(ctx, err), StatusCode: nameAvailabilityStatus(err)}, nil
	}
	cacheControl := "no-store"
	return hostapi.CheckHostNameAvailability200JSONResponse{
		Body:    hostapi.ResourceNameAvailability{Available: available},
		Headers: hostapi.CheckHostNameAvailability200ResponseHeaders{CacheControl: &cacheControl},
	}, nil
}

func (handler ConnectorHandler) CheckBastionNameAvailability(ctx context.Context, request connectorapi.CheckBastionNameAvailabilityRequestObject) (connectorapi.CheckBastionNameAvailabilityResponseObject, error) {
	p, apiError := handler.auth(ctx, false, "", "bastion_scope.manage")
	if apiError != nil {
		return connectorapi.CheckBastionNameAvailabilitydefaultJSONResponse{Body: *apiError, StatusCode: http.StatusForbidden}, nil
	}
	available, err := handler.Bastion.NameAvailable(ctx, p.EnterpriseIDValue(), request.Params.Name)
	if err != nil {
		return connectorapi.CheckBastionNameAvailabilitydefaultJSONResponse{Body: connectorError(ctx, err), StatusCode: nameAvailabilityStatus(err)}, nil
	}
	cacheControl := "no-store"
	return connectorapi.CheckBastionNameAvailability200JSONResponse{
		Body:    connectorapi.ResourceNameAvailability{Available: available},
		Headers: connectorapi.CheckBastionNameAvailability200ResponseHeaders{CacheControl: &cacheControl},
	}, nil
}

func nameAvailabilityStatus(err error) int {
	if errors.Is(err, resource.ErrInvalidResourceName) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}
