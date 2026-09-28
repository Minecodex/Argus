package httpapi

import (
	"context"
	"github.com/google/uuid"
	api "github.com/kakj-go/Argus/internal/gen/openapi/telemetryapi"
	"github.com/kakj-go/Argus/internal/identity"
	"github.com/kakj-go/Argus/internal/telemetry"
	"slices"
)

func (handler TelemetryHandler) queryActor(ctx context.Context, toolID string) (telemetry.Actor, identity.Principal, *api.ApiError) {
	principal, failure := handler.Identity.enterprisePrincipalAny(ctx, false, "", "host.read", "kubernetes.read")
	if failure != nil {
		value := dashboardConvert[api.ApiError](failure)
		return telemetry.Actor{}, principal, &value
	}
	kind := "user"
	if account := principal.ServiceAccount; account != nil {
		kind = "service_account"
		if !slices.Contains(account.AllowedToolIds, toolID) {
			value := telemetryError(ctx, telemetry.ErrDenied)
			return telemetry.Actor{}, principal, &value
		}
	}
	id, err := uuid.Parse(principal.ActorID())
	if err != nil {
		value := telemetryError(ctx, telemetry.ErrDenied)
		return telemetry.Actor{}, principal, &value
	}
	actor, err := handler.Service.ResourceQueryActor(ctx, principal.EnterpriseIDValue(), id, kind, principal.AuthorizationVersion())
	if err != nil {
		value := telemetryError(ctx, err)
		return telemetry.Actor{}, principal, &value
	}
	return actor, principal, nil
}
