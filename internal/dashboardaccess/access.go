package dashboardaccess

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Preserve the dashboard error contract while sharing current-subject checks.
var ErrDenied = errors.New("DASHBOARD_DENIED")

type Actor = authorization.Actor

func accessError(err error) error {
	if errors.Is(err, authorization.ErrAccessDenied) {
		return ErrDenied
	}
	return err
}

func Authorize(ctx context.Context, q *db.Queries, actor Actor, permission string) error {
	return accessError(authorization.Authorize(ctx, q, actor, permission))
}

func AuthorizedIDs(ctx context.Context, q *db.Queries, actor Actor, kind string) ([]uuid.UUID, error) {
	ids, err := authorization.AuthorizedResourceIDs(ctx, q, actor, kind)
	return ids, accessError(err)
}

func RequireObject(ctx context.Context, q *db.Queries, actor Actor, kind string, id uuid.UUID) error {
	if kind == "dashboard" {
		_, err := q.GetManagedDashboardByID(ctx, db.GetManagedDashboardByIDParams{EnterpriseID: actor.EnterpriseID, DashboardID: id})
		if err == nil {
			return Authorize(ctx, q, actor, "telemetry.dashboard.read")
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	return accessError(authorization.RequireResource(ctx, q, actor, kind, id))
}
