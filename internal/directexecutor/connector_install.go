package directexecutor

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/kakj-go/Argus/internal/collectormanager"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/operationsecret"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/trustbundle"
)

const connectorInstallTimeout = 10 * time.Minute

var (
	errControlTunnelUnavailable = errors.New("connector control tunnel is unavailable")
	errTunnelQuotaExceeded      = errors.New("tunnel quota exceeded")
	errCredentialVersionStale   = errors.New("credential version is stale")
)

func (executor *Executor) runConnectorInstallLoop(ctx context.Context) {
	ticker := time.NewTicker(700 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = executor.Store.Queries.RecoverConnectorInstallOperations(ctx)
			_, _ = executor.Store.Queries.ExpireConnectorInstallOperations(ctx)
			reserved := executor.reserveAvailable()
			if reserved == 0 {
				continue
			}
			operations, err := executor.Store.Queries.ClaimConnectorInstallOperations(ctx, db.ClaimConnectorInstallOperationsParams{
				Limit: int32(reserved), LeaseOwner: executor.InstanceID,
			})
			if err != nil {
				executor.release(reserved)
				continue
			}
			executor.release(reserved - len(operations))
			for _, operation := range operations {
				go func(value db.ConnectorInstallOperation) {
					defer executor.release(1)
					executor.executeConnectorInstall(ctx, value)
				}(operation)
			}
		}
	}
}

func (executor *Executor) executeConnectorInstall(parent context.Context, operation db.ConnectorInstallOperation) {
	ctx, cancel := context.WithDeadline(parent, operation.ExpiresAt.Time)
	defer cancel()
	leaseDone := make(chan struct{})
	go executor.renewConnectorInstallLease(ctx, operation, leaseDone, cancel)
	claimed := operation
	go watchInstallAuthority(ctx, cancel, leaseDone, func(readCtx context.Context) (bool, error) {
		current, err := executor.Store.Queries.GetConnectorInstallOperation(readCtx, db.GetConnectorInstallOperationParams{ID: claimed.ID, EnterpriseID: claimed.EnterpriseID})
		return err == nil && current.Status == "running" && current.LeaseOwner == claimed.LeaseOwner &&
			current.Fence == claimed.Fence && current.Attempts == claimed.Attempts && current.LeaseExpiresAt.Valid && time.Now().Before(current.LeaseExpiresAt.Time), err
	})
	defer close(leaseDone)

	command, material, err := executor.loadConnectorInstallPlan(ctx, operation)
	if err != nil {
		executor.retryOrFinishConnectorInstall(ctx, operation, connectorInstallErrorCode(err), err)
		return
	}
	executor.appendConnectorInstallEvent(ctx, operation, "queued", "succeeded", "")
	executor.appendConnectorInstallEvent(ctx, operation, "ssh_connecting", "started", "")
	if err = executor.installConnectorOverSSH(ctx, &operation, command, material); err != nil {
		executor.retryOrFinishConnectorInstall(ctx, operation, connectorInstallErrorCode(err), err)
		return
	}
	if err = executor.waitConnectorOnline(ctx, &operation); err != nil {
		executor.retryOrFinishConnectorInstall(ctx, operation, connectorInstallErrorCode(err), err)
		return
	}
	_, _ = executor.Store.Queries.MarkConnectorInstallOnline(ctx, db.MarkConnectorInstallOnlineParams{ID: operation.ID, EnterpriseID: operation.EnterpriseID})
	executor.finishConnectorInstall(ctx, operation, "succeeded", "")
}

