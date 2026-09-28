package dashboard

import (
	"context"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboardaccess"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type Actor = dashboardaccess.Actor

func authorize(ctx context.Context, q *db.Queries, a Actor, p string) error {
	return dashboardaccess.Authorize(ctx, q, a, p)
}
func authorizedIDs(ctx context.Context, q *db.Queries, a Actor, kind string) ([]uuid.UUID, error) {
	return dashboardaccess.AuthorizedIDs(ctx, q, a, kind)
}
func requireObject(ctx context.Context, q *db.Queries, a Actor, kind string, id uuid.UUID) error {
	return dashboardaccess.RequireObject(ctx, q, a, kind, id)
}

func checkFolder(ctx context.Context, q *db.Queries, enterprise uuid.UUID, id uuid.NullUUID, lock bool) error {
	if !id.Valid {
		return nil
	}
	var folder db.DashboardFolder
	var err error
	if lock {
		folder, err = q.LockDashboardFolder(ctx, db.LockDashboardFolderParams{ID: id.UUID, EnterpriseID: enterprise})
	} else {
		folder, err = q.GetDashboardFolder(ctx, db.GetDashboardFolderParams{ID: id.UUID, EnterpriseID: enterprise})
	}
	if err != nil {
		return ErrNotFound
	}
	if folder.Status != "active" {
		return ErrArchived
	}
	return nil
}

func nullID(value uuid.UUID) uuid.NullUUID {
	return uuid.NullUUID{UUID: value, Valid: value != uuid.Nil}
}
