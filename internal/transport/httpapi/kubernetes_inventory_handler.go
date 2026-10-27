package httpapi

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	api "github.com/kakj-go/Argus/internal/gen/openapi/kubernetesapi"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetespodlogs"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/secret"
)

func (h KubernetesHandler) inventoryActor(ctx context.Context) (kubernetesworkbench.Actor, *api.ApiError) {
	return h.inventoryRequestActor(ctx, false, "")
}
func (h KubernetesHandler) inventoryRequestActor(ctx context.Context, csrfRequired bool, csrf string) (kubernetesworkbench.Actor, *api.ApiError) {
	p, f := h.Identity.enterprisePrincipalAny(ctx, csrfRequired, csrf, "kubernetes.read")
	if f != nil {
		value := dashboardConvert[api.ApiError](f)
		return kubernetesworkbench.Actor{}, &value
	}
	id, err := uuid.Parse(p.ActorID())
	if err != nil {
		value, _ := inventoryError(ctx, authorization.ErrAccessDenied)
		return kubernetesworkbench.Actor{}, &value
	}
	kind := "user"
	if p.ServiceAccount != nil {
		kind = "service_account"
	}
	return kubernetesworkbench.Actor{EnterpriseID: p.EnterpriseIDValue(), SubjectID: id, SubjectType: kind, AuthorizationVersion: p.AuthorizationVersion()}, nil
}

