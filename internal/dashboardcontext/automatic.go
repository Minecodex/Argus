package dashboardcontext

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/dashboardaccess"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// Acquire records an authorized model discovery without changing the immutable
// admission record. Each discovered dashboard gets its own Run condition
// baseline; completed query contexts and historical Runs are never rewritten.
func Acquire(ctx context.Context, store *postgres.Store, p toolruntime.Principal, run, id uuid.UUID) (Snapshot, error) {
	var result Snapshot
	if store == nil || id == uuid.Nil || run == uuid.Nil {
		return result, invalid()
	}
	err := store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		conversation, err := q.LockConversation(ctx, db.LockConversationParams{ID: p.ConversationID, EnterpriseID: p.EnterpriseID, OwnerUserID: p.UserID})
		if err != nil || conversation.Status != "active" {
			return dashboardaccess.ErrDenied
		}
		active, err := q.GetRun(ctx, db.GetRunParams{ID: run, EnterpriseID: p.EnterpriseID})
		if err != nil || active.ConversationID != p.ConversationID || active.ActorUserID != p.UserID || !slices.Contains([]string{"pending", "running", "waiting_input", "waiting_approval", "waiting_system"}, active.Status) {
			return dashboardaccess.ErrDenied
		}
		result, err = ForRun(ctx, q, p, run)
		if err != nil {
			return err
		}
		if !slices.Contains(result.DashboardIDs, id) {
			result.DashboardIDs = append(result.DashboardIDs, id)
		}
		if result.Mode == "none" {
			result.Mode = "analyze"
		}
		if err := Validate(ctx, q, p, result); err != nil {
			return err
		}
		_, err = q.GetDashboardRunParameters(ctx, db.GetDashboardRunParametersParams{RunID: run, DashboardID: id, EnterpriseID: p.EnterpriseID, OwnerUserID: p.UserID})
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		return freezeParameters(ctx, q, p, run, Snapshot{DashboardIDs: []uuid.UUID{id}})
	})
	return result, err
}

func discoveredSnapshot(ctx context.Context, q *db.Queries, p toolruntime.Principal, run uuid.UUID, seed Snapshot) (Snapshot, error) {
	rows, err := q.ListDashboardRunParameters(ctx, db.ListDashboardRunParametersParams{RunID: run, EnterpriseID: p.EnterpriseID, OwnerUserID: p.UserID})
	if err != nil {
		return seed, err
	}
	for _, row := range rows {
		if !slices.Contains(seed.DashboardIDs, row.DashboardID) {
			seed.DashboardIDs = append(seed.DashboardIDs, row.DashboardID)
		}
	}
	if seed.Mode == "none" && len(seed.DashboardIDs) > 0 {
		seed.Mode = "analyze"
	}
	return seed, nil
}
