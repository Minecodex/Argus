package secret

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

var ErrMonitoringManagedSecret = errors.New("monitoring credentials must be managed through their collection")

// StageMonitoringWithQueries never exposes a platform-visible credential. The
// caller owns the subject/target-bound draft and commits it in the same transaction.
func (s Service) StageMonitoringWithQueries(ctx context.Context, q *db.Queries, enterprise, actor, draft uuid.UUID, value []byte) (uuid.UUID, error) {
	if enterprise == uuid.Nil || actor == uuid.Nil || draft == uuid.Nil || len(value) == 0 || len(value) > 64<<10 {
		return uuid.Nil, ErrSecretValueRequired
	}
	id := newUUID()
	envelope, err := s.Keyring.EncryptContext(ctx, value, secretAAD(enterprise, id, 1, "api_token"))
	if err != nil {
		return uuid.Nil, err
	}
	record, err := q.CreateSecret(ctx, db.CreateSecretParams{ID: id, EnterpriseID: enterprise, Name: "monitoring-draft-" + id.String(), Type: "api_token", CreatedBy: actor})
	if err != nil {
		return uuid.Nil, err
	}
	if _, err = q.CreateSecretVersion(ctx, envelopeParams(record, 1, envelope)); err != nil {
		return uuid.Nil, err
	}
	if err = q.MarkMonitoringManagedSecret(ctx, db.MarkMonitoringManagedSecretParams{EnterpriseID: enterprise, ID: id, OwnerType: "monitoring_draft", OwnerID: uuid.NullUUID{UUID: draft, Valid: true}}); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

// BindMonitoringWithQueries transfers an already encrypted draft to exactly one
// collection. No authentication material is written into the collection or plan.
func (s Service) BindMonitoringWithQueries(ctx context.Context, q *db.Queries, enterprise, draft, secretID, collection uuid.UUID) (db.Credential, error) {
	record, err := q.GetSecret(ctx, db.GetSecretParams{EnterpriseID: enterprise, ID: secretID})
	if err != nil || record.Purpose != "monitoring" || record.OwnerType != "monitoring_draft" || !record.OwnerID.Valid || record.OwnerID.UUID != draft || record.Status != "active" || collection == uuid.Nil {
		return db.Credential{}, ErrCredentialUnavailable
	}
	if err = q.MarkMonitoringManagedSecret(ctx, db.MarkMonitoringManagedSecretParams{EnterpriseID: enterprise, ID: secretID, OwnerType: "monitoring_collection", OwnerID: uuid.NullUUID{UUID: collection, Valid: true}}); err != nil {
		return db.Credential{}, err
	}
	id := newUUID()
	return q.CreateCredential(ctx, db.CreateCredentialParams{ID: id, EnterpriseID: enterprise, Name: "monitoring-" + id.String(), Protocol: "monitoring", SecretID: secretID})
}

func (s Service) RetireMonitoringWithQueries(ctx context.Context, q *db.Queries, enterprise, collection, credentialID uuid.UUID) error {
	used, err := q.MonitoringEffectiveCredentialInUse(ctx, db.MonitoringEffectiveCredentialInUseParams{EnterpriseID: enterprise, ID: collection, CredentialID: uuid.NullUUID{UUID: credentialID, Valid: true}})
	if err != nil {
		return err
	}
	if used {
		return nil // Keep the last verified configuration restartable until cutover.
	}
	c, err := q.GetCredential(ctx, db.GetCredentialParams{EnterpriseID: enterprise, ID: credentialID})
	if err != nil {
		return err
	}
	record, err := q.GetSecret(ctx, db.GetSecretParams{EnterpriseID: enterprise, ID: c.SecretID})
	if err != nil || c.Protocol != "monitoring" || record.Purpose != "monitoring" || record.OwnerType != "monitoring_collection" || !record.OwnerID.Valid || record.OwnerID.UUID != collection {
		return ErrCredentialUnavailable
	}
	if err = q.RetireMonitoringCredential(ctx, db.RetireMonitoringCredentialParams{EnterpriseID: enterprise, ID: credentialID}); err != nil {
		return err
	}
	if err = q.RevokeCredentialLeasesBySecret(ctx, db.RevokeCredentialLeasesBySecretParams{EnterpriseID: enterprise, SecretID: c.SecretID}); err != nil {
		return err
	}
	rows, err := q.DisableSecret(ctx, db.DisableSecretParams{ID: record.ID, EnterpriseID: enterprise, Version: record.Version})
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("monitoring secret still referenced: %w", ErrSecretReferenced)
	}
	return nil
}

func RetireUnusedMonitoringCredentials(ctx context.Context, q *db.Queries, enterprise, collection uuid.UUID) error {
	rows, err := q.ListUnusedMonitoringCredentials(ctx, db.ListUnusedMonitoringCredentialsParams{EnterpriseID: enterprise, OwnerID: uuid.NullUUID{UUID: collection, Valid: true}})
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := (Service{}).RetireMonitoringWithQueries(ctx, q, enterprise, collection, row.ID); err != nil {
			return err
		}
	}
	return nil
}
