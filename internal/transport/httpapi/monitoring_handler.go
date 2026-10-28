package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	api "github.com/kakj-go/Argus/internal/gen/openapi/monitoringapi"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/storage/postgres"
)

type MonitoringHandler struct {
	Identity EnterpriseIdentityHandler
	Service  monitoring.Service
}

func (h MonitoringHandler) auth(ctx context.Context, mutation bool, csrf string, permission string) (monitoring.Actor, *api.ApiError) {
	p, f := h.Identity.enterprisePrincipalAny(ctx, mutation, csrf, permission)
	if f != nil {
		v := dashboardConvert[api.ApiError](f)
		return monitoring.Actor{}, &v
	}
	id, err := uuid.Parse(p.ActorID())
	if err != nil {
		v := monitoringError(ctx, monitoring.ErrDenied)
		return monitoring.Actor{}, &v
	}
	kind := "user"
	if p.ServiceAccount != nil {
		kind = "service_account"
	}
	return monitoring.Actor{EnterpriseID: p.EnterpriseIDValue(), SubjectID: id, SubjectType: kind, AuthorizationVersion: p.AuthorizationVersion()}, nil
}

func monitoringStatus(err error) int {
	switch {
	case errors.Is(err, monitoring.ErrInvalid), errors.Is(err, pagination.ErrInvalid), errors.Is(err, pagination.ErrExpired), errors.Is(err, pagination.ErrAuthorizationVersionStale), errors.Is(err, monitoring.ErrCredential):
		return 400
	case errors.Is(err, monitoring.ErrDenied):
		return 403
	case errors.Is(err, monitoring.ErrNotFound):
		return 404
	case errors.Is(err, monitoring.ErrConflict), errors.Is(err, monitoring.ErrDependency), errors.Is(err, monitoring.ErrUnsupported), errors.Is(err, monitoring.ErrServiceLimit), errors.Is(err, monitoring.ErrLogOverlap), errors.Is(err, postgres.ErrIdempotencyConflict):
		return 409
	default:
		return 503
	}
}

func monitoringError(ctx context.Context, err error) api.ApiError {
	v := setupErrorBase(ctx, err)
	if stage := monitoring.PreparationStage(err); stage != "" {
		slog.WarnContext(ctx, "Monitoring preparation failed", "request_id", v.RequestId, "operation_stage", stage)
	}
	v.Code = "MONITORING_UNAVAILABLE"
	v.MessageKey = "monitoring.errors.unavailable"
	v.Retryable = pointer(true)
	for _, known := range []error{monitoring.ErrInvalid, monitoring.ErrNotFound, monitoring.ErrConflict, monitoring.ErrUnsupported, monitoring.ErrDependency, monitoring.ErrCredential, monitoring.ErrServiceLimit, monitoring.ErrLogOverlap} {
		if errors.Is(err, known) {
			v.Code = known.Error()
			v.MessageKey = "monitoring.errors." + strings.ToLower(strings.SplitN(v.Code, "_", 2)[1])
			v.Retryable = pointer(false)
			break
		}
	}
	if errors.Is(err, monitoring.ErrDenied) {
		v.Code = "MONITORING_DENIED"
		v.MessageKey = "monitoring.errors.denied"
		v.Retryable = pointer(false)
	}
	if errors.Is(err, postgres.ErrIdempotencyConflict) {
		v.Code = "MONITORING_VERSION_CONFLICT"
		v.MessageKey = "monitoring.errors.version_conflict"
		v.Retryable = pointer(false)
	}
	if errors.Is(err, pagination.ErrInvalid) || errors.Is(err, pagination.ErrExpired) || errors.Is(err, pagination.ErrAuthorizationVersionStale) {
		v.Code = "MONITORING_CURSOR_INVALID"
		v.MessageKey = "monitoring.errors.cursor_invalid"
		v.Retryable = pointer(false)
	}
	logMappedError(ctx, v.Code, err)
	return dashboardConvert[api.ApiError](v)
}

