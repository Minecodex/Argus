package argusdev

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const plan6ProtocolTrustDirectory = "/var/run/argus/plan6-protocol-ca"

func plan6ProtocolCertDirectories(existing string) string {
	// Keep Go's standard Debian/Linux directory roots and certificate files.
	// This is an additive trust anchor for this isolated receiver only.
	dirs := []string{"/etc/ssl/certs", "/etc/pki/tls/certs"}
	if existing != "" {
		dirs = append(dirs, strings.Split(existing, ":")...)
	}
	dirs = append(dirs, plan6ProtocolTrustDirectory)
	seen, unique := map[string]bool{}, []string{}
	for _, dir := range dirs {
		if dir != "" && !seen[dir] {
			seen[dir] = true
			unique = append(unique, dir)
		}
	}
	return strings.Join(unique, ":")
}

func verifyPlan6ProtocolCertificate(ca, leaf []byte, hostname string) error {
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return fmt.Errorf("protocol fixture CA is invalid")
	}
	block, _ := pem.Decode(leaf)
	if block == nil {
		return fmt.Errorf("protocol fixture leaf is invalid")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}
	_, err = certificate.Verify(x509.VerifyOptions{Roots: roots, DNSName: hostname})
	return err
}

func (a *App) preparePlan6ProtocolTrust(ctx context.Context, env *E2EEnvironment) (string, error) {
	if env.Options.Suite != "plan6" {
		return "", fmt.Errorf("protocol trust belongs only to the isolated Plan6 suite")
	}
	system, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, env.SystemNS, metav1.GetOptions{})
	if err != nil || system.Labels["argus.io/release-id"] != env.ReleaseID {
		return "", fmt.Errorf("protocol trust destination ownership is not established: %v", err)
	}
	namespace := env.ReplayNamespace()
	ns, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil || ns.Labels["argus.io/release-id"] != env.ReleaseID {
		return "", fmt.Errorf("protocol fixture namespace ownership is not established: %v", err)
	}
	secret, err := env.Kube.Client.CoreV1().Secrets(namespace).Get(ctx, "argus-replay-model-tls", metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	if secret.Annotations["meta.helm.sh/release-name"] != env.ReleaseID+"-e2e-fixtures" ||
		secret.Annotations["meta.helm.sh/release-namespace"] != env.SystemNS {
		return "", fmt.Errorf("protocol fixture certificate ownership mismatch")
	}
	ca := secret.Data["ca.crt"]
	hostname := "argus-replay-model." + namespace + ".svc"
	if err = verifyPlan6ProtocolCertificate(ca, secret.Data["tls.crt"], hostname); err != nil {
		return "", err
	}
	const name = "argus-plan6-protocol-ca"
	config, err := env.Kube.Client.CoreV1().ConfigMaps(env.SystemNS).Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"argus.io/release-id": env.ReleaseID, "app.kubernetes.io/part-of": "argus-e2e"}},
		Data:       map[string]string{"fixture-ca.crt": string(ca)},
	}, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(ca)
	report, _ := json.MarshalIndent(map[string]any{"run_id": env.Options.RunID, "fixture_namespace": namespace, "namespace_uid": ns.UID, "destination_namespace_uid": system.UID,
		"certificate_secret_uid": secret.UID, "public_ca_sha256": hex.EncodeToString(hash[:]), "hostname": hostname,
		"config_map_uid": config.UID, "system_roots_preserved": true, "hostname_verified": true,
		"tls_verification_disabled": false, "scope": "owned temporary server/worker only", "captured_at": time.Now().UTC()}, "", "  ")
	return name, writePrivate(filepath.Join(env.Options.Artifacts, "plan6-protocol-trust.json"), report)
}
