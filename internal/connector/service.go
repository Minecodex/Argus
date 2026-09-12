package connector

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/encoding/protojson"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/kakj-go/Argus/internal/artifactcheck"
	"github.com/kakj-go/Argus/internal/audit"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/hostonboarding"
	"github.com/kakj-go/Argus/internal/hostremoval"
	"github.com/kakj-go/Argus/internal/installation"
	"github.com/kakj-go/Argus/internal/installinstruction"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	redisstore "github.com/kakj-go/Argus/internal/storage/redis"
	"github.com/kakj-go/Argus/internal/telemetrybinding"
	"github.com/kakj-go/Argus/internal/trustbundle"
)

var (
	ErrEnrollmentInvalid  = errors.New("connector enrollment invalid")
	ErrEnrollmentConflict = errors.New("connector enrollment conflict")
	ErrConnectorFenced    = errors.New("connector session fenced")
	ErrCommandState       = errors.New("connector command state conflict")
)

type Certificate struct {
	PEM                    string
	CABundlePEM            string
	SerialNumber           string
	CertificateRequestName string
	IssuerGeneration       int32
	NotBefore, NotAfter    time.Time
}

type CertificateIssuer interface {
	Issue(context.Context, uuid.UUID, *x509.CertificateRequest, time.Duration) (Certificate, error)
}

type Service struct {
	Store           *postgres.Store
	Redis           *redisstore.Client
	Issuer          CertificateIssuer
	EnrollmentURL   string
	GatewayEndpoint string
	GatewayInstance string
	CertificateTTL  time.Duration
	RegistryTTL     time.Duration
	Credentials     secret.Service
	Artifacts       artifactcheck.Checker
	installinstruction.TrustConfig
	KubernetesImage string
	TrustBundles    trustbundle.Service
}

type EnrollInput struct {
	Token, CSRPem, DeviceFingerprint, InstanceID, Platform, Architecture, Name, SoftwareVersion string
	Capabilities                                                                                []string
}

type EnrollmentResult struct {
	Connector       db.Connector
	CertificatePEM  string
	TrustBundle     trustbundle.Bundle
	GatewayEndpoint string
	Result          string
}

type CreatedEnrollment struct {
	Record          db.ConnectorEnrollmentToken
	ConnectorID     uuid.UUID
	InstructionSets []installinstruction.Set
	Token           string
}

type CreateEnrollmentInput struct {
	Role, Purpose  string
	BastionScopeID uuid.NullUUID
	ClusterID      uuid.NullUUID
	HostID         uuid.NullUUID
	// ManualInstall is the only flow that returns a browser-visible curl
	// command. Its exact release is frozen in the Pending Action.
	ManualInstall    bool
	ReleaseVersionID uuid.NullUUID
	TargetPlatform   installation.Platform
	Policy           json.RawMessage
	TTL              time.Duration
	ImagePullSecrets []string
}

