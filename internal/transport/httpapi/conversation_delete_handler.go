package httpapi

import (
	"context"
	"github.com/google/uuid"
	api "github.com/kakj-go/Argus/internal/gen/openapi/conversationapi"
)

func (handler ConversationHandler) DeleteConversation(ctx context.Context, r api.DeleteConversationRequestObject) (api.DeleteConversationResponseObject, error) {
	p, failure := handler.auth(ctx, true, r.Params.XCSRFToken, "conversation.use")
	if failure != nil {
		return api.DeleteConversationdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	value, err := handler.Service.Delete(ctx, p.EnterpriseIDValue(), uuid.MustParse(p.ActorID()), r.ConversationId, r.Params.IdempotencyKey)
	if err != nil {
		return api.DeleteConversationdefaultJSONResponse{Body: conversationError(ctx, err), StatusCode: conversationStatus(err)}, nil
	}
	return api.DeleteConversation202JSONResponse(toConversation(value)), nil
}
