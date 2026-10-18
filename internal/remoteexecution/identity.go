package remoteexecution

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// Resolve only persisted enrollment facts. Manual resources without an explicit
// verified account cannot inherit the Connector service user as a convenience.
func (s Service) resolveIdentity(ctx context.Context, q *db.Queries, p toolruntime.Principal, t Target) (db.RemoteExecutionIdentity, error) {
	existing, err := q.GetRemoteIdentity(ctx, db.GetRemoteIdentityParams{EnterpriseID: p.EnterpriseID, ResourceType: t.ResourceType, ResourceID: t.ResourceID})
	if err == nil {
		credentialVersion := existing.CredentialVersion
		resourceVersion := existing.ResourceVersion
		if existing.CredentialID.Valid {
			c, e := q.GetCredential(ctx, db.GetCredentialParams{ID: existing.CredentialID.UUID, EnterpriseID: p.EnterpriseID})
			if e != nil || c.Status != "active" {
				return existing, Error("REMOTE_IDENTITY_CHANGED")
			}
			credentialVersion = c.Version
		}
		if t.ResourceType == "host" {
			h, e := q.GetHost(ctx, db.GetHostParams{ID: t.ResourceID, EnterpriseID: p.EnterpriseID})
			if e != nil || h.Status != "active" || h.ConnectorID != existing.ConnectorID {
				return existing, Error("REMOTE_IDENTITY_CHANGED")
			}
			resourceVersion = h.ResourceVersion
		}
		if t.ResourceType == "kubernetes_cluster" {
			c, e := q.GetKubernetesCluster(ctx, db.GetKubernetesClusterParams{ID: t.ResourceID, EnterpriseID: p.EnterpriseID})
			if e != nil || c.Status != "active" || c.CredentialID != existing.CredentialID {
				return existing, Error("REMOTE_IDENTITY_CHANGED")
			}
			route, e := clusterConnector(ctx, q, c)
			if e != nil || route != existing.ConnectorID {
				return existing, Error("REMOTE_IDENTITY_CHANGED")
			}
			resourceVersion = c.ResourceVersion
		}
		if credentialVersion != existing.CredentialVersion || resourceVersion != existing.ResourceVersion {
			return q.RefreshRemoteIdentity(ctx, db.RefreshRemoteIdentityParams{ID: existing.ID, EnterpriseID: existing.EnterpriseID, CredentialVersion: credentialVersion, ResourceVersion: resourceVersion})
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return existing, err
	}
	input := db.CreateRemoteIdentityParams{ID: uuid.New(), EnterpriseID: p.EnterpriseID, ResourceType: t.ResourceType, ResourceID: t.ResourceID}
	if t.ResourceType == "host" {
		host, e := q.GetHost(ctx, db.GetHostParams{ID: t.ResourceID, EnterpriseID: p.EnterpriseID})
		if e != nil || host.Status != "active" || !host.ConnectorID.Valid {
			return existing, Error("REMOTE_IDENTITY_UNAVAILABLE")
		}
		op, e := q.GetLatestHostOnboardingOperation(ctx, db.GetLatestHostOnboardingOperationParams{HostID: host.ID, EnterpriseID: p.EnterpriseID})
		if e != nil || op.Status != "succeeded" || op.InstallMethod != "ssh" {
			return existing, Error("REMOTE_IDENTITY_UNAVAILABLE")
		}
		var plan struct {
			IdentitySID       string        `json:"identity_sid"`
			Username          string        `json:"username"`
			CredentialID      uuid.NullUUID `json:"credential_id"`
			CredentialVersion int64         `json:"credential_version"`
		}
		if json.Unmarshal(op.Plan, &plan) != nil || plan.Username == "" || !plan.CredentialID.Valid {
			return existing, Error("REMOTE_IDENTITY_UNAVAILABLE")
		}
		input.Username = plan.Username
		input.IdentitySid = plan.IdentitySID
		if host.Platform == "windows" && input.IdentitySid == "" {
			return existing, Error("REMOTE_IDENTITY_UNAVAILABLE")
		}
		input.Platform = host.Platform
		input.CredentialID = plan.CredentialID
		input.CredentialVersion = plan.CredentialVersion
		input.ConnectorID = host.ConnectorID
		input.BindingSource = "ssh_onboarding"
		input.ResourceVersion = host.ResourceVersion
	} else {
		c, e := q.GetKubernetesCluster(ctx, db.GetKubernetesClusterParams{ID: t.ResourceID, EnterpriseID: p.EnterpriseID})
		if e != nil || c.Status != "active" {
			return existing, Error("REMOTE_IDENTITY_UNAVAILABLE")
		}
		input.Platform = "kubernetes"
		input.ConnectorID, e = clusterConnector(ctx, q, c)
		if e != nil {
			return existing, e
		}
		input.CredentialID = c.CredentialID
		input.ResourceVersion = c.ResourceVersion
		if c.CredentialID.Valid {
			cred, e := q.GetCredential(ctx, db.GetCredentialParams{ID: c.CredentialID.UUID, EnterpriseID: p.EnterpriseID})
			if e != nil {
				return existing, e
			}
			input.CredentialVersion = cred.Version
			input.Username = "kubeconfig:" + cred.ID.String()
			input.BindingSource = "kubeconfig"
		} else if c.ConnectionMode == "in_cluster" {
			input.Username = "in-cluster ServiceAccount"
			input.BindingSource = "in_cluster"
		} else {
			return existing, Error("REMOTE_IDENTITY_UNAVAILABLE")
		}
	}
	result, err := q.CreateRemoteIdentity(ctx, input)
	if errors.Is(err, pgx.ErrNoRows) {
		return q.GetRemoteIdentity(ctx, db.GetRemoteIdentityParams{EnterpriseID: p.EnterpriseID, ResourceType: t.ResourceType, ResourceID: t.ResourceID})
	}
	return result, err
}

func clusterConnector(ctx context.Context, q *db.Queries, c db.KubernetesCluster) (uuid.NullUUID, error) {
	route := c.ConnectorID
	if c.ConnectionMode == "direct" && c.CredentialID.Valid {
		return uuid.NullUUID{}, nil
	}
	if c.ConnectionMode == "via_bastion" && c.BastionScopeID.Valid {
		scope, err := q.GetBastionScope(ctx, db.GetBastionScopeParams{ID: c.BastionScopeID.UUID, EnterpriseID: c.EnterpriseID})
		if err != nil || scope.Status != "active" {
			return route, Error("REMOTE_ROUTE_UNAVAILABLE")
		}
		route = scope.ActiveConnectorID
	}
	if !route.Valid {
		return route, Error("REMOTE_ROUTE_UNAVAILABLE")
	}
	return route, nil
}
