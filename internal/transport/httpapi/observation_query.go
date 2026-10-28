package httpapi

import (
	"context"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/dashboard"
	api "github.com/kakj-go/Argus/internal/gen/openapi/telemetryapi"
	"github.com/kakj-go/Argus/internal/observationscope"
	"github.com/kakj-go/Argus/internal/telemetry"
)

func (h TelemetryHandler) ListObservationQueryResources(ctx context.Context, r api.ListObservationQueryResourcesRequestObject) (api.ListObservationQueryResourcesResponseObject, error) {
	a, _, failure := h.queryActor(ctx, "")
	if failure != nil {
		return api.ListObservationQueryResourcesdefaultJSONResponse{Body: *failure, StatusCode: telemetryAuthStatus(*failure)}, nil
	}
	in := observationscope.DirectoryInput{Kind: string(r.Params.Kind)}
	if r.Params.Search != nil {
		in.Search = *r.Params.Search
	}
	if r.Params.Cursor != nil {
		in.Cursor = *r.Params.Cursor
	}
	if r.Params.Limit != nil {
		in.Limit = *r.Params.Limit
	}
	actor := authorization.Actor{EnterpriseID: a.EnterpriseID, SubjectID: a.SubjectID, SubjectType: a.SubjectType, AuthorizationVersion: a.AuthorizationVersion}
	result, err := (observationscope.Resolver{Queries: h.Service.Store.Queries}).ResourceDirectory(ctx, actor, h.Service.Actions.Key, in)
	if err != nil {
		return api.ListObservationQueryResourcesdefaultJSONResponse{Body: telemetryError(ctx, observationError(err)), StatusCode: telemetryStatus(observationError(err))}, nil
	}
	return api.ListObservationQueryResources200JSONResponse(dashboardConvert[api.ObservationResourcePage](result)), nil
}

func (h TelemetryHandler) QueryObservationCatalog(ctx context.Context, r api.QueryObservationCatalogRequestObject) (api.QueryObservationCatalogResponseObject, error) {
	if r.Body == nil {
		return api.QueryObservationCatalogdefaultJSONResponse{Body: telemetryError(ctx, telemetry.ErrQueryInvalid), StatusCode: 400}, nil
	}
	tool := map[string]string{"metrics": "telemetry.promql.query", "logs": "telemetry.kql.query", "traces": "telemetry.skywalking.trace"}[string(r.Body.Signal)]
	a, _, failure := h.queryActor(ctx, tool)
	if failure != nil {
		return api.QueryObservationCatalogdefaultJSONResponse{Body: *failure, StatusCode: telemetryAuthStatus(*failure)}, nil
	}
	result, err := h.Service.ObservationCatalog(ctx, a, dashboardConvert[telemetry.ObservationCatalogInput](r.Body), dashboard.ObservationRecipes())
	if err != nil {
		return api.QueryObservationCatalogdefaultJSONResponse{Body: telemetryError(ctx, err), StatusCode: telemetryStatus(err)}, nil
	}
	return api.QueryObservationCatalog200JSONResponse(dashboardConvert[api.ObservationCatalogResult](result)), nil
}

func observationError(err error) error {
	if err == observationscope.ErrInvalid {
		return telemetry.ErrQueryInvalid
	}
	if err == observationscope.ErrDenied {
		return telemetry.ErrDenied
	}
	return err
}
func legacyQueryResources(value []uuid.UUID) []uuid.UUID {
	if value == nil {
		return nil
	}
	return append([]uuid.UUID{}, value...)
}
func structuredQueryScope(value *api.TelemetryQueryScope) *telemetry.QueryScope {
	if value == nil {
		return nil
	}
	out := dashboardConvert[telemetry.QueryScope](value)
	return &out
}
