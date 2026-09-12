package resource

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Retry only an installation that never enrolled. Registered identities require
// the explicit replacement lifecycle, not reuse of a failed pre-enrollment plan.
func (service Service) PreviewRetryHost(ctx context.Context, subject Subject, enterpriseID, hostID uuid.UUID, input HostInput, key string) (db.PendingAction, error) {
	host, err := service.GetHost(ctx, enterpriseID, hostID, subject.AuthorizedResourceIDs)
	if err != nil || host.Role != "managed_host" || host.Status != "active" || host.ConnectorID.Valid {
		return db.PendingAction{}, ErrActionInvalidated
	}
	previous, err := service.Store.Queries.GetLatestHostOnboardingOperation(ctx, db.GetLatestHostOnboardingOperationParams{HostID: hostID, EnterpriseID: enterpriseID})
	if err != nil || (previous.Status != "failed" && previous.Status != "expired") || input.InstallMethod != "ssh" || input.Platform != host.Platform || input.ControlPath != host.ControlPath || input.BastionScopeID != host.BastionScopeID || input.Address != host.Address.String || input.Port != host.Port {
		return db.PendingAction{}, ErrActionInvalidated
	}
	input.ExpectedVersion = host.ResourceVersion
	plan, err := service.prepareHostInstall(ctx, enterpriseID, hostID, input)
	if err != nil {
		return db.PendingAction{}, err
	}
	plan.Operation = "retry"
	plan.RetryOf = previous.ID
	return service.prepareAction(ctx, subject, enterpriseID, PrepareActionInput{ActionType: "host.onboarding.retry", Title: "Retry host installation", Summary: "Retry SSH installation for " + host.Name, Risk: "write", ResourceType: "host", ResourceID: uuid.NullUUID{UUID: hostID, Valid: true}, ExpectedResourceVersion: pgtype.Int8{Int64: host.ResourceVersion, Valid: true}, AuthorizationVersion: subject.AuthorizationVersion, Preview: map[string]any{"host_id": hostID, "name": host.Name, "retry_of": previous.ID}, Diff: []map[string]string{{"kind": "change", "text": "Retry installation for " + host.Name}}, ImmutablePlan: plan, ResourceScopeSnapshot: NewResourceAuthorizationSnapshot("host", hostID), CommitHandler: "argus.host.onboarding.retry.commit"}, key)
}

func retirePreviousHostControlTunnel(ctx context.Context, q *db.Queries, enterpriseID, hostID, previousOperationID uuid.UUID) error {
	previous, err := q.GetHostOnboardingOperation(ctx, db.GetHostOnboardingOperationParams{
		ID: previousOperationID, EnterpriseID: enterpriseID,
	})
	if err != nil || previous.HostID != hostID || previous.Status != "failed" && previous.Status != "expired" {
		return ErrActionInvalidated
	}
	if previous.ControlPath != "executor_tunnel" {
		return nil
	}
	if _, err = q.RevokeConnectorControlTunnelLeases(ctx, db.RevokeConnectorControlTunnelLeasesParams{
		ConnectorID: previous.ConnectorID, EnterpriseID: enterpriseID,
	}); err != nil {
		return err
	}
	_, err = q.MarkConnectorControlTunnelRemoved(ctx, db.MarkConnectorControlTunnelRemovedParams{
		ConnectorID: previous.ConnectorID, EnterpriseID: enterpriseID, LastDropReason: "host_onboarding_retry",
	})
	return err
}
