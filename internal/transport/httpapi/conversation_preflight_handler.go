package httpapi

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	conversationapi "github.com/kakj-go/Argus/internal/gen/openapi/conversationapi"
	"net/http"
)

func (handler ConversationHandler) PreflightConversation(ctx context.Context, request conversationapi.PreflightConversationRequestObject) (conversationapi.PreflightConversationResponseObject, error) {
	p, apiError := handler.auth(ctx, true, request.Params.XCSRFToken, "conversation.use")
	if apiError != nil {
		return conversationapi.PreflightConversationdefaultJSONResponse{Body: *apiError, StatusCode: http.StatusForbidden}, nil
	}
	var files []uuid.UUID
	if request.Body.FileIds != nil {
		files = *request.Body.FileIds
	}
	value, err := handler.Service.Preflight(ctx, p.EnterpriseIDValue(), uuid.MustParse(p.ActorID()), request.ConversationId, request.Body.Content, files, dashboardSelection(request.Body.DashboardContext))
	if err == nil && !value.Ready {
		err = value.CapacityError()
	}
	if err != nil {
		return conversationapi.PreflightConversationdefaultJSONResponse{Body: conversationError(ctx, err), StatusCode: conversationStatus(err)}, nil
	}
	encoded, _ := json.Marshal(value)
	var result conversationapi.ConversationPreflight
	_ = json.Unmarshal(encoded, &result)
	return conversationapi.PreflightConversation200JSONResponse(result), nil
}
