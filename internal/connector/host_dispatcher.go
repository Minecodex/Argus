package connector

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/kakj-go/Argus/internal/hostonboarding"
	"github.com/kakj-go/Argus/internal/installation"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// RunHostOnboardingDispatcher owns the durable bastion-assisted SSH install
// queue. The Bastion Connector receives the SSH credential through its normal
// lease and the enrollment token through the separately encrypted operation
// secret grant.
func (service BastionService) RunHostOnboardingDispatcher(ctx context.Context, owner string) error {
	if service.Store == nil || service.Enrollment.Credentials.Store == nil || owner == "" {
		return errors.New("Host onboarding dispatcher is unavailable")
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			_, _ = service.Store.Queries.RecoverHostOnboardingOperations(ctx)
			_, _ = service.Store.Queries.ExpireHostOnboardingOperations(ctx)
			operations, err := service.Store.Queries.ClaimBastionHostOnboardingOperations(ctx, db.ClaimBastionHostOnboardingOperationsParams{Limit: 8, LeaseOwner: owner})
			if err != nil {
				return err
			}
			for _, operation := range operations {
				if err = hostonboarding.RecordClaim(ctx, service.Store.Queries, operation); err != nil {
					service.retryHostOnboarding(ctx, operation, "HOST_ONBOARDING_PROGRESS_FAILED")
					continue
				}
				if err = service.dispatchHostOnboarding(ctx, operation); err != nil {
					service.retryHostOnboarding(ctx, operation, "HOST_ONBOARDING_DISPATCH_FAILED")
				}
			}
			if err = service.reconcileBastionHostOnboarding(ctx, owner); err != nil {
				return err
			}
			if err = service.dispatchBastionHostRemovals(ctx, owner); err != nil {
				return err
			}
		}
	}
}

func (service BastionService) dispatchHostOnboarding(ctx context.Context, operation db.HostOnboardingOperation) error {
	var plan installation.HostConnectorInstallPlan
	if json.Unmarshal(operation.Plan, &plan) != nil || plan.SSHPath != "bastion_connector" || !operation.BastionScopeID.Valid {
		return resource.ErrActionInvalidated
	}
	canonical, _ := json.Marshal(plan)
	hash := sha256.Sum256(canonical)
	if !subtleEqual(hash[:], operation.PlanHash) {
		return resource.ErrActionInvalidated
	}
	scope, err := service.Store.Queries.GetBastionScope(ctx, db.GetBastionScopeParams{ID: operation.BastionScopeID.UUID, EnterpriseID: operation.EnterpriseID})
	if err != nil || scope.Status != "active" || !scope.ActiveConnectorID.Valid {
		return ErrControlTunnelUnavailable
	}
	connector, err := service.Store.Queries.GetConnector(ctx, db.GetConnectorParams{ID: scope.ActiveConnectorID.UUID, EnterpriseID: operation.EnterpriseID})
	if err != nil || connector.Status != "online" || connector.ConnectionEpoch < 1 {
		return ErrControlTunnelUnavailable
	}
	return service.Store.InTx(ctx, func(q *db.Queries) error {
		credential, err := q.GetCredential(ctx, db.GetCredentialParams{ID: plan.CredentialID, EnterpriseID: operation.EnterpriseID})
		if err != nil || credential.Status != "active" || credential.Version != plan.CredentialVersion {
			return resource.ErrActionInvalidated
		}
		lease, err := service.Enrollment.Credentials.PrepareLeaseWithQueries(ctx, q, operation.PendingActionID.String(), operation.EnterpriseID, secret.LeaseRequest{
			CredentialID: plan.CredentialID, OperationRef: operation.ID.String(), TargetResourceType: "host", TargetResourceID: operation.HostID,
			RecipientType: "connector", RecipientID: connector.ID.String(), Protocol: "ssh", TTL: 5 * time.Minute})
		if err != nil {
			return err
		}
		commandID, err := randomID("cmd_")
		if err != nil {
			return err
		}
		_, err = q.CreateConnectorCommand(ctx, db.CreateConnectorCommandParams{ID: newID(), CommandID: commandID, EnterpriseID: operation.EnterpriseID,
			ConnectorID: connector.ID, ConnectionEpoch: connector.ConnectionEpoch, OperationRef: operation.ID.String(), CredentialLeaseID: uuid.NullUUID{UUID: lease.ID, Valid: true},
			CommandType: "host_connector_install", PayloadSchemaVersion: "argus.host_connector_install/v1", Payload: operation.Plan, PayloadHash: operation.PlanHash,
			IdempotencyKey: fmt.Sprintf("%s:%d", operation.ID, operation.Attempts), ExpiresAt: operation.ExpiresAt})
		if err != nil {
			return err
		}
		service.Enrollment.NotifyConnectorCommand(ctx, connector.ID, connector.ConnectionEpoch)
		return nil
	})
}

