package httpapi

import (
	"context"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	api "github.com/kakj-go/Argus/internal/gen/openapi/kubernetesapi"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
)

func (h KubernetesHandler) logSessionActor(ctx context.Context, csrf string, mutate bool) (kubernetesworkbench.Actor, uuid.UUID, *api.ApiError) {
	p, f := h.Identity.enterprisePrincipalAny(ctx, mutate, csrf, "kubernetes.operational_logs.read")
	if f != nil {
		converted := dashboardConvert[api.ApiError](f)
		return kubernetesworkbench.Actor{}, uuid.Nil, &converted
	}
	if p.EnterpriseUser == nil || p.Session.ID == uuid.Nil {
		converted, _ := inventoryError(ctx, authorization.ErrAccessDenied)
		return kubernetesworkbench.Actor{}, uuid.Nil, &converted
	}
	return kubernetesworkbench.Actor{EnterpriseID: p.EnterpriseIDValue(), SubjectID: p.EnterpriseUser.ID, SubjectType: "user", AuthorizationVersion: p.AuthorizationVersion()}, p.Session.ID, nil
}
func (h KubernetesHandler) CreateKubernetesLogSession(ctx context.Context, r api.CreateKubernetesLogSessionRequestObject) (api.CreateKubernetesLogSessionResponseObject, error) {
	a, httpID, f := h.logSessionActor(ctx, r.Params.XCSRFToken, true)
	if f != nil {
		return api.CreateKubernetesLogSessiondefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	if r.Body == nil {
		body, status := inventoryError(ctx, kubernetesinventory.ErrInvalid)
		return api.CreateKubernetesLogSessiondefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	var in kubernetesworkbench.LogSessionInput
	in = dashboardConvert[kubernetesworkbench.LogSessionInput](r.Body)
	result, err := h.Workbench.CreateLogSession(ctx, a, httpID, r.Id, in)
	if err != nil {
		body, status := inventoryError(ctx, err)
		return api.CreateKubernetesLogSessiondefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return api.CreateKubernetesLogSession201JSONResponse{Body: dashboardConvert[api.KubernetesLogSessionTicket](result), Headers: api.CreateKubernetesLogSession201ResponseHeaders{CacheControl: "no-store"}}, nil
}
func (h KubernetesHandler) AttachKubernetesLogSession(ctx context.Context, r api.AttachKubernetesLogSessionRequestObject) (api.AttachKubernetesLogSessionResponseObject, error) {
	a, httpID, f := h.logSessionActor(ctx, r.Params.XCSRFToken, true)
	if f != nil {
		return api.AttachKubernetesLogSessiondefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	if r.Body == nil {
		body, status := inventoryError(ctx, kubernetesinventory.ErrInvalid)
		return api.AttachKubernetesLogSessiondefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	result, err := h.Workbench.AttachLogSession(ctx, a, httpID, r.Id, r.SessionId, r.Body.Ticket)
	if err != nil {
		body, status := inventoryError(ctx, err)
		return api.AttachKubernetesLogSessiondefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return api.AttachKubernetesLogSession200JSONResponse{Body: dashboardConvert[api.KubernetesLogSession](result), Headers: api.AttachKubernetesLogSession200ResponseHeaders{CacheControl: "no-store"}}, nil
}
func (h KubernetesHandler) ReadKubernetesLogSession(ctx context.Context, r api.ReadKubernetesLogSessionRequestObject) (api.ReadKubernetesLogSessionResponseObject, error) {
	a, httpID, f := h.logSessionActor(ctx, r.Params.XCSRFToken, true)
	if f != nil {
		return api.ReadKubernetesLogSessiondefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	if r.Body == nil {
		body, status := inventoryError(ctx, kubernetesinventory.ErrInvalid)
		return api.ReadKubernetesLogSessiondefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	cursor := ""
	if r.Body.Cursor != nil {
		cursor = *r.Body.Cursor
	}
	result, err := h.Workbench.ReadLogSession(ctx, a, httpID, r.Id, r.SessionId, cursor)
	if err != nil {
		body, status := inventoryError(ctx, err)
		return api.ReadKubernetesLogSessiondefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return api.ReadKubernetesLogSession200JSONResponse{Body: dashboardConvert[api.KubernetesLogSessionBatch](result), Headers: api.ReadKubernetesLogSession200ResponseHeaders{CacheControl: "no-store"}}, nil
}
func (h KubernetesHandler) CloseKubernetesLogSession(ctx context.Context, r api.CloseKubernetesLogSessionRequestObject) (api.CloseKubernetesLogSessionResponseObject, error) {
	a, httpID, f := h.logSessionActor(ctx, r.Params.XCSRFToken, true)
	if f != nil {
		return api.CloseKubernetesLogSessiondefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	result, err := h.Workbench.CloseLogSession(ctx, a, httpID, r.Id, r.SessionId)
	if err != nil {
		body, status := inventoryError(ctx, err)
		return api.CloseKubernetesLogSessiondefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return api.CloseKubernetesLogSession200JSONResponse{Body: dashboardConvert[api.KubernetesLogSession](result), Headers: api.CloseKubernetesLogSession200ResponseHeaders{CacheControl: "no-store"}}, nil
}

func (h KubernetesHandler) GetKubernetesLogSession(ctx context.Context, r api.GetKubernetesLogSessionRequestObject) (api.GetKubernetesLogSessionResponseObject, error) {
	a, httpID, f := h.logSessionActor(ctx, "", false)
	if f != nil {
		return api.GetKubernetesLogSessiondefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	result, err := h.Workbench.GetLogSession(ctx, a, httpID, r.Id, r.SessionId)
	if err != nil {
		body, status := inventoryError(ctx, err)
		return api.GetKubernetesLogSessiondefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return api.GetKubernetesLogSession200JSONResponse{Body: dashboardConvert[api.KubernetesLogSession](result), Headers: api.GetKubernetesLogSession200ResponseHeaders{CacheControl: "no-store"}}, nil
}
