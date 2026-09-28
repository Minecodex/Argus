package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboard"
	api "github.com/kakj-go/Argus/internal/gen/openapi/dashboardapi"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
)

type DashboardHandler struct {
	Identity EnterpriseIdentityHandler
	Service  dashboard.Service
	Runtime  *dashboard.Runtime
	Queries  *dashboard.QueryJobs
}

func (handler DashboardHandler) auth(ctx context.Context, mutation bool, csrf string) (dashboard.Actor, *api.ApiError) {
	permission := "telemetry.dashboard.read"
	if mutation {
		permission = "telemetry.dashboard.manage"
	}
	return handler.authPermission(ctx, mutation, csrf, permission)
}

func (handler DashboardHandler) authPermission(ctx context.Context, mutation bool, csrf, permission string) (dashboard.Actor, *api.ApiError) {
	principal, failure := handler.Identity.enterprisePrincipal(ctx, mutation, csrf, permission)
	if failure != nil {
		converted := dashboardConvert[api.ApiError](failure)
		return dashboard.Actor{}, &converted
	}
	kind := "user"
	if principal.ServiceAccount != nil {
		kind = "service_account"
	}
	id, err := uuid.Parse(principal.ActorID())
	if err != nil {
		failure := dashboardError(ctx, dashboard.ErrDenied)
		return dashboard.Actor{}, &failure
	}
	return dashboard.Actor{EnterpriseID: principal.EnterpriseIDValue(), SubjectID: id, SubjectType: kind, AuthorizationVersion: principal.AuthorizationVersion()}, nil
}

func dashboardConvert[T any](value any) T {
	var output T
	data, err := json.Marshal(value)
	if err == nil {
		_ = json.Unmarshal(data, &output)
	}
	return output
}

func dashboardStatus(err error) int {
	switch {
	case errors.Is(err, telemetry.ErrQueryBudget):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, dashboard.ErrInvalid):
		return http.StatusBadRequest
	case errors.Is(err, dashboard.ErrDenied):
		return http.StatusForbidden
	case errors.Is(err, dashboard.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, dashboard.ErrSelectionStale), errors.Is(err, dashboard.ErrContextExpired), errors.Is(err, dashboard.ErrConflict), errors.Is(err, dashboard.ErrArchived), errors.Is(err, postgres.ErrIdempotencyConflict):
		return http.StatusConflict
	default:
		return http.StatusServiceUnavailable
	}
}

func dashboardError(ctx context.Context, err error) api.ApiError {
	base := setupErrorBase(ctx, err)
	base.Code = "DASHBOARD_UNAVAILABLE"
	base.MessageKey = "errors.dashboard.unavailable"
	base.Retryable = pointer(true)
	if errors.Is(err, telemetry.ErrQueryBudget) {
		base.Code = "QUERY_BUDGET_EXCEEDED"
		base.MessageKey = "errors.telemetry.query_budget_exceeded"
		base.Retryable = pointer(false)
	}
	for _, known := range []error{dashboard.ErrSelectionStale, dashboard.ErrContextExpired, dashboard.ErrInvalid, dashboard.ErrDenied, dashboard.ErrNotFound, dashboard.ErrConflict, dashboard.ErrArchived} {
		if errors.Is(err, known) {
			base.Code = known.Error()
			base.Retryable = pointer(false)
			break
		}
	}
	switch base.Code {
	case "DASHBOARD_SELECTION_STALE":
		base.MessageKey = "errors.dashboard.selection_stale"
	case "DASHBOARD_CONTEXT_EXPIRED":
		base.MessageKey = "errors.dashboard.context_expired"
	case "DASHBOARD_INVALID":
		base.MessageKey = "errors.dashboard.invalid"
	case "DASHBOARD_DENIED":
		base.MessageKey = "errors.dashboard.denied"
	case "DASHBOARD_NOT_FOUND":
		base.MessageKey = "errors.dashboard.not_found"
	case "DASHBOARD_VERSION_CONFLICT":
		base.MessageKey = "errors.dashboard.version_conflict"
	case "DASHBOARD_ARCHIVED":
		base.MessageKey = "errors.dashboard.archived"
	}
	logMappedError(ctx, base.Code, err)
	return dashboardConvert[api.ApiError](base)
}

