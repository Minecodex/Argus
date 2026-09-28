package dashboardcontext

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/dashboardparams"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// The conversation lock makes selection, inherited conditions and the accepted
// user message one admission fact. Each Run starts from its own saved baseline.
func freezeParameters(ctx context.Context, q *db.Queries, p toolruntime.Principal, run uuid.UUID, s Snapshot) error {
	for _, id := range s.DashboardIDs {
		raw, _ := json.Marshal(dashboardparams.Empty())
		version := int64(0)
		prior, err := q.GetDashboardParameterState(ctx, db.GetDashboardParameterStateParams{ConversationID: p.ConversationID, DashboardID: id, EnterpriseID: p.EnterpriseID, OwnerUserID: p.UserID})
		if err == nil {
			raw, version = prior.State, prior.Version
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err = q.CreateDashboardRunParameters(ctx, db.CreateDashboardRunParametersParams{RunID: run, EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, DashboardID: id, OwnerUserID: p.UserID, Version: version, State: raw}); err != nil {
			return err
		}
	}
	return nil
}

func parameterFact(ctx context.Context, q *db.Queries, p toolruntime.Principal, id, run uuid.UUID) (map[string]any, error) {
	raw, version := []byte(nil), int64(0)
	var err error
	if run == uuid.Nil {
		var row db.DashboardParameterState
		row, err = q.GetDashboardParameterState(ctx, db.GetDashboardParameterStateParams{ConversationID: p.ConversationID, DashboardID: id, EnterpriseID: p.EnterpriseID, OwnerUserID: p.UserID})
		raw, version = row.State, row.Version
	} else {
		var row db.DashboardRunParameter
		row, err = q.GetDashboardRunParameters(ctx, db.GetDashboardRunParametersParams{RunID: run, DashboardID: id, EnterpriseID: p.EnterpriseID, OwnerUserID: p.UserID})
		raw, version = row.State, row.Version
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	state := dashboardparams.Empty()
	if len(raw) > 0 && json.Unmarshal(raw, &state) != nil {
		return nil, invalid()
	}
	// Defaults are resolved against the latest publication by context.resolve.
	// Historical inferred panel selection and implicit defaults are not inherited.
	return map[string]any{"version": version, "explicit_conditions": state.Overrides, "revision_validation": "required"}, nil
}
