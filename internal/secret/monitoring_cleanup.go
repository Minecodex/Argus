package secret

import (
	"context"
	"errors"
	"github.com/google/uuid"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// CleanupMonitoringDrafts removes only unused, owner-verified private drafts.
// Claiming locks both the draft and Secret, fencing concurrent confirmation.
func (s Service) CleanupMonitoringDrafts(ctx context.Context, limit int32) (int, error) {
	if s.Store == nil || limit < 1 || limit > 500 {
		return 0, errors.New("invalid monitoring draft cleanup budget")
	}
	count := 0
	err := s.Store.InTx(ctx, func(q *db.Queries) error {
		drafts, err := q.LockExpiredMonitoringCredentialDrafts(ctx, limit)
		if err != nil {
			return err
		}
		for _, draft := range drafts {
			rows, err := q.DeleteUnusedMonitoringCredentialDraft(ctx, db.DeleteUnusedMonitoringCredentialDraftParams{EnterpriseID: draft.EnterpriseID, ID: draft.ID})
			if err != nil {
				return err
			}
			if rows != 1 {
				return ErrCredentialUnavailable
			}
			if err = q.DeleteUnusedMonitoringDraftVersions(ctx, db.DeleteUnusedMonitoringDraftVersionsParams{EnterpriseID: draft.EnterpriseID, ID: draft.SecretID, OwnerID: uuid.NullUUID{UUID: draft.ID, Valid: true}}); err != nil {
				return err
			}
			rows, err = q.DeleteUnusedMonitoringDraftSecret(ctx, db.DeleteUnusedMonitoringDraftSecretParams{EnterpriseID: draft.EnterpriseID, ID: draft.SecretID, OwnerID: uuid.NullUUID{UUID: draft.ID, Valid: true}})
			if err != nil {
				return err
			}
			if rows != 1 {
				return ErrCredentialUnavailable
			}
			if err = appendAudit(WithActorType(ctx, "system"), q, "monitoring-draft-reconciler", draft.EnterpriseID, "monitoring.credential_draft.discarded", "monitoring_credential_draft", draft.ID, map[string]any{"reason": "expired_or_cancelled"}); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}
