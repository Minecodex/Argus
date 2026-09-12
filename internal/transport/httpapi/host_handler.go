package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	actionservice "github.com/kakj-go/Argus/internal/action"
	hostapi "github.com/kakj-go/Argus/internal/gen/openapi/hostapi"
	"github.com/kakj-go/Argus/internal/hostremoval"
	"github.com/kakj-go/Argus/internal/identity"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	telemetryservice "github.com/kakj-go/Argus/internal/telemetry"
)

type HostWindowsRDPActionService interface {
	PreviewWindowsRDPEnable(context.Context, resource.Subject, uuid.UUID, uuid.UUID, int64, string) (db.PendingAction, error)
}

type HostHandler struct {
	Identity   EnterpriseIdentityHandler
	Service    resource.Service
	Queries    *db.Queries
	WindowsRDP HostWindowsRDPActionService
	Removal    hostremoval.Service
}

func (handler HostHandler) PreviewHostRemoval(ctx context.Context, request hostapi.PreviewHostRemovalRequestObject) (hostapi.PreviewHostRemovalResponseObject, error) {
	p, apiError := handler.auth(ctx, true, request.Params.XCSRFToken, "host.manage")
	if apiError != nil {
		return hostapi.PreviewHostRemovaldefaultJSONResponse{Body: *apiError, StatusCode: http.StatusForbidden}, nil
	}
	if request.Body == nil {
		return hostapi.PreviewHostRemovaldefaultJSONResponse{Body: hostError(ctx, hostremoval.ErrInvalidTarget), StatusCode: http.StatusBadRequest}, nil
	}
	input := hostremoval.PreviewInput{TargetType: string(request.Body.TargetType), TargetID: uuid.UUID(request.Body.TargetId), ExpectedVersion: request.Body.ExpectedVersion,
		Mode: string(request.Body.Mode), ConnectionTestID: optionalUUID(request.Body.ConnectionTestId), CredentialID: optionalUUID(request.Body.CredentialId)}
	if request.Body.ConfirmationName != nil {
		input.ConfirmationName = *request.Body.ConfirmationName
	}
	action, err := handler.Removal.Preview(ctx, resourceSubject(p), p.EnterpriseIDValue(), input, request.Params.IdempotencyKey)
	if err != nil {
		return hostapi.PreviewHostRemovaldefaultJSONResponse{Body: hostError(ctx, err), StatusCode: hostRemovalStatus(err)}, nil
	}
	return hostapi.PreviewHostRemoval201JSONResponse(pendingForHost(action)), nil
}

func (handler HostHandler) GetHostRemovalOperation(ctx context.Context, request hostapi.GetHostRemovalOperationRequestObject) (hostapi.GetHostRemovalOperationResponseObject, error) {
	p, apiError := handler.auth(ctx, false, "", "host.read")
	if apiError != nil {
		return hostapi.GetHostRemovalOperationdefaultJSONResponse{Body: *apiError, StatusCode: http.StatusForbidden}, nil
	}
	view, err := handler.Removal.Get(ctx, p.EnterpriseIDValue(), uuid.UUID(request.Id))
	if err != nil || !handler.Service.Access.CanAccess(p.AuthorizedResourceIDs, view.Operation.HostID) {
		if err == nil {
			err = resource.ErrResourceDenied
		}
		return hostapi.GetHostRemovalOperationdefaultJSONResponse{Body: hostError(ctx, err), StatusCode: hostRemovalStatus(err)}, nil
	}
	return hostapi.GetHostRemovalOperation200JSONResponse(toHostRemovalOperation(view)), nil
}

func (handler HostHandler) RetryHostRemovalOperation(ctx context.Context, request hostapi.RetryHostRemovalOperationRequestObject) (hostapi.RetryHostRemovalOperationResponseObject, error) {
	p, apiError := handler.auth(ctx, true, request.Params.XCSRFToken, "host.manage")
	if apiError != nil {
		return hostapi.RetryHostRemovalOperationdefaultJSONResponse{Body: *apiError, StatusCode: http.StatusForbidden}, nil
	}
	current, err := handler.Removal.Get(ctx, p.EnterpriseIDValue(), uuid.UUID(request.Id))
	if err != nil || !handler.Service.Access.CanAccess(p.AuthorizedResourceIDs, current.Operation.HostID) {
		if err == nil {
			err = resource.ErrResourceDenied
		}
		return hostapi.RetryHostRemovalOperationdefaultJSONResponse{Body: hostError(ctx, err), StatusCode: hostRemovalStatus(err)}, nil
	}
	view, err := handler.Removal.Retry(ctx, p.ActorID(), p.EnterpriseIDValue(), uuid.UUID(request.Id), request.Params.IdempotencyKey)
	if err != nil {
		return hostapi.RetryHostRemovalOperationdefaultJSONResponse{Body: hostError(ctx, err), StatusCode: hostRemovalStatus(err)}, nil
	}
	return hostapi.RetryHostRemovalOperation202JSONResponse(toHostRemovalOperation(view)), nil
}

