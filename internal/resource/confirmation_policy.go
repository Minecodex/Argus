package resource

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Sample policy in the same serializable transaction that creates the action.
// Later conversation changes cannot alter an existing action's consent policy.
func freezeConfirmationPolicy(ctx context.Context, q *db.Queries, action db.PendingAction, subject ActionSubject, runID uuid.UUID) (db.PendingAction, error) {
	run, err := q.GetRun(ctx, db.GetRunParams{ID: runID, EnterpriseID: action.EnterpriseID})
	if err != nil {
		return db.PendingAction{}, err
	}
	if run.ActorUserID.String() != subject.ID || run.AuthorizationVersion != subject.AuthorizationVersion {
		return db.PendingAction{}, ErrActionUnavailable
	}
	c, err := q.GetConversation(ctx, db.GetConversationParams{ID: run.ConversationID, EnterpriseID: action.EnterpriseID, OwnerUserID: run.ActorUserID})
	if err != nil {
		return db.PendingAction{}, err
	}
	return q.FreezeActionConfirmationPolicy(ctx, db.FreezeActionConfirmationPolicyParams{ID: action.ID, EnterpriseID: action.EnterpriseID, ConfirmationMode: c.AccessMode, ConfirmationPolicyVersion: pgtype.Int8{Int64: c.Version, Valid: true}})
}
