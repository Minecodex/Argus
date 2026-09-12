package connector

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/kakj-go/Argus/internal/installation"
	"github.com/kakj-go/Argus/internal/operationsecret"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type hostConnectorOnboardingPlan struct {
	HostID              uuid.UUID             `json:"host_id"`
	TargetPlatform      installation.Platform `json:"target_platform"`
	DistributionVersion string                `json:"distribution_version,omitempty"`
	InstallMethod       string                `json:"install_method"`
	SSHPath             string                `json:"ssh_path"`
	ControlPath         string                `json:"control_path"`
	BastionScopeID      uuid.NullUUID         `json:"bastion_scope_id,omitempty"`
	Address             string                `json:"address,omitempty"`
	Port                int32                 `json:"port,omitempty"`
	Username            string                `json:"username,omitempty"`
	CredentialID        uuid.NullUUID         `json:"credential_id,omitempty"`
	CredentialVersion   int64                 `json:"credential_version,omitempty"`
	ConnectionTestID    uuid.NullUUID         `json:"connection_test_id,omitempty"`
	PinnedHostKey       string                `json:"pinned_host_key,omitempty"`
	ReleaseVersionID    uuid.UUID             `json:"release_version_id"`
	ReleaseManifestHash []byte                `json:"release_manifest_hash"`
	EnrollDialAddress   string                `json:"enroll_dial_address,omitempty"`
	GatewayDialAddress  string                `json:"gateway_dial_address,omitempty"`
	RelayPortGeneration int64                 `json:"relay_port_generation,omitempty"`
}

func (service BastionService) PrepareHostConnectorOnboarding(ctx context.Context, q *db.Queries, enterpriseID, hostID uuid.UUID, input resource.HostInput, targetPlatform string) (json.RawMessage, error) {
	platform := installation.Platform(targetPlatform)
	if !platform.Valid() || input.Role != "managed_host" || input.InstallMethod != "manual" && input.InstallMethod != "ssh" {
		return nil, resource.ErrInvalidConnectionMode
	}
	release, err := q.GetActiveConnectorReleaseVersion(ctx)
	if err != nil {
		return nil, ErrConnectorArtifactUnavailable
	}
	manifest, artifact, err := connectorReleaseForPlatform(release.Manifest, targetPlatform)
	if err != nil {
		return nil, err
	}
	installer, err := connectorInstallerFromManifest(manifest, platform.OS())
	if err != nil {
		return nil, err
	}
	if service.Enrollment.Artifacts != nil {
		if err = service.Enrollment.Artifacts.Check(ctx, manifest.ManifestURI, installer.URI, artifact.URI); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrConnectorArtifactUnavailable, err)
		}
	}
	plan := hostConnectorOnboardingPlan{HostID: hostID, TargetPlatform: platform, InstallMethod: input.InstallMethod, SSHPath: input.SSHPath,
		ControlPath: input.ControlPath, BastionScopeID: input.BastionScopeID, Address: input.Address, Port: input.Port, Username: input.Username,
		CredentialID: input.CredentialID, ConnectionTestID: input.ConnectionTestID, ReleaseVersionID: release.ID, ReleaseManifestHash: bytes.Clone(release.ManifestHash)}
	if input.InstallMethod == "ssh" {
		test, err := q.GetConnectionTest(ctx, db.GetConnectionTestParams{ID: input.ConnectionTestID.UUID, EnterpriseID: enterpriseID})
		if err != nil || test.Status != "succeeded" || !test.CredentialID.Valid || test.CredentialID != input.CredentialID || !test.CredentialVersion.Valid {
			return nil, resource.ErrConnectionTestNeeded
		}
		var result resource.ConnectionTestResult
		if json.Unmarshal(test.Result, &result) != nil || result.Platform != input.Platform || input.Platform+"_"+result.Architecture != targetPlatform || result.HostKeyFingerprint == "" {
			return nil, resource.ErrConnectionTestNeeded
		}
		if err := service.validateCallbackConnectionTest(ctx, q, enterpriseID, test, input.ControlPath, input.SSHPath, input.BastionScopeID); err != nil {
			return nil, err
		}
		plan.PinnedHostKey = result.HostKeyFingerprint
		plan.CredentialVersion = test.CredentialVersion.Int64
		plan.DistributionVersion = result.DistributionVersion
	}
	if input.ControlPath == "bastion_relay" {
		scope, err := q.GetBastionScope(ctx, db.GetBastionScopeParams{ID: input.BastionScopeID.UUID, EnterpriseID: enterpriseID})
		if err != nil || scope.Status != "active" || scope.RelayStatus != "ready" || scope.RelayAddress == "" || scope.RelayPortGeneration < 1 {
			return nil, ErrControlTunnelUnavailable
		}
		plan.EnrollDialAddress = net.JoinHostPort(scope.RelayAddress, fmt.Sprint(scope.RelayHttpsPort))
		plan.GatewayDialAddress = net.JoinHostPort(scope.RelayAddress, fmt.Sprint(scope.RelayGatewayPort))
		plan.RelayPortGeneration = scope.RelayPortGeneration
	} else if input.ControlPath == "executor_tunnel" {
		plan.EnrollDialAddress, plan.GatewayDialAddress = "127.0.0.1:8443", "127.0.0.1:9443"
	}
	return json.Marshal(plan)
}