func (handler HostHandler) RegenerateHostRemovalCommand(ctx context.Context, request hostapi.RegenerateHostRemovalCommandRequestObject) (hostapi.RegenerateHostRemovalCommandResponseObject, error) {
	p, apiError := handler.auth(ctx, true, request.Params.XCSRFToken, "host.manage")
	if apiError != nil {
		return hostapi.RegenerateHostRemovalCommanddefaultJSONResponse{Body: *apiError, StatusCode: http.StatusForbidden}, nil
	}
	current, err := handler.Removal.Get(ctx, p.EnterpriseIDValue(), uuid.UUID(request.Id))
	if err != nil || !handler.Service.Access.CanAccess(p.AuthorizedResourceIDs, current.Operation.HostID) {
		if err == nil {
			err = resource.ErrResourceDenied
		}
		return hostapi.RegenerateHostRemovalCommanddefaultJSONResponse{Body: hostError(ctx, err), StatusCode: hostRemovalStatus(err)}, nil
	}
	instruction, err := handler.Removal.RegenerateInstruction(ctx, p.ActorID(), p.EnterpriseIDValue(), uuid.UUID(request.Id), request.Params.IdempotencyKey)
	if err != nil {
		return hostapi.RegenerateHostRemovalCommanddefaultJSONResponse{Body: hostError(ctx, err), StatusCode: hostRemovalStatus(err)}, nil
	}
	command := instruction.Set.Command
	platform := "linux"
	if instruction.Set.Shell == "powershell" {
		platform = "windows"
	}
	return hostapi.RegenerateHostRemovalCommand201JSONResponse{OperationId: openapi_types.UUID(instruction.OperationID), Platform: hostapi.HostRemovalInstructionPlatform(platform),
		Shell: hostapi.HostRemovalInstructionShell(instruction.Set.Shell), Privilege: hostapi.HostRemovalInstructionPrivilege("system"), Command: &command, ExpiresAt: instruction.Set.ExpiresAt}, nil
}

func (handler HostHandler) GetHostRemovalBootstrapScript(ctx context.Context, request hostapi.GetHostRemovalBootstrapScriptRequestObject) (hostapi.GetHostRemovalBootstrapScriptResponseObject, error) {
	script, contentType, err := handler.Removal.ClaimBootstrap(ctx, uuid.UUID(request.Params.OperationId), request.Params.XArgusRemovalToken)
	if err != nil {
		return hostapi.GetHostRemovalBootstrapScriptdefaultJSONResponse{Body: hostError(ctx, err), StatusCode: hostRemovalStatus(err)}, nil
	}
	if contentType == "text/x-powershell" {
		return hostapi.GetHostRemovalBootstrapScript200TextxPowershellResponse{Body: bytes.NewBufferString(script), ContentLength: int64(len(script))}, nil
	}
	return hostapi.GetHostRemovalBootstrapScript200TextxShellscriptResponse{Body: bytes.NewBufferString(script), ContentLength: int64(len(script))}, nil
}

func (handler HostHandler) SubmitHostRemovalReceipt(ctx context.Context, request hostapi.SubmitHostRemovalReceiptRequestObject) (hostapi.SubmitHostRemovalReceiptResponseObject, error) {
	if request.Body == nil {
		return hostapi.SubmitHostRemovalReceiptdefaultJSONResponse{Body: hostError(ctx, hostremoval.ErrTokenInvalid), StatusCode: http.StatusBadRequest}, nil
	}
	code := ""
	if request.Body.ErrorCode != nil {
		code = *request.Body.ErrorCode
	}
	evidence, marshalErr := json.Marshal(request.Body.Evidence)
	if marshalErr != nil {
		return hostapi.SubmitHostRemovalReceiptdefaultJSONResponse{Body: hostError(ctx, hostremoval.ErrTokenInvalid), StatusCode: http.StatusBadRequest}, nil
	}
	view, err := handler.Removal.SubmitReceipt(ctx, request.Params.XArgusRemovalToken, hostremoval.Receipt{OperationID: uuid.UUID(request.Body.OperationId),
		RemovalGeneration: request.Body.RemovalGeneration, ConnectorID: uuid.UUID(request.Body.ConnectorId), Stage: string(request.Body.Stage),
		LocalCleanup: string(request.Body.LocalCleanup), ResultHash: request.Body.ResultHash, ErrorCode: code, Evidence: evidence})
	if err != nil {
		return hostapi.SubmitHostRemovalReceiptdefaultJSONResponse{Body: hostError(ctx, err), StatusCode: hostRemovalStatus(err)}, nil
	}
	return hostapi.SubmitHostRemovalReceipt200JSONResponse(toHostRemovalOperation(view)), nil
}

func (handler HostHandler) PreviewEnableHostWindowsRDP(ctx context.Context, request hostapi.PreviewEnableHostWindowsRDPRequestObject) (hostapi.PreviewEnableHostWindowsRDPResponseObject, error) {
	p, apiError := handler.auth(ctx, true, request.Params.XCSRFToken, "host.manage")
	if apiError != nil {
		return hostapi.PreviewEnableHostWindowsRDPdefaultJSONResponse{Body: *apiError, StatusCode: http.StatusForbidden}, nil
	}
	if request.Body == nil || handler.WindowsRDP == nil {
		return hostapi.PreviewEnableHostWindowsRDPdefaultJSONResponse{Body: hostError(ctx, errors.New("invalid request")), StatusCode: http.StatusBadRequest}, nil
	}
	action, err := handler.WindowsRDP.PreviewWindowsRDPEnable(ctx, resourceSubject(p), p.EnterpriseIDValue(), uuid.UUID(request.Id), request.Body.ExpectedVersion, request.Params.IdempotencyKey)
	if err != nil {
		return hostapi.PreviewEnableHostWindowsRDPdefaultJSONResponse{Body: hostError(ctx, err), StatusCode: resourceStatus(err)}, nil
	}
	return hostapi.PreviewEnableHostWindowsRDP201JSONResponse(pendingForHost(action)), nil
}

