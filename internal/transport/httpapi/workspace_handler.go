package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/conversation"
	api "github.com/kakj-go/Argus/internal/gen/openapi/workspaceapi"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"github.com/kakj-go/Argus/internal/workspace"
)

type WorkspaceHandler struct {
	Identity EnterpriseIdentityHandler
	Service  workspace.Service
}

func (handler WorkspaceHandler) auth(ctx context.Context, conversationID uuid.UUID, mutation bool, csrf string) (toolruntime.Principal, *api.ApiError) {
	p, failure := handler.Identity.enterprisePrincipal(ctx, mutation, csrf, "workspace.use")
	if failure != nil {
		data, _ := json.Marshal(failure)
		var value api.ApiError
		_ = json.Unmarshal(data, &value)
		return toolruntime.Principal{}, &value
	}
	principal, err := (conversation.Service{Store: handler.Service.Store}).ToolPrincipal(ctx, p.EnterpriseIDValue(), uuid.MustParse(p.ActorID()), conversationID)
	if err == nil {
		err = handler.Service.Authorize(ctx, principal)
	}
	if err != nil {
		value := planV5Error[api.ApiError](ctx, err)
		return toolruntime.Principal{}, &value
	}
	return principal, nil
}

func workspaceView(value db.Workspace) api.Workspace {
	return api.Workspace{Id: value.ID, ConversationId: value.ConversationID, CapacityBytes: value.CapacityBytes, Status: api.WorkspaceStatus(value.Status), Version: value.Version, CreatedAt: value.CreatedAt.Time}
}
func workspaceFileView(value db.WorkspaceFile) api.WorkspaceFile {
	return api.WorkspaceFile{Id: value.ID, Name: value.Name, Path: "/workspace/" + value.Path, ByteSize: value.ByteSize, ContentHash: value.ContentHash, MediaType: value.MediaType, CreatedAt: value.CreatedAt.Time}
}

func (handler WorkspaceHandler) GetConversationWorkspace(ctx context.Context, r api.GetConversationWorkspaceRequestObject) (api.GetConversationWorkspaceResponseObject, error) {
	p, failure := handler.auth(ctx, r.ConversationId, false, "")
	if failure != nil {
		return api.GetConversationWorkspacedefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	value, err := handler.Service.Get(ctx, p)
	if err != nil {
		return api.GetConversationWorkspacedefaultJSONResponse{Body: planV5Error[api.ApiError](ctx, err), StatusCode: planV5Status(err)}, nil
	}
	return api.GetConversationWorkspace200JSONResponse(workspaceView(value)), nil
}
func (handler WorkspaceHandler) DeleteConversationWorkspace(ctx context.Context, r api.DeleteConversationWorkspaceRequestObject) (api.DeleteConversationWorkspaceResponseObject, error) {
	p, failure := handler.auth(ctx, r.ConversationId, true, r.Params.XCSRFToken)
	if failure != nil {
		return api.DeleteConversationWorkspacedefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	value, err := handler.Service.Delete(ctx, p, r.Params.IdempotencyKey)
	if err != nil {
		return api.DeleteConversationWorkspacedefaultJSONResponse{Body: planV5Error[api.ApiError](ctx, err), StatusCode: planV5Status(err)}, nil
	}
	return api.DeleteConversationWorkspace202JSONResponse(workspaceView(value)), nil
}
func (handler WorkspaceHandler) ListWorkspaceFiles(ctx context.Context, r api.ListWorkspaceFilesRequestObject) (api.ListWorkspaceFilesResponseObject, error) {
	p, failure := handler.auth(ctx, r.ConversationId, false, "")
	if failure != nil {
		return api.ListWorkspaceFilesdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	values, err := handler.Service.ListFiles(ctx, p)
	if err != nil {
		return api.ListWorkspaceFilesdefaultJSONResponse{Body: planV5Error[api.ApiError](ctx, err), StatusCode: planV5Status(err)}, nil
	}
	items := make([]api.WorkspaceFile, 0, len(values))
	for _, value := range values {
		items = append(items, workspaceFileView(value))
	}
	return api.ListWorkspaceFiles200JSONResponse{Items: items}, nil
}
func (handler WorkspaceHandler) CreateWorkspaceUpload(ctx context.Context, r api.CreateWorkspaceUploadRequestObject) (api.CreateWorkspaceUploadResponseObject, error) {
	p, failure := handler.auth(ctx, r.ConversationId, true, r.Params.XCSRFToken)
	if failure != nil {
		return api.CreateWorkspaceUploaddefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	value, err := handler.Service.CreateUpload(ctx, p, r.Body.Name, r.Body.ByteSize, r.Params.IdempotencyKey)
	if err != nil {
		return api.CreateWorkspaceUploaddefaultJSONResponse{Body: planV5Error[api.ApiError](ctx, err), StatusCode: planV5Status(err)}, nil
	}
	return api.CreateWorkspaceUpload201JSONResponse{Id: value.ID, Name: value.Name, ByteSize: value.ExpectedBytes, Status: api.WorkspaceUploadStatus(value.Status), ExpiresAt: value.ExpiresAt.Time}, nil
}
func (handler WorkspaceHandler) UploadWorkspaceContent(ctx context.Context, r api.UploadWorkspaceContentRequestObject) (api.UploadWorkspaceContentResponseObject, error) {
	p, failure := handler.auth(ctx, r.ConversationId, true, r.Params.XCSRFToken)
	if failure != nil {
		return api.UploadWorkspaceContentdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	value, err := handler.Service.Upload(ctx, p, r.UploadId, r.Body)
	if err != nil {
		return api.UploadWorkspaceContentdefaultJSONResponse{Body: planV5Error[api.ApiError](ctx, err), StatusCode: planV5Status(err)}, nil
	}
	return api.UploadWorkspaceContent201JSONResponse(workspaceFileView(value)), nil
}
func (handler WorkspaceHandler) DownloadWorkspaceFile(ctx context.Context, r api.DownloadWorkspaceFileRequestObject) (api.DownloadWorkspaceFileResponseObject, error) {
	p, failure := handler.auth(ctx, r.ConversationId, false, "")
	if failure != nil {
		return api.DownloadWorkspaceFiledefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	reader, size, name, err := handler.Service.ReadFile(ctx, p, r.FileId, 0, 0)
	if err != nil {
		return api.DownloadWorkspaceFiledefaultJSONResponse{Body: planV5Error[api.ApiError](ctx, err), StatusCode: planV5Status(err)}, nil
	}
	return fileContentResponse{reader: reader, size: size, name: name, requestedRange: r.Params.Range}, nil
}
func (handler WorkspaceHandler) DownloadFileDelivery(ctx context.Context, r api.DownloadFileDeliveryRequestObject) (api.DownloadFileDeliveryResponseObject, error) {
	p, failure := handler.auth(ctx, r.ConversationId, false, "")
	if failure != nil {
		return api.DownloadFileDeliverydefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	reader, file, err := handler.Service.ReadDelivery(ctx, p, r.DeliveryId, 0, 0)
	if err != nil {
		return api.DownloadFileDeliverydefaultJSONResponse{Body: planV5Error[api.ApiError](ctx, err), StatusCode: planV5Status(err)}, nil
	}
	return fileContentResponse{reader: reader, size: file.ByteSize, name: file.Name, hash: file.ContentHash, requestedRange: r.Params.Range}, nil
}
func workspaceRequestContext(next api.StrictHandlerFunc, _ string) api.StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, value any) (any, error) {
		return next(WithRequestContext(ctx, w, r), w, r, value)
	}
}
