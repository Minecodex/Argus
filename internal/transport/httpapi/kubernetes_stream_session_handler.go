package httpapi

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	api "github.com/kakj-go/Argus/internal/gen/openapi/kubernetesapi"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
)

func (h KubernetesHandler) streamActor(ctx context.Context, csrf string, mutate bool) (kubernetesworkbench.Actor, uuid.UUID, *api.ApiError) {
	p, f := h.Identity.enterprisePrincipalAny(ctx, mutate, csrf, "kubernetes.read")
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
func (h KubernetesHandler) streamTicketURL(ctx context.Context, v kubernetesworkbench.StreamTicket) kubernetesworkbench.StreamTicket {
	base := h.WebsocketURL
	if m, ok := RequestFromContext(ctx); ok && m.Request != nil && isLoopbackHost(m.Request.Host) {
		scheme := "ws"
		if strings.EqualFold(m.Request.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "wss"
		}
		base = scheme + "://" + m.Request.Host
	}
	v.WebsocketURL = strings.TrimRight(base, "/") + "/v1/kubernetes/sessions/" + v.Session.ID.String()
	return v
}
func (h KubernetesHandler) CreateKubernetesStreamSession(ctx context.Context, r api.CreateKubernetesStreamSessionRequestObject) (api.CreateKubernetesStreamSessionResponseObject, error) {
	a, httpID, f := h.streamActor(ctx, r.Params.XCSRFToken, true)
	if f != nil {
		return api.CreateKubernetesStreamSessiondefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	if r.Body == nil {
		b, c := inventoryError(ctx, kubernetesinventory.ErrInvalid)
		return api.CreateKubernetesStreamSessiondefaultJSONResponse{Body: b, StatusCode: c}, nil
	}
	v, e := h.Workbench.CreateStreamSession(ctx, a, httpID, r.Id, dashboardConvert[kubernetesworkbench.StreamInput](r.Body))
	if e != nil {
		b, c := inventoryError(ctx, e)
		return api.CreateKubernetesStreamSessiondefaultJSONResponse{Body: b, StatusCode: c}, nil
	}
	return api.CreateKubernetesStreamSession201JSONResponse{Body: dashboardConvert[api.KubernetesStreamTicket](h.streamTicketURL(ctx, v)), Headers: api.CreateKubernetesStreamSession201ResponseHeaders{CacheControl: "no-store"}}, nil
}
func (h KubernetesHandler) GetKubernetesStreamSession(ctx context.Context, r api.GetKubernetesStreamSessionRequestObject) (api.GetKubernetesStreamSessionResponseObject, error) {
	a, httpID, f := h.streamActor(ctx, "", false)
	if f != nil {
		return api.GetKubernetesStreamSessiondefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	v, e := h.Workbench.GetStreamSession(ctx, a, httpID, r.Id, r.SessionId)
	if e != nil {
		b, c := inventoryError(ctx, e)
		return api.GetKubernetesStreamSessiondefaultJSONResponse{Body: b, StatusCode: c}, nil
	}
	return api.GetKubernetesStreamSession200JSONResponse{Body: dashboardConvert[api.KubernetesStreamSession](v), Headers: api.GetKubernetesStreamSession200ResponseHeaders{CacheControl: "no-store"}}, nil
}
func (h KubernetesHandler) CloseKubernetesStreamSession(ctx context.Context, r api.CloseKubernetesStreamSessionRequestObject) (api.CloseKubernetesStreamSessionResponseObject, error) {
	a, httpID, f := h.streamActor(ctx, r.Params.XCSRFToken, true)
	if f != nil {
		return api.CloseKubernetesStreamSessiondefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	v, e := h.Workbench.CloseStreamSession(ctx, a, httpID, r.Id, r.SessionId)
	if e != nil {
		b, c := inventoryError(ctx, e)
		return api.CloseKubernetesStreamSessiondefaultJSONResponse{Body: b, StatusCode: c}, nil
	}
	return api.CloseKubernetesStreamSession200JSONResponse{Body: dashboardConvert[api.KubernetesStreamSession](v), Headers: api.CloseKubernetesStreamSession200ResponseHeaders{CacheControl: "no-store"}}, nil
}
func (h KubernetesHandler) IssueKubernetesStreamTicket(ctx context.Context, r api.IssueKubernetesStreamTicketRequestObject) (api.IssueKubernetesStreamTicketResponseObject, error) {
	a, httpID, f := h.streamActor(ctx, r.Params.XCSRFToken, true)
	if f != nil {
		return api.IssueKubernetesStreamTicketdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	v, e := h.Workbench.IssueStreamTicket(ctx, a, httpID, r.Id, r.SessionId)
	if e != nil {
		b, c := inventoryError(ctx, e)
		return api.IssueKubernetesStreamTicketdefaultJSONResponse{Body: b, StatusCode: c}, nil
	}
	return api.IssueKubernetesStreamTicket200JSONResponse{Body: dashboardConvert[api.KubernetesStreamTicket](h.streamTicketURL(ctx, v)), Headers: api.IssueKubernetesStreamTicket200ResponseHeaders{CacheControl: "no-store"}}, nil
}
