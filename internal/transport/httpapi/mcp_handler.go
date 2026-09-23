package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/enterprisemcp"
	mcpapi "github.com/kakj-go/Argus/internal/gen/openapi/mcpapi"
	"github.com/kakj-go/Argus/internal/identity"
)

type MCPHandler struct {
	Identity EnterpriseIdentityHandler
	Service  enterprisemcp.Service
}

func (handler MCPHandler) auth(ctx context.Context, mutation bool, csrf string) (identity.Principal, *mcpapi.ApiError) {
	permission := "conversation.use"
	if mutation {
		permission = "mcp_connection.manage"
	}
	p, value := handler.Identity.enterprisePrincipal(ctx, mutation, csrf, permission)
	if value == nil {
		return p, nil
	}
	encoded, _ := json.Marshal(value)
	var result mcpapi.ApiError
	_ = json.Unmarshal(encoded, &result)
	return identity.Principal{}, &result
}

func toMCPConnection(view enterprisemcp.View) mcpapi.MCPConnection {
	encoded, _ := json.Marshal(view)
	var result mcpapi.MCPConnection
	_ = json.Unmarshal(encoded, &result)
	return result
}

func (handler MCPHandler) ListMCPConnections(ctx context.Context, _ mcpapi.ListMCPConnectionsRequestObject) (mcpapi.ListMCPConnectionsResponseObject, error) {
	p, failure := handler.auth(ctx, false, "")
	if failure != nil {
		return mcpapi.ListMCPConnectionsdefaultJSONResponse{Body: *failure, StatusCode: http.StatusForbidden}, nil
	}
	views, err := handler.Service.List(ctx, p.EnterpriseIDValue(), uuid.MustParse(p.ActorID()), p.EnterpriseAdmin)
	if err != nil {
		return mcpapi.ListMCPConnectionsdefaultJSONResponse{Body: planV5Error[mcpapi.ApiError](ctx, err), StatusCode: planV5Status(err)}, nil
	}
	items := make([]mcpapi.MCPConnection, 0, len(views))
	for _, view := range views {
		items = append(items, toMCPConnection(view))
	}
	return mcpapi.ListMCPConnections200JSONResponse{Items: items}, nil
}

func (handler MCPHandler) GetMCPConnection(ctx context.Context, request mcpapi.GetMCPConnectionRequestObject) (mcpapi.GetMCPConnectionResponseObject, error) {
	p, failure := handler.auth(ctx, false, "")
	if failure != nil {
		return mcpapi.GetMCPConnectiondefaultJSONResponse{Body: *failure, StatusCode: http.StatusForbidden}, nil
	}
	value, err := handler.Service.Get(ctx, p.EnterpriseIDValue(), uuid.MustParse(p.ActorID()), request.Id, p.EnterpriseAdmin)
	if err != nil {
		return mcpapi.GetMCPConnectiondefaultJSONResponse{Body: planV5Error[mcpapi.ApiError](ctx, err), StatusCode: planV5Status(err)}, nil
	}
	return mcpapi.GetMCPConnection200JSONResponse(toMCPConnection(value)), nil
}

func mcpInput(body mcpapi.MCPConnectionWrite) enterprisemcp.Input {
	input := enterprisemcp.Input{Name: body.Name, Endpoint: body.Endpoint, AuthType: string(body.AuthType), Value: body.CredentialValue, Members: body.MemberIds}
	if body.ExpectedVersion != nil {
		input.ExpectedVersion = *body.ExpectedVersion
	}
	return input
}

func (handler MCPHandler) CreateMCPConnection(ctx context.Context, request mcpapi.CreateMCPConnectionRequestObject) (mcpapi.CreateMCPConnectionResponseObject, error) {
	p, failure := handler.auth(ctx, true, request.Params.XCSRFToken)
	if failure != nil {
		return mcpapi.CreateMCPConnectiondefaultJSONResponse{Body: *failure, StatusCode: http.StatusForbidden}, nil
	}
	value, err := handler.Service.Save(ctx, p.EnterpriseIDValue(), uuid.MustParse(p.ActorID()), uuid.Nil, mcpInput(*request.Body), request.Params.IdempotencyKey)
	if err != nil {
		return mcpapi.CreateMCPConnectiondefaultJSONResponse{Body: planV5Error[mcpapi.ApiError](ctx, err), StatusCode: planV5Status(err)}, nil
	}
	return mcpapi.CreateMCPConnection201JSONResponse(toMCPConnection(value)), nil
}

