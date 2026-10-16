package connector

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"time"
)

type hostReconnectPlan struct {
	HostID            uuid.UUID `json:"host_id"`
	HostVersion       int64     `json:"host_version"`
	ConnectorID       uuid.UUID `json:"connector_id"`
	TunnelID          uuid.UUID `json:"tunnel_id"`
	CredentialID      uuid.UUID `json:"credential_id"`
	CredentialVersion int64     `json:"credential_version"`
	TestID            uuid.UUID `json:"test_id"`
	ActorID           uuid.UUID `json:"actor_id"`
}

// Reconnect only reauthorizes an existing fixed target and account, after a
// fresh SSH key and platform callback check. It does not replace the Host.
func (service BastionService) HostReconnectInput(ctx context.Context, enterpriseID, hostID uuid.UUID, version int64) (resource.HostInput, error) {
	_, tunnel, credential, err := service.hostReconnectTarget(ctx, service.Store.Queries, enterpriseID, hostID, version)
	if err != nil {
		return resource.HostInput{}, err
	}
	host, err := service.Store.Queries.GetHost(ctx, db.GetHostParams{ID: hostID, EnterpriseID: enterpriseID})
	if err != nil {
		return resource.HostInput{}, err
	}
	return resource.HostInput{Address: tunnel.TargetAddress, Port: tunnel.TargetPort, Platform: host.Platform, Username: tunnel.TargetUsername, CredentialID: uuid.NullUUID{UUID: credential.ID, Valid: true}, SSHPath: "direct_executor", OnboardingControlPath: "executor_tunnel"}, nil
}

func (service BastionService) hostReconnectTarget(ctx context.Context, q *db.Queries, enterpriseID, hostID uuid.UUID, version int64) (db.Host, db.ConnectorControlTunnel, db.Credential, error) {
	host, err := q.GetHost(ctx, db.GetHostParams{ID: hostID, EnterpriseID: enterpriseID})
	if err != nil || host.Status != "active" || host.ResourceVersion != version || host.ControlPath != "executor_tunnel" || !host.ConnectorID.Valid {
		return db.Host{}, db.ConnectorControlTunnel{}, db.Credential{}, resource.ErrActionInvalidated
	}
	tunnel, err := q.GetConnectorControlTunnelByConnector(ctx, db.GetConnectorControlTunnelByConnectorParams{ConnectorID: host.ConnectorID.UUID, EnterpriseID: enterpriseID})
	if err != nil || tunnel.Status == "removed" || tunnel.HostID != host.ID || tunnel.TargetAddress != host.Address.String || tunnel.TargetPort != host.Port || tunnel.PinnedHostKey != host.PinnedHostKey {
		return host, tunnel, db.Credential{}, resource.ErrActionInvalidated
	}
	credential, err := q.GetCredential(ctx, db.GetCredentialParams{ID: tunnel.CredentialID, EnterpriseID: enterpriseID})
	if err != nil || credential.Status != "active" || credential.Protocol != "ssh" || credential.Username != tunnel.TargetUsername {
		return host, tunnel, credential, resource.ErrActionInvalidated
	}
	return host, tunnel, credential, nil
}

func validReconnectTest(test db.ConnectionTest, host db.Host, tunnel db.ConnectorControlTunnel, credential db.Credential, actorID uuid.UUID) bool {
	if test.Status != "succeeded" || test.TargetType != "host" || test.Path != "direct" || test.CreatedBy != actorID || !test.ExpiresAt.Valid || !test.ExpiresAt.Time.After(time.Now()) || !test.CredentialID.Valid || test.CredentialID.UUID != credential.ID || !test.CredentialVersion.Valid || test.CredentialVersion.Int64 != credential.Version {
		return false
	}
	digest := sha256.Sum256(test.RequestPlan)
	if !subtleEqual(digest[:], test.RequestHash) {
		return false
	}
	var input struct {
		Address, Platform, Username string
		Port                        int32
		SSHPath                     string `json:"ssh_path"`
		Onboarding                  struct {
			ControlPath string `json:"control_path"`
		} `json:"onboarding"`
	}
	var result resource.ConnectionTestResult
	return json.Unmarshal(test.RequestPlan, &input) == nil && json.Unmarshal(test.Result, &result) == nil && input.Address == tunnel.TargetAddress && input.Port == tunnel.TargetPort && input.Platform == host.Platform && input.Username == tunnel.TargetUsername && input.SSHPath == "direct_executor" && input.Onboarding.ControlPath == "executor_tunnel" && result.HostKeyFingerprint == host.PinnedHostKey && result.CallbackVerified && result.CallbackControlPath == "executor_tunnel"
}

