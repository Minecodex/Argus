package httpapi

import (
	"context"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/dashboardparams"
	api "github.com/kakj-go/Argus/internal/gen/openapi/dashboardapi"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"slices"
)

func (h DashboardHandler) overviewSelection(ctx context.Context, a dashboard.Actor, id uuid.UUID) (dashboard.OverviewSelection, kubernetesworkbench.Scope, error) {
	if h.Runtime == nil || h.Runtime.Inventory == nil {
		return dashboard.OverviewSelection{}, kubernetesworkbench.Scope{}, dashboard.ErrUnavailable
	}
	scope, err := h.Runtime.Inventory.AccessScope(ctx, kubernetesworkbench.Actor(a), id)
	if err != nil {
		return dashboard.OverviewSelection{}, scope, dashboard.ErrDenied
	}
	selection, err := h.Service.Overview(ctx, a, id, scope.ClusterVersion)
	return selection, scope, err
}
func (h DashboardHandler) GetKubernetesOverview(ctx context.Context, r api.GetKubernetesOverviewRequestObject) (api.GetKubernetesOverviewResponseObject, error) {
	a, f := h.auth(ctx, false, "")
	if f != nil {
		return api.GetKubernetesOverviewdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	value, _, err := h.overviewSelection(ctx, a, r.Id)
	if err != nil {
		return api.GetKubernetesOverviewdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	defaultView, err := h.managedDashboardView(ctx, value.DefaultDashboard)
	if err != nil {
		return nil, err
	}
	out := api.KubernetesOverviewSelection{ClusterId: value.ClusterID, ResourceVersion: value.ResourceVersion, SelectionVersion: value.SelectionVersion, Mode: api.KubernetesOverviewSelectionMode(value.Mode), DefaultDashboard: defaultView}
	if value.Dashboard != nil {
		view, e := h.managedDashboardView(ctx, *value.Dashboard)
		if e != nil {
			return nil, e
		}
		out.Dashboard = &view
	}
	if value.UnavailableCode != "" {
		out.UnavailableCode = &value.UnavailableCode
	}
	return api.GetKubernetesOverview200JSONResponse(out), nil
}
func (h DashboardHandler) PreviewKubernetesOverview(ctx context.Context, r api.PreviewKubernetesOverviewRequestObject) (api.PreviewKubernetesOverviewResponseObject, error) {
	a, f := h.authPermission(ctx, true, r.Params.XCSRFToken, "telemetry.dashboard.read")
	if f != nil {
		return api.PreviewKubernetesOverviewdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	if r.Body == nil {
		return api.PreviewKubernetesOverviewdefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrInvalid), StatusCode: 400}, nil
	}
	action, err := h.Service.PreviewOverview(ctx, a, r.Id, dashboardConvert[dashboard.OverviewInput](*r.Body), r.Params.IdempotencyKey)
	if err != nil {
		return api.PreviewKubernetesOverviewdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	return api.PreviewKubernetesOverview201JSONResponse(convertPending[api.PendingActionPublicSchema](action)), nil
}
func (h DashboardHandler) ExecuteKubernetesOverview(ctx context.Context, r api.ExecuteKubernetesOverviewRequestObject) (api.ExecuteKubernetesOverviewResponseObject, error) {
	a, f := h.auth(ctx, false, "")
	if f != nil {
		return api.ExecuteKubernetesOverviewdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	if r.Body == nil {
		return api.ExecuteKubernetesOverviewdefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrInvalid), StatusCode: 400}, nil
	}
	selection, scope, err := h.overviewSelection(ctx, a, r.Id)
	if err != nil {
		return api.ExecuteKubernetesOverviewdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	if selection.Dashboard == nil || selection.Dashboard.ID != r.Body.ExpectedDashboardId {
		return api.ExecuteKubernetesOverviewdefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrSelectionStale), StatusCode: 409}, nil
	}
	input := dashboardConvert[dashboard.ExecutionInput](*r.Body)
	if input.KubernetesScope == nil {
		input.KubernetesScope = &dashboardparams.KubernetesScope{ClusterID: r.Id}
	}
	if input.KubernetesScope.ClusterID != r.Id {
		return api.ExecuteKubernetesOverviewdefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrDenied), StatusCode: 403}, nil
	}
	if !scope.ClusterWide {
		allowed := []uuid.UUID{}
		for _, ns := range scope.Namespaces {
			allowed = append(allowed, ns.ID)
		}
		if len(input.KubernetesScope.NamespaceIDs) == 0 {
			input.KubernetesScope.NamespaceIDs = allowed
		}
		for _, id := range input.KubernetesScope.NamespaceIDs {
			if !slices.Contains(allowed, id) {
				return api.ExecuteKubernetesOverviewdefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrDenied), StatusCode: 403}, nil
			}
		}
	}
	input.ResourceMode = "explicit"
	input.ResourceIDs = []uuid.UUID{r.Id}
	input.ResourceTypes = []string{"kubernetes_cluster"}
	input.ApplicationMode = ""
	input.ApplicationIDs = nil
	result, err := h.Runtime.Execute(ctx, a, selection.Dashboard.ID, input)
	if err != nil {
		return api.ExecuteKubernetesOverviewdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	current, _, err := h.overviewSelection(ctx, a, r.Id)
	if err != nil || current.SelectionVersion != selection.SelectionVersion || current.Dashboard == nil || current.Dashboard.ID != selection.Dashboard.ID {
		return api.ExecuteKubernetesOverviewdefaultJSONResponse{Body: dashboardError(ctx, dashboard.ErrSelectionStale), StatusCode: 409}, nil
	}
	return api.ExecuteKubernetesOverview200JSONResponse(dashboardConvert[api.DashboardExecution](result)), nil
}

func (h DashboardHandler) ListKubernetesOverviewCandidates(ctx context.Context, r api.ListKubernetesOverviewCandidatesRequestObject) (api.ListKubernetesOverviewCandidatesResponseObject, error) {
	a, f := h.auth(ctx, false, "")
	if f != nil {
		return api.ListKubernetesOverviewCandidatesdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	search, cursor, limit := "", "", 50
	if r.Params.Search != nil {
		search = *r.Params.Search
	}
	if r.Params.Cursor != nil {
		cursor = *r.Params.Cursor
	}
	if r.Params.Limit != nil {
		limit = *r.Params.Limit
	}
	value, err := h.Service.OverviewCandidates(ctx, a, r.Id, search, cursor, limit)
	if err != nil {
		return api.ListKubernetesOverviewCandidatesdefaultJSONResponse{Body: dashboardError(ctx, err), StatusCode: dashboardStatus(err)}, nil
	}
	out := api.KubernetesOverviewCandidates{Items: []api.DashboardItem{}}
	for _, row := range value.Items {
		view, e := h.managedDashboardView(ctx, row)
		if e != nil {
			return nil, e
		}
		out.Items = append(out.Items, view)
	}
	if value.NextCursor != "" {
		out.NextCursor = &value.NextCursor
	}
	return api.ListKubernetesOverviewCandidates200JSONResponse(out), nil
}
