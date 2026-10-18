package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"

	"github.com/google/uuid"
	remoteapi "github.com/kakj-go/Argus/internal/gen/openapi/remoteexecutionapi"
	"github.com/kakj-go/Argus/internal/remoteexecution"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type RemoteExecutionHandler struct {
	Identity EnterpriseIdentityHandler
	Service  *remoteexecution.Service
}

func (h RemoteExecutionHandler) ReadRemoteExecutionInput(ctx context.Context, r remoteapi.ReadRemoteExecutionInputRequestObject) (remoteapi.ReadRemoteExecutionInputResponseObject, error) {
	p, e := h.auth(ctx, r.ConversationId, false, "")
	if e != nil {
		return remoteapi.ReadRemoteExecutionInputdefaultJSONResponse{Body: *e, StatusCode: 403}, nil
	}
	reader, err := h.Service.ReadInput(ctx, p, r.ExecutionId, r.FileId)
	if err != nil {
		body, status := remoteFailure(ctx, err)
		return remoteapi.ReadRemoteExecutionInputdefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return remoteapi.ReadRemoteExecutionInput200ApplicationoctetStreamResponse{Body: reader, ContentLength: reader.Size, Headers: remoteapi.ReadRemoteExecutionInput200ResponseHeaders{ContentDisposition: mime.FormatMediaType("attachment", map[string]string{"filename": reader.Name}), XContentSHA256: reader.Hash}}, nil
}

func (h RemoteExecutionHandler) auth(ctx context.Context, convo uuid.UUID, mutation bool, csrf string) (toolruntime.Principal, *remoteapi.ApiError) {
	principal, e := h.Identity.enterprisePrincipal(ctx, mutation, csrf, "conversation.use")
	if e != nil {
		value := remoteConvert[remoteapi.ApiError](e)
		return toolruntime.Principal{}, &value
	}
	return toolruntime.Principal{EnterpriseID: principal.EnterpriseIDValue(), UserID: uuid.MustParse(principal.ActorID()), ConversationID: convo, AuthorizationVersion: principal.AuthorizationVersion()}, nil
}
func remoteConvert[T any](v any) (out T) {
	b, _ := json.Marshal(v)
	_ = json.Unmarshal(b, &out)
	return out
}
func remoteFailure(ctx context.Context, err error) (remoteapi.ApiError, int) {
	code := "REMOTE_EXECUTION_UNAVAILABLE"
	var coded interface{ Code() string }
	if errors.As(err, &coded) {
		code = coded.Code()
	}
	status := http.StatusConflict
	if code == "REMOTE_INPUT_INVALID" {
		status = http.StatusBadRequest
	}
	if code == "REMOTE_EXECUTION_FORBIDDEN" || code == "REMOTE_RESOURCE_FORBIDDEN" {
		status = http.StatusForbidden
	}
	body := remoteConvert[remoteapi.ApiError](conversationError(ctx, toolruntime.Error{Kind: code}))
	body.MessageKey = "errors.codes." + code
	return body, status
}
func (h RemoteExecutionHandler) ListRemoteExecutions(ctx context.Context, r remoteapi.ListRemoteExecutionsRequestObject) (remoteapi.ListRemoteExecutionsResponseObject, error) {
	p, e := h.auth(ctx, r.ConversationId, false, "")
	if e != nil {
		return remoteapi.ListRemoteExecutionsdefaultJSONResponse{Body: *e, StatusCode: 403}, nil
	}
	views, err := h.Service.List(ctx, p)
	if err != nil {
		body, status := remoteFailure(ctx, err)
		return remoteapi.ListRemoteExecutionsdefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return remoteapi.ListRemoteExecutions200JSONResponse{Items: remoteConvert[[]remoteapi.RemoteExecution](views)}, nil
}
func (h RemoteExecutionHandler) GetRemoteExecution(ctx context.Context, r remoteapi.GetRemoteExecutionRequestObject) (remoteapi.GetRemoteExecutionResponseObject, error) {
	p, e := h.auth(ctx, r.ConversationId, false, "")
	if e != nil {
		return remoteapi.GetRemoteExecutiondefaultJSONResponse{Body: *e, StatusCode: 403}, nil
	}
	view, err := h.Service.Get(ctx, p, r.ExecutionId)
	if err != nil {
		body, status := remoteFailure(ctx, err)
		return remoteapi.GetRemoteExecutiondefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return remoteapi.GetRemoteExecution200JSONResponse(remoteConvert[remoteapi.RemoteExecution](view)), nil
}
func (h RemoteExecutionHandler) StopRemoteExecution(ctx context.Context, r remoteapi.StopRemoteExecutionRequestObject) (remoteapi.StopRemoteExecutionResponseObject, error) {
	p, e := h.auth(ctx, r.ConversationId, true, r.Params.XCSRFToken)
	if e != nil {
		return remoteapi.StopRemoteExecutiondefaultJSONResponse{Body: *e, StatusCode: 403}, nil
	}
	view, err := h.Service.Stop(ctx, p, r.ExecutionId)
	if err != nil {
		body, status := remoteFailure(ctx, err)
		return remoteapi.StopRemoteExecutiondefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return remoteapi.StopRemoteExecution200JSONResponse(remoteConvert[remoteapi.RemoteExecution](view)), nil
}
func (h RemoteExecutionHandler) ReviseRemotePlan(ctx context.Context, r remoteapi.ReviseRemotePlanRequestObject) (remoteapi.ReviseRemotePlanResponseObject, error) {
	p, e := h.auth(ctx, r.ConversationId, true, r.Params.XCSRFToken)
	if e != nil {
		return remoteapi.ReviseRemotePlandefaultJSONResponse{Body: *e, StatusCode: 403}, nil
	}
	view, err := h.Service.Feedback(ctx, p, r.ExecutionId, r.Body.Feedback, r.Params.IdempotencyKey)
	if err != nil {
		body, status := remoteFailure(ctx, err)
		return remoteapi.ReviseRemotePlandefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return remoteapi.ReviseRemotePlan202JSONResponse(remoteConvert[remoteapi.RemoteExecution](view)), nil
}