func (handler HostHandler) GetHostOnboardingOperation(ctx context.Context, request hostapi.GetHostOnboardingOperationRequestObject) (hostapi.GetHostOnboardingOperationResponseObject, error) {
	p, apiError := handler.auth(ctx, false, "", "host.read")
	if apiError != nil {
		return hostapi.GetHostOnboardingOperationdefaultJSONResponse{Body: *apiError, StatusCode: http.StatusForbidden}, nil
	}
	operation, err := handler.Queries.GetHostOnboardingOperation(ctx, db.GetHostOnboardingOperationParams{ID: uuid.UUID(request.Id), EnterpriseID: p.EnterpriseIDValue()})
	if err != nil || !handler.Service.Access.CanAccess(p.AuthorizedResourceIDs, operation.HostID) {
		return hostapi.GetHostOnboardingOperationdefaultJSONResponse{Body: hostError(ctx, resource.ErrResourceDenied), StatusCode: http.StatusNotFound}, nil
	}
	events, err := handler.Queries.ListHostOnboardingOperationEvents(ctx, db.ListHostOnboardingOperationEventsParams{OperationID: operation.ID, EnterpriseID: operation.EnterpriseID})
	if err != nil {
		return hostapi.GetHostOnboardingOperationdefaultJSONResponse{Body: hostError(ctx, err), StatusCode: http.StatusInternalServerError}, nil
	}
	return hostapi.GetHostOnboardingOperation200JSONResponse(toHostOnboardingOperation(operation, events)), nil
}

func (handler HostHandler) ListHosts(ctx context.Context, _ hostapi.ListHostsRequestObject) (hostapi.ListHostsResponseObject, error) {
	p, apiError := handler.auth(ctx, false, "", "host.read")
	if apiError != nil {
		return hostapi.ListHostsdefaultJSONResponse{Body: *apiError, StatusCode: http.StatusForbidden}, nil
	}
	items, err := handler.Service.ListHosts(ctx, p.EnterpriseIDValue(), p.AuthorizedResourceIDs)
	if err != nil {
		return hostapi.ListHostsdefaultJSONResponse{Body: hostError(ctx, err), StatusCode: resourceStatus(err)}, nil
	}
	converted := make([]hostapi.Host, 0, len(items))
	runtimeStates := handler.runtimeStates(ctx, p.EnterpriseIDValue(), items)
	onboarding := loadHostOnboarding(ctx, handler.Queries, p.EnterpriseIDValue(), items)
	for _, item := range items {
		converted = append(converted, toHost(item, runtimeStates[item.ID], onboarding[item.ID]))
	}
	return hostapi.ListHosts200JSONResponse{Items: converted, Page: emptyHostPage()}, nil
}