func (service Service) CreateEnrollment(ctx context.Context, q *db.Queries, actorID string, enterpriseID uuid.UUID, input CreateEnrollmentInput) (CreatedEnrollment, error) {
	if (input.Role == "host" && (!input.HostID.Valid || input.ClusterID.Valid || !input.ReleaseVersionID.Valid || !input.TargetPlatform.Valid())) ||
		(input.Role == "bastion" && (!input.BastionScopeID.Valid || !input.HostID.Valid || input.ClusterID.Valid || input.ManualInstall != input.ReleaseVersionID.Valid || !input.TargetPlatform.Valid() || input.TargetPlatform.OS() != "linux")) ||
		(input.Role == "kubernetes" && (!input.ClusterID.Valid || input.BastionScopeID.Valid || input.HostID.Valid || input.ManualInstall || input.ReleaseVersionID.Valid)) ||
		(input.Role != "host" && input.Role != "bastion" && input.Role != "kubernetes") ||
		(input.Role != "kubernetes" && len(input.ImagePullSecrets) != 0) || !validImagePullSecrets(input.ImagePullSecrets) {
		return CreatedEnrollment{}, ErrEnrollmentInvalid
	}
	ttl := input.TTL
	if ttl <= 0 || ttl > 24*time.Hour {
		ttl = 30 * time.Minute
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return CreatedEnrollment{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	clear(raw)
	tokenHash := sha256.Sum256([]byte(token))
	connectorID := newID()
	expiresAt := time.Now().UTC().Add(ttl)
	record, err := q.CreateConnectorEnrollmentToken(ctx, db.CreateConnectorEnrollmentTokenParams{ID: newID(), PreallocatedConnectorID: connectorID,
		EnterpriseID: enterpriseID, Role: input.Role, Purpose: input.Purpose, BastionScopeID: input.BastionScopeID,
		KubernetesClusterID: input.ClusterID, PreallocatedHostID: input.HostID, TokenHash: tokenHash[:], Policy: input.Policy,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true}, CreatedBy: uuid.MustParse(actorID), ReleaseVersionID: input.ReleaseVersionID})
	if err != nil {
		return CreatedEnrollment{}, err
	}
	if service.EnrollmentURL == "" || service.GatewayEndpoint == "" {
		return CreatedEnrollment{}, ErrEnrollmentInvalid
	}
	var instructionSets []installinstruction.Set
	if ((input.Role == "bastion" || input.Role == "host") && input.ManualInstall) || input.Role == "kubernetes" {
		releaseVersionID := input.ReleaseVersionID
		if input.Role == "kubernetes" {
			release, releaseErr := q.GetActiveConnectorReleaseVersion(ctx)
			if releaseErr != nil {
				return CreatedEnrollment{}, ErrConnectorArtifactUnavailable
			}
			releaseVersionID = uuid.NullUUID{UUID: release.ID, Valid: true}
		}
		release, releaseErr := q.GetConnectorReleaseVersion(ctx, releaseVersionID.UUID)
		if releaseErr != nil {
			return CreatedEnrollment{}, ErrConnectorArtifactUnavailable
		}
		targetPlatform := input.TargetPlatform
		if input.Role == "kubernetes" {
			targetPlatform = installation.LinuxAMD64
		}
		manifest, manifestErr := connectorManualInstallRelease(release.Manifest, targetPlatform.Architecture())
		if manifestErr != nil {
			return CreatedEnrollment{}, manifestErr
		}
		installer, installerErr := connectorInstallerFromManifest(manifest, targetPlatform.OS())
		if installerErr != nil {
			return CreatedEnrollment{}, installerErr
		}
		if service.Artifacts != nil {
			urls := []string{manifest.ManifestURI, installer.URI}
			for _, artifact := range manifest.Artifacts {
				urls = append(urls, artifact.URI)
			}
			if checkErr := service.Artifacts.Check(ctx, urls...); checkErr != nil {
				return CreatedEnrollment{}, fmt.Errorf("%w: %v", ErrConnectorArtifactUnavailable, checkErr)
			}
		}
		bundle, bundleErr := service.installationTrustBundle(ctx)
		if bundleErr != nil {
			return CreatedEnrollment{}, bundleErr
		}
		arguments := []string{"--manifest", manifest.ManifestURI, "--key-id", manifest.SigningKeyID,
			"--public-key", manifest.SigningPublicKey, "--connector-id", connectorID.String(),
			"--server", service.EnrollmentURL, "--role", input.Role}
		policy := connectorEnrollmentPolicy(input.Policy)
		if policy.EnrollDialAddress != "" {
			arguments = append(arguments, "--enroll-dial-address", policy.EnrollDialAddress)
		}
		if policy.GatewayDialAddress != "" {
			arguments = append(arguments, "--gateway-dial-address", policy.GatewayDialAddress)
		}
		scopes := []installinstruction.Scope{installinstruction.ScopeLinuxSystem}
		if input.Role == "kubernetes" {
			if strings.TrimSpace(service.KubernetesImage) == "" {
				return CreatedEnrollment{}, ErrEnrollmentInvalid
			}
			arguments = append(arguments, "--connector-image", service.KubernetesImage)
			if len(input.ImagePullSecrets) != 0 {
				arguments = append(arguments, "--image-pull-secrets", strings.Join(input.ImagePullSecrets, ","))
			}
			scopes = []installinstruction.Scope{installinstruction.ScopeKubernetes}
		}
		for _, scope := range scopes {
			warnings := []string{}
			bootstrapScriptURL := ""
			bootstrapTLSMode := installinstruction.DownloadTLSMode("")
			if input.Role == "bastion" || input.Role == "host" {
				bootstrapScriptURL = connectorBootstrapScriptURL(service.EnrollmentURL)
				bootstrapTLSMode = service.BootstrapTLSMode
			}
			if targetPlatform == installation.WindowsAMD64 {
				warnings = []string{"Requires elevated PowerShell 5.1+, curl.exe, and OpenSSL for Ed25519 artifact verification"}
				windowsArguments := []string{"-Manifest", manifest.ManifestURI, "-KeyId", manifest.SigningKeyID, "-PublicKey", manifest.SigningPublicKey,
					"-ConnectorId", connectorID.String(), "-Server", service.EnrollmentURL, "-Role", input.Role}
				if policy.EnrollDialAddress != "" {
					windowsArguments = append(windowsArguments, "-EnrollDialAddress", policy.EnrollDialAddress)
				}
				if policy.GatewayDialAddress != "" {
					windowsArguments = append(windowsArguments, "-GatewayDialAddress", policy.GatewayDialAddress)
				}
				instruction, buildErr := installinstruction.BuildWindows(installinstruction.WindowsOptions{Platform: targetPlatform,
					InstallerURL: installer.URI, InstallerSHA256: installer.SHA256, BootstrapScriptURL: bootstrapScriptURL,
					HTTPSDialAddress: policy.EnrollDialAddress,
					DownloadTLSMode:  bootstrapTLSMode, TrustBundlePEM: bundle.Material.PEM, TrustBundleEpoch: bundle.Epoch,
					Token: token, ExpiresAt: record.ExpiresAt.Time, InstallerArguments: windowsArguments, CapabilityWarnings: warnings, ReleaseVersion: release.Version})
				if buildErr != nil {
					return CreatedEnrollment{}, buildErr
				}
				instructionSets = append(instructionSets, instruction)
				continue
			}
			instruction, buildErr := installinstruction.BuildPOSIX(installinstruction.POSIXOptions{Scope: scope,
				Platform:     targetPlatform,
				InstallerURL: installer.URI, InstallerSHA256: installer.SHA256,
				BootstrapScriptURL: bootstrapScriptURL, DownloadTLSMode: bootstrapTLSMode,
				HTTPSDialAddress: policy.EnrollDialAddress,
				TrustBundlePEM:   bundle.Material.PEM, TrustBundleEpoch: bundle.Epoch, Token: token,
				ExpiresAt: record.ExpiresAt.Time, InstallerArguments: arguments, CapabilityWarnings: warnings, ReleaseVersion: release.Version})
			if buildErr != nil {
				return CreatedEnrollment{}, buildErr
			}
			instructionSets = append(instructionSets, instruction)
		}
	}
	return CreatedEnrollment{Record: record, ConnectorID: connectorID, InstructionSets: instructionSets, Token: token}, nil
}

type enrollmentPolicy struct {
	TargetPlatform     string `json:"target_platform"`
	ControlPath        string `json:"control_path"`
	EnrollDialAddress  string `json:"enroll_dial_address"`
	GatewayDialAddress string `json:"gateway_dial_address"`
}

func connectorEnrollmentPolicy(raw json.RawMessage) enrollmentPolicy {
	var policy enrollmentPolicy
	_ = json.Unmarshal(raw, &policy)
	return policy
}

func (service Service) installationTrustBundle(ctx context.Context) (trustbundle.Bundle, error) {
	if service.TrustBundles.Store != nil {
		return service.TrustBundles.Current(ctx)
	}
	value, err := os.ReadFile(service.TrustBundlePath)
	if err != nil {
		return trustbundle.Bundle{}, fmt.Errorf("read installation Trust Bundle: %w", err)
	}
	material, err := trustbundle.Parse(value, time.Now().UTC())
	if err != nil {
		return trustbundle.Bundle{}, err
	}
	epoch := service.TrustBundleEpoch
	if epoch < 1 {
		return trustbundle.Bundle{}, errors.New("installation Trust Bundle epoch is invalid")
	}
	return trustbundle.Bundle{Epoch: epoch, State: trustbundle.StateStable, Material: material,
		CurrentCAFingerprints: material.Fingerprints, StartedAt: time.Now().UTC()}, nil
}

func connectorBootstrapScriptURL(enrollmentURL string) string {
	return strings.TrimRight(enrollmentURL, "/") + "/api/v1/connectors/bootstrap-script"
}

// BootstrapScript rebuilds the strict self-contained installer behind the
// short download command. Reading the script does not consume enrollment; the
// installer still performs the existing one-time enrollment transaction.
func (service Service) BootstrapScript(ctx context.Context, token string, scope installinstruction.Scope) (string, error) {
	if service.Store == nil || strings.TrimSpace(token) == "" ||
		(scope != installinstruction.ScopeLinuxSystem && scope != installinstruction.ScopeWindowsSystem) {
		return "", ErrEnrollmentInvalid
	}
	tokenHash := sha256.Sum256([]byte(token))
	record, err := service.Store.Queries.GetEnrollmentTokenByHash(ctx, tokenHash[:])
	if err != nil || record.Status != "active" || !record.ExpiresAt.Valid || !time.Now().UTC().Before(record.ExpiresAt.Time) ||
		(record.Role != "host" && record.Role != "bastion") || !record.ReleaseVersionID.Valid ||
		(record.Purpose != "host_registration" && record.Purpose != "initial_registration" && record.Purpose != "connector_replacement") {
		return "", ErrEnrollmentInvalid
	}
	release, err := service.Store.Queries.GetConnectorReleaseVersion(ctx, record.ReleaseVersionID.UUID)
	if err != nil {
		return "", ErrConnectorArtifactUnavailable
	}
	policy := connectorEnrollmentPolicy(record.Policy)
	targetPlatform := installation.Platform(policy.TargetPlatform)
	if !targetPlatform.Valid() || (scope == installinstruction.ScopeWindowsSystem) != (targetPlatform == installation.WindowsAMD64) {
		return "", ErrEnrollmentInvalid
	}
	manifest, err := connectorManualInstallRelease(release.Manifest, targetPlatform.Architecture())
	if err != nil {
		return "", ErrConnectorArtifactUnavailable
	}
	installer, installerErr := connectorInstallerFromManifest(manifest, targetPlatform.OS())
	if installerErr != nil {
		return "", installerErr
	}
	if service.Artifacts != nil {
		urls := []string{manifest.ManifestURI, installer.URI}
		for _, artifact := range manifest.Artifacts {
			urls = append(urls, artifact.URI)
		}
		if err = service.Artifacts.Check(ctx, urls...); err != nil {
			return "", fmt.Errorf("%w: %v", ErrConnectorArtifactUnavailable, err)
		}
	}
	bundle, err := service.installationTrustBundle(ctx)
	if err != nil {
		return "", err
	}
	arguments := []string{"--manifest", manifest.ManifestURI, "--key-id", manifest.SigningKeyID,
		"--public-key", manifest.SigningPublicKey, "--connector-id", record.PreallocatedConnectorID.String(),
		"--server", service.EnrollmentURL, "--role", record.Role}
	if policy.EnrollDialAddress != "" {
		arguments = append(arguments, "--enroll-dial-address", policy.EnrollDialAddress)
	}
	if policy.GatewayDialAddress != "" {
		arguments = append(arguments, "--gateway-dial-address", policy.GatewayDialAddress)
	}
	var instruction installinstruction.Set
	if targetPlatform == installation.WindowsAMD64 {
		windowsArguments := []string{"-Manifest", manifest.ManifestURI, "-KeyId", manifest.SigningKeyID, "-PublicKey", manifest.SigningPublicKey,
			"-ConnectorId", record.PreallocatedConnectorID.String(), "-Server", service.EnrollmentURL, "-Role", record.Role}
		if policy.EnrollDialAddress != "" {
			windowsArguments = append(windowsArguments, "-EnrollDialAddress", policy.EnrollDialAddress)
		}
		if policy.GatewayDialAddress != "" {
			windowsArguments = append(windowsArguments, "-GatewayDialAddress", policy.GatewayDialAddress)
		}
		instruction, err = installinstruction.BuildWindows(installinstruction.WindowsOptions{Platform: targetPlatform,
			InstallerURL: installer.URI, InstallerSHA256: installer.SHA256, BootstrapScriptURL: connectorBootstrapScriptURL(service.EnrollmentURL),
			HTTPSDialAddress: policy.EnrollDialAddress,
			DownloadTLSMode:  service.BootstrapTLSMode, TrustBundlePEM: bundle.Material.PEM, TrustBundleEpoch: bundle.Epoch,
			Token: token, ExpiresAt: record.ExpiresAt.Time, InstallerArguments: windowsArguments,
			CapabilityWarnings: []string{"Requires elevated PowerShell 5.1+, curl.exe, and OpenSSL for Ed25519 artifact verification"}, ReleaseVersion: release.Version})
	} else {
		instruction, err = installinstruction.BuildPOSIX(installinstruction.POSIXOptions{Scope: scope, Platform: targetPlatform,
			InstallerURL: installer.URI, InstallerSHA256: installer.SHA256,
			BootstrapScriptURL: connectorBootstrapScriptURL(service.EnrollmentURL), DownloadTLSMode: service.BootstrapTLSMode,
			HTTPSDialAddress: policy.EnrollDialAddress,
			TrustBundlePEM:   bundle.Material.PEM, TrustBundleEpoch: bundle.Epoch, Token: token,
			ExpiresAt: record.ExpiresAt.Time, InstallerArguments: arguments, ReleaseVersion: release.Version})
	}
	if err != nil {
		return "", err
	}
	if record.Role == "host" {
		_ = hostonboarding.AdvanceByConnector(ctx, service.Store.Queries, record.PreallocatedConnectorID, record.EnterpriseID, "installing")
	}
	_, _ = audit.Append(ctx, service.Store.Queries, audit.Entry{Domain: "enterprise",
		EnterpriseID: uuid.NullUUID{UUID: record.EnterpriseID, Valid: true}, ActorType: "system", ActorID: "connector-bootstrap",
		Action: "connector.bootstrap_script.download", ResourceType: "connector", ResourceID: record.PreallocatedConnectorID.String(),
		Result: "success", Details: map[string]any{"scope": scope, "tls_mode": service.BootstrapTLSMode}})
	return instruction.BootstrapScript + "\n", nil
}

func (service Service) CreateKubernetesEnrollment(ctx context.Context, q *db.Queries, actorID string, enterpriseID, clusterID uuid.UUID, imagePullSecrets []string) (resource.EnrollmentResult, error) {
	created, err := service.CreateEnrollment(ctx, q, actorID, enterpriseID, CreateEnrollmentInput{Role: "kubernetes", Purpose: "kubernetes_registration",
		ClusterID: uuid.NullUUID{UUID: clusterID, Valid: true}, ImagePullSecrets: imagePullSecrets,
		Policy: json.RawMessage(`{"capabilities":["kubernetes.connection_probe","kubernetes.query","credential.lease","connector.uninstall"]}`)})
	if err != nil {
		return resource.EnrollmentResult{}, err
	}
	return resource.EnrollmentResult{EnrollmentID: created.Record.ID, InstructionSets: created.InstructionSets, ExpiresAt: created.Record.ExpiresAt.Time}, nil
}

func validImagePullSecrets(values []string) bool {
	if len(values) > 16 {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if len(validation.IsDNS1123Subdomain(value)) != 0 {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

type TrustedIdentity struct {
	ConnectorID  uuid.UUID
	EnterpriseID uuid.UUID
	SerialNumber string
}

func (service Service) Enroll(ctx context.Context, input EnrollInput) (EnrollmentResult, error) {
	if service.Issuer == nil || input.Token == "" || input.DeviceFingerprint == "" || input.InstanceID == "" || (input.Platform != "linux" && input.Platform != "windows") ||
		(input.Architecture != "amd64" && input.Architecture != "arm64") || service.GatewayEndpoint == "" {
		return EnrollmentResult{}, ErrEnrollmentInvalid
	}
	csr, publicKeyHash, err := parseCSR(input.CSRPem)
	if err != nil {
		return EnrollmentResult{}, err
	}
	deviceHash := sha256.Sum256([]byte(input.DeviceFingerprint))
	tokenHash := sha256.Sum256([]byte(input.Token))
	if candidate, candidateErr := service.Store.Queries.GetEnrollmentTokenByHash(ctx, tokenHash[:]); candidateErr == nil && candidate.Role == "host" && candidate.Status == "active" {
		_ = hostonboarding.AdvanceByConnector(ctx, service.Store.Queries, candidate.PreallocatedConnectorID, candidate.EnterpriseID, "enrolling")
	}
	var result EnrollmentResult
	var takeoverConnectorID uuid.NullUUID
	err = service.Store.InTx(ctx, func(q *db.Queries) error {
		token, err := q.GetEnrollmentTokenForUpdate(ctx, tokenHash[:])
		if err != nil {
			return ErrEnrollmentInvalid
		}
		if token.Status == "consumed" {
			if !subtleEqual(token.ConsumedDeviceHash, deviceHash[:]) || !token.RegisteredConnectorID.Valid {
				return ErrEnrollmentConflict
			}
			connector, err := q.GetConnector(ctx, db.GetConnectorParams{ID: token.RegisteredConnectorID.UUID, EnterpriseID: token.EnterpriseID})
			if err != nil || !subtleEqual(connector.PublicKeyHash, publicKeyHash) || connector.InstanceID != input.InstanceID {
				return ErrEnrollmentConflict
			}
			certificate, err := q.GetActiveConnectorCertificate(ctx, db.GetActiveConnectorCertificateParams{ConnectorID: connector.ID, EnterpriseID: connector.EnterpriseID})
			if err != nil {
				return err
			}
			bundle, bundleErr := service.enrollmentTrustBundle(ctx, certificate.CaBundlePem)
			if bundleErr != nil {
				return bundleErr
			}
			result = EnrollmentResult{Connector: connector, CertificatePEM: certificate.CertificatePem, TrustBundle: bundle,
				GatewayEndpoint: service.GatewayEndpoint, Result: "idempotent_retry"}
			return nil
		}
		if token.Status != "active" || !time.Now().UTC().Before(token.ExpiresAt.Time) || !capabilitiesAllowed(token.Role, input.Capabilities) {
			return ErrEnrollmentInvalid
		}
		if token.Role == "host" || token.Role == "bastion" {
			policy := connectorEnrollmentPolicy(token.Policy)
			if policy.TargetPlatform != input.Platform+"_"+input.Architecture {
				return ErrEnrollmentInvalid
			}
		}
		connectorID := token.PreallocatedConnectorID
		if len(csr.URIs) != 1 || csr.URIs[0].String() != CertificateURI(connectorID) {
			return ErrEnrollmentInvalid
		}
		repair := token.Purpose == "pki_repair"
		if !repair {
			existing, existingErr := q.GetLiveConnectorByInstanceForUpdate(ctx, db.GetLiveConnectorByInstanceForUpdateParams{
				EnterpriseID: token.EnterpriseID, InstanceID: input.InstanceID,
			})
			if existingErr == nil && existing.ID != connectorID {
				if !subtleEqual(existing.DeviceFingerprintHash, deviceHash[:]) {
					return ErrEnrollmentConflict
				}
				if _, existingErr = q.DeleteConnectorSessionsForReplacement(ctx, db.DeleteConnectorSessionsForReplacementParams{
					ConnectorID: existing.ID, EnterpriseID: existing.EnterpriseID,
				}); existingErr != nil {
					return existingErr
				}
				if existingErr = q.RevokeConnectorCertificates(ctx, db.RevokeConnectorCertificatesParams{
					ConnectorID: existing.ID, EnterpriseID: existing.EnterpriseID,
				}); existingErr != nil {
					return existingErr
				}
				if existingErr = q.RevokePKISubjectCertificates(ctx, db.RevokePKISubjectCertificatesParams{
					SubjectKind: "connector", SubjectID: existing.ID.String(), RevocationReason: "connector_takeover",
				}); existingErr != nil {
					return existingErr
				}
				if rows, fenceErr := q.FenceConnectorForReplacement(ctx, db.FenceConnectorForReplacementParams{
					ID: existing.ID, EnterpriseID: existing.EnterpriseID,
				}); fenceErr != nil || rows != 1 {
					return ErrEnrollmentConflict
				}
				_, _ = q.RevokeRemovalConnectorCommands(ctx, db.RevokeRemovalConnectorCommandsParams{
					EnterpriseID: existing.EnterpriseID, ConnectorID: existing.ID,
				})
				_, _ = q.RevokeConnectorControlTunnelLeases(ctx, db.RevokeConnectorControlTunnelLeasesParams{
					ConnectorID: existing.ID, EnterpriseID: existing.EnterpriseID,
				})
				_, _ = q.MarkConnectorControlTunnelRemoved(ctx, db.MarkConnectorControlTunnelRemovedParams{
					ConnectorID: existing.ID, EnterpriseID: existing.EnterpriseID, LastDropReason: "connector_takeover",
				})
				_, _ = q.MarkHostConnectorOffline(ctx, db.MarkHostConnectorOfflineParams{
					EnterpriseID: existing.EnterpriseID, ConnectorID: uuid.NullUUID{UUID: existing.ID, Valid: true},
				})
				_, _ = q.MarkBastionConnectorOfflineForTakeover(ctx, db.MarkBastionConnectorOfflineForTakeoverParams{
					EnterpriseID: existing.EnterpriseID, ActiveConnectorID: uuid.NullUUID{UUID: existing.ID, Valid: true},
				})
				if existing.HostID.Valid {
					_, _ = q.TerminateRemoteAccessSessionsByHostRemoval(ctx, db.TerminateRemoteAccessSessionsByHostRemovalParams{
						EnterpriseID: existing.EnterpriseID, HostID: existing.HostID.UUID,
					})
					_, _ = q.InvalidateHostTelemetryForRemoval(ctx, db.InvalidateHostTelemetryForRemovalParams{
						EnterpriseID: existing.EnterpriseID, ResourceID: existing.HostID.UUID,
					})
				}
				takeoverConnectorID = uuid.NullUUID{UUID: existing.ID, Valid: true}
			} else if existingErr != nil && !errors.Is(existingErr, pgx.ErrNoRows) {
				return existingErr
			}
		}
		var connector db.Connector
		if repair {
			connector, err = q.GetConnector(ctx, db.GetConnectorParams{ID: connectorID, EnterpriseID: token.EnterpriseID})
			if err != nil || connector.Role != token.Role || connector.InstanceID != input.InstanceID || connector.Name != input.Name ||
				!slices.Equal(connector.Capabilities, input.Capabilities) || connector.Status == "uninstalled" || connector.Status == "revoked" {
				return ErrEnrollmentConflict
			}
		}
		certificate, err := service.Issuer.Issue(ctx, connectorID, csr, service.certificateTTL())
		if err != nil {
			return err
		}
		parsedCertificate, err := validateCertificate(certificate, connectorID, publicKeyHash)
		if err != nil {
			return err
		}
		if repair {
			if _, err = q.DeleteConnectorSessionsForReplacement(ctx, db.DeleteConnectorSessionsForReplacementParams{
				ConnectorID: connector.ID, EnterpriseID: connector.EnterpriseID,
			}); err != nil {
				return err
			}
			if err = q.RevokeConnectorCertificates(ctx, db.RevokeConnectorCertificatesParams{
				ConnectorID: connector.ID, EnterpriseID: connector.EnterpriseID,
			}); err != nil {
				return err
			}
			if err = q.RevokePKISubjectCertificates(ctx, db.RevokePKISubjectCertificatesParams{SubjectKind: "connector",
				SubjectID: connector.ID.String(), RevocationReason: "pki_repair"}); err != nil {
				return err
			}
		} else {
			connector, err = q.CreateConnector(ctx, db.CreateConnectorParams{ID: connectorID, EnterpriseID: token.EnterpriseID, Role: token.Role, Name: input.Name,
				HostID: token.PreallocatedHostID, BastionScopeID: token.BastionScopeID, KubernetesClusterID: token.KubernetesClusterID,
				InstanceID: input.InstanceID, DeviceFingerprintHash: deviceHash[:], PublicKeyHash: publicKeyHash,
				SoftwareVersion: input.SoftwareVersion, Capabilities: input.Capabilities,
				CertificateExpiresAt: pgtype.Timestamptz{Time: certificate.NotAfter.UTC(), Valid: true}})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return ErrEnrollmentConflict
				}
				return err
			}
		}
		if _, err := q.CreateConnectorCertificate(ctx, db.CreateConnectorCertificateParams{ID: newID(), ConnectorID: connector.ID, EnterpriseID: connector.EnterpriseID,
			SerialNumber: certificate.SerialNumber, IssuerGeneration: certificate.IssuerGeneration, CertificateRequestName: certificate.CertificateRequestName,
			CertificatePem: certificate.PEM, CaBundlePem: certificate.CABundlePEM, NotBefore: pgtype.Timestamptz{Time: certificate.NotBefore.UTC(), Valid: true},
			NotAfter: pgtype.Timestamptz{Time: certificate.NotAfter.UTC(), Valid: true}}); err != nil {
			return err
		}
		if err := trustbundle.RegisterCertificateIdentity(ctx, q, parsedCertificate, trustbundle.CertificateIdentity{Kind: "connector",
			SubjectID: connector.ID.String(), EnterpriseID: uuid.NullUUID{UUID: connector.EnterpriseID, Valid: true},
			Usage: x509.ExtKeyUsageClientAuth, IssuerGeneration: certificate.IssuerGeneration}); err != nil {
			return err
		}
		if repair {
			connector, err = q.CompleteConnectorCertificateRotation(ctx, db.CompleteConnectorCertificateRotationParams{ID: connector.ID,
				EnterpriseID: connector.EnterpriseID, PublicKeyHash: publicKeyHash,
				CertificateExpiresAt: pgtype.Timestamptz{Time: certificate.NotAfter.UTC(), Valid: true}})
			if err != nil {
				return err
			}
		}
		if _, err := q.ConsumeEnrollmentToken(ctx, db.ConsumeEnrollmentTokenParams{ID: token.ID, ConsumedDeviceHash: deviceHash[:], RegisteredConnectorID: uuid.NullUUID{UUID: connector.ID, Valid: true}}); err != nil {
			return ErrEnrollmentConflict
		}
		if !repair && token.Role == "host" {
			if _, err := q.ActivateHostConnector(ctx, db.ActivateHostConnectorParams{ID: token.PreallocatedHostID.UUID, EnterpriseID: token.EnterpriseID,
				ConnectorID: uuid.NullUUID{UUID: connector.ID, Valid: true}, Hostname: input.Name, Architecture: input.Architecture}); err != nil {
				return err
			}
		} else if !repair && token.Role == "bastion" {
			rows, updateErr := q.ActivateBastionRootHost(ctx, db.ActivateBastionRootHostParams{
				ID: token.PreallocatedHostID.UUID, EnterpriseID: token.EnterpriseID,
				ConnectorID: uuid.NullUUID{UUID: connector.ID, Valid: true}, Architecture: pgtype.Text{String: input.Architecture, Valid: true},
			})
			if updateErr != nil || rows != 1 {
				return ErrEnrollmentConflict
			}
			if _, err := q.ActivateBastionConnector(ctx, db.ActivateBastionConnectorParams{ID: token.BastionScopeID.UUID, EnterpriseID: token.EnterpriseID,
				ConnectorHostID: token.PreallocatedHostID, ActiveConnectorID: uuid.NullUUID{UUID: connector.ID, Valid: true}}); err != nil {
				return err
			}
		} else if !repair {
			if _, err := q.ActivateKubernetesConnector(ctx, db.ActivateKubernetesConnectorParams{ID: token.KubernetesClusterID.UUID, EnterpriseID: token.EnterpriseID,
				ConnectorID: uuid.NullUUID{UUID: connector.ID, Valid: true}}); err != nil {
				return err
			}
		}
		action, summary := "connector.enroll", "connector enrolled"
		if repair {
			action, summary = "connector.pki_repair", "connector identity repaired"
		}
		details := map[string]any{"summary": summary, "status": connector.Status}
		if takeoverConnectorID.Valid {
			details["replaced_connector_id"] = takeoverConnectorID.UUID.String()
		}
		if _, err := audit.Append(ctx, q, audit.Entry{Domain: "enterprise", EnterpriseID: uuid.NullUUID{UUID: token.EnterpriseID, Valid: true},
			ActorType: "connector", ActorID: connector.ID.String(), Action: action, ResourceType: "connector", ResourceID: connector.ID.String(),
			Result: "success", Details: details}); err != nil {
			return err
		}
		bundle, bundleErr := service.enrollmentTrustBundle(ctx, certificate.CABundlePEM)
		if bundleErr != nil {
			return bundleErr
		}
		resultName := "registered"
		if repair {
			resultName = "identity_repaired"
		}
		result = EnrollmentResult{Connector: connector, CertificatePEM: certificate.PEM, TrustBundle: bundle,
			GatewayEndpoint: service.GatewayEndpoint, Result: resultName}
		return nil
	})
	// Onboarding progress is an operational projection owned by the dispatcher.
	// Keep it outside the serializable identity transaction: its lease renewal
	// updates the same row and must never abort an otherwise valid enrollment.
	if err == nil && result.Connector.Role == "host" {
		_ = hostonboarding.AdvanceByConnector(ctx, service.Store.Queries, result.Connector.ID, result.Connector.EnterpriseID, "waiting_online")
	}
	if err == nil && takeoverConnectorID.Valid && service.Redis != nil {
		_ = service.Redis.DeleteConnectorRegistry(ctx, takeoverConnectorID.UUID.String())
	}
	return result, err
}

func (service Service) enrollmentTrustBundle(ctx context.Context, issuerBundle string) (trustbundle.Bundle, error) {
	if service.TrustBundles.Store != nil {
		return service.TrustBundles.Current(ctx)
	}
	material, err := trustbundle.Parse([]byte(issuerBundle), time.Now().UTC())
	if err != nil {
		return trustbundle.Bundle{}, err
	}
	epoch := service.TrustBundleEpoch
	if epoch < 1 {
		epoch = 1
	}
	return trustbundle.Bundle{Epoch: epoch, State: trustbundle.StateStable, Material: material,
		CurrentCAFingerprints: material.Fingerprints, StartedAt: time.Now().UTC()}, nil
}

func (service Service) OpenSession(ctx context.Context, identity TrustedIdentity, capabilities []string) (db.Connector, error) {
	var connector db.Connector
	err := service.Store.InTx(ctx, func(q *db.Queries) error {
		current, err := q.GetConnector(ctx, db.GetConnectorParams{ID: identity.ConnectorID, EnterpriseID: identity.EnterpriseID})
		if err != nil || current.Status == "revoked" || current.Status == "uninstalled" || !capabilitiesAllowed(current.Role, capabilities) {
			return ErrConnectorFenced
		}
		_, err = q.GetValidConnectorCertificateBySerial(ctx, db.GetValidConnectorCertificateBySerialParams{ConnectorID: current.ID,
			EnterpriseID: current.EnterpriseID, SerialNumber: identity.SerialNumber})
		if err != nil {
			return ErrConnectorFenced
		}
		connector, err = q.AdvanceConnectorEpoch(ctx, db.AdvanceConnectorEpochParams{ID: current.ID, EnterpriseID: current.EnterpriseID})
		if err != nil {
			return err
		}
		switch connector.Role {
		case "host":
			if _, err = q.RestoreHostConnectorOnline(ctx, db.RestoreHostConnectorOnlineParams{EnterpriseID: connector.EnterpriseID,
				ConnectorID: uuid.NullUUID{UUID: connector.ID, Valid: true}}); err != nil {
				return err
			}
		case "bastion":
			if _, err = q.RestoreBastionConnectorOnline(ctx, db.RestoreBastionConnectorOnlineParams{EnterpriseID: connector.EnterpriseID,
				ActiveConnectorID: uuid.NullUUID{UUID: connector.ID, Valid: true}}); err != nil {
				return err
			}
		case "kubernetes":
			if _, err = q.RestoreKubernetesConnectorOnline(ctx, db.RestoreKubernetesConnectorOnlineParams{EnterpriseID: connector.EnterpriseID,
				ConnectorID: uuid.NullUUID{UUID: connector.ID, Valid: true}}); err != nil {
				return err
			}
		}
		_, err = q.UpsertConnectorSession(ctx, db.UpsertConnectorSessionParams{ConnectorID: connector.ID, EnterpriseID: connector.EnterpriseID,
			GatewayInstanceID: service.GatewayInstance, ConnectionEpoch: connector.ConnectionEpoch, Capabilities: capabilities})
		return err
	})
	if err != nil {
		return db.Connector{}, err
	}
	// As with enrollment progress, completing a Host onboarding operation is a
	// projection update. Running it outside the serializable session/fencing
	// transaction prevents the dispatcher lease heartbeat from aborting the
	// Connector's otherwise valid session establishment.
	if connector.Role == "host" {
		_ = hostonboarding.CompleteByConnector(ctx, service.Store.Queries, connector.ID, connector.EnterpriseID)
	}
	if service.Redis != nil {
		_ = service.Redis.SetConnectorRegistry(ctx, connector.ID.String(), redisstore.ConnectorRegistryEntry{GatewayInstanceID: service.GatewayInstance,
			ConnectionEpoch: connector.ConnectionEpoch}, service.registryTTL())
	}
	return connector, nil
}

func (service Service) RequestCertificateRotation(ctx context.Context, enterpriseID, connectorID uuid.UUID, expectedVersion int64) (db.Connector, error) {
	if expectedVersion < 1 {
		return db.Connector{}, ErrCommandState
	}
	value, err := service.Store.Queries.RequestConnectorCertificateRotation(ctx, db.RequestConnectorCertificateRotationParams{
		ID: connectorID, EnterpriseID: enterpriseID, Version: expectedVersion,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Connector{}, resource.ErrVersionConflict
	}
	return value, err
}

func (service Service) RotateCertificate(ctx context.Context, identity TrustedIdentity, epoch int64, csrPEM []byte) (Certificate, error) {
	if service.Issuer == nil || epoch < 1 || len(csrPEM) == 0 || len(csrPEM) > MaxMessageBytes {
		return Certificate{}, ErrEnrollmentInvalid
	}
	csr, publicKeyHash, err := parseCSR(string(csrPEM))
	if err != nil || len(csr.URIs) != 1 || csr.URIs[0].String() != CertificateURI(identity.ConnectorID) {
		return Certificate{}, ErrEnrollmentInvalid
	}
	current, err := service.Store.Queries.GetConnector(ctx, db.GetConnectorParams{ID: identity.ConnectorID, EnterpriseID: identity.EnterpriseID})
	if err != nil || current.ConnectionEpoch != epoch || current.Status != "online" {
		return Certificate{}, ErrConnectorFenced
	}
	certificate, err := service.Issuer.Issue(ctx, identity.ConnectorID, csr, service.certificateTTL())
	if err != nil {
		return Certificate{}, err
	}
	parsedCertificate, err := validateCertificate(certificate, identity.ConnectorID, publicKeyHash)
	if err != nil {
		return Certificate{}, err
	}
	err = service.Store.InTx(ctx, func(q *db.Queries) error {
		current, err := q.GetConnector(ctx, db.GetConnectorParams{ID: identity.ConnectorID, EnterpriseID: identity.EnterpriseID})
		if err != nil || current.ConnectionEpoch != epoch || current.Status != "online" {
			return ErrConnectorFenced
		}
		if _, err := q.GetValidConnectorCertificateBySerial(ctx, db.GetValidConnectorCertificateBySerialParams{ConnectorID: identity.ConnectorID,
			EnterpriseID: identity.EnterpriseID, SerialNumber: identity.SerialNumber}); err != nil {
			return ErrConnectorFenced
		}
		if err := q.MarkConnectorCertificatesOverlap(ctx, db.MarkConnectorCertificatesOverlapParams{ConnectorID: identity.ConnectorID,
			EnterpriseID: identity.EnterpriseID}); err != nil {
			return err
		}
		if err := q.MarkPKISubjectCertificatesOverlap(ctx, db.MarkPKISubjectCertificatesOverlapParams{SubjectKind: "connector", SubjectID: identity.ConnectorID.String()}); err != nil {
			return err
		}
		if _, err := q.CreateConnectorCertificate(ctx, db.CreateConnectorCertificateParams{ID: newID(), ConnectorID: identity.ConnectorID,
			EnterpriseID: identity.EnterpriseID, SerialNumber: certificate.SerialNumber, IssuerGeneration: certificate.IssuerGeneration,
			CertificateRequestName: certificate.CertificateRequestName, CertificatePem: certificate.PEM, CaBundlePem: certificate.CABundlePEM,
			NotBefore: pgtype.Timestamptz{Time: certificate.NotBefore.UTC(), Valid: true}, NotAfter: pgtype.Timestamptz{Time: certificate.NotAfter.UTC(), Valid: true}}); err != nil {
			return err
		}
		if err := trustbundle.RegisterCertificateIdentity(ctx, q, parsedCertificate, trustbundle.CertificateIdentity{Kind: "connector",
			SubjectID: identity.ConnectorID.String(), EnterpriseID: uuid.NullUUID{UUID: identity.EnterpriseID, Valid: true},
			Usage: x509.ExtKeyUsageClientAuth, IssuerGeneration: certificate.IssuerGeneration}); err != nil {
			return err
		}
		_, err = q.CompleteConnectorCertificateRotation(ctx, db.CompleteConnectorCertificateRotationParams{ID: identity.ConnectorID,
			EnterpriseID: identity.EnterpriseID, PublicKeyHash: publicKeyHash,
			CertificateExpiresAt: pgtype.Timestamptz{Time: certificate.NotAfter.UTC(), Valid: true}})
		return err
	})
	return certificate, err
}

func (service Service) Heartbeat(ctx context.Context, identity TrustedIdentity, epoch int64) error {
	rows, err := service.Store.Queries.HeartbeatConnectorSession(ctx, db.HeartbeatConnectorSessionParams{ConnectorID: identity.ConnectorID,
		EnterpriseID: identity.EnterpriseID, ConnectionEpoch: epoch})
	if err != nil || rows != 1 {
		return ErrConnectorFenced
	}
	_, _ = service.Store.Queries.RestoreHostConnectorOnline(ctx, db.RestoreHostConnectorOnlineParams{EnterpriseID: identity.EnterpriseID,
		ConnectorID: uuid.NullUUID{UUID: identity.ConnectorID, Valid: true}})
	if service.Redis != nil {
		_ = service.Redis.SetConnectorRegistry(ctx, identity.ConnectorID.String(), redisstore.ConnectorRegistryEntry{GatewayInstanceID: service.GatewayInstance,
			ConnectionEpoch: epoch}, service.registryTTL())
	}
	return nil
}

func (service Service) UpdateBastionRelayStatus(ctx context.Context, identity TrustedIdentity, status *connectorv1.BastionRelayStatus) error {
	connector, err := service.Store.Queries.GetConnector(ctx, db.GetConnectorParams{ID: identity.ConnectorID, EnterpriseID: identity.EnterpriseID})
	if err != nil {
		return ErrConnectorFenced
	}
	if connector.Role != "bastion" {
		if status != nil {
			return ErrCommandState
		}
		return nil
	}
	address, httpsPort, gatewayPort, generation, err := validateBastionRelayStatus(status)
	if err != nil {
		return ErrCommandState
	}
	rows, err := service.Store.Queries.SetBastionRelayStatus(ctx, db.SetBastionRelayStatusParams{EnterpriseID: identity.EnterpriseID,
		ActiveConnectorID: uuid.NullUUID{UUID: identity.ConnectorID, Valid: true}, RelayStatus: status.GetStatus(), RelayAddress: address,
		RelayHttpsPort: httpsPort, RelayGatewayPort: gatewayPort, RelayPortGeneration: generation, RelayErrorCode: status.GetErrorCode()})
	if err != nil || rows != 1 {
		return ErrConnectorFenced
	}
	return nil
}

func validateBastionRelayStatus(status *connectorv1.BastionRelayStatus) (string, int32, int32, int64, error) {
	if status == nil || (status.GetStatus() != "ready" && status.GetStatus() != "degraded") || len(status.GetListeners()) != 2 || status.GetGeneration() > uint64(^uint64(0)>>1) {
		return "", 0, 0, 0, ErrCommandState
	}
	if (status.GetStatus() == "ready" && status.GetErrorCode() != "") || (status.GetStatus() == "degraded" && !validRelayErrorCode(status.GetErrorCode())) {
		return "", 0, 0, 0, ErrCommandState
	}
	ports := map[string]int32{}
	for _, listener := range status.GetListeners() {
		if listener == nil || (listener.GetKind() != "https" && listener.GetKind() != "gateway") || listener.GetPort() < 1 || listener.GetPort() > 65535 ||
			(listener.GetStatus() != "ready" && listener.GetStatus() != "closed" && listener.GetStatus() != "port_conflict" && listener.GetStatus() != "error") {
			return "", 0, 0, 0, ErrCommandState
		}
		if _, duplicate := ports[listener.GetKind()]; duplicate {
			return "", 0, 0, 0, ErrCommandState
		}
		ports[listener.GetKind()] = int32(listener.GetPort())
		if status.GetStatus() == "ready" && listener.GetStatus() != "ready" {
			return "", 0, 0, 0, ErrCommandState
		}
	}
	if ports["https"] == ports["gateway"] {
		return "", 0, 0, 0, ErrCommandState
	}
	address := strings.TrimSpace(status.GetAdvertiseAddress())
	generation := int64(status.GetGeneration())
	if status.GetStatus() == "ready" {
		if generation < 1 || !validBastionRelayAddress(address) {
			return "", 0, 0, 0, ErrCommandState
		}
	} else if generation == 0 {
		address = ""
	}
	return address, ports["https"], ports["gateway"], generation, nil
}

func validRelayErrorCode(value string) bool {
	if value == "" || value[0] < 'A' || value[0] > 'Z' {
		return false
	}
	for _, character := range value[1:] {
		if character != '_' && (character < 'A' || character > 'Z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func validBastionRelayAddress(value string) bool {
	if value == "" || len(value) > 253 || strings.ContainsAny(value, "/\\@?#") {
		return false
	}
	if net.ParseIP(value) != nil {
		return true
	}
	return len(validation.IsDNS1123Subdomain(strings.ToLower(value))) == 0
}

func (service Service) UpdateHostRuntimeObservation(ctx context.Context, identity TrustedIdentity, observation *connectorv1.HostRuntimeObservation) error {
	connector, err := service.Store.Queries.GetConnector(ctx, db.GetConnectorParams{ID: identity.ConnectorID, EnterpriseID: identity.EnterpriseID})
	if err != nil {
		return ErrConnectorFenced
	}
	if connector.Role == "kubernetes" {
		if observation != nil {
			return ErrCommandState
		}
		return nil
	}
	if observation == nil || observation.GetObservedAt() == nil || !observation.GetObservedAt().IsValid() ||
		(observation.GetPlatform() != "linux" && observation.GetPlatform() != "windows") ||
		(observation.GetOpensshStatus() != "available" && observation.GetOpensshStatus() != "unavailable" && observation.GetOpensshStatus() != "unknown") ||
		(observation.GetRdpStatus() != "enabled" && observation.GetRdpStatus() != "disabled" && observation.GetRdpStatus() != "unavailable" && observation.GetRdpStatus() != "unknown") ||
		time.Since(observation.GetObservedAt().AsTime()) > 5*time.Minute || observation.GetObservedAt().AsTime().After(time.Now().Add(time.Minute)) {
		return ErrCommandState
	}
	_, err = service.Store.Queries.UpsertHostRuntimeObservation(ctx, db.UpsertHostRuntimeObservationParams{ID: identity.ConnectorID,
		EnterpriseID: identity.EnterpriseID, Platform: observation.GetPlatform(), OpensshStatus: observation.GetOpensshStatus(), RdpStatus: observation.GetRdpStatus(),
		RdpNlaEnabled: observation.GetRdpNlaEnabled(), RdpFirewallEnabled: observation.GetRdpFirewallEnabled(), RdpServiceRunning: observation.GetRdpServiceRunning(),
		ObservedAt: pgtype.Timestamptz{Time: observation.GetObservedAt().AsTime(), Valid: true}})
	if err != nil {
		return ErrCommandState
	}
	return nil
}

func (service Service) Disconnect(ctx context.Context, identity TrustedIdentity, epoch int64) {
	_, _ = service.Store.Queries.CloseConnectorSession(ctx, db.CloseConnectorSessionParams{ConnectorID: identity.ConnectorID,
		EnterpriseID: identity.EnterpriseID, ConnectionEpoch: epoch})
	rows, _ := service.Store.Queries.MarkConnectorDisconnected(ctx, db.MarkConnectorDisconnectedParams{ID: identity.ConnectorID,
		EnterpriseID: identity.EnterpriseID, ConnectionEpoch: epoch})
	if rows == 1 {
		_, _ = service.Store.Queries.MarkHostConnectorOffline(ctx, db.MarkHostConnectorOfflineParams{EnterpriseID: identity.EnterpriseID,
			ConnectorID: uuid.NullUUID{UUID: identity.ConnectorID, Valid: true}})
		_, _ = service.Store.Queries.MarkBastionScopeConnectorSuspectedOffline(ctx, db.MarkBastionScopeConnectorSuspectedOfflineParams{
			EnterpriseID: identity.EnterpriseID, ActiveConnectorID: uuid.NullUUID{UUID: identity.ConnectorID, Valid: true},
		})
	}
	if service.Redis != nil {
		_ = service.Redis.DeleteConnectorRegistry(ctx, identity.ConnectorID.String())
	}
}

func (service Service) EnqueueConnectionTest(ctx context.Context, q *db.Queries, test db.ConnectionTest) error {
	if !test.ConnectorID.Valid {
		return nil
	}
	if !test.CredentialID.Valid || service.Credentials.Store == nil {
		return ErrCommandState
	}
	commandType := "host_connection_probe"
	protocol := "ssh"
	var plan struct {
		Platform string `json:"platform"`
	}
	if err := json.Unmarshal(test.RequestPlan, &plan); err != nil {
		return ErrCommandState
	}
	if test.TargetType == "kubernetes_cluster" {
		commandType = "kubernetes_connection_probe"
		protocol = "kubernetes"
	} else if test.TargetType != "host" || (plan.Platform != "linux" && plan.Platform != "windows") {
		return ErrCommandState
	}
	leaseTTL := time.Until(test.ExpiresAt.Time)
	if leaseTTL > 5*time.Minute {
		leaseTTL = 5 * time.Minute
	}
	lease, err := service.Credentials.PrepareLeaseWithQueries(ctx, q, test.CreatedBy.String(), test.EnterpriseID, secret.LeaseRequest{
		CredentialID: test.CredentialID.UUID, OperationRef: test.ID.String(), TargetResourceType: "connection_test", TargetResourceID: test.ID,
		RecipientType: "connector", RecipientID: test.ConnectorID.UUID.String(), Protocol: protocol, TTL: leaseTTL,
	})
	if err != nil {
		return err
	}
	commandID, err := randomID("cmd_")
	if err != nil {
		return err
	}
	hash := sha256.Sum256(test.RequestPlan)
	_, err = q.CreateConnectorCommand(ctx, db.CreateConnectorCommandParams{ID: newID(), CommandID: commandID, EnterpriseID: test.EnterpriseID,
		ConnectorID: test.ConnectorID.UUID, ConnectionEpoch: test.ConnectionEpoch.Int64, OperationRef: test.ID.String(),
		CredentialLeaseID: uuid.NullUUID{UUID: lease.ID, Valid: true}, CommandType: commandType,
		PayloadSchemaVersion: "argus.connector_command/v1", Payload: test.RequestPlan, PayloadHash: hash[:], IdempotencyKey: test.ID.String(),
		ExpiresAt: test.ExpiresAt})
	return err
}

func (service Service) NotifyConnectorCommand(ctx context.Context, connectorID uuid.UUID, connectionEpoch int64) {
	if service.Redis == nil || connectionEpoch < 1 {
		return
	}
	entry, err := service.Redis.GetConnectorRegistry(ctx, connectorID.String())
	if err != nil || entry.ConnectionEpoch != connectionEpoch {
		return
	}
	_ = service.Redis.PublishConnectorDispatch(ctx, entry.GatewayInstanceID, redisstore.ConnectorDispatch{
		ConnectorID: connectorID.String(), ConnectionEpoch: connectionEpoch,
	})
}

func (service Service) TransitionCommand(ctx context.Context, identity TrustedIdentity, epoch int64, commandID, next string, result json.RawMessage, errorCode string) (db.ConnectorCommand, error) {
	current, err := service.Store.Queries.GetConnectorCommand(ctx, db.GetConnectorCommandParams{CommandID: commandID, ConnectorID: identity.ConnectorID, ConnectionEpoch: epoch})
	if err != nil || len(result) > 1<<20 {
		return db.ConnectorCommand{}, ErrCommandState
	}
	if cancelledHostInstallCommand(current) {
		return current, nil
	}
	if !allowedCommandTransition(current.Status, next) {
		return db.ConnectorCommand{}, ErrCommandState
	}
	var hash []byte
	if len(result) > 0 {
		sum := sha256.Sum256(result)
		hash = sum[:]
	}
	var transitioned db.ConnectorCommand
	err = service.Store.InTx(ctx, func(q *db.Queries) error {
		if next == "dispatched" && (current.CommandType == "host_connection_probe" || current.CommandType == "kubernetes_connection_probe") {
			testID, parseErr := uuid.Parse(current.OperationRef)
			if parseErr != nil {
				return ErrCommandState
			}
			if _, markErr := q.MarkConnectorConnectionTestRunning(ctx, db.MarkConnectorConnectionTestRunningParams{
				ID: testID, EnterpriseID: identity.EnterpriseID,
			}); markErr != nil {
				return ErrCommandState
			}
		}
		var transitionErr error
		transitioned, transitionErr = q.TransitionConnectorCommand(ctx, db.TransitionConnectorCommandParams{CommandID: commandID, ConnectorID: identity.ConnectorID,
			ConnectionEpoch: epoch, ExpectedStatus: current.Status, Status: next, Result: result, ResultHash: hash, ErrorCode: pgtype.Text{String: errorCode, Valid: errorCode != ""}})
		if transitionErr != nil {
			return transitionErr
		}
		if current.CommandType == "host_connector_install" {
			operationID, parseErr := uuid.Parse(current.OperationRef)
			stage := map[string]string{"dispatched": "transferring", "acknowledged": "transferring", "running": "installing", "succeeded": "waiting_online"}[next]
			if parseErr != nil {
				return ErrCommandState
			}
			if stage != "" {
				if stageErr := hostonboarding.Advance(ctx, q, operationID, identity.EnterpriseID, stage); stageErr != nil {
					return stageErr
				}
			}
		}
		if current.CommandType == "collector_management" && (next == "succeeded" || next == "failed") {
			return finalizeCollectorManagement(ctx, q, current, next, result, errorCode)
		}
		if current.CommandType == "connector_uninstall" && next == "succeeded" {
			return finalizeConnectorUninstall(ctx, q, identity, epoch)
		}
		if current.CommandType == "host_connector_removal" && next == "succeeded" {
			operationID, parseErr := uuid.Parse(current.OperationRef)
			if parseErr != nil {
				return ErrCommandState
			}
			operation, getErr := q.GetHostRemovalOperationForUpdate(ctx, db.GetHostRemovalOperationForUpdateParams{ID: operationID, EnterpriseID: identity.EnterpriseID})
			var outcome connectorv1.HostConnectorRemovalResult
			if getErr != nil || protojson.Unmarshal(result, &outcome) != nil || outcome.GetOperationId() != operationID.String() || !outcome.GetLocalCleanupVerified() {
				return ErrCommandState
			}
			evidence, _, evidenceErr := hostremoval.ParseCleanupEvidence(outcome.GetCleanupEvidenceJson(), operation.ID, operation.ConnectorID, operation.RemovalGeneration)
			if evidenceErr != nil || outcome.GetLocalConfigDrift() != (evidence.RDPConfigStatus == "drifted") {
				return ErrCommandState
			}
			return (hostremoval.Service{Store: service.Store}).FinalizeTrustedWithQueries(ctx, q, operation, outcome.GetCleanupEvidenceJson(), outcome.GetCleanupHash())
		}
		if current.CommandType == "host_windows_rdp_configure" && next == "succeeded" {
			return finalizeWindowsRDPChangeJournal(ctx, q, current, result)
		}
		return nil
	})
	if err == nil && current.CommandType == "connector_uninstall" && next == "succeeded" && service.Redis != nil {
		_ = service.Redis.DeleteConnectorRegistry(ctx, identity.ConnectorID.String())
	}
	if err != nil {
		latest, readErr := service.Store.Queries.GetConnectorCommand(ctx, db.GetConnectorCommandParams{CommandID: commandID, ConnectorID: identity.ConnectorID, ConnectionEpoch: epoch})
		if readErr == nil && cancelledHostInstallCommand(latest) {
			return latest, nil
		}
	}
	return transitioned, err
}

func finalizeWindowsRDPChangeJournal(ctx context.Context, q *db.Queries, command db.ConnectorCommand, raw json.RawMessage) error {
	var request connectorv1.HostWindowsRDPConfigure
	var result connectorv1.HostWindowsRDPConfigureResult
	if json.Unmarshal(command.Payload, &request) != nil || protojson.Unmarshal(raw, &result) != nil || request.GetHostId() == "" ||
		result.GetHostId() != request.GetHostId() || !json.Valid(result.GetBeforeStateJson()) || !json.Valid(result.GetAppliedStateJson()) {
		return ErrCommandState
	}
	hostID, err := uuid.Parse(request.GetHostId())
	if err != nil {
		return ErrCommandState
	}
	state := append(append([]byte(nil), result.GetBeforeStateJson()...), result.GetAppliedStateJson()...)
	hash := sha256.Sum256(state)
	_, err = q.UpsertHostManagedChangeJournal(ctx, db.UpsertHostManagedChangeJournalParams{ID: uuid.New(), EnterpriseID: command.EnterpriseID,
		HostID: hostID, ConnectorID: command.ConnectorID, ChangeType: "windows_rdp", BeforeState: result.GetBeforeStateJson(),
		AppliedState: result.GetAppliedStateJson(), StateHash: hash[:]})
	return err
}

type collectorManagementRequest struct {
	CollectorID     string `json:"collector_id"`
	Operation       string `json:"operation"`
	ResourceID      string `json:"resource_id"`
	ResourceType    string `json:"resource_type"`
	DesiredRevision uint64 `json:"desired_revision"`
	ConfigSHA256    string `json:"config_sha256"`
	// Transport 为 protojson(UseProtoNames) 渲染的 route transport 字段;
	// 隧道形态的成员隧道状态随命令结果收敛(PlanV4)。
	Transport string `json:"transport"`
}

type collectorManagementResult struct {
	CollectorID         string `json:"collector_id"`
	EffectiveRevision   uint64 `json:"effective_revision"`
	AppliedConfigSHA256 string `json:"applied_config_sha256"`
	Status              string `json:"status"`
	KubernetesNodes     []telemetrybinding.NodeEvidence
}

func finalizeCollectorManagement(ctx context.Context, q *db.Queries, command db.ConnectorCommand, status string, raw json.RawMessage, errorCode string) error {
	request, result, err := validateCollectorManagementOutcome(command.Payload, status, raw)
	if err != nil {
		return ErrCommandState
	}
	collectorID, _ := uuid.Parse(request.CollectorID)
	if status == "succeeded" {
		if result.Status != "converged" && result.Status != "uninstalled" {
			return ErrCommandState
		}
		if request.Operation != "uninstall" && (result.EffectiveRevision != request.DesiredRevision ||
			!strings.EqualFold(result.AppliedConfigSHA256, request.ConfigSHA256)) {
			return ErrCommandState
		}
		if _, err = q.ApplyCollectorOperationSuccess(ctx, db.ApplyCollectorOperationSuccessParams{ID: collectorID, EnterpriseID: command.EnterpriseID, Column3: request.Operation}); err != nil {
			return err
		}
		// Tunnel health is reported by the long-lived supervisor heartbeat. A
		// successful Collector install must not fabricate an established tunnel.
		if request.Transport == "bastion_tunnel" || request.Transport == "executor_tunnel" {
			if request.Operation == "uninstall" {
				_, _ = q.MarkTelemetryTunnelRemoved(ctx, db.MarkTelemetryTunnelRemovedParams{
					CollectorID: collectorID, EnterpriseID: command.EnterpriseID, Column3: "", LastDropReason: "collector_uninstall"})
			}
		}
		if request.Operation != "uninstall" {
			if request.ResourceType == "kubernetes_cluster" {
				clusterID, parseErr := uuid.Parse(request.ResourceID)
				if parseErr != nil || telemetrybinding.Upsert(ctx, q, command.EnterpriseID, clusterID, result.KubernetesNodes) != nil {
					return ErrCommandState
				}
			}
			if _, err = q.MarkCollectorConfigEffective(ctx, db.MarkCollectorConfigEffectiveParams{CollectorID: collectorID, Revision: int64(request.DesiredRevision)}); err != nil {
				return err
			}
			ready, readyErr := collectorTelemetryTunnelReady(ctx, q, command.EnterpriseID, collectorID, request.Transport)
			if readyErr != nil {
				return readyErr
			}
			if ready {
				if _, err = q.MarkTelemetryRouteActive(ctx, db.MarkTelemetryRouteActiveParams{CollectorID: collectorID, EnterpriseID: command.EnterpriseID}); err != nil {
					return err
				}
				_, err = q.FinalizeCollectorClaimMigrations(ctx, db.FinalizeCollectorClaimMigrationsParams{EnterpriseID: command.EnterpriseID, CollectorID: collectorID})
			}
		}
		return err
	}
	if errorCode == "" {
		errorCode = "COLLECTOR_MANAGEMENT_FAILED"
	}
	if _, err = q.ApplyCollectorOperationFailure(ctx, db.ApplyCollectorOperationFailureParams{ID: collectorID, EnterpriseID: command.EnterpriseID, Status: "degraded"}); err != nil {
		return err
	}
	if request.Operation != "uninstall" {
		_, _ = q.MarkCollectorConfigFailed(ctx, db.MarkCollectorConfigFailedParams{CollectorID: collectorID, Revision: int64(request.DesiredRevision), FailureCode: pgtype.Text{String: errorCode, Valid: true}})
	}
	_, _ = q.RollbackCollectorClaimMigrations(ctx, db.RollbackCollectorClaimMigrationsParams{EnterpriseID: command.EnterpriseID, CollectorID: collectorID})
	_, err = q.MarkTelemetryRouteDegraded(ctx, db.MarkTelemetryRouteDegradedParams{CollectorID: collectorID, EnterpriseID: command.EnterpriseID})
	return err
}

func collectorTelemetryTunnelReady(ctx context.Context, q *db.Queries, enterpriseID, collectorID uuid.UUID, transport string) (bool, error) {
	if transport != "executor_tunnel" && transport != "bastion_tunnel" {
		return true, nil
	}
	tunnel, err := q.GetTelemetryTunnelByCollector(ctx, collectorID)
	if err != nil || tunnel.EnterpriseID != enterpriseID || tunnel.Transport != transport || tunnel.Status == "removed" {
		return false, ErrCommandState
	}
	return tunnel.Status == "established", nil
}

func validateCollectorManagementOutcome(payload json.RawMessage, status string, raw json.RawMessage) (collectorManagementRequest, collectorManagementResult, error) {
	var requestMessage connectorv1.CollectorManagementCommand
	if protojson.Unmarshal(payload, &requestMessage) != nil {
		return collectorManagementRequest{}, collectorManagementResult{}, ErrCommandState
	}
	request := collectorManagementRequest{CollectorID: requestMessage.GetCollectorId(), Operation: requestMessage.GetOperation(),
		ResourceID: requestMessage.GetResourceId(), ResourceType: requestMessage.GetResourceType(),
		DesiredRevision: requestMessage.GetDesiredRevision(), ConfigSHA256: requestMessage.GetConfigSha256(),
		Transport: requestMessage.GetTransport()}
	var result collectorManagementResult
	if request.CollectorID == "" {
		return request, result, ErrCommandState
	}
	if _, err := uuid.Parse(request.CollectorID); err != nil {
		return request, result, ErrCommandState
	}
	if status != "succeeded" {
		return request, result, nil
	}
	var resultMessage connectorv1.CollectorManagementResult
	if protojson.Unmarshal(raw, &resultMessage) != nil {
		return request, result, ErrCommandState
	}
	result = collectorManagementResult{CollectorID: resultMessage.GetCollectorId(), EffectiveRevision: resultMessage.GetEffectiveRevision(),
		AppliedConfigSHA256: resultMessage.GetAppliedConfigSha256(), Status: resultMessage.GetStatus()}
	for _, node := range resultMessage.GetKubernetesNodes() {
		result.KubernetesNodes = append(result.KubernetesNodes, telemetrybinding.NodeEvidence{NodeUID: node.GetNodeUid(), NodeName: node.GetNodeName(),
			ProviderID: node.GetProviderId(), MachineID: node.GetMachineId(), SystemUUID: node.GetSystemUuid(), InternalIPs: append([]string(nil), node.GetInternalIps()...)})
	}
	if result.CollectorID != request.CollectorID {
		return request, result, ErrCommandState
	}
	if request.ResourceType == "kubernetes_cluster" && request.Operation != "uninstall" {
		if _, err := uuid.Parse(request.ResourceID); err != nil || telemetrybinding.Validate(result.KubernetesNodes) != nil {
			return request, result, ErrCommandState
		}
	} else if len(result.KubernetesNodes) != 0 {
		return request, result, ErrCommandState
	}
	return request, result, nil
}

func finalizeConnectorUninstall(ctx context.Context, q *db.Queries, identity TrustedIdentity, epoch int64) error {
	connector, err := q.FinalizeConnectorUninstall(ctx, db.FinalizeConnectorUninstallParams{ID: identity.ConnectorID, EnterpriseID: identity.EnterpriseID})
	if err != nil {
		return ErrCommandState
	}
	if err := q.RevokeConnectorCertificates(ctx, db.RevokeConnectorCertificatesParams{ConnectorID: connector.ID, EnterpriseID: connector.EnterpriseID}); err != nil {
		return err
	}
	if err := q.RevokePKISubjectCertificates(ctx, db.RevokePKISubjectCertificatesParams{SubjectKind: "connector",
		SubjectID: connector.ID.String(), RevocationReason: "connector_uninstalled"}); err != nil {
		return err
	}
	_, _ = q.RevokeConnectorControlTunnelLeases(ctx, db.RevokeConnectorControlTunnelLeasesParams{
		ConnectorID: connector.ID, EnterpriseID: connector.EnterpriseID})
	_, _ = q.MarkConnectorControlTunnelRemoved(ctx, db.MarkConnectorControlTunnelRemovedParams{
		ConnectorID: connector.ID, EnterpriseID: connector.EnterpriseID, LastDropReason: "connector_uninstalled"})
	_, _ = q.CloseConnectorSession(ctx, db.CloseConnectorSessionParams{ConnectorID: connector.ID, EnterpriseID: connector.EnterpriseID, ConnectionEpoch: epoch})
	if connector.Role != "kubernetes" {
		return ErrCommandState
	}
	rows, err := q.FinalizeKubernetesConnectorUninstall(ctx, db.FinalizeKubernetesConnectorUninstallParams{EnterpriseID: connector.EnterpriseID,
		ConnectorID: uuid.NullUUID{UUID: connector.ID, Valid: true}})
	if err != nil || rows != 1 {
		return ErrCommandState
	}
	return nil
}

func (service Service) ListCommands(ctx context.Context, identity TrustedIdentity, epoch int64, limit int32) ([]db.ConnectorCommand, error) {
	if limit < 1 || limit > 32 {
		limit = 16
	}
	return service.Store.Queries.ListDispatchableConnectorCommands(ctx, db.ListDispatchableConnectorCommandsParams{ConnectorID: identity.ConnectorID,
		ConnectionEpoch: epoch, Limit: limit})
}

func (service Service) ListUncertainCommands(ctx context.Context, identity TrustedIdentity, limit int32) ([]db.ConnectorCommand, error) {
	if limit < 1 || limit > 128 {
		limit = 64
	}
	return service.Store.Queries.ListUncertainConnectorCommands(ctx, db.ListUncertainConnectorCommandsParams{ConnectorID: identity.ConnectorID, Limit: limit})
}

func (service Service) SweepCommandTimeouts(ctx context.Context) error {
	return service.Store.InTx(ctx, func(q *db.Queries) error {
		expired, err := q.ExpireQueuedConnectorCommands(ctx)
		if err != nil {
			return err
		}
		timedOut, err := q.TimeoutActiveConnectorCommands(ctx)
		if err != nil {
			return err
		}
		for _, command := range append(expired, timedOut...) {
			if command.CredentialLeaseID.Valid {
				if err := q.RevokeCredentialLease(ctx, db.RevokeCredentialLeaseParams{ID: command.CredentialLeaseID.UUID, EnterpriseID: command.EnterpriseID}); err != nil {
					return err
				}
			}
			if command.CommandType != "host_connection_probe" && command.CommandType != "kubernetes_connection_probe" {
				continue
			}
			testID, err := uuid.Parse(command.OperationRef)
			if err != nil {
				continue
			}
			status := "expired"
			errorCode := "CONNECTOR_COMMAND_EXPIRED"
			if command.Status == "timed_out" {
				status, errorCode = "failed", "CONNECTOR_COMMAND_TIMED_OUT"
			}
			_, _ = q.CompleteConnectionTest(ctx, db.CompleteConnectionTestParams{ID: testID, EnterpriseID: command.EnterpriseID,
				Status: status, Result: []byte("{}"), ErrorCode: pgtype.Text{String: errorCode, Valid: true}})
		}
		return nil
	})
}

func (service Service) certificateTTL() time.Duration {
	if service.CertificateTTL <= 0 || service.CertificateTTL > 24*time.Hour {
		return 24 * time.Hour
	}
	return service.CertificateTTL
}

func (service Service) registryTTL() time.Duration {
	if service.RegistryTTL <= 0 {
		return 95 * time.Second
	}
	return service.RegistryTTL
}

func parseCSR(value string) (*x509.CertificateRequest, []byte, error) {
	block, rest := pem.Decode([]byte(value))
	if block == nil || block.Type != "CERTIFICATE REQUEST" || len(strings.TrimSpace(string(rest))) != 0 {
		return nil, nil, ErrEnrollmentInvalid
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil || csr.CheckSignature() != nil || len(csr.DNSNames) != 0 || len(csr.IPAddresses) != 0 || len(csr.URIs) != 1 {
		return nil, nil, ErrEnrollmentInvalid
	}
	publicKey, err := x509.MarshalPKIXPublicKey(csr.PublicKey)
	if err != nil {
		return nil, nil, ErrEnrollmentInvalid
	}
	hash := sha256.Sum256(publicKey)
	return csr, hash[:], nil
}

func validateCertificate(value Certificate, connectorID uuid.UUID, publicKeyHash []byte) (*x509.Certificate, error) {
	block, rest := pem.Decode([]byte(value.PEM))
	if block == nil || block.Type != "CERTIFICATE" || value.NotAfter.Sub(value.NotBefore) > 24*time.Hour+time.Minute {
		return nil, ErrEnrollmentInvalid
	}
	if len(strings.TrimSpace(string(rest))) != 0 {
		return nil, ErrEnrollmentInvalid
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil || len(certificate.URIs) != 1 || certificate.URIs[0].String() != "spiffe://argus.io/connector/"+connectorID.String() ||
		len(certificate.DNSNames) != 0 || len(certificate.IPAddresses) != 0 || len(certificate.ExtKeyUsage) != 1 ||
		certificate.ExtKeyUsage[0] != x509.ExtKeyUsageClientAuth {
		return nil, ErrEnrollmentInvalid
	}
	publicKey, err := x509.MarshalPKIXPublicKey(certificate.PublicKey)
	if err != nil {
		return nil, ErrEnrollmentInvalid
	}
	hash := sha256.Sum256(publicKey)
	if !subtleEqual(hash[:], publicKeyHash) || certificate.SerialNumber.String() != value.SerialNumber {
		return nil, ErrEnrollmentInvalid
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(value.CABundlePEM)) {
		return nil, ErrEnrollmentInvalid
	}
	if _, err = certificate.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		return nil, ErrEnrollmentInvalid
	}
	return certificate, nil
}

func capabilitiesAllowed(role string, values []string) bool {
	allowed := map[string]map[string]bool{
		"host":       {"host.local_command": true, "host.local_shell": true, "host.collector_manage": true, "host.tcp_tunnel": true, "host.windows_rdp_manage": true, "connector.uninstall": true},
		"bastion":    {"host.connection_probe": true, "host.ssh_install": true, "bastion.tls_relay": true, "kubernetes.connection_probe": true, "kubernetes.query": true, "credential.lease": true, "connector.uninstall": true},
		"kubernetes": {"kubernetes.connection_probe": true, "kubernetes.query": true, "credential.lease": true, "connector.uninstall": true},
	}
	seen := map[string]bool{}
	for _, value := range values {
		if !allowed[role][value] || seen[value] {
			return false
		}
		seen[value] = true
	}
	return len(values) > 0
}

func allowedCommandTransition(current, next string) bool {
	allowed := map[string]map[string]bool{
		"queued":           {"dispatched": true, "expired": true, "delivery_unknown": true},
		"dispatched":       {"acknowledged": true, "delivery_unknown": true, "timed_out": true},
		"acknowledged":     {"running": true, "failed": true, "timed_out": true, "result_unknown": true},
		"running":          {"succeeded": true, "failed": true, "timed_out": true, "result_unknown": true},
		"delivery_unknown": {"acknowledged": true, "running": true, "succeeded": true, "failed": true, "result_unknown": true},
		"result_unknown":   {"succeeded": true, "failed": true},
	}
	return allowed[current][next]
}

func randomID(prefix string) (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return prefix + strings.ReplaceAll(id.String(), "-", ""), nil
}

func newID() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return id
}

func subtleEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}

func CertificateURI(connectorID uuid.UUID) string {
	return fmt.Sprintf("spiffe://argus.io/connector/%s", connectorID)
}

var _ resource.CommandEnqueuer = Service{}
var _ resource.KubernetesEnrollmentCreator = Service{}
