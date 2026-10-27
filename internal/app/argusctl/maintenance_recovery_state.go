package argusctl

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/kakj-go/Argus/internal/trustbundle"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

const backupRecoveryFile = "platform-recovery.json"

var recoveryRuntimeKeys = []string{"idempotency-encryption-key", "cursor-signing-key", "pending-action-encryption-key"}

// Private material is serialized only inside the authenticated encrypted backup.
// Customer ClusterIssuer private keys are never read or exported.
type backupRecoveryState struct {
	Schema           string            `json:"schema_version"`
	SourceReleaseID  string            `json:"source_release_id"`
	Mode             PKIMode           `json:"pki_mode"`
	BundleEpoch      int64             `json:"bundle_epoch"`
	IssuerGeneration int64             `json:"issuer_generation"`
	BundlePEM        string            `json:"bundle_pem"`
	RootCertificate  []byte            `json:"managed_root_certificate,omitempty"`
	RootPrivateKey   []byte            `json:"managed_root_private_key,omitempty"`
	RuntimeKeys      map[string]string `json:"runtime_keys"`
}

func validateBackupRecoveryState(state *backupRecoveryState) error {
	invalid := errors.New("backup platform recovery material is invalid")
	if state == nil || state.Schema != "argus.platform_recovery/v1" || len(validation.IsDNS1123Label(state.SourceReleaseID)) != 0 || state.BundleEpoch < 1 || state.IssuerGeneration < 1 || len(state.RuntimeKeys) != len(recoveryRuntimeKeys) {
		return invalid
	}
	material, err := trustbundle.Parse([]byte(state.BundlePEM), time.Now().UTC())
	if err != nil {
		return invalid
	}
	for _, key := range recoveryRuntimeKeys {
		value, err := base64.RawURLEncoding.DecodeString(state.RuntimeKeys[key])
		if err != nil || len(value) != 32 {
			return invalid
		}
	}
	switch state.Mode {
	case PKIModeManaged:
		secret := recoveryManagedRoot(state, state.SourceReleaseID)
		if validateManagedRootCA(secret, state.SourceReleaseID) != nil {
			return invalid
		}
		root, err := trustbundle.Parse(state.RootCertificate, time.Now().UTC())
		if err != nil || len(root.Certificates) != 1 || root.SHA256 != material.SHA256 {
			return invalid
		}
	case PKIModeExistingClusterIssuer:
		if len(state.RootCertificate) != 0 || len(state.RootPrivateKey) != 0 {
			return invalid
		}
	default:
		return invalid
	}
	return nil
}