func (handler HostHandler) runtimeStates(ctx context.Context, enterpriseID uuid.UUID, items []db.Host) map[uuid.UUID]db.HostRuntimeObservation {
	result := map[uuid.UUID]db.HostRuntimeObservation{}
	if handler.Queries == nil || len(items) == 0 {
		return result
	}
	ids := make([]uuid.UUID, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	values, err := handler.Queries.ListHostRuntimeObservations(ctx, db.ListHostRuntimeObservationsParams{EnterpriseID: enterpriseID, Column2: ids})
	if err != nil {
		return result
	}
	for _, value := range values {
		result[value.HostID] = value
	}
	return result
}

func (handler HostHandler) GetHost(ctx context.Context, request hostapi.GetHostRequestObject) (hostapi.GetHostResponseObject, error) {
	p, apiError := handler.auth(ctx, false, "", "host.read")
	if apiError != nil {
		return hostapi.GetHostdefaultJSONResponse{Body: *apiError, StatusCode: http.StatusForbidden}, nil
	}
	item, err := handler.Service.GetHost(ctx, p.EnterpriseIDValue(), uuid.UUID(request.Id), p.AuthorizedResourceIDs)
	if err != nil {
		return hostapi.GetHostdefaultJSONResponse{Body: hostError(ctx, err), StatusCode: resourceStatus(err)}, nil
	}
	var runtimeState db.HostRuntimeObservation
	if handler.Queries != nil {
		runtimeState, _ = handler.Queries.GetHostRuntimeObservation(ctx, db.GetHostRuntimeObservationParams{EnterpriseID: p.EnterpriseIDValue(), HostID: uuid.UUID(request.Id)})
	}
	onboarding := loadHostOnboarding(ctx, handler.Queries, p.EnterpriseIDValue(), []db.Host{item})
	return hostapi.GetHost200JSONResponse(toHost(item, runtimeState, onboarding[item.ID])), nil
}

func (handler HostHandler) CreateHostConnectionTest(ctx context.Context, request hostapi.CreateHostConnectionTestRequestObject) (hostapi.CreateHostConnectionTestResponseObject, error) {
	p, apiError := handler.auth(ctx, true, request.Params.XCSRFToken, "host.test")
	if apiError != nil {
		return hostapi.CreateHostConnectionTestdefaultJSONResponse{Body: *apiError, StatusCode: http.StatusForbidden}, nil
	}
	if request.Body == nil {
		return hostapi.CreateHostConnectionTestdefaultJSONResponse{Body: hostError(ctx, errors.New("invalid request")), StatusCode: http.StatusBadRequest}, nil
	}
	input := resource.HostInput{Address: request.Body.Address, Port: int32(request.Body.Port), Platform: string(request.Body.Platform), SSHPath: string(request.Body.SshPath),
		BastionScopeID: optionalUUID(request.Body.BastionScopeId), CredentialID: uuid.NullUUID{UUID: uuid.UUID(request.Body.CredentialId), Valid: true}, Username: request.Body.Username}
	if request.Body.OnboardingControlPath != nil {
		input.OnboardingControlPath = string(*request.Body.OnboardingControlPath)
	}
	test, err := handler.Service.CreateHostConnectionTest(ctx, resourceSubject(p), p.EnterpriseIDValue(), input, request.Params.IdempotencyKey)
	if err != nil {
		return hostapi.CreateHostConnectionTestdefaultJSONResponse{Body: hostError(ctx, err), StatusCode: resourceStatus(err)}, nil
	}
	return hostapi.CreateHostConnectionTest202JSONResponse(toHostConnectionTest(test)), nil
}

func (handler HostHandler) PreviewCreateHost(ctx context.Context, request hostapi.PreviewCreateHostRequestObject) (hostapi.PreviewCreateHostResponseObject, error) {
	p, apiError := handler.auth(ctx, true, request.Params.XCSRFToken, "host.manage")
	if apiError != nil {
		return hostapi.PreviewCreateHostdefaultJSONResponse{Body: *apiError, StatusCode: http.StatusForbidden}, nil
	}
	if request.Body == nil {
		return hostapi.PreviewCreateHostdefaultJSONResponse{Body: hostError(ctx, errors.New("invalid request")), StatusCode: http.StatusBadRequest}, nil
	}
	input := hostCreateInput(*request.Body)
	action, err := handler.Service.PreviewCreateHost(ctx, resourceSubject(p), p.EnterpriseIDValue(), input, request.Params.IdempotencyKey)
	if err != nil {
		return hostapi.PreviewCreateHostdefaultJSONResponse{Body: hostError(ctx, err), StatusCode: resourceStatus(err)}, nil
	}
	return hostapi.PreviewCreateHost201JSONResponse(pendingForHost(action)), nil
}

func hostCreateInput(value hostapi.HostPreviewCreate) resource.HostInput {
	input := resource.HostInput{Name: value.Name, Platform: string(value.Platform), Role: string(value.Role), ControlPath: string(value.ControlPath),
		InstallMethod: string(value.InstallMethod), SSHPath: string(value.SshPath),
		Environment: string(value.Environment), Labels: stringMap(value.Labels), BastionScopeID: optionalUUID(value.BastionScopeId)}
	if value.Hostname != nil {
		input.Hostname = *value.Hostname
	}
	if value.Address != nil {
		input.Address = *value.Address
	}
	if value.Port != nil {
		input.Port = int32(*value.Port)
	}
	if value.Architecture != nil {
		input.Architecture = string(*value.Architecture)
	}
	if value.CredentialId != nil {
		input.CredentialID = uuid.NullUUID{UUID: uuid.UUID(*value.CredentialId), Valid: true}
	}
	if value.Username != nil {
		input.Username = *value.Username
	}
	if value.ConnectionTestId != nil {
		input.ConnectionTestID = uuid.NullUUID{UUID: uuid.UUID(*value.ConnectionTestId), Valid: true}
	}
	return input
}

func (handler HostHandler) PreviewUpdateHost(ctx context.Context, request hostapi.PreviewUpdateHostRequestObject) (hostapi.PreviewUpdateHostResponseObject, error) {
	p, apiError := handler.auth(ctx, true, request.Params.XCSRFToken, "host.manage")
	if apiError != nil {
		return hostapi.PreviewUpdateHostdefaultJSONResponse{Body: *apiError, StatusCode: http.StatusForbidden}, nil
	}
	if request.Body == nil {
		return hostapi.PreviewUpdateHostdefaultJSONResponse{Body: hostError(ctx, errors.New("invalid request")), StatusCode: http.StatusBadRequest}, nil
	}
	input := hostUpdateInput(*request.Body)
	action, err := handler.Service.PreviewUpdateHost(ctx, resourceSubject(p), p.EnterpriseIDValue(), uuid.UUID(request.Id), input, request.Params.IdempotencyKey)
	if err != nil {
		return hostapi.PreviewUpdateHostdefaultJSONResponse{Body: hostError(ctx, err), StatusCode: resourceStatus(err)}, nil
	}
	return hostapi.PreviewUpdateHost201JSONResponse(pendingForHost(action)), nil
}

func (handler HostHandler) PreviewDeleteHost(ctx context.Context, request hostapi.PreviewDeleteHostRequestObject) (hostapi.PreviewDeleteHostResponseObject, error) {
	p, apiError := handler.auth(ctx, true, request.Params.XCSRFToken, "host.manage")
	if apiError != nil {
		return hostapi.PreviewDeleteHostdefaultJSONResponse{Body: *apiError, StatusCode: http.StatusForbidden}, nil
	}
	if request.Body == nil {
		return hostapi.PreviewDeleteHostdefaultJSONResponse{Body: hostError(ctx, errors.New("invalid request")), StatusCode: http.StatusBadRequest}, nil
	}
	action, err := handler.Service.PreviewDeleteHost(ctx, resourceSubject(p), p.EnterpriseIDValue(), uuid.UUID(request.Id), request.Body.ExpectedVersion, request.Params.IdempotencyKey)
	if err != nil {
		return hostapi.PreviewDeleteHostdefaultJSONResponse{Body: hostError(ctx, err), StatusCode: resourceStatus(err)}, nil
	}
	return hostapi.PreviewDeleteHost201JSONResponse(pendingForHost(action)), nil
}

func (handler HostHandler) auth(ctx context.Context, mutation bool, csrf, permission string) (identity.Principal, *hostapi.ApiError) {
	p, value := handler.Identity.enterprisePrincipal(ctx, mutation, csrf, permission)
	if value == nil {
		return p, nil
	}
	return identity.Principal{}, &hostapi.ApiError{Code: value.Code, Message: value.Message, MessageKey: value.MessageKey,
		Params: copyErrorParams[map[string]hostapi.ApiError_Params_AdditionalProperties](value.Params), RequestId: value.RequestId,
		Retryable: value.Retryable, TraceId: value.TraceId}
}

func resourceSubject(value identity.Principal) resource.Subject {
	return resource.Subject{ActorID: value.ActorID(), AuthorizationVersion: value.AuthorizationVersion(), AuthorizedResourceIDs: value.AuthorizedResourceIDs}
}

func hostUpdateInput(value hostapi.HostPreviewUpdate) resource.HostInput {
	result := resource.HostInput{ExpectedVersion: value.ExpectedVersion}
	if value.Name != nil {
		result.Name = *value.Name
	}
	if value.Hostname != nil {
		result.Hostname = *value.Hostname
	}
	if value.Environment != nil {
		result.Environment = string(*value.Environment)
	}
	if value.Labels != nil {
		result.Labels = stringMap(*value.Labels)
	}
	return result
}

func toHost(value db.Host, runtimeState db.HostRuntimeObservation, onboarding onboardingView) hostapi.Host {
	labels, _ := resource.DecodeLabels(value.Labels)
	result := hostapi.Host{Id: openapi_types.UUID(value.ID), EnterpriseId: pointerUUID(value.EnterpriseID), Name: value.Name, Address: value.Address.String, Port: int(value.Port),
		Platform: hostapi.HostPlatform(value.Platform), Role: hostapi.HostRole(value.Role), ControlPath: hostapi.HostControlPath(value.ControlPath), Environment: hostapi.Environment(value.Environment),
		Labels: hostapi.Labels(labels), LabelsVersion: value.LabelsVersion, ResourceVersion: value.ResourceVersion, ConnectionStatus: hostapi.HostConnectionStatus(value.ConnectionStatus),
		Status: hostapi.HostStatus(value.Status), Onboarding: toHostOnboarding(onboarding), CreatedAt: value.CreatedAt.Time, UpdatedAt: value.UpdatedAt.Time,
		RemovalGeneration: &value.RemovalGeneration, LocalCleanup: pointerHostCleanup(value.LocalCleanup)}
	if value.Hostname != "" {
		result.Hostname = &value.Hostname
	}
	if value.BastionScopeID.Valid {
		id := openapi_types.UUID(value.BastionScopeID.UUID)
		result.BastionScopeId = &id
	}
	if value.ConnectorID.Valid {
		id := openapi_types.UUID(value.ConnectorID.UUID)
		result.ConnectorId = &id
	}
	if value.PinnedHostKey != "" {
		result.PinnedHostKey = &value.PinnedHostKey
	}
	if value.Architecture.Valid {
		architecture := hostapi.HostArchitecture(value.Architecture.String)
		result.Architecture = &architecture
	}
	if value.LastSeenAt.Valid {
		result.LastSeenAt = &value.LastSeenAt.Time
	}
	if onboarding.RemovalOperationID.Valid {
		id := openapi_types.UUID(onboarding.RemovalOperationID.UUID)
		result.RemovalOperationId = &id
	}
	if runtimeState.HostID != uuid.Nil {
		result.Runtime = &hostapi.HostRuntimeObservation{Platform: hostapi.HostRuntimeObservationPlatform(runtimeState.Platform),
			OpensshStatus: hostapi.HostRuntimeObservationOpensshStatus(runtimeState.OpensshStatus), RdpStatus: hostapi.HostRuntimeObservationRdpStatus(runtimeState.RdpStatus),
			RdpNlaEnabled: runtimeState.RdpNlaEnabled, RdpFirewallEnabled: runtimeState.RdpFirewallEnabled,
			RdpServiceRunning: runtimeState.RdpServiceRunning, ObservedAt: runtimeState.ObservedAt.Time}
	}
	return result
}

func toHostOnboarding(value onboardingView) hostapi.OnboardingProjection {
	result := hostapi.OnboardingProjection{State: hostapi.OnboardingProjectionState(value.State), UpdatedAt: value.UpdatedAt}
	if value.PendingActionRef != "" {
		result.PendingActionRef = &value.PendingActionRef
	}
	if value.ExecutionID.Valid {
		id := openapi_types.UUID(value.ExecutionID.UUID)
		result.ExecutionId = &id
	}
	if value.OperationID.Valid {
		id := openapi_types.UUID(value.OperationID.UUID)
		result.OperationId = &id
	}
	if value.ErrorCode != "" {
		result.ErrorCode = &value.ErrorCode
	}
	if value.InstallMethod != "" {
		method := hostapi.HostInstallMethod(value.InstallMethod)
		result.InstallMethod = &method
	}
	if value.SSHPath != "" {
		path := hostapi.HostSSHPath(value.SSHPath)
		result.SshPath = &path
	}
	return result
}

func toHostConnectionTest(value db.ConnectionTest) hostapi.ConnectionTest {
	var resultValue resource.ConnectionTestResult
	_ = json.Unmarshal(value.Result, &resultValue)
	checks := make([]struct {
		Detail *string                            `json:"detail,omitempty"`
		Name   string                             `json:"name"`
		Status hostapi.ConnectionTestChecksStatus `json:"status"`
	}, 0, len(resultValue.Checks))
	for _, check := range resultValue.Checks {
		name, status, detail := check["name"], check["status"], check["detail"]
		item := struct {
			Detail *string                            `json:"detail,omitempty"`
			Name   string                             `json:"name"`
			Status hostapi.ConnectionTestChecksStatus `json:"status"`
		}{Name: name, Status: hostapi.ConnectionTestChecksStatus(status)}
		if detail != "" {
			item.Detail = &detail
		}
		checks = append(checks, item)
	}
	result := hostapi.ConnectionTest{Id: openapi_types.UUID(value.ID), EnterpriseId: pointerUUID(value.EnterpriseID), TargetType: hostapi.ConnectionTestTargetType(value.TargetType),
		Path: hostapi.ConnectionTestPath(value.Path), Status: hostapi.ConnectionTestStatus(value.Status), Checks: checks, ExpiresAt: value.ExpiresAt.Time,
		CreatedAt: value.CreatedAt.Time, UpdatedAt: value.UpdatedAt.Time}
	if value.ResourceID.Valid {
		id := openapi_types.UUID(value.ResourceID.UUID)
		result.ResourceId = &id
	}
	if resultValue.LatencyMS > 0 {
		latency := int(resultValue.LatencyMS)
		result.LatencyMs = &latency
	}
	if len(resultValue.ResolvedIPs) > 0 {
		result.ResolvedIps = &resultValue.ResolvedIPs
	}
	if resultValue.HostKeyFingerprint != "" {
		result.HostKeyFingerprint = &resultValue.HostKeyFingerprint
	}
	if resultValue.RemoteVersion != "" {
		result.RemoteVersion = &resultValue.RemoteVersion
	}
	if resultValue.Platform != "" {
		platform := hostapi.ConnectionTestPlatform(resultValue.Platform)
		result.Platform = &platform
	}
	if resultValue.Architecture != "" {
		architecture := hostapi.ConnectionTestArchitecture(resultValue.Architecture)
		result.Architecture = &architecture
	}
	if resultValue.DistributionVersion != "" {
		result.DistributionVersion = &resultValue.DistributionVersion
	}
	if resultValue.ServiceManager != "" {
		serviceManager := hostapi.ConnectionTestServiceManager(resultValue.ServiceManager)
		result.ServiceManager = &serviceManager
	}
	if resultValue.Platform != "" {
		privileged := resultValue.Privileged
		freeDiskBytes := int64(resultValue.FreeDiskBytes)
		result.Privileged = &privileged
		result.FreeDiskBytes = &freeDiskBytes
	}
	if value.ErrorCode.Valid {
		result.ErrorCode = &value.ErrorCode.String
	}
	return result
}

func toHostOnboardingOperation(operation db.HostOnboardingOperation, events []db.HostOnboardingOperationEvent) hostapi.HostOnboardingOperation {
	converted := make([]hostapi.HostOnboardingOperationEvent, 0, len(events))
	for _, event := range events {
		item := hostapi.HostOnboardingOperationEvent{Id: openapi_types.UUID(event.ID), Stage: hostapi.HostOnboardingOperationEventStage(event.Stage),
			Status: hostapi.HostOnboardingOperationEventStatus(event.Status), OccurredAt: event.OccurredAt.Time}
		if event.ErrorCode.Valid {
			item.ErrorCode = &event.ErrorCode.String
		}
		converted = append(converted, item)
	}
	result := hostapi.HostOnboardingOperation{Id: openapi_types.UUID(operation.ID), HostId: openapi_types.UUID(operation.HostID), ConnectorId: openapi_types.UUID(operation.ConnectorID),
		InstallMethod: hostapi.HostOnboardingOperationInstallMethod(operation.InstallMethod), SshPath: hostapi.HostOnboardingOperationSshPath(operation.SshPath),
		TargetPlatform: hostapi.HostOnboardingOperationTargetPlatform(operation.TargetPlatform), ControlPath: hostapi.HostControlPath(operation.ControlPath),
		Stage: hostapi.HostOnboardingOperationStage(operation.Stage), Status: hostapi.HostOnboardingOperationStatus(operation.Status), Attempts: int(operation.Attempts),
		MaxAttempts: 3, Events: converted, ExpiresAt: operation.ExpiresAt.Time, CreatedAt: operation.CreatedAt.Time, UpdatedAt: operation.UpdatedAt.Time}
	if operation.BastionScopeID.Valid {
		id := openapi_types.UUID(operation.BastionScopeID.UUID)
		result.BastionScopeId = &id
	}
	if operation.ConnectionTestID.Valid {
		id := openapi_types.UUID(operation.ConnectionTestID.UUID)
		result.ConnectionTestId = &id
	}
	if operation.RetryOf.Valid {
		id := openapi_types.UUID(operation.RetryOf.UUID)
		result.RetryOf = &id
	}
	id := openapi_types.UUID(operation.ReleaseVersionID)
	result.ReleaseVersionId = &id
	if operation.ConnectorOnlineAt.Valid {
		result.ConnectorOnlineAt = &operation.ConnectorOnlineAt.Time
	}
	if operation.CompletedAt.Valid {
		result.CompletedAt = &operation.CompletedAt.Time
	}
	if operation.ErrorCode.Valid {
		result.ErrorCode = &operation.ErrorCode.String
	}
	return result
}

func toHostRemovalOperation(view hostremoval.View) hostapi.HostRemovalOperation {
	operation := view.Operation
	events := make([]hostapi.HostRemovalOperationEvent, 0, len(view.Events))
	for _, value := range view.Events {
		item := hostapi.HostRemovalOperationEvent{Id: openapi_types.UUID(value.ID), Sequence: value.Sequence,
			Stage: hostapi.HostRemovalStage(value.Stage), Status: hostapi.HostRemovalOperationEventStatus(value.Status), OccurredAt: value.OccurredAt.Time}
		if value.ErrorCode.Valid {
			item.ErrorCode = &value.ErrorCode.String
		}
		events = append(events, item)
	}
	targetID := operation.HostID
	if operation.TargetType == hostremoval.TargetBastion && operation.BastionScopeID.Valid {
		targetID = operation.BastionScopeID.UUID
	}
	result := hostapi.HostRemovalOperation{Id: openapi_types.UUID(operation.ID), TargetType: hostapi.HostRemovalTargetType(operation.TargetType),
		TargetId: openapi_types.UUID(targetID), ConnectorId: openapi_types.UUID(operation.ConnectorID), Mode: hostapi.HostRemovalMode(operation.RemovalMode),
		DeliveryMethod: hostapi.HostRemovalOperationDeliveryMethod(operation.DeliveryMethod), SshPath: hostapi.HostSSHPath(operation.SshPath),
		TargetPlatform: hostapi.HostRemovalOperationTargetPlatform(operation.TargetPlatform), Status: hostapi.HostRemovalStatus(operation.Status),
		Stage: hostapi.HostRemovalStage(operation.Stage), Attempt: int(operation.Attempts), MaxAttempts: 10,
		LocalCleanup: hostapi.HostRemovalOperationLocalCleanup(operation.LocalCleanup), Events: events, ExpiresAt: operation.ExpiresAt.Time,
		CreatedAt: operation.CreatedAt.Time, UpdatedAt: operation.UpdatedAt.Time}
	if operation.ErrorCode.Valid {
		result.ErrorCode = &operation.ErrorCode.String
	}
	if operation.CompletedAt.Valid {
		result.CompletedAt = &operation.CompletedAt.Time
	}
	return result
}

func pendingForHost(value db.PendingAction) hostapi.PendingActionPublicSchema {
	return convertPending[hostapi.PendingActionPublicSchema](value)
}

func convertPending[T any](value db.PendingAction) T {
	var preview any = map[string]any{}
	var diff any = []any{}
	_ = json.Unmarshal(value.Preview, &preview)
	_ = json.Unmarshal(value.Diff, &diff)
	available := []string{}
	switch value.Status {
	case "awaiting_confirmation":
		available = []string{"confirm", "cancel"}
	case "awaiting_approval":
		available = []string{"approve", "reject", "cancel"}
	}
	body := map[string]any{"schema_version": "argus.pending_action/v1", "action_ref": value.ActionRef, "action_type": value.ActionType, "title": value.Title, "summary": value.Summary,
		"risk": value.Risk, "preview": preview, "diff": diff, "status": value.Status, "available_actions": available,
		"expires_at": value.ExpiresAt.Time, "created_at": value.CreatedAt.Time, "updated_at": value.UpdatedAt.Time}
	if value.ResultSummary != "" {
		body["result_summary"] = value.ResultSummary
	}
	encoded, _ := json.Marshal(body)
	var result T
	_ = json.Unmarshal(encoded, &result)
	return result
}

func optionalUUID(value *openapi_types.UUID) uuid.NullUUID {
	if value == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: uuid.UUID(*value), Valid: true}
}