func (h MonitoringHandler) ListMonitoringApplications(ctx context.Context, r api.ListMonitoringApplicationsRequestObject) (api.ListMonitoringApplicationsResponseObject, error) {
	a, f := h.auth(ctx, false, "", "application.read")
	if f != nil {
		return api.ListMonitoringApplicationsdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	input := monitoring.ListInput{}
	if r.Params.Environment != nil {
		input.Environment = *r.Params.Environment
	}
	if r.Params.Search != nil {
		input.Search = *r.Params.Search
	}
	if r.Params.Cursor != nil {
		input.Cursor = *r.Params.Cursor
	}
	if r.Params.Limit != nil {
		input.Limit = *r.Params.Limit
	}
	if r.Params.Status != nil {
		input.Status = string(*r.Params.Status)
	}
	result, err := h.Service.Applications(ctx, a, input)
	if err != nil {
		return api.ListMonitoringApplicationsdefaultJSONResponse{Body: monitoringError(ctx, err), StatusCode: monitoringStatus(err)}, nil
	}
	return api.ListMonitoringApplications200JSONResponse(dashboardConvert[api.MonitoringApplicationPage](result)), nil
}

func (h MonitoringHandler) GetMonitoringApplication(ctx context.Context, r api.GetMonitoringApplicationRequestObject) (api.GetMonitoringApplicationResponseObject, error) {
	a, f := h.auth(ctx, false, "", "application.read")
	if f != nil {
		return api.GetMonitoringApplicationdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	result, err := h.Service.Application(ctx, a, r.Id)
	if err != nil {
		return api.GetMonitoringApplicationdefaultJSONResponse{Body: monitoringError(ctx, err), StatusCode: monitoringStatus(err)}, nil
	}
	return api.GetMonitoringApplication200JSONResponse(dashboardConvert[api.MonitoringApplication](result)), nil
}

func (h MonitoringHandler) ListMonitoringServices(ctx context.Context, r api.ListMonitoringServicesRequestObject) (api.ListMonitoringServicesResponseObject, error) {
	a, f := h.auth(ctx, false, "", "application.read")
	if f != nil {
		return api.ListMonitoringServicesdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	in := monitoring.ListInput{}
	if r.Params.Search != nil {
		in.Search = *r.Params.Search
	}
	if r.Params.Cursor != nil {
		in.Cursor = *r.Params.Cursor
	}
	if r.Params.Limit != nil {
		in.Limit = *r.Params.Limit
	}
	if r.Params.ApplicationIds != nil {
		in.ApplicationIDs = append([]uuid.UUID{}, (*r.Params.ApplicationIds)...)
	}
	if r.Params.ApplicationMode != nil {
		if *r.Params.ApplicationMode == api.Explicit && in.ApplicationIDs == nil {
			in.ApplicationIDs = []uuid.UUID{}
		}
		if *r.Params.ApplicationMode == api.AllAuthorized && in.ApplicationIDs != nil {
			return api.ListMonitoringServicesdefaultJSONResponse{Body: monitoringError(ctx, monitoring.ErrInvalid), StatusCode: 400}, nil
		}
	}
	result, err := h.Service.Services(ctx, a, in)
	if err != nil {
		return api.ListMonitoringServicesdefaultJSONResponse{Body: monitoringError(ctx, err), StatusCode: monitoringStatus(err)}, nil
	}
	return api.ListMonitoringServices200JSONResponse(dashboardConvert[api.MonitoringServicePage](result)), nil
}

func (h MonitoringHandler) PreviewMonitoringCommand(ctx context.Context, r api.PreviewMonitoringCommandRequestObject) (api.PreviewMonitoringCommandResponseObject, error) {
	permission := "application.manage"
	if r.Body != nil && string(r.Body.Kind) == "collection" {
		permission = "monitoring.manage"
	}
	a, f := h.auth(ctx, true, r.Params.XCSRFToken, permission)
	if f != nil {
		return api.PreviewMonitoringCommanddefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	if r.Body == nil {
		return api.PreviewMonitoringCommanddefaultJSONResponse{Body: monitoringError(ctx, monitoring.ErrInvalid), StatusCode: 400}, nil
	}
	var credentials json.RawMessage
	if r.Body.Credentials != nil {
		credentials, _ = json.Marshal(r.Body.Credentials)
	}
	defer clear(credentials)
	result, err := h.Service.PreviewWithCredentials(ctx, a, dashboardConvert[monitoring.Mutation](r.Body), credentials, r.Params.IdempotencyKey)
	if err != nil {
		return api.PreviewMonitoringCommanddefaultJSONResponse{Body: monitoringError(ctx, err), StatusCode: monitoringStatus(err)}, nil
	}
	return api.PreviewMonitoringCommand201JSONResponse(convertPending[api.PendingActionPublicSchema](result)), nil
}

func (h MonitoringHandler) ListMonitoringIntegrations(ctx context.Context, r api.ListMonitoringIntegrationsRequestObject) (api.ListMonitoringIntegrationsResponseObject, error) {
	a, f := h.auth(ctx, false, "", "monitoring.read")
	if f != nil {
		return api.ListMonitoringIntegrationsdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	result, err := h.Service.Integrations(ctx, a)
	if err != nil {
		return api.ListMonitoringIntegrationsdefaultJSONResponse{Body: monitoringError(ctx, err), StatusCode: monitoringStatus(err)}, nil
	}
	return api.ListMonitoringIntegrations200JSONResponse(dashboardConvert[[]api.MonitoringIntegration](result)), nil
}

func (h MonitoringHandler) ListMonitoringCollections(ctx context.Context, r api.ListMonitoringCollectionsRequestObject) (api.ListMonitoringCollectionsResponseObject, error) {
	a, f := h.auth(ctx, false, "", "monitoring.read")
	if f != nil {
		return api.ListMonitoringCollectionsdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	in := monitoringList(r.Params.Search, r.Params.Cursor, r.Params.Integration, r.Params.Limit)
	in.DesiredEnabled = r.Params.DesiredEnabled
	if r.Params.ApplyStatus != nil {
		in.ApplyStatus = string(*r.Params.ApplyStatus)
	}
	if r.Params.Health != nil {
		in.Health = string(*r.Params.Health)
	}
	if r.Params.Freshness != nil {
		in.Freshness = string(*r.Params.Freshness)
	}
	result, err := h.Service.Collections(ctx, a, in)
	if err != nil {
		return api.ListMonitoringCollectionsdefaultJSONResponse{Body: monitoringError(ctx, err), StatusCode: monitoringStatus(err)}, nil
	}
	return api.ListMonitoringCollections200JSONResponse(dashboardConvert[api.MonitoringCollectionPage](result)), nil
}

func (h MonitoringHandler) ListMonitoringTargets(ctx context.Context, r api.ListMonitoringTargetsRequestObject) (api.ListMonitoringTargetsResponseObject, error) {
	a, f := h.auth(ctx, false, "", "monitoring.read")
	if f != nil {
		return api.ListMonitoringTargetsdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	in := monitoringList(r.Params.Search, r.Params.Cursor, r.Params.Integration, r.Params.Limit)
	result, err := h.Service.Targets(ctx, a, in)
	if err != nil {
		return api.ListMonitoringTargetsdefaultJSONResponse{Body: monitoringError(ctx, err), StatusCode: monitoringStatus(err)}, nil
	}
	return api.ListMonitoringTargets200JSONResponse(dashboardConvert[api.MonitoringTargetPage](result)), nil
}

func (h MonitoringHandler) GetMonitoringCollection(ctx context.Context, r api.GetMonitoringCollectionRequestObject) (api.GetMonitoringCollectionResponseObject, error) {
	a, f := h.auth(ctx, false, "", "monitoring.read")
	if f != nil {
		return api.GetMonitoringCollectiondefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	result, err := h.Service.Collection(ctx, a, r.Id)
	if err != nil {
		return api.GetMonitoringCollectiondefaultJSONResponse{Body: monitoringError(ctx, err), StatusCode: monitoringStatus(err)}, nil
	}
	return api.GetMonitoringCollection200JSONResponse(dashboardConvert[api.MonitoringCollection](result)), nil
}

func monitoringList(search, cursor, integration *string, limit *int) monitoring.ListInput {
	in := monitoring.ListInput{}
	if search != nil {
		in.Search = *search
	}
	if cursor != nil {
		in.Cursor = *cursor
	}
	if integration != nil {
		in.Integration = *integration
	}
	if limit != nil {
		in.Limit = *limit
	}
	return in
}

func (h MonitoringHandler) ListMonitoringCollectorChoices(ctx context.Context, r api.ListMonitoringCollectorChoicesRequestObject) (api.ListMonitoringCollectorChoicesResponseObject, error) {
	a, f := h.auth(ctx, false, "", "collector.use")
	if f != nil {
		return api.ListMonitoringCollectorChoicesdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	result, err := h.Service.CollectorChoices(ctx, a, monitoringList(r.Params.Search, r.Params.Cursor, nil, r.Params.Limit))
	if err != nil {
		return api.ListMonitoringCollectorChoicesdefaultJSONResponse{Body: monitoringError(ctx, err), StatusCode: monitoringStatus(err)}, nil
	}
	return api.ListMonitoringCollectorChoices200JSONResponse(dashboardConvert[api.MonitoringCollectorChoicePage](result)), nil
}
