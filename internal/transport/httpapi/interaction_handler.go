package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	interactionapi "github.com/kakj-go/Argus/internal/gen/openapi/interactionapi"
	"github.com/kakj-go/Argus/internal/interaction"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"net/http"
)

type InteractionHandler struct {
	Identity EnterpriseIdentityHandler
	Service  interaction.Service
}

func (h InteractionHandler) auth(ctx context.Context, id uuid.UUID, mutation bool, csrf string) (toolruntime.Principal, *interactionapi.ApiError) {
	p, e := h.Identity.enterprisePrincipal(ctx, mutation, csrf, "conversation.use")
	if e != nil {
		v := remoteConvert[interactionapi.ApiError](e)
		return toolruntime.Principal{}, &v
	}
	return toolruntime.Principal{EnterpriseID: p.EnterpriseIDValue(), UserID: uuid.MustParse(p.ActorID()), ConversationID: id, AuthorizationVersion: p.AuthorizationVersion()}, nil
}
func interactionFailure(ctx context.Context, err error) (interactionapi.ApiError, int) {
	return remoteConvert[interactionapi.ApiError](conversationError(ctx, err)), conversationStatus(err)
}
func (h InteractionHandler) ListConversationInteractions(ctx context.Context, r interactionapi.ListConversationInteractionsRequestObject) (interactionapi.ListConversationInteractionsResponseObject, error) {
	p, e := h.auth(ctx, r.ConversationId, false, "")
	if e != nil {
		return interactionapi.ListConversationInteractionsdefaultJSONResponse{Body: *e, StatusCode: 403}, nil
	}
	items, err := h.Service.List(ctx, p)
	if err != nil {
		body, status := interactionFailure(ctx, err)
		return interactionapi.ListConversationInteractionsdefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	response := interactionapi.ConversationInteractionPage{Items: remoteConvert[[]interactionapi.ConversationInteraction](items)}
	run, err := h.Service.Store.Queries.GetActiveRunForConversation(ctx, db.GetActiveRunForConversationParams{ConversationID: p.ConversationID, EnterpriseID: p.EnterpriseID})
	if err == nil {
		response.ActiveRunId = &run.ID
		response.ActiveRunStatus = &run.Status
	} else if !errors.Is(err, pgx.ErrNoRows) {
		body, status := interactionFailure(ctx, err)
		return interactionapi.ListConversationInteractionsdefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return interactionapi.ListConversationInteractions200JSONResponse(response), nil
}
func (h InteractionHandler) GetConversationInteraction(ctx context.Context, r interactionapi.GetConversationInteractionRequestObject) (interactionapi.GetConversationInteractionResponseObject, error) {
	p, e := h.auth(ctx, r.ConversationId, false, "")
	if e != nil {
		return interactionapi.GetConversationInteractiondefaultJSONResponse{Body: *e, StatusCode: 403}, nil
	}
	v, err := h.Service.Get(ctx, p, r.InteractionId)
	if err != nil {
		body, status := interactionFailure(ctx, err)
		return interactionapi.GetConversationInteractiondefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return interactionapi.GetConversationInteraction200JSONResponse(remoteConvert[interactionapi.ConversationInteraction](v)), nil
}
func (h InteractionHandler) RespondConversationInteraction(ctx context.Context, r interactionapi.RespondConversationInteractionRequestObject) (interactionapi.RespondConversationInteractionResponseObject, error) {
	p, e := h.auth(ctx, r.ConversationId, true, r.Params.XCSRFToken)
	if e != nil {
		return interactionapi.RespondConversationInteractiondefaultJSONResponse{Body: *e, StatusCode: 403}, nil
	}
	if r.Body == nil {
		return interactionapi.RespondConversationInteractiondefaultJSONResponse{Body: remoteConvert[interactionapi.ApiError](conversationError(ctx, toolruntime.Error{Kind: "TOOL_INPUT_INVALID"})), StatusCode: http.StatusBadRequest}, nil
	}
	raw, _ := json.Marshal(r.Body)
	var input interaction.Response
	if json.Unmarshal(raw, &input) != nil {
		return interactionapi.RespondConversationInteractiondefaultJSONResponse{Body: remoteConvert[interactionapi.ApiError](conversationError(ctx, toolruntime.Error{Kind: "TOOL_INPUT_INVALID"})), StatusCode: http.StatusBadRequest}, nil
	}
	v, err := h.Service.Respond(ctx, p, r.InteractionId, input, r.Params.IdempotencyKey, LocaleFromContext(ctx))
	if err != nil {
		body, status := interactionFailure(ctx, err)
		return interactionapi.RespondConversationInteractiondefaultJSONResponse{Body: body, StatusCode: status}, nil
	}
	return interactionapi.RespondConversationInteraction200JSONResponse(remoteConvert[interactionapi.ConversationInteraction](v)), nil
}