func pointerHostCleanup(value string) *hostapi.HostLocalCleanup {
	converted := hostapi.HostLocalCleanup(value)
	return &converted
}

func stringMap[T ~string](value map[string]T) map[string]string {
	result := make(map[string]string, len(value))
	for key, item := range value {
		result[key] = string(item)
	}
	return result
}

func emptyHostPage() hostapi.CursorPage {
	return hostapi.CursorPage{NextCursor: nil, HasMore: false, Partial: hostapi.PartialMetadata{Partial: false, Reasons: []hostapi.PartialMetadataReasons{}}}
}

func hostError(ctx context.Context, err error) hostapi.ApiError {
	base := hostErrorBase(ctx, err)
	logMappedError(ctx, base.Code, err)
	return base
}

func hostErrorBase(ctx context.Context, err error) hostapi.ApiError {
	base := setupErrorBase(ctx, err)
	switch {
	case errors.Is(err, resource.ErrResourceDenied), errors.Is(err, hostremoval.ErrInvalidTarget):
		base.Code, base.MessageKey = "RESOURCE_NOT_FOUND", "errors.common.resource_not_found"
	case errors.Is(err, resource.ErrVersionConflict):
		base.Code, base.MessageKey = "VERSION_CONFLICT", "errors.common.version_conflict"
	case errors.Is(err, resource.ErrResourceNameConflict):
		base.Code, base.MessageKey = "RESOURCE_NAME_CONFLICT", "errors.common.resource_name_conflict"
		base.Retryable = retryablePointer(false)
	case errors.Is(err, resource.ErrInvalidResourceName):
		base.Code, base.MessageKey = "INVALID_ARGUMENT", "errors.common.invalid_argument"
		base.Retryable = retryablePointer(false)
	case errors.Is(err, resource.ErrConnectionTestNeeded):
		base.Code, base.MessageKey = "CONNECTION_TEST_REQUIRED", "errors.connection_test.required"
	case errors.Is(err, resource.ErrDirectTargetDenied):
		base.Code, base.MessageKey = "DIRECT_TARGET_DENIED", "errors.direct.target_denied"
	case errors.Is(err, resource.ErrActionInvalidated):
		base.Code, base.MessageKey = "PENDING_ACTION_INVALIDATED", "errors.actions.pending_action_invalidated"
	case errors.Is(err, resource.ErrHostOnboardingUnsupported):
		base.Code, base.MessageKey = "HOST_ONBOARDING_UNSUPPORTED_PLATFORM", "errors.host_install.unsupported_platform"
	case errors.Is(err, resource.ErrHostOnboardingInvalid):
		base.Code, base.MessageKey = "INVALID_ARGUMENT", "errors.common.invalid_argument"
	case errors.Is(err, telemetryservice.ErrDistributionPending):
		base.Code, base.MessageKey = "COLLECTOR_DISTRIBUTION_VALIDATION_PENDING", "errors.telemetry.distribution_validation_pending"
	case errors.Is(err, telemetryservice.ErrCollectorArtifactUnavailable):
		base.Code, base.MessageKey = "COLLECTOR_ARTIFACT_UNAVAILABLE", "errors.telemetry.artifact_unavailable"
		base.Retryable = retryablePointer(true)
	case errors.Is(err, hostremoval.ErrDependenciesExist):
		base.Code, base.MessageKey = "HOST_REMOVAL_DEPENDENCIES_EXIST", "errors.host_removal.dependencies_exist"
	case errors.Is(err, hostremoval.ErrConnectionTestNeeded):
		base.Code, base.MessageKey = "HOST_REMOVAL_CONNECTION_TEST_REQUIRED", "errors.host_removal.connection_test_required"
	case errors.Is(err, hostremoval.ErrNotInstalled):
		base.Code, base.MessageKey = "HOST_REMOVAL_NOT_INSTALLED", "errors.host_removal.not_installed"
	case errors.Is(err, hostremoval.ErrIdentityChanged):
		base.Code, base.MessageKey = "TARGET_IDENTITY_CHANGED", "errors.host_removal.target_identity_changed"
	case errors.Is(err, hostremoval.ErrTokenInvalid):
		base.Code, base.MessageKey = "HOST_REMOVAL_TOKEN_INVALID", "errors.host_removal.token_invalid"
	case errors.Is(err, hostremoval.ErrOperationState):
		base.Code, base.MessageKey = "HOST_REMOVAL_STATE_CONFLICT", "errors.host_removal.state_conflict"
	}
	result := hostapi.ApiError{Code: base.Code, Message: base.Message, MessageKey: base.MessageKey,
		Params: copyErrorParams[map[string]hostapi.ApiError_Params_AdditionalProperties](base.Params), RequestId: base.RequestId,
		Retryable: base.Retryable, TraceId: base.TraceId}
	if errors.Is(err, resource.ErrInvalidResourceName) {
		result.Params = copyErrorParams[map[string]hostapi.ApiError_Params_AdditionalProperties](map[string]string{"field": "name"})
	}
	return result
}

