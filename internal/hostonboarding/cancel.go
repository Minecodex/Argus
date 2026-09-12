package hostonboarding

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

var ErrTargetRegistered = errors.New("onboarding target already registered")

const (
	HostCancellationErrorCode    = "HOST_ONBOARDING_CANCELLED_BY_DELETE"
	BastionCancellationErrorCode = "CONNECTOR_INSTALL_CANCELLED_BY_DELETE"
)

func CancelHost(ctx context.Context, q *db.Queries, enterpriseID, hostID uuid.UUID) error {
	tokens, err := q.LockHostEnrollmentTokensForCancellation(ctx, db.LockHostEnrollmentTokensForCancellationParams{
		EnterpriseID: enterpriseID, PreallocatedHostID: uuid.NullUUID{UUID: hostID, Valid: true},
	})
	if err != nil {
		return err
	}
	if enrollmentWasRegistered(tokens) {
		return ErrTargetRegistered
	}
	for _, operation := range []func() error{
		func() error {
			_, err := q.RevokeActiveHostEnrollmentTokens(ctx, db.RevokeActiveHostEnrollmentTokensParams{EnterpriseID: enterpriseID, PreallocatedHostID: uuid.NullUUID{UUID: hostID, Valid: true}})
			return err
		},
		func() error {
			_, err := q.ExpireHostOnboardingConnectorCommandsByHost(ctx, db.ExpireHostOnboardingConnectorCommandsByHostParams{HostID: hostID, EnterpriseID: enterpriseID})
			return err
		},
		func() error {
			_, err := q.CancelHostOnboardingOperationsByHost(ctx, db.CancelHostOnboardingOperationsByHostParams{HostID: hostID, EnterpriseID: enterpriseID})
			return err
		},
		func() error {
			_, err := q.DeleteHostOnboardingOperationSecretsByHost(ctx, db.DeleteHostOnboardingOperationSecretsByHostParams{HostID: hostID, EnterpriseID: enterpriseID})
			return err
		},
		func() error {
			_, err := q.RevokeHostOnboardingCredentialLeasesByHost(ctx, db.RevokeHostOnboardingCredentialLeasesByHostParams{HostID: hostID, EnterpriseID: enterpriseID})
			return err
		},
		func() error {
			_, err := q.RevokeHostControlTunnelLeasesByHost(ctx, db.RevokeHostControlTunnelLeasesByHostParams{HostID: hostID, EnterpriseID: enterpriseID})
			return err
		},
		func() error {
			_, err := q.MarkHostControlTunnelsRemovedByHost(ctx, db.MarkHostControlTunnelsRemovedByHostParams{HostID: hostID, EnterpriseID: enterpriseID})
			return err
		},
	} {
		if err = operation(); err != nil {
			return err
		}
	}
	return nil
}

func CancelBastion(ctx context.Context, q *db.Queries, enterpriseID, scopeID uuid.UUID) error {
	tokens, err := q.LockBastionEnrollmentTokensForCancellation(ctx, db.LockBastionEnrollmentTokensForCancellationParams{
		EnterpriseID: enterpriseID, BastionScopeID: uuid.NullUUID{UUID: scopeID, Valid: true},
	})
	if err != nil {
		return err
	}
	if enrollmentWasRegistered(tokens) {
		return ErrTargetRegistered
	}
	for _, operation := range []func() error{
		func() error {
			return q.RevokeActiveEnrollmentTokens(ctx, db.RevokeActiveEnrollmentTokensParams{EnterpriseID: enterpriseID, BastionScopeID: uuid.NullUUID{UUID: scopeID, Valid: true}})
		},
		func() error {
			_, err := q.CancelConnectorInstallOperationsByScope(ctx, db.CancelConnectorInstallOperationsByScopeParams{BastionScopeID: scopeID, EnterpriseID: enterpriseID})
			return err
		},
		func() error {
			_, err := q.DeleteConnectorInstallOperationSecretsByScope(ctx, db.DeleteConnectorInstallOperationSecretsByScopeParams{BastionScopeID: scopeID, EnterpriseID: enterpriseID})
			return err
		},
		func() error {
			_, err := q.RevokeConnectorInstallCredentialLeasesByScope(ctx, db.RevokeConnectorInstallCredentialLeasesByScopeParams{BastionScopeID: scopeID, EnterpriseID: enterpriseID})
			return err
		},
		func() error {
			_, err := q.RevokeConnectorControlTunnelLeasesByScope(ctx, db.RevokeConnectorControlTunnelLeasesByScopeParams{BastionScopeID: uuid.NullUUID{UUID: scopeID, Valid: true}, EnterpriseID: enterpriseID})
			return err
		},
		func() error {
			_, err := q.MarkConnectorControlTunnelsRemovedByScope(ctx, db.MarkConnectorControlTunnelsRemovedByScopeParams{BastionScopeID: uuid.NullUUID{UUID: scopeID, Valid: true}, EnterpriseID: enterpriseID, LastDropReason: "bastion_onboarding_cancelled_by_delete"})
			return err
		},
	} {
		if err = operation(); err != nil {
			return err
		}
	}
	return nil
}

func enrollmentWasRegistered(tokens []db.ConnectorEnrollmentToken) bool {
	for _, token := range tokens {
		if token.Status == "consumed" || token.RegisteredConnectorID.Valid {
			return true
		}
	}
	return false
}