func (service BastionService) PreviewHostReconnect(ctx context.Context, subject resource.Subject, enterpriseID, hostID, testID uuid.UUID, version int64, key string) (db.PendingAction, error) {
	if !(resource.AccessService{}).CanAccess(subject.AuthorizedResourceIDs, hostID) {
		return db.PendingAction{}, resource.ErrResourceDenied
	}
	host, tunnel, credential, err := service.hostReconnectTarget(ctx, service.Store.Queries, enterpriseID, hostID, version)
	if err != nil {
		return db.PendingAction{}, err
	}
	actor, err := uuid.Parse(subject.ActorID)
	if err != nil {
		return db.PendingAction{}, resource.ErrResourceDenied
	}
	test, err := service.Store.Queries.GetConnectionTest(ctx, db.GetConnectionTestParams{ID: testID, EnterpriseID: enterpriseID})
	if err != nil || !validReconnectTest(test, host, tunnel, credential, actor) {
		return db.PendingAction{}, resource.ErrConnectionTestNeeded
	}
	plan := hostReconnectPlan{host.ID, host.ResourceVersion, host.ConnectorID.UUID, tunnel.ID, credential.ID, credential.Version, test.ID, actor}
	return service.Actions.Prepare(ctx, subject.ActorID, enterpriseID, resource.PrepareActionInput{RunID: subject.RunID, ActionType: "host.control_tunnel.reconnect", Title: "Restore host connection", Summary: "Reauthorize existing control and telemetry tunnels for " + host.Name, Risk: "write", ResourceType: "host", ResourceID: uuid.NullUUID{UUID: host.ID, Valid: true}, ExpectedResourceVersion: pgtype.Int8{Int64: host.ResourceVersion, Valid: true}, AuthorizationVersion: subject.AuthorizationVersion, Preview: map[string]any{"host_id": host.ID, "name": host.Name, "address": tunnel.TargetAddress, "credential_id": credential.ID, "credential_version": credential.Version}, Diff: []map[string]string{{"kind": "change", "text": "Reauthorize existing tunnels with the current verified credential"}}, ImmutablePlan: plan, ResourceScopeSnapshot: resource.NewResourceAuthorizationSnapshot("host", host.ID), CommitHandler: "argus.host.control_tunnel.reconnect.commit"}, key)
}

func (service BastionService) revalidateHostReconnect(ctx context.Context, q *db.Queries, enterpriseID uuid.UUID, plan hostReconnectPlan) error {
	host, tunnel, credential, err := service.hostReconnectTarget(ctx, q, enterpriseID, plan.HostID, plan.HostVersion)
	if err != nil || host.ConnectorID.UUID != plan.ConnectorID || tunnel.ID != plan.TunnelID || credential.ID != plan.CredentialID || credential.Version != plan.CredentialVersion {
		return resource.ErrActionInvalidated
	}
	test, err := q.GetConnectionTest(ctx, db.GetConnectionTestParams{ID: plan.TestID, EnterpriseID: enterpriseID})
	if err != nil || !validReconnectTest(test, host, tunnel, credential, plan.ActorID) {
		return resource.ErrActionInvalidated
	}
	return nil
}

func (service BastionService) commitHostReconnect(ctx context.Context, q *db.Queries, action db.PendingAction, plan hostReconnectPlan) (resource.ActionCommitResult, error) {
	if err := service.revalidateHostReconnect(ctx, q, action.EnterpriseID, plan); err != nil {
		return resource.ActionCommitResult{}, err
	}
	host, err := q.UpdateHost(ctx, db.UpdateHostParams{ID: plan.HostID, EnterpriseID: action.EnterpriseID, ResourceVersion: plan.HostVersion})
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	if _, err = q.ReauthorizeHostControlTunnel(ctx, db.ReauthorizeHostControlTunnelParams{ID: plan.TunnelID, EnterpriseID: action.EnterpriseID, CredentialVersion: plan.CredentialVersion}); err != nil {
		return resource.ActionCommitResult{}, err
	}
	if _, err = q.ReauthorizeHostTelemetryTunnels(ctx, db.ReauthorizeHostTelemetryTunnelsParams{HostID: plan.HostID, EnterpriseID: action.EnterpriseID, CredentialID: plan.CredentialID, CredentialVersion: plan.CredentialVersion}); err != nil {
		return resource.ActionCommitResult{}, err
	}
	return resource.ActionCommitResult{ResourceType: "host", ResourceID: host.ID, ResourceVersion: host.ResourceVersion}, nil
}