func (executor *Executor) loadConnectorInstallPlan(ctx context.Context, operation db.ConnectorInstallOperation) (*connectorv1.ConnectorInstallCommand, operationsecret.Material, error) {
	canonical, err := resource.CanonicalJSON(operation.Plan)
	if err != nil {
		return nil, operationsecret.Material{}, err
	}
	hash := sha256.Sum256(canonical)
	if !strings.EqualFold(fmt.Sprintf("%x", hash), fmt.Sprintf("%x", operation.PlanHash)) {
		return nil, operationsecret.Material{}, errors.New("connector install plan hash mismatch")
	}
	var command connectorv1.ConnectorInstallCommand
	if protojson.Unmarshal(canonical, &command) != nil || (command.GetOperation() != "install" && command.GetOperation() != "replace") ||
		command.GetConnectorId() != operation.ConnectorID.String() || command.GetBastionScopeId() != operation.BastionScopeID.String() ||
		command.GetHostId() != operation.HostID.String() || command.GetEnrollmentEndpoint() == "" || command.GetArtifact() == nil ||
		command.GetReleaseVersionId() != operation.ReleaseVersionID.String() {
		return nil, operationsecret.Material{}, errors.New("connector install plan is invalid")
	}
	if _, err = connectorInstallTrustBundle(&command); err != nil {
		return nil, operationsecret.Material{}, err
	}
	secretRecord, err := executor.Store.Queries.GetConnectorInstallOperationSecret(ctx, db.GetConnectorInstallOperationSecretParams{
		OperationID: operation.ID, EnterpriseID: operation.EnterpriseID,
	})
	if err != nil || secretRecord.KeyVersion != operationsecret.KeyVersion {
		return nil, operationsecret.Material{}, errors.New("connector install secret is unavailable")
	}
	material, err := operationsecret.Decrypt(executor.OperationSecretKey, secretRecord.Nonce, secretRecord.Ciphertext,
		operation.EnterpriseID, operation.ID)
	if err != nil {
		return nil, operationsecret.Material{}, err
	}
	return &command, material, nil
}