func readBackupRecoveryState(stage string) (*backupRecoveryState, error) {
	raw, err := os.ReadFile(filepath.Join(stage, backupRecoveryFile))
	if err != nil || len(raw) == 0 || len(raw) > 256<<10 {
		return nil, errors.New("backup platform recovery material is missing or oversized")
	}
	var state backupRecoveryState
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&state) != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("backup platform recovery material is invalid")
	}
	if err := validateBackupRecoveryState(&state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (a *App) captureBackupRecoveryState(ctx context.Context, cfg *InstallConfig) ([]byte, error) {
	session, err := a.openPKISession(ctx, cfg)
	if err != nil {
		return nil, errors.New("read backup PKI database state failed; private connection details suppressed")
	}
	defer session.Close()
	record, err := session.store.Queries.GetCurrentTrustBundle(ctx)
	if err != nil || record.State != trustbundle.StateStable || len(record.NextCaFingerprints) != 0 {
		return nil, errors.New("backup requires a stable completed PKI rotation")
	}
	material, err := trustbundle.Parse([]byte(record.BundlePem), time.Now().UTC())
	if err != nil || material.SHA256 != record.BundleSha256 || !sameRecoveryFingerprints(material.Fingerprints, record.CurrentCaFingerprints) {
		return nil, errors.New("backup database Trust Bundle is inconsistent")
	}
	source, err := session.clients.typed.CoreV1().ConfigMaps("cert-manager").Get(ctx, cfg.trustSourceName(), metav1.GetOptions{})
	if err != nil || source.Labels["argus.io/release-id"] != cfg.Spec.ReleaseID || source.Annotations["argus.io/trust-bundle-sha256"] != material.SHA256 || source.Annotations["argus.io/trust-bundle-epoch"] != strconv.FormatInt(record.Epoch, 10) {
		return nil, errors.New("backup public trust source has not converged")
	}
	public, err := trustbundle.Parse([]byte(source.Data["ca.crt"]), time.Now().UTC())
	if err != nil || public.SHA256 != material.SHA256 {
		return nil, errors.New("backup public trust source differs from database")
	}
	runtimeState, err := backupRuntimePKIState(ctx, session.clients, cfg, record.Epoch)
	if err != nil {
		return nil, err
	}
	state := backupRecoveryState{Schema: "argus.platform_recovery/v1", SourceReleaseID: cfg.Spec.ReleaseID, Mode: cfg.Spec.PKI.Mode, BundleEpoch: record.Epoch, IssuerGeneration: runtimeState.IssuerGeneration, BundlePEM: string(material.PEM), RuntimeKeys: map[string]string{}}
	if state.Mode == PKIModeManaged {
		root, err := session.clients.typed.CoreV1().Secrets("cert-manager").Get(ctx, cfg.Spec.ReleaseID+"-root-ca", metav1.GetOptions{})
		if err != nil || validateManagedRootCA(root, cfg.Spec.ReleaseID) != nil || root.Annotations["argus.io/pki-epoch"] != strconv.FormatInt(state.IssuerGeneration, 10) {
			return nil, errors.New("backup managed issuer is unavailable or inconsistent")
		}
		state.RootCertificate, state.RootPrivateKey = root.Data[corev1.TLSCertKey], root.Data[corev1.TLSPrivateKeyKey]
	} else {
		configured, err := cfg.CABundlePEM()
		if err != nil {
			return nil, errors.New("backup customer trust configuration is unavailable")
		}
		configuredMaterial, err := trustbundle.Parse(configured, time.Now().UTC())
		if err != nil || configuredMaterial.SHA256 != material.SHA256 {
			return nil, errors.New("backup customer issuer trust differs from the stable Bundle")
		}
	}
	secrets, err := session.clients.typed.CoreV1().Secrets(cfg.Spec.Namespaces.System).Get(ctx, cfg.Spec.ReleaseID+"-generated-secrets", metav1.GetOptions{})
	if err != nil || secrets.Labels["app.kubernetes.io/part-of"] != "argus" {
		return nil, errors.New("backup runtime encryption keys are unavailable")
	}
	for _, key := range recoveryRuntimeKeys {
		state.RuntimeKeys[key] = string(secrets.Data[key])
	}
	if err = validateBackupRecoveryState(&state); err != nil {
		return nil, err
	}
	return json.Marshal(&state)
}

func backupRuntimePKIState(ctx context.Context, clients *kubeClients, cfg *InstallConfig, epoch int64) (runtimePKIState, error) {
	config, err := clients.typed.CoreV1().ConfigMaps(cfg.Spec.Namespaces.System).Get(ctx, "argus-runtime-config", metav1.GetOptions{})
	if err != nil {
		return runtimePKIState{}, errors.New("backup runtime PKI state is unavailable")
	}
	initialEpoch, epochErr := strconv.ParseInt(config.Data["ARGUS_TRUST_BUNDLE_EPOCH"], 10, 64)
	generation, generationErr := strconv.ParseInt(config.Data["ARGUS_CONNECTOR_ISSUER_GENERATION"], 10, 64)
	if epochErr != nil || initialEpoch < 1 || initialEpoch > epoch || generationErr != nil || generation < 1 || config.Data["ARGUS_TELEMETRY_ISSUER_GENERATION"] != strconv.FormatInt(generation, 10) {
		return runtimePKIState{}, errors.New("backup runtime PKI generations are inconsistent")
	}
	// Runtime's initial epoch can legitimately predate a completed rotation.
	// Reuse the installer's authoritative trust-source/root-generation logic.
	state, err := preserveRuntimePKIState(ctx, clients, cfg, map[string]any{"runtime": map[string]any{}})
	if err != nil || state.TrustBundleEpoch != epoch {
		return runtimePKIState{}, errors.New("backup current PKI state differs from the database")
	}
	return state, nil
}

func sameRecoveryFingerprints(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}
