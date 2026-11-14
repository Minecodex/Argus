package httpapi

import (
	"context"
	api "github.com/kakj-go/Argus/internal/gen/openapi/kubernetesapi"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetesmetrics"
)

func (h KubernetesHandler) PrepareKubernetesListMetrics(ctx context.Context, r api.PrepareKubernetesListMetricsRequestObject) (api.PrepareKubernetesListMetricsResponseObject, error) {
	actor, failure := h.inventoryActor(ctx)
	if failure != nil {
		return api.PrepareKubernetesListMetricsdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	if r.Body == nil {
		body, status := inventoryError(ctx, kubernetesinventory.ErrInvalid)
		return api.PrepareKubernetesListMetricsdefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	if h.Metrics == nil {
		body, status := inventoryError(ctx, kubernetesinventory.ErrUnavailable)
		return api.PrepareKubernetesListMetricsdefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	input := dashboardConvert[kubernetesmetrics.PrepareInput](*r.Body)
	value, err := h.Metrics.Prepare(ctx, actor, r.Id, input)
	if err != nil {
		body, status := inventoryError(ctx, err)
		return api.PrepareKubernetesListMetricsdefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return api.PrepareKubernetesListMetrics200JSONResponse(dashboardConvert[api.KubernetesMetricContext](value)), nil
}
func (h KubernetesHandler) ReadKubernetesListMetrics(ctx context.Context, r api.ReadKubernetesListMetricsRequestObject) (api.ReadKubernetesListMetricsResponseObject, error) {
	actor, failure := h.inventoryActor(ctx)
	if failure != nil {
		return api.ReadKubernetesListMetricsdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	if r.Body == nil {
		body, status := inventoryError(ctx, kubernetesinventory.ErrInvalid)
		return api.ReadKubernetesListMetricsdefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	if h.Metrics == nil {
		body, status := inventoryError(ctx, kubernetesinventory.ErrUnavailable)
		return api.ReadKubernetesListMetricsdefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	// Preserve the service-account tool allowlist used by normal telemetry reads.
	_, _, denied := (TelemetryHandler{Identity: h.Identity, Service: h.Metrics.Telemetry}).queryActor(ctx, "telemetry.promql.query")
	if denied != nil {
		return api.ReadKubernetesListMetricsdefaultJSONResponse{Body: dashboardConvert[api.ApiError](*denied), StatusCode: telemetryAuthStatus(*denied)}, nil
	}
	value, err := h.Metrics.Read(ctx, actor, r.Id, kubernetesmetrics.ReadInput{Token: r.Body.Token, Metric: string(r.Body.Metric)})
	if err != nil {
		body, status := inventoryError(ctx, err)
		return api.ReadKubernetesListMetricsdefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return api.ReadKubernetesListMetrics200JSONResponse(dashboardConvert[api.KubernetesMetricResult](value)), nil
}