func (executor *Executor) installConnectorOverSSH(ctx context.Context, operation *db.ConnectorInstallOperation, command *connectorv1.ConnectorInstallCommand, material operationsecret.Material) error {
	publicKey, err := connectorInstallSigningKey(command)
	if err != nil {
		return err
	}
	installTrust, err := connectorInstallTrustBundle(command)
	if err != nil {
		return err
	}
	addresses, err := executor.Validator.Resolve(ctx, command.GetTargetAddress())
	if err != nil || len(addresses) == 0 {
		return resource.ErrDirectTargetDenied
	}
	credentialID, err := uuid.Parse(command.GetCredentialId())
	if err != nil || command.GetCredentialVersion() < 1 {
		return errors.New("connector install credential reference is invalid")
	}
	credential, err := executor.Store.Queries.GetCredential(ctx, db.GetCredentialParams{
		ID: credentialID, EnterpriseID: operation.EnterpriseID,
	})
	if err != nil || credential.Status != "active" {
		return secret.ErrCredentialUnavailable
	}
	if credential.Version != int64(command.GetCredentialVersion()) {
		return errCredentialVersionStale
	}
	lease, err := executor.Secrets.IssueLease(secret.WithActorType(ctx, "direct_executor"), executor.InstanceID, operation.EnterpriseID, secret.LeaseRequest{
		CredentialID: credentialID, OperationRef: "connector_install:" + operation.ID.String(), TargetResourceType: "host",
		TargetResourceID: operation.HostID, RecipientType: "direct_executor", RecipientID: executor.InstanceID,
		Protocol: "ssh", TTL: secret.MaxLeaseTTL,
	})
	if err != nil {
		return err
	}
	defer clear(lease.Value)
	client, err := executor.dialPinnedSSH(ctx, addresses[0], int32(command.GetTargetPort()), command.GetTargetUsername(),
		command.GetPinnedHostKey(), lease.Value)
	if err != nil {
		return err
	}
	defer client.Close()
	stopOnCancel := context.AfterFunc(ctx, func() { _ = client.Close() })
	defer stopOnCancel()
	if err = executor.Validator.Revalidate(ctx, command.GetTargetAddress(), addresses); err != nil {
		return err
	}
	if err = executor.advanceConnectorInstall(ctx, operation, "artifact_verifying"); err != nil {
		return err
	}
	artifactFile, err := os.CreateTemp("", ".argus-connector-artifact-*")
	if err != nil {
		return err
	}
	artifactPath := artifactFile.Name()
	defer func() {
		_ = artifactFile.Close()
		_ = os.Remove(artifactPath)
	}()
	if err = artifactFile.Chmod(0o600); err != nil {
		return err
	}
	if err = (collectormanager.Manager{HTTPClient: executor.CollectorArtifactHTTPClient,
		TrustedSigningKeys: map[string]ed25519.PublicKey{command.GetArtifact().GetSigningKeyId(): publicKey}}).
		FetchArtifactTo(ctx, command.GetArtifact(), artifactFile); err != nil {
		return err
	}
	if _, err = artifactFile.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err = executor.advanceConnectorInstall(ctx, operation, "artifact_transferring"); err != nil {
		return err
	}
	stage := "/var/lib/argus-connector-install/" + command.GetConnectorId()
	upload := `set -eu; umask 077; install -d -m 0700 ` + shellQuote(stage) + `; tmp=$(mktemp ` + shellQuote(stage+`/.argus-connector.XXXXXX`) + `); cat > "$tmp"; chmod 0700 "$tmp"; mv -f "$tmp" ` + shellQuote(stage+`/argus-connector`)
	if err = runSSHCommandReader(client, upload, artifactFile); err != nil {
		return err
	}
	if err = executor.advanceConnectorInstall(ctx, operation, "service_installing"); err != nil {
		return err
	}
	prepareRuntime := `set -eu; umask 077; id argus-connector >/dev/null 2>&1 || useradd --system --home-dir /var/lib/argus-connector --shell /usr/sbin/nologin argus-connector; install -d -m 0755 /etc/argus-connector; install -d -m 0700 -o argus-connector -g argus-connector /var/lib/argus-connector; install -d -m 0700 /var/lib/argus-otelcol /etc/argus-otelcol; current=""; if [ -f /var/lib/argus-connector/identity.json ]; then current=$(sed -n 's/.*"connector_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' /var/lib/argus-connector/identity.json | sed -n '1p'); fi; [ -n "$current" ] || { [ ! -f /var/lib/argus-connector/.desired-connector-id ] || current=$(sed -n '1p' /var/lib/argus-connector/.desired-connector-id); }; printf '%s' "$current" > ` + shellQuote(stage+`/previous-connector-id`)
	if err = runSSHCommand(client, prepareRuntime, nil); err != nil {
		return err
	}
	trust, _ := json.Marshal(map[string]string{command.GetArtifact().GetSigningKeyId(): command.GetArtifactSigningPublicKey()})
	writeTrust := `umask 077; cat > ` + shellQuote(stage+`/otelcol-signing-keys.json`) + `; chmod 0600 ` + shellQuote(stage+`/otelcol-signing-keys.json`)
	if err = runSSHCommand(client, writeTrust, trust); err != nil {
		return err
	}
	writeServerCA := `umask 077; cat > ` + shellQuote(stage+`/server-ca.pem`) + `; chmod 0600 ` + shellQuote(stage+`/server-ca.pem`)
	if err = runSSHCommand(client, writeServerCA, installTrust.PEM); err != nil {
		return err
	}
	hasArtifactCA := len(executor.CollectorArtifactCABundle) > 0
	if hasArtifactCA {
		if len(executor.CollectorArtifactCABundle) > 1<<20 {
			return errors.New("collector artifact CA bundle is too large")
		}
		writeCA := `umask 077; cat > ` + shellQuote(stage+`/otelcol-artifact-ca.pem`) + `; chmod 0600 ` + shellQuote(stage+`/otelcol-artifact-ca.pem`)
		if err = runSSHCommand(client, writeCA, executor.CollectorArtifactCABundle); err != nil {
			return err
		}
	}
	unit := connectorSystemdUnit(command, hasArtifactCA)
	writeUnit := `umask 077; tmp=$(mktemp /etc/systemd/system/.argus-connector.XXXXXX); cat > "$tmp"; chmod 0644 "$tmp"; mv -f "$tmp" /etc/systemd/system/argus-connector.service; systemctl daemon-reload`
	helperUnit := connectorPrivilegedSystemdUnit(command, hasArtifactCA)
	writeHelper := `umask 077; tmp=$(mktemp /etc/systemd/system/.argus-connector-privileged.XXXXXX); cat > "$tmp"; chmod 0644 "$tmp"; mv -f "$tmp" /etc/systemd/system/argus-connector-privileged.service; systemctl daemon-reload`
	if operation.InstallMode == "direct_install_tunnel" {
		if err = executor.advanceConnectorInstall(ctx, operation, "control_tunnel_establishing"); err != nil {
			return err
		}
		if err = executor.waitControlTunnelEstablished(ctx, *operation); err != nil {
			return err
		}
	}
	if err = executor.advanceConnectorInstall(ctx, operation, "enrolling"); err != nil {
		return err
	}
	enroll := connectorEnrollCommand(command, material.EnrollmentToken, stage)
	if err = runSSHCommand(client, enroll, nil); err != nil {
		return err
	}
	promote := `set -eu; current=$(cat ` + shellQuote(stage+`/previous-connector-id`) + `); systemctl disable --now argus-connector-privileged.service >/dev/null 2>&1 || true; systemctl disable --now argus-connector.service >/dev/null 2>&1 || true; if [ -n "$current" ] && [ "$current" != ` + shellQuote(command.GetConnectorId()) + ` ]; then systemctl disable --now argus-otelcol.service >/dev/null 2>&1 || true; rm -f /etc/systemd/system/argus-otelcol.service /usr/local/bin/argus-otelcol; rm -rf /var/lib/argus-otelcol /etc/argus-otelcol; install -d -m 0700 /var/lib/argus-otelcol /etc/argus-otelcol; fi; install -m 0755 ` + shellQuote(stage+`/argus-connector`) + ` /usr/local/bin/argus-connector; install -m 0644 ` + shellQuote(stage+`/server-ca.pem`) + ` /etc/argus-connector/server-ca.pem; install -m 0644 ` + shellQuote(stage+`/otelcol-signing-keys.json`) + ` /etc/argus-connector/otelcol-signing-keys.json; `
	if hasArtifactCA {
		promote += `install -m 0644 ` + shellQuote(stage+`/otelcol-artifact-ca.pem`) + ` /etc/argus-connector/otelcol-artifact-ca.pem; `
	} else {
		promote += `rm -f /etc/argus-connector/otelcol-artifact-ca.pem; `
	}
	promote += `printf '%s' ` + shellQuote(command.GetConnectorId()) + ` > /var/lib/argus-connector/.desired-connector-id; chown -R argus-connector:argus-connector /var/lib/argus-connector /etc/argus-connector`
	if err = runSSHCommand(client, promote, nil); err != nil {
		return err
	}
	if err = runSSHCommand(client, writeUnit, []byte(unit)); err != nil {
		return err
	}
	if err = runSSHCommand(client, writeHelper, []byte(helperUnit)); err != nil {
		return err
	}
	_, _ = executor.Store.Queries.ConsumeConnectorInstallOperationSecret(ctx, db.ConsumeConnectorInstallOperationSecretParams{
		OperationID: operation.ID, EnterpriseID: operation.EnterpriseID,
	})
	activate := "systemctl enable --now argus-connector-privileged.service; systemctl enable --now argus-connector.service; systemctl is-active --quiet argus-connector.service; rm -rf " + shellQuote(stage) + "; rmdir /var/lib/argus-connector-install >/dev/null 2>&1 || true"
	if err = runSSHCommand(client, activate, nil); err != nil {
		return err
	}
	return executor.advanceConnectorInstall(ctx, operation, "waiting_connector_online")
}