func dashboardView(item db.Dashboard) api.DashboardItem {
	view := api.DashboardItem{Id: item.ID, ActiveRevisionId: item.ActiveRevisionID.UUID, Name: item.Name, Description: item.Description, Version: item.Version, Lifecycle: item.Lifecycle, UpdatedAt: item.UpdatedAt.Time}
	if item.FolderID.Valid {
		view.FolderId = &item.FolderID.UUID
	}
	return view
}
func dashboardDraftView(item db.DashboardDraft) api.DashboardDraft {
	view := api.DashboardDraft{Id: item.ID, Name: item.Name, Description: item.Description, BaseObjectVersion: item.BaseObjectVersion, DraftVersion: item.DraftVersion, Status: item.Status, UpdatedAt: item.UpdatedAt.Time}
	if item.DashboardID.Valid {
		view.DashboardId = &item.DashboardID.UUID
	}
	if item.BaseRevisionID.Valid {
		view.BaseRevisionId = &item.BaseRevisionID.UUID
	}
	if item.FolderID.Valid {
		view.FolderId = &item.FolderID.UUID
	}
	_ = json.Unmarshal(item.Spec, &view.Spec)
	_ = json.Unmarshal(item.ProposedBindings, &view.ProposedBindings)
	return view
}
func dashboardRevisionView(item db.DashboardRevision) api.DashboardRevision {
	view := api.DashboardRevision{Id: item.ID, DashboardId: item.DashboardID, RevisionNumber: item.RevisionNumber, Name: item.Name, Description: item.Description, SpecHash: item.SpecHash, CreatedAt: item.CreatedAt.Time}
	_ = json.Unmarshal(item.Spec, &view.Spec)
	_ = json.Unmarshal(item.ValidationReport, &view.Validation)
	// Stored samples are data, not configuration. Reading the dashboard alone
	// must not expose another editor's sampled resources or observations.
	view.Sample = map[string]any{"status": "requires_resource_authorization"}
	return view
}