func (handler MCPHandler) UpdateMCPConnection(ctx context.Context, request mcpapi.UpdateMCPConnectionRequestObject) (mcpapi.UpdateMCPConnectionResponseObject, error) {
	p, failure := handler.auth(ctx, true, request.Params.XCSRFToken)
	if failure != nil {
		return mcpapi.UpdateMCPConnectiondefaultJSONResponse{Body: *failure, StatusCode: http.StatusForbidden}, nil
	}
	value, err := handler.Service.Save(ctx, p.EnterpriseIDValue(), uuid.MustParse(p.ActorID()), request.Id, mcpInput(*request.Body), "")
	if err != nil {
		return mcpapi.UpdateMCPConnectiondefaultJSONResponse{Body: planV5Error[mcpapi.ApiError](ctx, err), StatusCode: planV5Status(err)}, nil
	}
	return mcpapi.UpdateMCPConnection200JSONResponse(toMCPConnection(value)), nil
}

func (handler MCPHandler) SetMCPConnectionMembers(ctx context.Context, request mcpapi.SetMCPConnectionMembersRequestObject) (mcpapi.SetMCPConnectionMembersResponseObject, error) {
	p, failure := handler.auth(ctx, true, request.Params.XCSRFToken)
	if failure != nil {
		return mcpapi.SetMCPConnectionMembersdefaultJSONResponse{Body: *failure, StatusCode: http.StatusForbidden}, nil
	}
	value, err := handler.Service.SetMembers(ctx, p.EnterpriseIDValue(), uuid.MustParse(p.ActorID()), request.Id, request.Body.ExpectedVersion, request.Body.MemberIds)
	if err != nil {
		return mcpapi.SetMCPConnectionMembersdefaultJSONResponse{Body: planV5Error[mcpapi.ApiError](ctx, err), StatusCode: planV5Status(err)}, nil
	}
	return mcpapi.SetMCPConnectionMembers200JSONResponse(toMCPConnection(value)), nil
}

func (handler MCPHandler) SetMCPConnectionState(ctx context.Context, request mcpapi.SetMCPConnectionStateRequestObject) (mcpapi.SetMCPConnectionStateResponseObject, error) {
	p, failure := handler.auth(ctx, true, request.Params.XCSRFToken)
	if failure != nil {
		return mcpapi.SetMCPConnectionStatedefaultJSONResponse{Body: *failure, StatusCode: http.StatusForbidden}, nil
	}
	value, err := handler.Service.SetState(ctx, p.EnterpriseIDValue(), uuid.MustParse(p.ActorID()), request.Id, request.Body.ExpectedVersion, string(request.Body.Status))
	if err != nil {
		return mcpapi.SetMCPConnectionStatedefaultJSONResponse{Body: planV5Error[mcpapi.ApiError](ctx, err), StatusCode: planV5Status(err)}, nil
	}
	return mcpapi.SetMCPConnectionState200JSONResponse(toMCPConnection(value)), nil
}

func (handler MCPHandler) TestMCPConnection(ctx context.Context, request mcpapi.TestMCPConnectionRequestObject) (mcpapi.TestMCPConnectionResponseObject, error) {
	p, failure := handler.auth(ctx, true, request.Params.XCSRFToken)
	if failure != nil {
		return mcpapi.TestMCPConnectiondefaultJSONResponse{Body: *failure, StatusCode: http.StatusForbidden}, nil
	}
	value, err := handler.Service.Test(ctx, p.EnterpriseIDValue(), uuid.MustParse(p.ActorID()), request.Id)
	if err != nil {
		return mcpapi.TestMCPConnectiondefaultJSONResponse{Body: planV5Error[mcpapi.ApiError](ctx, err), StatusCode: planV5Status(err)}, nil
	}
	return mcpapi.TestMCPConnection200JSONResponse(toMCPConnection(value)), nil
}

func mcpRequestContext(next mcpapi.StrictHandlerFunc, _ string) mcpapi.StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, value any) (any, error) {
		return next(WithRequestContext(ctx, w, r), w, r, value)
	}
}