func connectorEnrollCommand(command *connectorv1.ConnectorInstallCommand, token, stage string) string {
	value := shellQuote(stage+"/argus-connector") + " enroll --connector-id " + shellQuote(command.GetConnectorId()) +
		" --token " + shellQuote(token) + " --server " + shellQuote(command.GetEnrollmentEndpoint()) +
		" --ca-file " + shellQuote(stage+"/server-ca.pem") + " --role bastion"
	if command.GetEnrollDialAddress() != "" {
		value = "ARGUS_CONNECTOR_ENROLL_ADDRESS=" + shellQuote(command.GetEnrollDialAddress()) + " " + value
	}
	return value
}

func connectorInstallTrustBundle(command *connectorv1.ConnectorInstallCommand) (trustbundle.Material, error) {
	if command == nil || command.GetTrustBundleEpoch() < 1 || len(command.GetTrustBundlePem()) == 0 ||
		len(command.GetTrustBundlePem()) > 1<<20 || command.GetTrustBundleSha256() == "" ||
		len(command.GetTrustBundleCaFingerprints()) == 0 {
		return trustbundle.Material{}, errors.New("connector install Trust Bundle is invalid")
	}
	material, err := trustbundle.Parse(command.GetTrustBundlePem(), time.Now().UTC())
	if err != nil || material.SHA256 != command.GetTrustBundleSha256() ||
		!slices.Equal(material.Fingerprints, command.GetTrustBundleCaFingerprints()) {
		return trustbundle.Material{}, errors.New("connector install Trust Bundle digest or fingerprints do not match")
	}
	return material, nil
}