func (handler DashboardHandler) ListDashboards(ctx context.Context, r api.ListDashboardsRequestObject) (api.ListDashboardsResponseObject, error) {
	actor, failure := handler.auth(ctx, false, "")
	if failure != nil {
		return api.ListDashboardsdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	items, err := handler.Service.List(ctx, actor)
	if err != nil {
		return api.ListDashboardsdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	result := []api.DashboardItem{}
	for _, item := range items {
		result = append(result, dashboardView(item))
	}
	return api.ListDashboards200JSONResponse(result), nil
}
func (handler DashboardHandler) GetDashboard(ctx context.Context, r api.GetDashboardRequestObject) (api.GetDashboardResponseObject, error) {
	actor, failure := handler.auth(ctx, false, "")
	if failure != nil {
		return api.GetDashboarddefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	item, revision, err := handler.Service.Get(ctx, actor, r.Id)
	if err != nil {
		return api.GetDashboarddefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	return api.GetDashboard200JSONResponse{Dashboard: dashboardView(item), Revision: dashboardRevisionView(revision)}, nil
}
func (handler DashboardHandler) ListDashboardRevisions(ctx context.Context, r api.ListDashboardRevisionsRequestObject) (api.ListDashboardRevisionsResponseObject, error) {
	actor, failure := handler.auth(ctx, false, "")
	if failure != nil {
		return api.ListDashboardRevisionsdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	items, err := handler.Service.Revisions(ctx, actor, r.Id)
	if err != nil {
		return api.ListDashboardRevisionsdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	result := []api.DashboardRevision{}
	for _, item := range items {
		result = append(result, dashboardRevisionView(item))
	}
	return api.ListDashboardRevisions200JSONResponse(result), nil
}
func (handler DashboardHandler) ListDashboardDrafts(ctx context.Context, r api.ListDashboardDraftsRequestObject) (api.ListDashboardDraftsResponseObject, error) {
	actor, failure := handler.auth(ctx, false, "")
	if failure != nil {
		return api.ListDashboardDraftsdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	items, err := handler.Service.ListDrafts(ctx, actor)
	if err != nil {
		return api.ListDashboardDraftsdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	result := []api.DashboardDraft{}
	for _, item := range items {
		result = append(result, dashboardDraftView(item))
	}
	return api.ListDashboardDrafts200JSONResponse(result), nil
}
func (handler DashboardHandler) GetDashboardDraft(ctx context.Context, r api.GetDashboardDraftRequestObject) (api.GetDashboardDraftResponseObject, error) {
	actor, failure := handler.auth(ctx, false, "")
	if failure != nil {
		return api.GetDashboardDraftdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	item, err := handler.Service.Draft(ctx, actor, r.Id)
	if err != nil {
		return api.GetDashboardDraftdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	return api.GetDashboardDraft200JSONResponse(dashboardDraftView(item)), nil
}
func (handler DashboardHandler) CreateDashboardDraft(ctx context.Context, r api.CreateDashboardDraftRequestObject) (api.CreateDashboardDraftResponseObject, error) {
	actor, failure := handler.auth(ctx, true, r.Params.XCSRFToken)
	if failure != nil {
		return api.CreateDashboardDraftdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	if r.Body == nil {
		return api.CreateDashboardDraftdefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrInvalid), StatusCode: 400}, nil
	}
	item, err := handler.Service.CreateDraft(ctx, actor, dashboardConvert[dashboard.DraftInput](r.Body))
	if err != nil {
		return api.CreateDashboardDraftdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	return api.CreateDashboardDraft201JSONResponse(dashboardDraftView(item)), nil
}
func (handler DashboardHandler) SaveDashboardDraft(ctx context.Context, r api.SaveDashboardDraftRequestObject) (api.SaveDashboardDraftResponseObject, error) {
	actor, failure := handler.auth(ctx, true, r.Params.XCSRFToken)
	if failure != nil {
		return api.SaveDashboardDraftdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	if r.Body == nil || r.Body.ExpectedVersion == nil {
		return api.SaveDashboardDraftdefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrInvalid), StatusCode: 400}, nil
	}
	item, err := handler.Service.SaveDraft(ctx, actor, r.Id, dashboardConvert[dashboard.DraftInput](r.Body))
	if err != nil {
		return api.SaveDashboardDraftdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	return api.SaveDashboardDraft200JSONResponse(dashboardDraftView(item)), nil
}
func (handler DashboardHandler) DiscardDashboardDraft(ctx context.Context, r api.DiscardDashboardDraftRequestObject) (api.DiscardDashboardDraftResponseObject, error) {
	actor, failure := handler.auth(ctx, true, r.Params.XCSRFToken)
	if failure != nil {
		return api.DiscardDashboardDraftdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	if r.Body == nil {
		return api.DiscardDashboardDraftdefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrInvalid), StatusCode: 400}, nil
	}
	if err := handler.Service.DiscardDraft(ctx, actor, r.Id, r.Body.ExpectedVersion); err != nil {
		return api.DiscardDashboardDraftdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	return api.DiscardDashboardDraft204Response{}, nil
}
func (handler DashboardHandler) RebaseDashboardDraft(ctx context.Context, r api.RebaseDashboardDraftRequestObject) (api.RebaseDashboardDraftResponseObject, error) {
	actor, failure := handler.auth(ctx, true, r.Params.XCSRFToken)
	if failure != nil {
		return api.RebaseDashboardDraftdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	if r.Body == nil {
		return api.RebaseDashboardDraftdefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrInvalid), StatusCode: 400}, nil
	}
	item, err := handler.Service.RebaseDraft(ctx, actor, r.Id, r.Body.ExpectedVersion, r.Body.ObjectVersion, r.Body.RevisionId)
	if err != nil {
		return api.RebaseDashboardDraftdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	return api.RebaseDashboardDraft200JSONResponse(dashboardDraftView(item)), nil
}
func (handler DashboardHandler) PreviewDashboardPublication(ctx context.Context, r api.PreviewDashboardPublicationRequestObject) (api.PreviewDashboardPublicationResponseObject, error) {
	actor, failure := handler.auth(ctx, true, r.Params.XCSRFToken)
	if failure != nil {
		return api.PreviewDashboardPublicationdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	if r.Body == nil {
		return api.PreviewDashboardPublicationdefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrInvalid), StatusCode: 400}, nil
	}
	preview, err := handler.Service.PreviewPublish(ctx, actor, r.Id, r.Body.ExpectedVersion, r.Params.IdempotencyKey)
	if err != nil {
		return api.PreviewDashboardPublicationdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	return api.PreviewDashboardPublication201JSONResponse(convertPending[api.PendingActionPublicSchema](preview.Action)), nil
}
func (handler DashboardHandler) ValidateDashboardSpec(ctx context.Context, r api.ValidateDashboardSpecRequestObject) (api.ValidateDashboardSpecResponseObject, error) {
	_, failure := handler.auth(ctx, true, r.Params.XCSRFToken)
	if failure != nil {
		return api.ValidateDashboardSpecdefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	if r.Body == nil {
		return api.ValidateDashboardSpecdefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrInvalid), StatusCode: 400}, nil
	}
	return api.ValidateDashboardSpec200JSONResponse(dashboardConvert[api.DashboardValidationReport](dashboard.Validate(dashboardConvert[dashboard.Spec](r.Body)))), nil
}

func (handler DashboardHandler) ConvertDashboardPanel(ctx context.Context, r api.ConvertDashboardPanelRequestObject) (api.ConvertDashboardPanelResponseObject, error) {
	_, failure := handler.auth(ctx, true, r.Params.XCSRFToken)
	if failure != nil {
		return api.ConvertDashboardPaneldefaultJSONResponse{Body: *failure, StatusCode: 403}, nil
	}
	if r.Body == nil {
		return api.ConvertDashboardPaneldefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrInvalid), StatusCode: 400}, nil
	}
	result := dashboard.ConvertPanel(dashboardConvert[dashboard.ConvertPanelInput](r.Body))
	return api.ConvertDashboardPanel200JSONResponse(dashboardConvert[api.DashboardConvertedPanel](result)), nil
}
