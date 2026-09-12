package hostremoval

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/installation"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/encoding/protojson"
)

// ConnectionDefaults contains references only. It never loads secret material
// or reuses the installation's expired ConnectionTest or credential lease.
type ConnectionDefaults struct {
	Username       string     `json:"username"`
	Status         string     `json:"status"`
	CredentialID   *uuid.UUID `json:"credential_id,omitempty"`
	CredentialName *string    `json:"credential_name,omitempty"`
}

type connectionDefaultsQueries interface {
	GetHost(context.Context, db.GetHostParams) (db.Host, error)
	GetBastionScope(context.Context, db.GetBastionScopeParams) (db.GetBastionScopeRow, error)
	GetLatestHostOnboardingOperationByConnector(context.Context, db.GetLatestHostOnboardingOperationByConnectorParams) (db.HostOnboardingOperation, error)
	GetLatestConnectorInstallOperation(context.Context, db.GetLatestConnectorInstallOperationParams) (db.ConnectorInstallOperation, error)
	GetCredential(context.Context, db.GetCredentialParams) (db.Credential, error)
	GetSecret(context.Context, db.GetSecretParams) (db.GetSecretRow, error)
}

func (service Service) ConnectionDefaults(ctx context.Context, subject resource.Subject, enterpriseID uuid.UUID, input PreviewInput) (ConnectionDefaults, error) {
	return readConnectionDefaults(ctx, service.Store.Queries, subject, enterpriseID, input)
}

func readConnectionDefaults(ctx context.Context, q connectionDefaultsQueries, subject resource.Subject, enterpriseID uuid.UUID, input PreviewInput) (ConnectionDefaults, error) {
	result := ConnectionDefaults{Status: "unavailable"}
	if input.ExpectedVersion < 1 || input.TargetID == uuid.Nil {
		return result, ErrInvalidTarget
	}
	var username string
	var credentialID, hostID, connectorID uuid.UUID
	if input.TargetType == TargetManagedHost {
		if !(resource.AccessService{}).CanAccess(subject.AuthorizedResourceIDs, input.TargetID) {
			return result, ErrInvalidTarget
		}
		host, err := q.GetHost(ctx, db.GetHostParams{ID: input.TargetID, EnterpriseID: enterpriseID})
		if errors.Is(err, pgx.ErrNoRows) {
			return result, ErrInvalidTarget
		}
		if err != nil {
			return result, err
		}
		if host.Role != TargetManagedHost || host.Status == "deleted" {
			return result, ErrInvalidTarget
		}
		if host.ResourceVersion != input.ExpectedVersion {
			return result, resource.ErrVersionConflict
		}
		if !host.ConnectorID.Valid {
			return result, ErrNotInstalled
		}
		hostID, connectorID = host.ID, host.ConnectorID.UUID
		origin, err := q.GetLatestHostOnboardingOperationByConnector(ctx, db.GetLatestHostOnboardingOperationByConnectorParams{
			HostID: hostID, ConnectorID: connectorID, EnterpriseID: enterpriseID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return result, nil
		}
		if err != nil {
			return result, err
		}
		if origin.ConnectorID != connectorID {
			return result, ErrIdentityChanged
		}
		if origin.InstallMethod != "ssh" {
			result.Status = "not_applicable"
			return result, nil
		}
		var installed installation.HostConnectorInstallPlan
		if json.Unmarshal(origin.Plan, &installed) != nil || installed.HostID != hostID || installed.ConnectorID != connectorID {
			return result, ErrIdentityChanged
		}
		username, credentialID = installed.Username, installed.CredentialID
	} else if input.TargetType == TargetBastion {
		scope, err := q.GetBastionScope(ctx, db.GetBastionScopeParams{ID: input.TargetID, EnterpriseID: enterpriseID})
		if errors.Is(err, pgx.ErrNoRows) {
			return result, ErrInvalidTarget
		}
		if err != nil {
			return result, err
		}
		if !scope.ConnectorHostID.Valid || scope.Status == "deleted" {
			return result, ErrInvalidTarget
		}
		if !(resource.AccessService{}).CanAccess(subject.AuthorizedResourceIDs, scope.ConnectorHostID.UUID) {
			return result, ErrInvalidTarget
		}
		if scope.ResourceVersion != input.ExpectedVersion {
			return result, resource.ErrVersionConflict
		}
		if !scope.ActiveConnectorID.Valid {
			return result, ErrNotInstalled
		}
		hostID, connectorID = scope.ConnectorHostID.UUID, scope.ActiveConnectorID.UUID
		origin, err := q.GetLatestConnectorInstallOperation(ctx, db.GetLatestConnectorInstallOperationParams{ConnectorID: connectorID, EnterpriseID: enterpriseID})
		if errors.Is(err, pgx.ErrNoRows) {
			if scope.OnboardingMode == "command" {
				result.Status = "not_applicable"
			}
			return result, nil
		}
		if err != nil {
			return result, err
		}
		if origin.HostID != hostID || origin.ConnectorID != connectorID || origin.BastionScopeID != scope.ID {
			return result, ErrIdentityChanged
		}
		var installed connectorv1.ConnectorInstallCommand
		if protojson.Unmarshal(origin.Plan, &installed) != nil {
			return result, ErrIdentityChanged
		}
		installedHostID, hostErr := uuid.Parse(installed.GetHostId())
		installedConnectorID, connectorErr := uuid.Parse(installed.GetConnectorId())
		installedScopeID, scopeErr := uuid.Parse(installed.GetBastionScopeId())
		installedCredentialID, credentialErr := uuid.Parse(installed.GetCredentialId())
		if hostErr != nil || connectorErr != nil || scopeErr != nil || credentialErr != nil || installedHostID != hostID ||
			installedConnectorID != connectorID || installedScopeID != scope.ID {
			return result, ErrIdentityChanged
		}
		username, credentialID = installed.GetTargetUsername(), installedCredentialID
	} else {
		return result, ErrInvalidTarget
	}

	result.Username = username
	if result.Username == "" || credentialID == uuid.Nil {
		return result, nil
	}
	result.Status = "credential_unavailable"
	credential, err := q.GetCredential(ctx, db.GetCredentialParams{ID: credentialID, EnterpriseID: enterpriseID})
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if credential.Status != "active" || credential.Protocol != "ssh" {
		return result, nil
	}
	secret, err := q.GetSecret(ctx, db.GetSecretParams{ID: credential.SecretID, EnterpriseID: enterpriseID})
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if secret.Status != "active" {
		return result, nil
	}
	result.Status, result.CredentialID, result.CredentialName = "available", &credential.ID, &credential.Name
	return result, nil
}
