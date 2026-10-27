package argusctl

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kakj-go/Argus/internal/trustbundle"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func testBackupRecoveryState(t *testing.T) *backupRecoveryState {
	t.Helper()
	root, err := newManagedRootCASecret("backup-source", "backup-source-root-ca", 2, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	material, err := trustbundle.Parse(root.Data[corev1.TLSCertKey], time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	state := &backupRecoveryState{Schema: "argus.platform_recovery/v1", SourceReleaseID: "backup-source", Mode: PKIModeManaged,
		BundleEpoch: 3, IssuerGeneration: 2, BundlePEM: string(material.PEM), RootCertificate: root.Data[corev1.TLSCertKey], RootPrivateKey: root.Data[corev1.TLSPrivateKeyKey], RuntimeKeys: map[string]string{}}
	for i, key := range recoveryRuntimeKeys {
		state.RuntimeKeys[key] = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{byte(i + 1)}, 32))
	}
	return state
}

func testBackupRecoveryJSON(t *testing.T) []byte {
	t.Helper()
	raw, err := json.Marshal(testBackupRecoveryState(t))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func recoveryTestTarget(t *testing.T) (*InstallConfig, *fake.Clientset) {
	t.Helper()
	root, err := findRepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(filepath.Join(root, "deploy", "profiles", "local-hardening.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Spec.ReleaseID = "restore-target"
	cfg.Spec.Namespaces.System = "restore-target-system"
	client := fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: cfg.Spec.Namespaces.System, Labels: map[string]string{"argus.io/release-id": cfg.Spec.ReleaseID}}})
	return cfg, client
}