func hostRemovalStatus(err error) int {
	switch {
	case errors.Is(err, hostremoval.ErrTokenInvalid):
		return http.StatusUnauthorized
	case errors.Is(err, hostremoval.ErrConnectionTestNeeded):
		return http.StatusUnprocessableEntity
	case errors.Is(err, hostremoval.ErrInvalidTarget), errors.Is(err, resource.ErrResourceDenied), errors.Is(err, pgx.ErrNoRows):
		return http.StatusNotFound
	case errors.Is(err, hostremoval.ErrOperationState), errors.Is(err, hostremoval.ErrIdentityChanged),
		errors.Is(err, hostremoval.ErrDependenciesExist), errors.Is(err, hostremoval.ErrNotInstalled), errors.Is(err, resource.ErrVersionConflict):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func resourceStatus(err error) int {
	switch {
	case errors.Is(err, resource.ErrInvalidResourceName):
		return http.StatusBadRequest
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, resource.ErrResourceDenied):
		return http.StatusNotFound
	case errors.Is(err, resource.ErrDirectTargetDenied):
		return http.StatusForbidden
	case errors.Is(err, actionservice.ErrStepUpRequired):
		return http.StatusForbidden
	case errors.Is(err, resource.ErrConnectionTestNeeded), errors.Is(err, resource.ErrHostOnboardingUnsupported):
		return http.StatusUnprocessableEntity
	case errors.Is(err, telemetryservice.ErrCollectorArtifactUnavailable):
		return http.StatusServiceUnavailable
	default:
		return http.StatusConflict
	}
}
