package httpapi

import (
	"context"
	"errors"

	api "github.com/kakj-go/Argus/internal/gen/openapi/kubernetesapi"
	"github.com/kakj-go/Argus/internal/kuberneteshelm"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
)

func (h KubernetesHandler) ListKubernetesHelmReleases(ctx context.Context, r api.ListKubernetesHelmReleasesRequestObject) (api.ListKubernetesHelmReleasesResponseObject, error) {
	a, f := h.inventoryActor(ctx)
	if f != nil {
		return api.ListKubernetesHelmReleasesdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	rows, e := h.Workbench.HelmReleases(ctx, a, r.Id, r.Params.NamespaceId, "", false)
	if e != nil {
		b, s := helmHTTPError(ctx, e)
		return api.ListKubernetesHelmReleasesdefaultJSONResponse{Body: b, StatusCode: s}, nil
	}
	return api.ListKubernetesHelmReleases200JSONResponse{Body: api.KubernetesHelmReleases{Items: dashboardConvert[[]api.KubernetesHelmRelease](rows)}, Headers: api.ListKubernetesHelmReleases200ResponseHeaders{CacheControl: "no-store"}}, nil
}
func (h KubernetesHandler) ListKubernetesHelmHistory(ctx context.Context, r api.ListKubernetesHelmHistoryRequestObject) (api.ListKubernetesHelmHistoryResponseObject, error) {
	a, f := h.inventoryActor(ctx)
	if f != nil {
		return api.ListKubernetesHelmHistorydefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	rows, e := h.Workbench.HelmReleases(ctx, a, r.Id, r.Params.NamespaceId, r.ReleaseName, true)
	if e != nil {
		b, s := helmHTTPError(ctx, e)
		return api.ListKubernetesHelmHistorydefaultJSONResponse{Body: b, StatusCode: s}, nil
	}
	return api.ListKubernetesHelmHistory200JSONResponse{Body: api.KubernetesHelmReleases{Items: dashboardConvert[[]api.KubernetesHelmRelease](rows)}, Headers: api.ListKubernetesHelmHistory200ResponseHeaders{CacheControl: "no-store"}}, nil
}
func (h KubernetesHandler) PreviewKubernetesHelm(ctx context.Context, r api.PreviewKubernetesHelmRequestObject) (api.PreviewKubernetesHelmResponseObject, error) {
	a, f := h.inventoryRequestActor(ctx, true, r.Params.XCSRFToken)
	if f != nil {
		return api.PreviewKubernetesHelmdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	if r.Body == nil {
		b, s := helmHTTPError(ctx, kuberneteshelm.ErrInvalid)
		return api.PreviewKubernetesHelmdefaultJSONResponse{Body: b, StatusCode: s}, nil
	}
	input := kubernetesworkbench.HelmInput{NamespaceID: r.Body.NamespaceId, Operation: string(r.Body.Operation), Name: r.Body.Name, ExpectedRevision: r.Body.ExpectedRevision, TimeoutSeconds: r.Body.TimeoutSeconds, Reason: r.Body.Reason}
	if r.Body.Source != nil {
		input.Source = dashboardConvert[*kuberneteshelm.ChartSource](r.Body.Source)
	}
	var material kuberneteshelm.Material
	if r.Body.SourceCredentials != nil {
		material.SourceCredentials = dashboardConvert[*kuberneteshelm.SourceCredentials](r.Body.SourceCredentials)
		*r.Body.SourceCredentials = api.KubernetesHelmSourceCredentials{}
		defer material.SourceCredentials.Clear()
	}
	if r.Body.RollbackRevision != nil {
		input.RollbackRevision = *r.Body.RollbackRevision
	}
	var chart, values []byte
	if r.Body.ChartArchive != nil {
		chart = *r.Body.ChartArchive
		defer clear(chart)
	}
	if r.Body.Values != nil {
		values = []byte(*r.Body.Values)
		*r.Body.Values = ""
		defer clear(values)
	}
	material.Chart, material.Values = chart, values
	result, e := h.Workbench.PreviewHelmProtected(ctx, a, r.Id, input, material, r.Params.IdempotencyKey)
	if e != nil {
		b, s := helmHTTPError(ctx, e)
		return api.PreviewKubernetesHelmdefaultJSONResponse{Body: b, StatusCode: s}, nil
	}
	return api.PreviewKubernetesHelm201JSONResponse{Body: convertPending[api.PendingActionPublicSchema](result), Headers: api.PreviewKubernetesHelm201ResponseHeaders{CacheControl: "no-store"}}, nil
}
func helmHTTPError(ctx context.Context, e error) (api.ApiError, int) {
	status, key := 503, "unavailable"
	switch {
	case errors.Is(e, kuberneteshelm.ErrSourceAuth):
		b := setupErrorBase(ctx, e)
		b.Code, b.MessageKey = kuberneteshelm.ErrorCode(e), "kubernetesHelm.errors.sourceAuth"
		return dashboardConvert[api.ApiError](b), 422
	case errors.Is(e, kuberneteshelm.ErrSourceIntegrity):
		b := setupErrorBase(ctx, e)
		b.Code, b.MessageKey = kuberneteshelm.ErrorCode(e), "kubernetesHelm.errors.sourceIntegrity"
		return dashboardConvert[api.ApiError](b), 409
	case errors.Is(e, kuberneteshelm.ErrSourceUnavailable):
		b := setupErrorBase(ctx, e)
		b.Code, b.MessageKey = kuberneteshelm.ErrorCode(e), "kubernetesHelm.errors.sourceUnavailable"
		b.Retryable = pointer(true)
		return dashboardConvert[api.ApiError](b), 503
	case errors.Is(e, kuberneteshelm.ErrInvalid):
		status, key = 400, "mutationInvalid"
	case errors.Is(e, kuberneteshelm.ErrConflict):
		status, key = 409, "mutationConflict"
	case errors.Is(e, kuberneteshelm.ErrDenied):
		status, key = 403, "forbidden"
	case errors.Is(e, kuberneteshelm.ErrBudget):
		status, key = 413, "budget"
	case errors.Is(e, kuberneteshelm.ErrUnknown):
		status, key = 409, "mutationConflict"
	default:
		return inventoryError(ctx, e)
	}
	b := setupErrorBase(ctx, e)
	b.Code = kuberneteshelm.ErrorCode(e)
	b.MessageKey = "kubernetesWorkbench.errors." + key
	b.Retryable = pointer(status == 503)
	return dashboardConvert[api.ApiError](b), status
}
