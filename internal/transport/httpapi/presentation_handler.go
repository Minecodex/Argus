package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/conversation"
	api "github.com/kakj-go/Argus/internal/gen/openapi/presentationapi"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type PresentationHandler struct {
	Identity EnterpriseIdentityHandler
	Store    *postgres.Store
}

func (handler PresentationHandler) GetToolPresentation(ctx context.Context, r api.GetToolPresentationRequestObject) (api.GetToolPresentationResponseObject, error) {
	fail := func(err error) (api.GetToolPresentationResponseObject, error) {
		return api.GetToolPresentationdefaultJSONResponse{Body: planV5Error[api.ApiError](ctx, err), StatusCode: planV5Status(err)}, nil
	}
	p, failure := handler.Identity.enterprisePrincipal(ctx, false, "", "conversation.use")
	if failure != nil {
		data, _ := json.Marshal(failure)
		var value api.ApiError
		_ = json.Unmarshal(data, &value)
		return api.GetToolPresentationdefaultJSONResponse{Body: value, StatusCode: 403}, nil
	}
	user := uuid.MustParse(p.ActorID())
	enterprise := p.EnterpriseIDValue()
	service := conversation.Service{Store: handler.Store}
	principal, err := service.ToolPrincipal(ctx, enterprise, user, r.ConversationId)
	if err != nil {
		return fail(err)
	}
	if err := service.ValidateToolPrincipal(ctx, principal); err != nil {
		return fail(err)
	}
	value, err := handler.Store.Queries.GetToolPresentation(ctx, db.GetToolPresentationParams{ToolCallID: r.ToolCallId, EnterpriseID: enterprise, ConversationID: r.ConversationId})
	if err != nil {
		return fail(err)
	}
	scope, err := presentation.Scope(ctx, handler.Store, enterprise, user)
	if err != nil {
		return fail(err)
	}
	if scope != value.AuthorizationScope {
		return fail(toolruntime.Error{Kind: "PRESENTATION_FORBIDDEN"})
	}
	asset := &toolruntime.TemplateAsset{Source: value.TemplateSource, Hash: value.TemplateHash, Version: value.ToolVersion, Runtime: "argus-template/v1"}
	if err := presentation.Validate(asset); err != nil {
		return fail(toolruntime.Error{Kind: "PRESENTATION_INVALID"})
	}
	var details map[string]any
	if json.Unmarshal(value.DetailData, &details) != nil {
		return fail(toolruntime.Error{Kind: "PRESENTATION_INVALID"})
	}
	response := api.GetToolPresentation200JSONResponse{ToolCallId: r.ToolCallId, Runtime: api.ArgusTemplatev1, Status: api.ToolPresentationStatus(value.Status), TemplateHash: value.TemplateHash, TemplateSource: value.TemplateSource, DetailData: details}
	if err := json.Unmarshal(value.ResourceRefs, &response.ResourceRefs); err != nil {
		return fail(toolruntime.Error{Kind: "PRESENTATION_INVALID"})
	}
	refs := []string{"result_" + r.ToolCallId.String()}
	response.ResultRefs = &refs
	return response, nil
}
func presentationRequestContext(next api.StrictHandlerFunc, _ string) api.StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, value any) (any, error) {
		return next(WithRequestContext(ctx, w, r), w, r, value)
	}
}