func (service BastionService) reconcileBastionHostOnboarding(ctx context.Context, owner string) error {
	operations, err := service.Store.Queries.ListRunningBastionHostOnboardingOperations(ctx)
	if err != nil {
		return err
	}
	for _, operation := range operations {
		if operation.LeaseOwner != owner {
			continue
		}
		if rows, err := service.Store.Queries.RenewHostOnboardingOperationLease(ctx, db.RenewHostOnboardingOperationLeaseParams{ID: operation.ID, EnterpriseID: operation.EnterpriseID, LeaseOwner: owner}); err != nil || rows != 1 {
			continue
		}
		command, commandErr := service.Store.Queries.GetLatestConnectorCommandForOperation(ctx, db.GetLatestConnectorCommandForOperationParams{EnterpriseID: operation.EnterpriseID, OperationRef: operation.ID.String()})
		if errors.Is(commandErr, pgx.ErrNoRows) {
			continue
		}
		if commandErr != nil {
			return commandErr
		}
		switch command.Status {
		case "succeeded":
			connector, getErr := service.Store.Queries.GetConnector(ctx, db.GetConnectorParams{ID: operation.ConnectorID, EnterpriseID: operation.EnterpriseID})
			if getErr == nil && connector.Status == "online" {
				_, _ = service.Store.Queries.ConsumeHostOnboardingOperationSecret(ctx, db.ConsumeHostOnboardingOperationSecretParams{OperationID: operation.ID, EnterpriseID: operation.EnterpriseID})
				_ = hostonboarding.Complete(ctx, service.Store.Queries, operation.ID, operation.EnterpriseID)
			}
		case "failed", "timed_out", "expired", "result_unknown":
			code := "HOST_ONBOARDING_CONNECTOR_FAILED"
			if strings.HasPrefix(command.ErrorCode.String, "HOST_ONBOARDING_") {
				code = command.ErrorCode.String
			}
			service.retryHostOnboarding(ctx, operation, code)
		}
	}
	return nil
}

func (service BastionService) retryHostOnboarding(ctx context.Context, operation db.HostOnboardingOperation, code string) {
	if operation.Attempts < 3 && time.Now().Before(operation.ExpiresAt.Time) {
		_ = hostonboarding.RecordOutcome(ctx, service.Store.Queries, operation.ID, operation.EnterpriseID, "retrying", code)
		if _, err := service.Store.Queries.RetryHostOnboardingOperation(ctx, db.RetryHostOnboardingOperationParams{ID: operation.ID, EnterpriseID: operation.EnterpriseID, ErrorCode: pgtype.Text{String: code, Valid: true}}); err == nil {
			return
		}
	}
	_ = hostonboarding.RecordOutcome(ctx, service.Store.Queries, operation.ID, operation.EnterpriseID, "failed", code)
	_, _ = service.Store.Queries.FailHostOnboardingOperation(ctx, db.FailHostOnboardingOperationParams{ID: operation.ID, EnterpriseID: operation.EnterpriseID, ErrorCode: pgtype.Text{String: code, Valid: true}})
}
