package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/conversation"
	api "github.com/kakj-go/Argus/internal/gen/openapi/presentationapi"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func (handler PresentationHandler) ReadToolPresentation(ctx context.Context, r api.ReadToolPresentationRequestObject) (api.ReadToolPresentationResponseObject, error) {
	fail := func(err error) (api.ReadToolPresentationResponseObject, error) {
		return api.ReadToolPresentationdefaultJSONResponse{Body: planV5Error[api.ApiError](ctx, err), StatusCode: planV5Status(err)}, nil
	}
	actor, failure := handler.Identity.enterprisePrincipal(ctx, true, r.Params.XCSRFToken, "conversation.use")
	if failure != nil {
		raw, _ := json.Marshal(failure)
		var value api.ApiError
		_ = json.Unmarshal(raw, &value)
		return api.ReadToolPresentationdefaultJSONResponse{Body: value, StatusCode: 403}, nil
	}
	if r.Body == nil || !r.Body.Operation.Valid() {
		return fail(presentation.ErrReadInvalid)
	}
	parameters := map[string]any{}
	encodedParameters, parameterError := json.Marshal(r.Body.Parameters)
	if parameterError != nil || json.Unmarshal(encodedParameters, &parameters) != nil {
		return fail(presentation.ErrReadInvalid)
	}
	enterprise, user := actor.EnterpriseIDValue(), uuid.MustParse(actor.ActorID())
	service := conversation.Service{Store: handler.Store}
	principal, err := service.ToolPrincipal(ctx, enterprise, user, r.ConversationId)
	if err != nil {
		return fail(err)
	}
	if err = service.ValidateToolPrincipal(ctx, principal); err != nil {
		return fail(err)
	}
	p, err := handler.Store.Queries.GetToolPresentation(ctx, db.GetToolPresentationParams{ToolCallID: r.ToolCallId, EnterpriseID: enterprise, ConversationID: r.ConversationId})
	if err != nil {
		return fail(err)
	}
	scope, err := presentation.Scope(ctx, handler.Store, enterprise, user)
	if err != nil {
		return fail(err)
	}
	if scope != p.AuthorizationScope || p.TemplateRuntime != "argus-template/v2" || !slices.Contains(p.ReadOperations, string(r.Body.Operation)) {
		return fail(toolruntime.Error{Kind: "PRESENTATION_FORBIDDEN"})
	}
	data, binding := map[string]any{}, map[string]any{}
	if json.Unmarshal(p.DetailData, &data) != nil || json.Unmarshal(p.ResultBinding, &binding) != nil {
		return fail(toolruntime.Error{Kind: "PRESENTATION_INVALID"})
	}
	version := p.TemplateHash
	for _, key := range []string{"required", "any_required"} {
		values, _ := binding[key].([]any)
		permissions := []string{}
		for _, value := range values {
			if permission, ok := value.(string); ok && permission != "" {
				permissions = append(permissions, permission)
			}
		}
		allowed := principal.Allows(permissions...)
		if key == "any_required" && len(permissions) > 0 {
			allowed = false
			for _, permission := range permissions {
				if principal.Allows(permission) {
					allowed = true
				}
			}
		}
		if !allowed {
			return fail(toolruntime.Error{Kind: "PRESENTATION_FORBIDDEN"})
		}
	}
	if snapshot, e := handler.Store.Queries.GetPresentationSnapshot(ctx, db.GetPresentationSnapshotParams{PresentationID: p.ID, EnterpriseID: enterprise}); e == nil {
		if json.Unmarshal(snapshot.Data, &data) != nil {
			return fail(toolruntime.Error{Kind: "PRESENTATION_INVALID"})
		}
		version = snapshot.SourceVersion
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return fail(e)
	} else if binding["kind"] == "dashboard_query" {
		return api.ReadToolPresentation200JSONResponse{Status: api.PresentationReadResultStatusPending, Data: map[string]any{"status": "pending"}}, nil
	} else if payload, ok := binding["data"].(map[string]any); ok {
		data = payload
		version = presentation.SnapshotVersion(p.ResultBinding)
	}
	var next string
	if r.Body.Operation == api.ResourcesContext {
		if len(parameters) != 0 {
			return fail(presentation.ErrReadInvalid)
		}
		var refs []toolruntime.ResourceRef
		if json.Unmarshal(p.ResourceRefs, &refs) != nil {
			return fail(presentation.ErrReadInvalid)
		}
		ids := []uuid.UUID{}
		for _, ref := range refs {
			if ref.Type == "host" || ref.Type == "kubernetes_cluster" {
				id, e := uuid.Parse(ref.ID)
				if e != nil {
					return fail(e)
				}
				ids = append(ids, id)
			}
		}
		resources, e := handler.Store.Queries.ResolvePresentationResources(ctx, db.ResolvePresentationResourcesParams{EnterpriseID: enterprise, Ids: ids})
		if e != nil {
			return fail(e)
		}
		data = map[string]any{"resources": resources}
	} else if r.Body.Operation == api.ResultPage {
		data, next, err = presentation.PageData(handler.CursorSigner, pagination.Binding{Audience: "enterprise", EnterpriseID: enterprise.String(), SubjectType: "user", SubjectID: user.String(), AuthorizationVersion: principal.AuthorizationVersion}, p, data, version, parameters)
		if err != nil {
			return fail(err)
		}
	} else {
		if len(parameters) != 0 {
			return fail(presentation.ErrReadInvalid)
		}
		data = presentation.ViewData(data)
	}
	response := api.ReadToolPresentation200JSONResponse{Status: api.PresentationReadResultStatusReady, Data: data}
	if next != "" {
		response.NextCursor = &next
	}
	return response, nil
}