func connectorSystemdUnit(command *connectorv1.ConnectorInstallCommand, hasArtifactCA bool) string {
	environment := "Environment=ARGUS_OTELCOL_SIGNING_PUBLIC_KEYS_FILE=/etc/argus-connector/otelcol-signing-keys.json\n"
	environment += fmt.Sprintf("Environment=ARGUS_BASTION_RELAY_ARTIFACT_ENDPOINT=%s\n", command.GetArtifact().GetUri())
	if hasArtifactCA {
		environment += "Environment=ARGUS_OTELCOL_ARTIFACT_CA_PATH=/etc/argus-connector/otelcol-artifact-ca.pem\n"
	}
	if command.GetEnrollDialAddress() != "" {
		environment += fmt.Sprintf("Environment=ARGUS_CONNECTOR_ENROLL_ADDRESS=%s\n", command.GetEnrollDialAddress())
		environment += fmt.Sprintf("Environment=ARGUS_ARTIFACT_DIAL_ADDRESS=%s\n", command.GetEnrollDialAddress())
	}
	if command.GetGatewayDialAddress() != "" {
		environment += fmt.Sprintf("Environment=ARGUS_CONNECTOR_DIAL_ADDRESS=%s\n", command.GetGatewayDialAddress())
	}
	return fmt.Sprintf(`[Unit]
Description=Argus Connector
After=network-online.target argus-connector-privileged.service
Wants=network-online.target argus-connector-privileged.service

[Service]
Type=simple
User=argus-connector
Group=argus-connector
%sExecStart=/usr/local/bin/argus-connector run
Restart=always
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
ReadWritePaths=/var/lib/argus-connector /etc/argus-connector

[Install]
WantedBy=multi-user.target
`, environment)
}

func connectorPrivilegedSystemdUnit(command *connectorv1.ConnectorInstallCommand, hasArtifactCA bool) string {
	environment := "Environment=ARGUS_OTELCOL_SIGNING_PUBLIC_KEYS_FILE=/etc/argus-connector/otelcol-signing-keys.json\n"
	if hasArtifactCA {
		environment += "Environment=ARGUS_OTELCOL_ARTIFACT_CA_PATH=/etc/argus-connector/otelcol-artifact-ca.pem\n"
	}
	if command.GetEnrollDialAddress() != "" {
		environment += fmt.Sprintf("Environment=ARGUS_ARTIFACT_DIAL_ADDRESS=%s\n", command.GetEnrollDialAddress())
	}
	return fmt.Sprintf(`[Unit]
Description=Argus Connector privileged local lifecycle helper
After=network-online.target

[Service]
Type=simple
User=root
Group=argus-connector
RuntimeDirectory=argus-connector
RuntimeDirectoryMode=0750
%sExecStart=/usr/local/bin/argus-connector privileged-helper
Restart=always
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
ReadWritePaths=/run/argus-connector /var/lib/argus-otelcol /etc/argus-otelcol /usr/local/bin /etc/systemd/system /run/systemd/system
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true

[Install]
WantedBy=multi-user.target
`, environment)
}

