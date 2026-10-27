package httpapi

import (
	"context"
	"encoding/json"
	api "github.com/kakj-go/Argus/internal/gen/openapi/kubernetesapi"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
)

func (h KubernetesHandler) CancelKubernetesDrain(ctx context.Context, r api.CancelKubernetesDrainRequestObject) (api.CancelKubernetesDrainResponseObject, error) {
	a, f := h.inventoryRequestActor(ctx, true, r.Params.XCSRFToken)
	if f != nil {
		return api.CancelKubernetesDraindefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	if r.Body == nil {
		body, status := inventoryError(ctx, kubernetesinventory.ErrInvalid)
		return api.CancelKubernetesDraindefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	result, err := h.Workbench.CancelDrain(ctx, a, r.Id, r.OperationId, r.Body.ExpectedVersion)
	if err != nil {
		body, status := inventoryError(ctx, err)
		return api.CancelKubernetesDraindefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return api.CancelKubernetesDrain200JSONResponse(dashboardConvert[api.KubernetesOperation](result)), nil
}

func (h KubernetesHandler) PreviewKubernetesMutation(ctx context.Context, r api.PreviewKubernetesMutationRequestObject) (api.PreviewKubernetesMutationResponseObject, error) {
	a, f := h.inventoryRequestActor(ctx, true, r.Params.XCSRFToken)
	if f != nil {
		return api.PreviewKubernetesMutationdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	if r.Body == nil {
		body, status := inventoryError(ctx, kubernetesinventory.ErrInvalid)
		return api.PreviewKubernetesMutationdefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	publicInput := *r.Body
	publicInput.SecretValues = nil
	input := dashboardConvert[kubernetesworkbench.MutationInput](publicInput)
	input.ClusterID = r.Id
	var values []byte
	if r.Body.SecretValues != nil {
		values, _ = json.Marshal(*r.Body.SecretValues)
		defer clear(values)
		defer clear(*r.Body.SecretValues)
	}
	result, err := h.Workbench.PreviewMutationWithSecretValues(ctx, a, input, values, r.Params.IdempotencyKey)
	if err != nil {
		body, status := inventoryError(ctx, err)
		return api.PreviewKubernetesMutationdefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return api.PreviewKubernetesMutation201JSONResponse(convertPending[api.PendingActionPublicSchema](result)), nil
}
func (h KubernetesHandler) GetKubernetesOperation(ctx context.Context, r api.GetKubernetesOperationRequestObject) (api.GetKubernetesOperationResponseObject, error) {
	a, f := h.inventoryActor(ctx)
	if f != nil {
		return api.GetKubernetesOperationdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	result, err := h.Workbench.GetOperation(ctx, a, r.Id, r.OperationId)
	if err != nil {
		body, status := inventoryError(ctx, err)
		return api.GetKubernetesOperationdefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return api.GetKubernetesOperation200JSONResponse(dashboardConvert[api.KubernetesOperation](result)), nil
}
func (h KubernetesHandler) ListKubernetesOperations(ctx context.Context, r api.ListKubernetesOperationsRequestObject) (api.ListKubernetesOperationsResponseObject, error) {
	a, f := h.inventoryActor(ctx)
	if f != nil {
		return api.ListKubernetesOperationsdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	limit, cursor := 50, ""
	if r.Params.Limit != nil {
		limit = *r.Params.Limit
	}
	if r.Params.Cursor != nil {
		cursor = *r.Params.Cursor
	}
	result, err := h.Workbench.ListOperations(ctx, a, r.Id, limit, cursor)
	if err != nil {
		body, status := inventoryError(ctx, err)
		return api.ListKubernetesOperationsdefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return api.ListKubernetesOperations200JSONResponse(dashboardConvert[api.KubernetesOperationPage](result)), nil
}
