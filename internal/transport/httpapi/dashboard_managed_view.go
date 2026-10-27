package httpapi

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	api "github.com/kakj-go/Argus/internal/gen/openapi/dashboardapi"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func (h DashboardHandler) managedDashboardView(ctx context.Context, item db.Dashboard) (api.DashboardItem, error) {
	view := dashboardView(item)
	managed, err := h.Service.Store.Queries.GetManagedDashboardByID(ctx, db.GetManagedDashboardByIDParams{EnterpriseID: item.EnterpriseID, DashboardID: item.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return view, nil
	}
	if err != nil {
		return view, err
	}
	view.ManagedTemplateKey = &managed.TemplateKey
	view.ManagedTemplateVersion = &managed.AppliedVersion
	return view, nil
}