func (service BastionService) RevalidateHostConnectorOnboarding(ctx context.Context, q *db.Queries, enterpriseID uuid.UUID, raw json.RawMessage) error {
	var plan hostConnectorOnboardingPlan
	if json.Unmarshal(raw, &plan) != nil || plan.HostID == uuid.Nil || !plan.TargetPlatform.Valid() || len(plan.ReleaseManifestHash) != sha256.Size {
		return resource.ErrActionInvalidated
	}
	release, err := q.GetConnectorReleaseVersion(ctx, plan.ReleaseVersionID)
	if err != nil || !bytes.Equal(release.ManifestHash, plan.ReleaseManifestHash) {
		return resource.ErrActionInvalidated
	}
	manifest, artifact, err := connectorReleaseForPlatform(release.Manifest, string(plan.TargetPlatform))
	if err != nil {
		return err
	}
	installer, err := connectorInstallerFromManifest(manifest, plan.TargetPlatform.OS())
	if err != nil {
		return err
	}
	if service.Enrollment.Artifacts != nil {
		if err = service.Enrollment.Artifacts.Check(ctx, manifest.ManifestURI, installer.URI, artifact.URI); err != nil {
			return ErrConnectorArtifactUnavailable
		}
	}
	if plan.InstallMethod == "ssh" {
		if err = service.revalidateHostSSHPlan(ctx, q, enterpriseID, plan); err != nil {
			return err
		}
	}
	if plan.ControlPath == "bastion_relay" {
		scope, err := q.GetBastionScope(ctx, db.GetBastionScopeParams{ID: plan.BastionScopeID.UUID, EnterpriseID: enterpriseID})
		if err != nil || scope.Status != "active" || scope.RelayStatus != "ready" || scope.RelayPortGeneration < 1 || plan.RelayPortGeneration != scope.RelayPortGeneration ||
			plan.EnrollDialAddress != net.JoinHostPort(scope.RelayAddress, fmt.Sprint(scope.RelayHttpsPort)) || plan.GatewayDialAddress != net.JoinHostPort(scope.RelayAddress, fmt.Sprint(scope.RelayGatewayPort)) {
			return resource.ErrActionInvalidated
		}
	}
	return nil
}

type hostConnectionPlanSnapshot struct {
	TargetType        string        `json:"target_type"`
	Address           string        `json:"address"`
	Port              int32         `json:"port"`
	Platform          string        `json:"platform"`
	Username          string        `json:"username"`
	SSHPath           string        `json:"ssh_path"`
	BastionScopeID    uuid.NullUUID `json:"bastion_scope_id"`
	ConnectorID       uuid.NullUUID `json:"connector_id"`
	CredentialID      uuid.NullUUID `json:"credential_id"`
	CredentialVersion int64         `json:"credential_version"`
}