func TestRestoreSeedsOriginalManagedTrustAndRuntimeKeysBeforeLeaves(t *testing.T) {
	cfg, client := recoveryTestTarget(t)
	clients := &kubeClients{typed: client}
	state := testBackupRecoveryState(t)
	if err := installBackupRecoveryState(t.Context(), clients, cfg, state); err != nil {
		t.Fatal(err)
	}
	// The fake client does not assign API-server resource versions on creation.
	createdSource, err := client.CoreV1().ConfigMaps("cert-manager").Get(t.Context(), cfg.trustSourceName(), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	createdSource.ResourceVersion = "1"
	if _, err = client.CoreV1().ConfigMaps("cert-manager").Update(t.Context(), createdSource, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := ensurePKITrustSource(t.Context(), clients, cfg); err != nil {
		t.Fatal(err)
	}
	root, err := client.CoreV1().Secrets("cert-manager").Get(t.Context(), cfg.Spec.ReleaseID+"-root-ca", metav1.GetOptions{})
	if err != nil || validateManagedRootCA(root, cfg.Spec.ReleaseID) != nil || !bytes.Equal(root.Data[corev1.TLSCertKey], state.RootCertificate) || !bytes.Equal(root.Data[corev1.TLSPrivateKeyKey], state.RootPrivateKey) {
		t.Fatal("restore regenerated or lost the original managed root")
	}
	public, err := client.CoreV1().ConfigMaps("cert-manager").Get(t.Context(), cfg.trustSourceName(), metav1.GetOptions{})
	if err != nil || public.Annotations["argus.io/trust-bundle-epoch"] != "3" || public.Data["ca.crt"] != state.BundlePEM || strings.Contains(public.Data["ca.crt"], "PRIVATE KEY") {
		t.Fatal("public trust source lost its epoch or exposed private material")
	}
	values := map[string]any{"runtime": map[string]any{}}
	preserved, err := preserveRuntimePKIState(t.Context(), clients, cfg, values)
	if err != nil || preserved.TrustBundleEpoch != 3 || preserved.IssuerGeneration != 2 {
		t.Fatal("restored epochs were reset by normal installation")
	}
	keys, err := client.CoreV1().Secrets(cfg.Spec.Namespaces.System).Get(t.Context(), cfg.Spec.ReleaseID+"-generated-secrets", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range recoveryRuntimeKeys {
		if string(keys.Data[key]) != state.RuntimeKeys[key] {
			t.Fatal("runtime encryption material changed")
		}
	}
	keys.Data["setup-token"] = []byte("target-only")
	if _, err = client.CoreV1().Secrets(keys.Namespace).Update(t.Context(), keys, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	client.ClearActions()
	if err = installBackupRecoveryState(t.Context(), clients, cfg, state); err != nil {
		t.Fatal(err)
	}
	assertRecoveryDidNotWrite(t, client)
}

func TestRestoreRejectsConflictingMaterialBeforeAnyWrite(t *testing.T) {
	for _, conflict := range []string{"root", "trust", "runtime", "namespace"} {
		t.Run(conflict, func(t *testing.T) {
			cfg, client := recoveryTestTarget(t)
			state := testBackupRecoveryState(t)
			switch conflict {
			case "root":
				other, err := newManagedRootCASecret(cfg.Spec.ReleaseID, cfg.Spec.ReleaseID+"-root-ca", 2, time.Now().UTC())
				if err != nil {
					t.Fatal(err)
				}
				if _, err = client.CoreV1().Secrets("cert-manager").Create(t.Context(), other, metav1.CreateOptions{}); err != nil {
					t.Fatal(err)
				}
			case "trust":
				_, err := client.CoreV1().ConfigMaps("cert-manager").Create(t.Context(), &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: cfg.trustSourceName()}, Data: map[string]string{"ca.crt": "different"}}, metav1.CreateOptions{})
				if err != nil {
					t.Fatal(err)
				}
			case "runtime":
				_, err := client.CoreV1().Secrets(cfg.Spec.Namespaces.System).Create(t.Context(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: cfg.Spec.ReleaseID + "-generated-secrets", Labels: map[string]string{"argus.io/release-id": cfg.Spec.ReleaseID}}, Type: corev1.SecretTypeOpaque, Data: map[string][]byte{recoveryRuntimeKeys[0]: []byte("different")}}, metav1.CreateOptions{})
				if err != nil {
					t.Fatal(err)
				}
			case "namespace":
				namespace, _ := client.CoreV1().Namespaces().Get(t.Context(), cfg.Spec.Namespaces.System, metav1.GetOptions{})
				namespace.Labels["argus.io/release-id"] = "foreign"
				if _, err := client.CoreV1().Namespaces().Update(t.Context(), namespace, metav1.UpdateOptions{}); err != nil {
					t.Fatal(err)
				}
			}
			client.ClearActions()
			if installBackupRecoveryState(t.Context(), &kubeClients{typed: client}, cfg, state) == nil {
				t.Fatal("conflicting restore material was accepted")
			}
			assertRecoveryDidNotWrite(t, client)
		})
	}
}

func TestRestoreCustomerIssuerNeverCopiesPrivateCAKeys(t *testing.T) {
	cfg, client := recoveryTestTarget(t)
	state := testBackupRecoveryState(t)
	state.Mode, state.RootCertificate, state.RootPrivateKey = PKIModeExistingClusterIssuer, nil, nil
	cfg.Spec.PKI.Mode, cfg.Spec.PKI.CABundle.InlinePEM, cfg.Spec.PKI.CABundle.File = state.Mode, state.BundlePEM, ""
	if err := installBackupRecoveryState(t.Context(), &kubeClients{typed: client}, cfg, state); err != nil {
		t.Fatal(err)
	}
	secrets, err := client.CoreV1().Secrets("cert-manager").List(t.Context(), metav1.ListOptions{})
	if err != nil || len(secrets.Items) != 0 {
		t.Fatal("customer root private material was created")
	}
	values := map[string]any{"runtime": map[string]any{}}
	if restoredRuntimePKIState(state, values).IssuerGeneration != 2 || values["runtime"].(map[string]any)["trustBundleEpoch"] != int64(3) {
		t.Fatal("customer issuer generation was not retained")
	}
	state.RootPrivateKey = []byte("must-not-be-exported")
	if validateBackupRecoveryState(state) == nil {
		t.Fatal("customer private CA material was accepted")
	}
}

func TestBackupRecoveryRejectsInvalidKeysAndTrustWithoutPrivateErrors(t *testing.T) {
	for _, field := range []string{"key", "root", "bundle", "epoch", "mode"} {
		state := testBackupRecoveryState(t)
		switch field {
		case "key":
			state.RuntimeKeys[recoveryRuntimeKeys[0]] = "private-do-not-log"
		case "root":
			state.RootPrivateKey = []byte("private-do-not-log")
		case "bundle":
			state.BundlePEM = "private-do-not-log"
		case "epoch":
			state.BundleEpoch = 0
		case "mode":
			state.Mode = "unknown"
		}
		err := validateBackupRecoveryState(state)
		if err == nil || strings.Contains(err.Error(), "private-do-not-log") {
			t.Fatal("invalid private recovery material accepted or disclosed")
		}
	}
}

func TestBackupUsesCompletedRotationEpochInsteadOfInitialRuntimeEpoch(t *testing.T) {
	cfg, client := recoveryTestTarget(t)
	state := testBackupRecoveryState(t)
	clients := &kubeClients{typed: client}
	if err := installBackupRecoveryState(t.Context(), clients, cfg, state); err != nil {
		t.Fatal(err)
	}
	config := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "argus-runtime-config", Namespace: cfg.Spec.Namespaces.System}, Data: map[string]string{
		"ARGUS_TRUST_BUNDLE_EPOCH": "1", "ARGUS_CONNECTOR_ISSUER_GENERATION": "2", "ARGUS_TELEMETRY_ISSUER_GENERATION": "2",
	}}
	if _, err := client.CoreV1().ConfigMaps(config.Namespace).Create(t.Context(), config, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	current, err := backupRuntimePKIState(t.Context(), clients, cfg, 3)
	if err != nil || current.TrustBundleEpoch != 3 || current.IssuerGeneration != 2 {
		t.Fatal("completed rotation was replaced by initial epoch")
	}
	config.Data["ARGUS_TRUST_BUNDLE_EPOCH"] = "4"
	if _, err = client.CoreV1().ConfigMaps(config.Namespace).Update(t.Context(), config, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err = backupRuntimePKIState(t.Context(), clients, cfg, 3); err == nil {
		t.Fatal("runtime ahead of database epoch accepted")
	}
}

func assertRecoveryDidNotWrite(t *testing.T, client *fake.Clientset) {
	t.Helper()
	for _, action := range client.Actions() {
		if action.GetVerb() != "get" && action.GetVerb() != "list" {
			t.Fatal("unexpected recovery mutation", action.GetVerb(), action.GetResource().Resource)
		}
	}
}