func connectorInstallSigningKey(command *connectorv1.ConnectorInstallCommand) (ed25519.PublicKey, error) {
	if command == nil || command.GetArtifact() == nil || command.GetArtifact().GetSigningKeyId() == "" {
		return nil, errors.New("connector install signing trust is unavailable")
	}
	decoded, err := base64.RawStdEncoding.DecodeString(command.GetArtifactSigningPublicKey())
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return nil, errors.New("connector install signing trust is invalid")
	}
	return ed25519.PublicKey(decoded), nil
}

func (executor *Executor) waitControlTunnelEstablished(ctx context.Context, operation db.ConnectorInstallOperation) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		tunnel, err := executor.Store.Queries.GetConnectorControlTunnelByConnector(ctx, db.GetConnectorControlTunnelByConnectorParams{
			ConnectorID: operation.ConnectorID, EnterpriseID: operation.EnterpriseID,
		})
		if err == nil && tunnel.Status == "established" && tunnel.Epoch > 0 {
			return nil
		}
		if err == nil && tunnel.Status == "down" {
			switch tunnel.LastDropReason {
			case "tunnel_quota_exceeded":
				return errTunnelQuotaExceeded
			case "host_key_changed":
				return errHostKeyMismatch
			case "credential_revoked", "credential_unavailable":
				return secret.ErrCredentialUnavailable
			case "target_resolution_failed", "target_revalidation_failed":
				return resource.ErrDirectTargetDenied
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (executor *Executor) waitConnectorOnline(ctx context.Context, operation *db.ConnectorInstallOperation) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		connector, err := executor.Store.Queries.GetConnector(ctx, db.GetConnectorParams{ID: operation.ConnectorID, EnterpriseID: operation.EnterpriseID})
		if err == nil && connector.Status == "online" && connector.ConnectionEpoch > 0 {
			if operation.InstallMode != "direct_install_tunnel" {
				return executor.advanceConnectorInstall(ctx, operation, "completed")
			}
			tunnel, tunnelErr := executor.Store.Queries.GetConnectorControlTunnelByConnector(ctx, db.GetConnectorControlTunnelByConnectorParams{
				ConnectorID: operation.ConnectorID, EnterpriseID: operation.EnterpriseID,
			})
			if tunnelErr == nil && tunnel.Status == "established" {
				return executor.advanceConnectorInstall(ctx, operation, "completed")
			}
			if tunnelErr == nil && tunnel.Status == "down" && tunnel.LastDropReason == "tunnel_quota_exceeded" {
				return errTunnelQuotaExceeded
			}
		} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (executor *Executor) advanceConnectorInstall(ctx context.Context, operation *db.ConnectorInstallOperation, stage string) error {
	previous := operation.Stage
	updated, err := executor.Store.Queries.AdvanceConnectorInstallOperation(ctx, db.AdvanceConnectorInstallOperationParams{
		ID: operation.ID, EnterpriseID: operation.EnterpriseID, Fence: operation.Fence, Stage: stage,
	})
	if err == nil {
		executor.appendConnectorInstallEvent(ctx, *operation, previous, "succeeded", "")
		*operation = updated
		executor.appendConnectorInstallEvent(ctx, *operation, stage, "started", "")
	}
	return err
}

func (executor *Executor) appendConnectorInstallEvent(ctx context.Context, operation db.ConnectorInstallOperation, stage, status, errorCode string) {
	events, err := executor.Store.Queries.ListConnectorInstallOperationEvents(ctx, db.ListConnectorInstallOperationEventsParams{
		OperationID: operation.ID, EnterpriseID: operation.EnterpriseID,
	})
	if err != nil {
		return
	}
	sequence := int64(1)
	if len(events) > 0 {
		sequence = events[len(events)-1].Sequence + 1
	}
	_, _ = executor.Store.Queries.CreateConnectorInstallOperationEvent(ctx, db.CreateConnectorInstallOperationEventParams{
		ID: uuid.New(), OperationID: operation.ID, EnterpriseID: operation.EnterpriseID, Sequence: sequence,
		Stage: stage, Status: status, ErrorCode: pgtype.Text{String: errorCode, Valid: errorCode != ""},
	})
}

func (executor *Executor) retryOrFinishConnectorInstall(ctx context.Context, operation db.ConnectorInstallOperation, errorCode string, cause error) {
	slog.Warn("connector install attempt failed", "operation_id", operation.ID, "stage", operation.Stage,
		"attempt", operation.Attempts, "error_code", errorCode, "error", cause)
	if operation.Attempts < 3 && time.Now().UTC().Before(operation.ExpiresAt.Time) {
		if _, err := executor.Store.Queries.RequeueConnectorInstallOperation(ctx, db.RequeueConnectorInstallOperationParams{
			ID: operation.ID, EnterpriseID: operation.EnterpriseID, Fence: operation.Fence,
			ErrorCode: pgtype.Text{String: errorCode, Valid: true},
		}); err == nil {
			executor.appendConnectorInstallEvent(ctx, operation, operation.Stage, "retrying", errorCode)
			return
		}
	}
	executor.appendConnectorInstallEvent(ctx, operation, operation.Stage, "failed", errorCode)
	executor.finishConnectorInstall(ctx, operation, "failed", errorCode)
}

func (executor *Executor) finishConnectorInstall(ctx context.Context, operation db.ConnectorInstallOperation, status, errorCode string) {
	_, err := executor.Store.Queries.FinishConnectorInstallOperation(ctx, db.FinishConnectorInstallOperationParams{
		ID: operation.ID, EnterpriseID: operation.EnterpriseID, Fence: operation.Fence,
		Status: status, ErrorCode: pgtype.Text{String: errorCode, Valid: errorCode != ""},
	})
	if err != nil {
		slog.Error("finish connector install failed", "operation_id", operation.ID, "error", err)
		return
	}
	stage := operation.Stage
	if status == "succeeded" {
		stage = "completed"
	}
	executor.appendConnectorInstallEvent(ctx, operation, stage, status, errorCode)
}

func (executor *Executor) renewConnectorInstallLease(ctx context.Context, operation db.ConnectorInstallOperation, done <-chan struct{}, cancel context.CancelFunc) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			rows, _ := executor.Store.Queries.RenewConnectorInstallOperationLease(ctx, db.RenewConnectorInstallOperationLeaseParams{
				ID: operation.ID, EnterpriseID: operation.EnterpriseID, Fence: operation.Fence, LeaseOwner: executor.InstanceID,
			})
			if rows == 0 {
				cancel()
				return
			}
		}
	}
}

func connectorInstallErrorCode(err error) string {
	switch {
	case errors.Is(err, collectormanager.ErrArtifactInvalid):
		return "CONNECTOR_ARTIFACT_INVALID"
	case errors.Is(err, resource.ErrDirectTargetDenied):
		return "CONNECTOR_INSTALL_TARGET_DENIED"
	case errors.Is(err, errHostKeyMismatch):
		return "SSH_HOST_KEY_CHANGED"
	case errors.Is(err, errTunnelQuotaExceeded):
		return "TUNNEL_QUOTA_EXCEEDED"
	case errors.Is(err, secret.ErrCredentialUnavailable):
		return "CREDENTIAL_UNAVAILABLE"
	case errors.Is(err, errCredentialVersionStale):
		return "CREDENTIAL_VERSION_STALE"
	case errors.Is(err, errControlTunnelUnavailable):
		return "CONTROL_TUNNEL_UNAVAILABLE"
	case errors.Is(err, context.DeadlineExceeded):
		return "CONNECTOR_INSTALL_TIMEOUT"
	default:
		return "CONNECTOR_INSTALL_FAILED"
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