func (service BastionService) revalidateHostSSHPlan(ctx context.Context, q *db.Queries, enterpriseID uuid.UUID, plan hostConnectorOnboardingPlan) error {
	if !plan.ConnectionTestID.Valid || !plan.CredentialID.Valid || plan.CredentialVersion < 1 || plan.PinnedHostKey == "" {
		return resource.ErrActionInvalidated
	}
	test, err := q.GetConnectionTest(ctx, db.GetConnectionTestParams{ID: plan.ConnectionTestID.UUID, EnterpriseID: enterpriseID})
	if err != nil || test.Status != "succeeded" || !time.Now().UTC().Before(test.ExpiresAt.Time) || !test.CredentialID.Valid ||
		test.CredentialID != plan.CredentialID || !test.CredentialVersion.Valid || test.CredentialVersion.Int64 != plan.CredentialVersion {
		return resource.ErrConnectionTestNeeded
	}
	var frozen hostConnectionPlanSnapshot
	if json.Unmarshal(test.RequestPlan, &frozen) != nil || frozen.TargetType != "host" || frozen.Address != plan.Address || frozen.Port != plan.Port ||
		frozen.Platform != plan.TargetPlatform.OS() || frozen.Username != plan.Username || frozen.SSHPath != plan.SSHPath ||
		frozen.BastionScopeID != plan.BastionScopeID || frozen.CredentialID != plan.CredentialID || frozen.CredentialVersion != plan.CredentialVersion {
		return resource.ErrActionInvalidated
	}
	credential, err := q.GetCredential(ctx, db.GetCredentialParams{ID: plan.CredentialID.UUID, EnterpriseID: enterpriseID})
	if err != nil || credential.Status != "active" || credential.Protocol != "ssh" || credential.Version != plan.CredentialVersion {
		return resource.ErrConnectionTestNeeded
	}
	var result resource.ConnectionTestResult
	if json.Unmarshal(test.Result, &result) != nil || result.Platform+"_"+result.Architecture != string(plan.TargetPlatform) || result.HostKeyFingerprint != plan.PinnedHostKey ||
		result.DistributionVersion != plan.DistributionVersion || !result.Privileged {
		return resource.ErrActionInvalidated
	}
	if err := service.validateCallbackConnectionTest(ctx, q, enterpriseID, test, plan.ControlPath, plan.SSHPath, plan.BastionScopeID); err != nil {
		return err
	}
	if plan.SSHPath == "direct_executor" {
		if test.Path != "direct" || plan.BastionScopeID.Valid || frozen.ConnectorID.Valid {
			return resource.ErrActionInvalidated
		}
		return nil
	}
	if plan.SSHPath != "bastion_connector" || test.Path != "connector" || !plan.BastionScopeID.Valid || !test.ConnectorID.Valid || test.ConnectorID != frozen.ConnectorID {
		return resource.ErrActionInvalidated
	}
	scope, err := q.GetBastionScope(ctx, db.GetBastionScopeParams{ID: plan.BastionScopeID.UUID, EnterpriseID: enterpriseID})
	if err != nil || scope.Status != "active" || !scope.ActiveConnectorID.Valid || scope.ActiveConnectorID != test.ConnectorID {
		return resource.ErrActionInvalidated
	}
	connector, err := q.GetConnector(ctx, db.GetConnectorParams{ID: test.ConnectorID.UUID, EnterpriseID: enterpriseID})
	if err != nil || connector.Status != "online" || !test.ConnectionEpoch.Valid || connector.ConnectionEpoch < test.ConnectionEpoch.Int64 {
		return resource.ErrActionInvalidated
	}
	return nil
}

