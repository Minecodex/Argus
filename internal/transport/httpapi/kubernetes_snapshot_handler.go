package httpapi

import (
	"context"
	"github.com/google/uuid"
	api "github.com/kakj-go/Argus/internal/gen/openapi/kubernetesapi"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
)

func (h KubernetesHandler) GetKubernetesResourceSnapshot(ctx context.Context, r api.GetKubernetesResourceSnapshotRequestObject) (api.GetKubernetesResourceSnapshotResponseObject, error) {
	actor, failure := h.inventoryActor(ctx)
	if failure != nil {
		return api.GetKubernetesResourceSnapshotdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	input := kubernetesworkbench.SnapshotInput{Namespaces: kubernetesworkbench.Selection{Mode: "all_authorized"}}
	if r.Params.Types != nil {
		input.Types = append([]string{}, (*r.Params.Types)...)
	}
	if r.Params.NamespaceMode != nil {
		input.Namespaces.Mode = string(*r.Params.NamespaceMode)
	}
	if r.Params.NamespaceIds != nil {
		input.Namespaces.IDs = append([]uuid.UUID{}, (*r.Params.NamespaceIds)...)
	}
	if r.Params.MaxItems != nil {
		input.MaxItems = *r.Params.MaxItems
	}
	value, err := h.Workbench.Snapshot(ctx, actor, r.Id, input)
	if err != nil {
		body, status := inventoryError(ctx, err)
		return api.GetKubernetesResourceSnapshotdefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return api.GetKubernetesResourceSnapshot200JSONResponse(dashboardConvert[api.KubernetesResourceSnapshot](value)), nil
}

func (h KubernetesHandler) GetKubernetesRelatedResources(ctx context.Context, r api.GetKubernetesRelatedResourcesRequestObject) (api.GetKubernetesRelatedResourcesResponseObject, error) {
	actor, failure := h.inventoryActor(ctx)
	if failure != nil {
		return api.GetKubernetesRelatedResourcesdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	in := kubernetesworkbench.RelatedInput{ObjectInput: kubernetesworkbench.ObjectInput{Type: r.Params.Type, Name: r.Params.Name, UID: r.Params.Uid}, View: string(r.Params.View)}
	if r.Params.NamespaceId != nil {
		in.NamespaceID = *r.Params.NamespaceId
	}
	if r.Params.Limit != nil {
		in.Limit = *r.Params.Limit
	}
	if r.Params.Cursor != nil {
		in.Cursor = *r.Params.Cursor
	}
	page, err := h.Workbench.Related(ctx, actor, r.Id, in)
	if err != nil {
		body, status := inventoryError(ctx, err)
		return api.GetKubernetesRelatedResourcesdefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return api.GetKubernetesRelatedResources200JSONResponse(dashboardConvert[api.KubernetesRelatedPage](page)), nil
}