func (h KubernetesHandler) WatchKubernetesInventory(ctx context.Context, r api.WatchKubernetesInventoryRequestObject) (api.WatchKubernetesInventoryResponseObject, error) {
	a, f := h.inventoryRequestActor(ctx, true, r.Params.XCSRFToken)
	if f != nil {
		return api.WatchKubernetesInventorydefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	if r.Body == nil {
		body, status := inventoryError(ctx, kubernetesinventory.ErrInvalid)
		return api.WatchKubernetesInventorydefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	result, err := h.Workbench.Watch(ctx, a, r.Id, r.Body.Token)
	if err != nil {
		body, status := inventoryError(ctx, err)
		return api.WatchKubernetesInventorydefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return api.WatchKubernetesInventory200JSONResponse(dashboardConvert[api.KubernetesInventoryWatchResult](result)), nil
}
func inventoryError(ctx context.Context, cause error) (api.ApiError, int) {
	code := kubernetesinventory.ErrorCode(cause)
	httpStatus := 503
	key := "unavailable"
	retryable := true
	if errors.Is(cause, authorization.ErrAccessDenied) || errors.Is(cause, pagination.ErrAuthorizationVersionStale) {
		code = kubernetesinventory.ErrForbidden.Error()
	}
	if errors.Is(cause, pagination.ErrInvalid) {
		code = kubernetesinventory.ErrInvalid.Error()
	}
	if errors.Is(cause, pagination.ErrExpired) {
		code = kubernetesinventory.ErrExpired.Error()
	}
	if errors.Is(cause, secret.ErrKubernetesPrivateInput) {
		code = "KUBERNETES_PRIVATE_INPUT_UNAVAILABLE"
	}
	if errors.Is(cause, kubernetespodlogs.ErrGap) {
		code = kubernetespodlogs.ErrGap.Error()
	}
	switch code {
	case kubernetesinventory.ErrCRDHasInstances.Error():
		httpStatus, key, retryable = 409, "crdHasInstances", false
	case kubernetespodlogs.ErrGap.Error():
		httpStatus, key, retryable = 409, "logContinuityLost", false
	case "KUBERNETES_PRIVATE_INPUT_UNAVAILABLE":
		httpStatus, key, retryable = 409, "privateInputUnavailable", false
	case kubernetesinventory.ErrMutationConflict.Error():
		httpStatus, key, retryable = 409, "mutationConflict", false
	case kubernetesinventory.ErrDrainUnsafe.Error():
		httpStatus, key, retryable = 409, "drainUnsafe", false
	case kubernetesinventory.ErrMutationInvalid.Error():
		httpStatus, key, retryable = 400, "mutationInvalid", false
	case kubernetesinventory.ErrMutationUnknown.Error():
		httpStatus, key, retryable = 409, "mutationUnknown", false
	case kubernetesinventory.ErrInvalid.Error():
		httpStatus, key, retryable = 400, "invalid", false
	case kubernetesinventory.ErrUnsupported.Error():
		httpStatus, key, retryable = 409, "unsupported", false
	case kubernetesinventory.ErrIdentityChanged.Error():
		httpStatus, key, retryable = 409, "identityChanged", false
	case kubernetesinventory.ErrExpired.Error():
		httpStatus, key, retryable = 410, "expired", false
	case kubernetesinventory.ErrForbidden.Error():
		httpStatus, key, retryable = 403, "forbidden", false
	case kubernetesinventory.ErrBudget.Error():
		httpStatus, key, retryable = 413, "budget", false
	case kubernetesinventory.ErrProtocol.Error():
		httpStatus, key, retryable = 409, "readerVersion", false
	}
	value := setupErrorBase(ctx, cause)
	value.Code = code
	value.MessageKey = "kubernetesWorkbench.errors." + key
	value.Retryable = pointer(retryable)
	logMappedError(ctx, code, cause)
	return dashboardConvert[api.ApiError](value), httpStatus
}
func (h KubernetesHandler) ListKubernetesInventory(ctx context.Context, r api.ListKubernetesInventoryRequestObject) (api.ListKubernetesInventoryResponseObject, error) {
	a, f := h.inventoryActor(ctx)
	if f != nil {
		return api.ListKubernetesInventorydefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	in := kubernetesworkbench.ListInput{Type: r.Params.Type, Namespaces: kubernetesworkbench.Selection{Mode: "all_authorized"}}
	if r.Params.NamespaceMode != nil {
		in.Namespaces.Mode = string(*r.Params.NamespaceMode)
	}
	if r.Params.NamespaceIds != nil {
		in.Namespaces.IDs = append([]uuid.UUID{}, (*r.Params.NamespaceIds)...)
	}
	if r.Params.Name != nil {
		in.Name = *r.Params.Name
	}
	if r.Params.Labels != nil {
		in.Labels = *r.Params.Labels
	}
	if r.Params.Limit != nil {
		in.Limit = *r.Params.Limit
	}
	if r.Params.Cursor != nil {
		in.Cursor = *r.Params.Cursor
	}
	result, err := h.Workbench.List(ctx, a, r.Id, in)
	if err != nil {
		body, status := inventoryError(ctx, err)
		return api.ListKubernetesInventorydefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return api.ListKubernetesInventory200JSONResponse(dashboardConvert[api.KubernetesInventoryPage](result)), nil
}
func (h KubernetesHandler) GetKubernetesInventoryObject(ctx context.Context, r api.GetKubernetesInventoryObjectRequestObject) (api.GetKubernetesInventoryObjectResponseObject, error) {
	a, f := h.inventoryActor(ctx)
	if f != nil {
		return api.GetKubernetesInventoryObjectdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	in := kubernetesworkbench.ObjectInput{Type: r.Params.Type, Name: r.Params.Name, UID: r.Params.Uid}
	if r.Params.NamespaceId != nil {
		in.NamespaceID = *r.Params.NamespaceId
	}
	result, err := h.Workbench.Get(ctx, a, r.Id, in)
	if err != nil {
		body, status := inventoryError(ctx, err)
		return api.GetKubernetesInventoryObjectdefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return api.GetKubernetesInventoryObject200JSONResponse(dashboardConvert[api.KubernetesInventoryObject](result)), nil
}
func (h KubernetesHandler) GetKubernetesInventoryScope(ctx context.Context, r api.GetKubernetesInventoryScopeRequestObject) (api.GetKubernetesInventoryScopeResponseObject, error) {
	a, f := h.inventoryActor(ctx)
	if f != nil {
		return api.GetKubernetesInventoryScopedefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	result, err := h.Workbench.AccessScope(ctx, a, r.Id)
	if err != nil {
		body, status := inventoryError(ctx, err)
		return api.GetKubernetesInventoryScopedefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return api.GetKubernetesInventoryScope200JSONResponse(dashboardConvert[api.KubernetesInventoryScope](result)), nil
}
func (h KubernetesHandler) ListKubernetesInventoryTypes(ctx context.Context, r api.ListKubernetesInventoryTypesRequestObject) (api.ListKubernetesInventoryTypesResponseObject, error) {
	a, f := h.inventoryActor(ctx)
	if f != nil {
		return api.ListKubernetesInventoryTypesdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	catalog, err := h.Workbench.Types(ctx, a, r.Id)
	if err != nil {
		body, status := inventoryError(ctx, err)
		return api.ListKubernetesInventoryTypesdefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return api.ListKubernetesInventoryTypes200JSONResponse(dashboardConvert[api.KubernetesInventoryTypes](catalog)), nil
}