func (service BastionService) CommitHostConnectorOnboarding(ctx context.Context, q *db.Queries, action db.PendingAction, host db.Host, raw json.RawMessage) (resource.ActionCommitResult, error) {
	var plan hostConnectorOnboardingPlan
	if json.Unmarshal(raw, &plan) != nil || plan.HostID != host.ID || host.Role != "managed_host" || host.ControlPath != plan.ControlPath || host.Platform != plan.TargetPlatform.OS() {
		return resource.ActionCommitResult{}, resource.ErrActionInvalidated
	}
	if err := service.RevalidateHostConnectorOnboarding(ctx, q, action.EnterpriseID, raw); err != nil {
		return resource.ActionCommitResult{}, err
	}
	policy, _ := json.Marshal(enrollmentPolicy{TargetPlatform: string(plan.TargetPlatform), ControlPath: plan.ControlPath,
		EnrollDialAddress: plan.EnrollDialAddress, GatewayDialAddress: plan.GatewayDialAddress})
	enrollment, err := service.Enrollment.CreateEnrollment(ctx, q, action.CreatorSubjectID.String(), action.EnterpriseID, CreateEnrollmentInput{
		Role: "host", Purpose: "host_registration", HostID: uuid.NullUUID{UUID: host.ID, Valid: true}, BastionScopeID: plan.BastionScopeID,
		ManualInstall: plan.InstallMethod == "manual", ReleaseVersionID: uuid.NullUUID{UUID: plan.ReleaseVersionID, Valid: true},
		TargetPlatform: plan.TargetPlatform, Policy: policy})
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	if plan.InstallMethod == "manual" {
		encoded, encodeErr := json.Marshal(plan)
		if encodeErr != nil {
			return resource.ActionCommitResult{}, encodeErr
		}
		hash := sha256.Sum256(encoded)
		operation, createErr := q.CreateHostOnboardingOperation(ctx, db.CreateHostOnboardingOperationParams{ID: newID(), EnterpriseID: action.EnterpriseID,
			HostID: host.ID, ConnectorID: enrollment.ConnectorID, PendingActionID: action.ID, ReleaseVersionID: plan.ReleaseVersionID,
			InstallMethod: "manual", SshPath: "none", TargetPlatform: string(plan.TargetPlatform), ControlPath: plan.ControlPath,
			BastionScopeID: plan.BastionScopeID, Plan: encoded, PlanHash: hash[:], ExpiresAt: enrollment.Record.ExpiresAt})
		if createErr != nil {
			return resource.ActionCommitResult{}, createErr
		}
		_, _ = q.CreateHostOnboardingOperationEvent(ctx, db.CreateHostOnboardingOperationEventParams{ID: newID(), OperationID: operation.ID,
			EnterpriseID: action.EnterpriseID, Sequence: 1, Stage: "queued", Status: "started"})
		return resource.ActionCommitResult{Summary: "Host Connector install command created", OneTimeCommand: &resource.OneTimeCommandResult{
			InstructionSets: enrollment.InstructionSets, ExpiresAt: enrollment.Record.ExpiresAt.Time}, OneTimeResultKind: "connector_install_command",
			HostOnboardingOperationID: uuid.NullUUID{UUID: operation.ID, Valid: true}}, nil
	}
	credential, err := q.GetCredential(ctx, db.GetCredentialParams{ID: plan.CredentialID.UUID, EnterpriseID: action.EnterpriseID})
	if err != nil || credential.Status != "active" {
		return resource.ActionCommitResult{}, resource.ErrConnectionTestNeeded
	}
	release, err := q.GetConnectorReleaseVersion(ctx, plan.ReleaseVersionID)
	if err != nil {
		return resource.ActionCommitResult{}, ErrConnectorArtifactUnavailable
	}
	manifest, artifact, err := connectorReleaseForPlatform(release.Manifest, string(plan.TargetPlatform))
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	bundle, err := service.Enrollment.installationTrustBundle(ctx)
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	installPlan := installation.HostConnectorInstallPlan{HostID: host.ID, ConnectorID: enrollment.ConnectorID, TargetPlatform: plan.TargetPlatform, DistributionVersion: plan.DistributionVersion,
		SSHPath: plan.SSHPath, ControlPath: plan.ControlPath, BastionScopeID: plan.BastionScopeID, Address: plan.Address, Port: plan.Port,
		Username: plan.Username, CredentialID: credential.ID, CredentialVersion: plan.CredentialVersion, PinnedHostKey: plan.PinnedHostKey,
		ReleaseVersionID: release.ID, ManifestURI: manifest.ManifestURI, Artifact: installation.Artifact{Platform: artifact.Platform, URI: artifact.URI,
			SHA256: artifact.SHA256, Signature: artifact.Signature, SigningKeyID: artifact.SigningKeyID, ByteSize: artifact.ByteSize}, SigningPublicKey: manifest.SigningPublicKey,
		EnrollmentEndpoint: service.Enrollment.EnrollmentURL, GatewayEndpoint: service.Enrollment.GatewayEndpoint,
		EnrollDialAddress: plan.EnrollDialAddress, GatewayDialAddress: plan.GatewayDialAddress,
		TrustBundlePEM: bundle.Material.PEM, TrustBundleEpoch: bundle.Epoch, TrustBundleSHA256: bundle.Material.SHA256}
	encoded, err := json.Marshal(installPlan)
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	hash := sha256.Sum256(encoded)
	operationID := newID()
	expiresAt := minActionTime(action.ExpiresAt.Time, time.Now().UTC().Add(10*time.Minute))
	nonce, ciphertext, err := operationsecret.Encrypt(service.OperationSecretKey, action.EnterpriseID, operationID, operationsecret.Material{EnrollmentToken: enrollment.Token})
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	operation, err := q.CreateHostOnboardingOperation(ctx, db.CreateHostOnboardingOperationParams{ID: operationID, EnterpriseID: action.EnterpriseID,
		HostID: host.ID, ConnectorID: enrollment.ConnectorID, PendingActionID: action.ID, ReleaseVersionID: release.ID,
		ConnectionTestID: plan.ConnectionTestID, InstallMethod: "ssh", SshPath: plan.SSHPath, TargetPlatform: string(plan.TargetPlatform),
		ControlPath: plan.ControlPath, BastionScopeID: plan.BastionScopeID, Plan: encoded, PlanHash: hash[:],
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true}})
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	if _, err = q.CreateHostOnboardingOperationSecret(ctx, db.CreateHostOnboardingOperationSecretParams{OperationID: operation.ID, EnterpriseID: action.EnterpriseID,
		KeyVersion: operationsecret.KeyVersion, Nonce: nonce, Ciphertext: ciphertext, ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true}}); err != nil {
		return resource.ActionCommitResult{}, err
	}
	_, err = q.CreateHostOnboardingOperationEvent(ctx, db.CreateHostOnboardingOperationEventParams{ID: newID(), OperationID: operation.ID,
		EnterpriseID: action.EnterpriseID, Sequence: 1, Stage: "queued", Status: "started"})
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	if plan.ControlPath == "executor_tunnel" {
		if plan.SSHPath != "direct_executor" {
			return resource.ActionCommitResult{}, resource.ErrActionInvalidated
		}
		if _, err = q.CreateConnectorControlTunnel(ctx, db.CreateConnectorControlTunnelParams{ID: newID(), EnterpriseID: action.EnterpriseID,
			ConnectorID: enrollment.ConnectorID, HostID: host.ID, CredentialID: credential.ID, CredentialVersion: credential.Version,
			TargetAddress: plan.Address, TargetPort: plan.Port, TargetUsername: plan.Username, PinnedHostKey: plan.PinnedHostKey,
			EnrollForwardTarget: service.ConnectorEnrollForwardTarget, GatewayForwardTarget: service.ConnectorGatewayForwardTarget}); err != nil {
			return resource.ActionCommitResult{}, err
		}
	}
	return resource.ActionCommitResult{Summary: "Host Connector installation queued", HostOnboardingOperationID: uuid.NullUUID{UUID: operation.ID, Valid: true}}, nil
}

var _ resource.HostConnectorOnboardingExtension = BastionService{}

func validateHostInstallPlanHash(operation db.HostOnboardingOperation, plan installation.HostConnectorInstallPlan) error {
	encoded, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(encoded)
	if !bytes.Equal(hash[:], operation.PlanHash) {
		return errors.New("host onboarding plan hash mismatch")
	}
	return nil
}
