package httpapi

import (
	"context"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboardcontext"
	api "github.com/kakj-go/Argus/internal/gen/openapi/conversationapi"
	"net/http"
)

func dashboardSelection(input *api.DashboardChatSelection) *dashboardcontext.Selection {
	if input == nil {
		return nil
	}
	return &dashboardcontext.Selection{Mode: string(input.Mode), DashboardIDs: input.DashboardIds, ExpectedVersion: input.ExpectedVersion}
}

func (handler ConversationHandler) GetConversationDashboardContext(ctx context.Context, request api.GetConversationDashboardContextRequestObject) (api.GetConversationDashboardContextResponseObject, error) {
	p, failure := handler.auth(ctx, false, "", "conversation.read")
	if failure != nil {
		return api.GetConversationDashboardContextdefaultJSONResponse{Body: *failure, StatusCode: http.StatusForbidden}, nil
	}
	// Read ownership first. Keep revoked IDs removable without disclosing names
	// or requiring the Dashboard permissions that the user may have just lost.
	_, err := handler.Service.Get(ctx, p.EnterpriseIDValue(), uuid.MustParse(p.ActorID()), request.ConversationId)
	if err != nil {
		return api.GetConversationDashboardContextdefaultJSONResponse{Body: conversationError(ctx, err), StatusCode: conversationStatus(err)}, nil
	}
	principal, err := handler.Service.ToolPrincipal(ctx, p.EnterpriseIDValue(), uuid.MustParse(p.ActorID()), request.ConversationId)
	if err != nil {
		return api.GetConversationDashboardContextdefaultJSONResponse{Body: conversationError(ctx, err), StatusCode: conversationStatus(err)}, nil
	}
	selection, err := dashboardcontext.Current(ctx, handler.Service.Store.Queries, principal)
	if err != nil {
		return api.GetConversationDashboardContextdefaultJSONResponse{Body: conversationError(ctx, err), StatusCode: conversationStatus(err)}, nil
	}
	return api.GetConversationDashboardContext200JSONResponse(dashboardConvert[api.DashboardChatContext](selection)), nil
}
