package connector

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kakj-go/Argus/internal/hostremoval"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func (service BastionService) dispatchBastionHostRemovals(ctx context.Context, owner string) error {
	operations, err := service.Store.Queries.ClaimBastionHostRemovalOperations(ctx, db.ClaimBastionHostRemovalOperationsParams{Limit: 8, LeaseOwner: owner})
	if err != nil {
		return err
	}
	removals := hostremoval.Service{Store: service.Store}
	for _, operation := range operations {
		if err = removals.BeginRemote(ctx, operation); err != nil {
			_ = removals.Fail(ctx, operation, "TARGET_IDENTITY_CHANGED", false)
			continue
		}
		if err = service.dispatchBastionHostRemoval(ctx, operation); err != nil {
			_ = removals.Fail(ctx, operation, "HOST_REMOVAL_DISPATCH_FAILED", false)
		}
	}
	return service.reconcileBastionHostRemovals(ctx, owner)
}

func (service BastionService) dispatchBastionHostRemoval(ctx context.Context, operation db.HostRemovalOperation) error {
	plan, err := hostremoval.DecodeOperationPlan(operation)
	if err != nil || !plan.BastionScopeID.Valid || !plan.CredentialID.Valid {
		return hostremoval.ErrIdentityChanged
	}
	scope, err := service.Store.Queries.GetBastionScope(ctx, db.GetBastionScopeParams{ID: plan.BastionScopeID.UUID, EnterpriseID: operation.EnterpriseID})
	if err != nil || !scope.ActiveConnectorID.Valid || scope.Status != "active" {
		return ErrBastionState
	}
	executor, err := service.Store.Queries.GetConnector(ctx, db.GetConnectorParams{ID: scope.ActiveConnectorID.UUID, EnterpriseID: operation.EnterpriseID})
	if err != nil || executor.Status != "online" || executor.Role != "bastion" {
		return ErrBastionState
	}
	return service.Store.InTx(ctx, func(q *db.Queries) error {
		credential, err := q.GetCredential(ctx, db.GetCredentialParams{ID: plan.CredentialID.UUID, EnterpriseID: operation.EnterpriseID})
		if err != nil || credential.Status != "active" || credential.Version != plan.CredentialVersion {
			return hostremoval.ErrConnectionTestNeeded
		}
		lease, err := service.Enrollment.Credentials.PrepareLeaseWithQueries(ctx, q, operation.PendingActionID.String(), operation.EnterpriseID, secret.LeaseRequest{
			CredentialID: plan.CredentialID.UUID, OperationRef: operation.ID.String(), TargetResourceType: "host", TargetResourceID: operation.HostID,
			RecipientType: "connector", RecipientID: executor.ID.String(), Protocol: "ssh", TTL: 5 * time.Minute})
		if err != nil {
			return err
		}
		commandID, err := randomID("cmd_")
		if err != nil {
			return err
		}
		_, err = q.CreateConnectorCommand(ctx, db.CreateConnectorCommandParams{ID: newID(), CommandID: commandID, EnterpriseID: operation.EnterpriseID,
			ConnectorID: executor.ID, ConnectionEpoch: executor.ConnectionEpoch, OperationRef: operation.ID.String(), CredentialLeaseID: uuid.NullUUID{UUID: lease.ID, Valid: true},
			CommandType: "host_connector_removal", PayloadSchemaVersion: "argus.host_connector_removal/v1", Payload: operation.Plan,
			PayloadHash: operation.PlanHash, IdempotencyKey: fmt.Sprintf("%s:%d", operation.ID, operation.Attempts), ExpiresAt: operation.ExpiresAt})
		if err == nil {
			service.Enrollment.NotifyConnectorCommand(ctx, executor.ID, executor.ConnectionEpoch)
		}
		return err
	})
}

func (service BastionService) reconcileBastionHostRemovals(ctx context.Context, owner string) error {
	operations, err := service.Store.Queries.ListRunningBastionHostRemovalOperations(ctx)
	if err != nil {
		return err
	}
	removals := hostremoval.Service{Store: service.Store}
	for _, operation := range operations {
		if operation.LeaseOwner != owner {
			continue
		}
		if rows, err := service.Store.Queries.RenewHostRemovalOperationLease(ctx, db.RenewHostRemovalOperationLeaseParams{ID: operation.ID, EnterpriseID: operation.EnterpriseID, LeaseOwner: owner}); err != nil || rows != 1 {
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
		case "failed":
			_ = removals.Fail(ctx, operation, nonEmptyRemovalCode(command.ErrorCode.String, "HOST_REMOVAL_LOCAL_CLEANUP_UNKNOWN"), true)
		case "timed_out", "delivery_unknown", "result_unknown", "expired":
			_ = removals.Fail(ctx, operation, "HOST_REMOVAL_LOCAL_CLEANUP_UNKNOWN", true)
		}
	}
	return nil
}

func nonEmptyRemovalCode(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
