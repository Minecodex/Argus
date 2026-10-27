package argusctl

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/kakj-go/Argus/internal/trustbundle"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func recoveryManagedRoot(state *backupRecoveryState, targetRelease string) *corev1.Secret {
	immutable := true
	material, _ := trustbundle.Parse(state.RootCertificate, time.Now().UTC())
	fingerprint := ""
	if len(material.Fingerprints) == 1 {
		fingerprint = material.Fingerprints[0]
	}
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: targetRelease + "-root-ca", Namespace: "cert-manager",
		Labels:      map[string]string{"app.kubernetes.io/part-of": "argus", "argus.io/release-id": targetRelease, "argus.io/pki-role": "managed-root"},
		Annotations: map[string]string{"argus.io/ca-sha256": fingerprint, "argus.io/pki-epoch": strconv.FormatInt(state.IssuerGeneration, 10)}},
		Type: corev1.SecretTypeTLS, Immutable: &immutable,
		Data: map[string][]byte{corev1.TLSCertKey: bytes.Clone(state.RootCertificate), corev1.TLSPrivateKeyKey: bytes.Clone(state.RootPrivateKey), "ca.crt": bytes.Clone(state.RootCertificate)}}
}

// Run after the new namespaces and cert-manager exist, before the installer
// creates an issuer or any service leaf certificates. Conflicting material is
// never replaced, including a conflicting root owned by the same release.
func installBackupRecoveryState(ctx context.Context, clients *kubeClients, cfg *InstallConfig, state *backupRecoveryState) error {
	if err := validateBackupRecoveryState(state); err != nil {
		return err
	}
	if cfg.Spec.PKI.Mode != state.Mode || cfg.Spec.ReleaseID == state.SourceReleaseID {
		return errors.New("restore requires a distinct target and the original PKI mode")
	}
	namespace, err := clients.typed.CoreV1().Namespaces().Get(ctx, cfg.Spec.Namespaces.System, metav1.GetOptions{})
	if err != nil || namespace.Labels["argus.io/release-id"] != cfg.Spec.ReleaseID {
		return errors.New("restore target namespace ownership is not established")
	}
	material, err := trustbundle.Parse([]byte(state.BundlePEM), time.Now().UTC())
	if err != nil {
		return errors.New("restore public trust material is invalid")
	}
	var root *corev1.Secret
	createRoot := false
	if state.Mode == PKIModeManaged {
		root = recoveryManagedRoot(state, cfg.Spec.ReleaseID)
		existing, err := clients.typed.CoreV1().Secrets("cert-manager").Get(ctx, root.Name, metav1.GetOptions{})
		switch {
		case apierrors.IsNotFound(err):
			createRoot = true
		case err != nil:
			return errors.New("inspect restore root identity failed")
		default:
			if validateManagedRootCA(existing, cfg.Spec.ReleaseID) != nil || !bytes.Equal(existing.Data[corev1.TLSCertKey], root.Data[corev1.TLSCertKey]) || !bytes.Equal(existing.Data[corev1.TLSPrivateKeyKey], root.Data[corev1.TLSPrivateKeyKey]) || existing.Annotations["argus.io/pki-epoch"] != root.Annotations["argus.io/pki-epoch"] {
				return errors.New("restore target already has a different managed root")
			}
		}
	} else {
		configured, err := cfg.CABundlePEM()
		if err != nil {
			return errors.New("restore customer issuer trust is unavailable")
		}
		target, err := trustbundle.Parse(configured, time.Now().UTC())
		if err != nil || target.SHA256 != material.SHA256 {
			return errors.New("restore customer issuer must retain the original trust anchors")
		}
	}
	trust := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: cfg.trustSourceName(), Namespace: "cert-manager",
		Labels:      map[string]string{"app.kubernetes.io/part-of": "argus", "argus.io/release-id": cfg.Spec.ReleaseID},
		Annotations: map[string]string{"argus.io/trust-bundle-epoch": strconv.FormatInt(state.BundleEpoch, 10), "argus.io/trust-bundle-sha256": material.SHA256}}, Data: map[string]string{"ca.crt": string(material.PEM)}}
	existingTrust, err := clients.typed.CoreV1().ConfigMaps("cert-manager").Get(ctx, trust.Name, metav1.GetOptions{})
	createTrust := apierrors.IsNotFound(err)
	if err != nil && !createTrust {
		return errors.New("inspect restore public trust source failed")
	}
	if !createTrust && (existingTrust.Labels["argus.io/release-id"] != cfg.Spec.ReleaseID || existingTrust.Data["ca.crt"] != trust.Data["ca.crt"] || existingTrust.Annotations["argus.io/trust-bundle-epoch"] != trust.Annotations["argus.io/trust-bundle-epoch"] || existingTrust.Annotations["argus.io/trust-bundle-sha256"] != material.SHA256) {
		return errors.New("restore target already has a different trust source")
	}
	keys := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: cfg.Spec.ReleaseID + "-generated-secrets", Namespace: cfg.Spec.Namespaces.System,
		Labels: map[string]string{"app.kubernetes.io/part-of": "argus", "argus.io/release-id": cfg.Spec.ReleaseID}}, Type: corev1.SecretTypeOpaque, Data: map[string][]byte{}}
	for _, name := range recoveryRuntimeKeys {
		keys.Data[name] = []byte(state.RuntimeKeys[name])
	}
	existingKeys, err := clients.typed.CoreV1().Secrets(cfg.Spec.Namespaces.System).Get(ctx, keys.Name, metav1.GetOptions{})
	createKeys := apierrors.IsNotFound(err)
	if err != nil && !createKeys {
		return errors.New("inspect restore runtime keys failed")
	}
	if !createKeys {
		if existingKeys.Labels["argus.io/release-id"] != cfg.Spec.ReleaseID || existingKeys.Type != corev1.SecretTypeOpaque {
			return errors.New("restore target runtime key ownership differs")
		}
		for _, name := range recoveryRuntimeKeys {
			if !bytes.Equal(existingKeys.Data[name], keys.Data[name]) {
				return errors.New("restore target already has different runtime encryption keys")
			}
		}
	}
	// All pre-existing objects have been checked before creating any material.
	if createRoot {
		if _, err := clients.typed.CoreV1().Secrets("cert-manager").Create(ctx, root, metav1.CreateOptions{}); err != nil {
			return errors.New("create restored managed root failed; private material suppressed")
		}
	}
	if createTrust {
		if _, err := clients.typed.CoreV1().ConfigMaps("cert-manager").Create(ctx, trust, metav1.CreateOptions{}); err != nil {
			return errors.New("create restored public trust source failed")
		}
	}
	if createKeys {
		if _, err := clients.typed.CoreV1().Secrets(cfg.Spec.Namespaces.System).Create(ctx, keys, metav1.CreateOptions{}); err != nil {
			return errors.New("create restored runtime encryption keys failed; private material suppressed")
		}
	}
	return nil
}

func restoredRuntimePKIState(state *backupRecoveryState, values map[string]any) runtimePKIState {
	result := runtimePKIState{TrustBundleEpoch: state.BundleEpoch, IssuerGeneration: state.IssuerGeneration, ConnectorIssuerGeneration: state.IssuerGeneration, TelemetryIssuerGeneration: state.IssuerGeneration}
	runtime := values["runtime"].(map[string]any)
	runtime["trustBundleEpoch"], runtime["connectorIssuerGeneration"], runtime["telemetryIssuerGeneration"] = result.TrustBundleEpoch, result.IssuerGeneration, result.IssuerGeneration
	return result
}
