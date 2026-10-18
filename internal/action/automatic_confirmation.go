package action

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// AutoConfirmRun is an internal policy executor. Public confirmation requests
// cannot choose this origin or change the policy captured at preview creation.
func (s Service) AutoConfirmRun(ctx context.Context, run db.Run) error {
	if s.Store == nil || s.Resources == nil {
		return ErrUnavailable
	}
	actions, err := s.Store.Queries.ListRunPendingActions(ctx, db.ListRunPendingActionsParams{RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, EnterpriseID: run.EnterpriseID, CreatorSubjectID: run.ActorUserID})
	if err != nil {
		return err
	}
	for _, a := range actions {
		if a.Status != "awaiting_confirmation" || a.Risk == "critical" || (!a.ApprovedPlanCommandID.Valid && (a.ConfirmationMode != "full_access" || !a.ConfirmationPolicyVersion.Valid)) {
			continue
		}
		// Historic fixed remote plans retain their original explicit approval boundary.
		if a.ActionType == "remote_execution.execute" {
			var spec struct {
				Kind string `json:"kind"`
			}
			if json.Unmarshal(a.Preview, &spec) != nil || spec.Kind != "connect" {
				continue
			}
		}
		key := "auto:" + a.ActionRef
		_, err = postgres.ExecuteIdempotent(ctx, s.Store, s.Idempotency, "enterprise", run.ActorUserID.String(), "pending_action.auto_confirm", key, map[string]any{"action_ref": a.ActionRef, "policy_version": a.ConfirmationPolicyVersion.Int64}, 200, func(q *db.Queries) (Confirmation, error) {
			current, e := q.GetRun(ctx, db.GetRunParams{ID: run.ID, EnterpriseID: run.EnterpriseID})
			if e != nil {
				return Confirmation{}, e
			}
			if current.Status == "cancelled" || current.Status == "failed" || current.Status == "succeeded" || current.AuthorizationVersion != run.AuthorizationVersion {
				return Confirmation{}, ErrInvalidated
			}
			source := "access_mode_policy"
			if a.ApprovedPlanCommandID.Valid {
				if a.ActionType != "remote_execution.execute" || !a.ResourceID.Valid {
					return Confirmation{}, ErrInvalidated
				}
				r, e := q.GetRemoteRequest(ctx, db.GetRemoteRequestParams{ID: a.ResourceID.UUID, EnterpriseID: run.EnterpriseID})
				if e != nil {
					return Confirmation{}, e
				}
				valid, e := q.ValidActionPlanCommand(ctx, db.ValidActionPlanCommandParams{ID: a.ApprovedPlanCommandID.UUID, EnterpriseID: run.EnterpriseID, ImplementationRunID: uuid.NullUUID{UUID: run.ID, Valid: true}, ActionRef: a.ActionRef, ContentHash: r.ContentHash})
				if e != nil {
					return Confirmation{}, e
				}
				if !valid {
					return Confirmation{}, ErrInvalidated
				}
				source = "plan_approval"
			}
			return s.confirmWithQueries(ctx, q, run.ActorUserID, run.ActorUserID.String(), key, run.EnterpriseID, run.AuthorizationVersion, false, a.ActionRef, key, source)
		})
		if err != nil {
			return err
		}
	}
	return nil
}
