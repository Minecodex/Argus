package argusdev

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

type e2eExternalPKI struct {
	IssuerName string
	SecretName string
	CAPEM      string
}

func (a *App) runTLSE2EMatrix(ctx context.Context, options E2EOptions) error {
	for _, test := range []struct {
		name, suite, mode string
	}{
		{name: "managed-strict", suite: "tls-managed-strict", mode: "managed"},
		{name: "existing-strict", suite: "tls-existing-strict", mode: "existing-cluster-issuer"},
	} {
		caseArtifacts := filepath.Join(options.Artifacts, test.name)
		if data, err := os.ReadFile(filepath.Join(caseArtifacts, "result.json")); err == nil && bytes.Contains(data, []byte(`"status":"passed"`)) {
			_, _ = fmt.Fprintf(a.stdout, "TLS matrix %s already passed; reusing %s\n", test.name, caseArtifacts)
			continue
		}
		candidate := options
		candidate.Suite = test.suite
		candidate.RunID = options.RunID + "-" + test.name
		candidate.Artifacts = caseArtifacts
		candidate.PKIMode = test.mode
		candidate.BootstrapTLS = "strict"
		if err := a.runE2ECluster(ctx, candidate); err != nil {
			return fmt.Errorf("TLS matrix %s: %w", test.name, err)
		}
	}
	return writePrivate(filepath.Join(options.Artifacts, "result.json"), []byte(fmt.Sprintf(
		"{\"run_id\":%q,\"suite\":\"tls\",\"status\":\"passed\",\"cases\":[\"managed-strict\",\"existing-strict\"]}\n", options.RunID)))
}

func (a *App) prepareE2EExternalIssuer(ctx context.Context, env *E2EEnvironment) error {
	caPEM, keyPEM, err := generateE2ERootCA(env.ReleaseID + " customer issuer")
	if err != nil {
		return err
	}
	name := kubernetesNameForDev(env.ReleaseID + "-customer-ca")
	secret := map[string]any{
		"apiVersion": "v1", "kind": "Secret", "type": "kubernetes.io/tls",
		"metadata": map[string]any{"name": name, "namespace": "cert-manager", "labels": map[string]string{
			"app.kubernetes.io/part-of": "argus-e2e", "argus.io/release-id": env.ReleaseID,
		}},
		"stringData": map[string]string{"tls.crt": caPEM, "tls.key": keyPEM},
	}
	issuer := map[string]any{
		"apiVersion": "cert-manager.io/v1", "kind": "ClusterIssuer",
		"metadata": map[string]any{"name": name, "labels": map[string]string{
			"app.kubernetes.io/part-of": "argus-e2e", "argus.io/release-id": env.ReleaseID,
		}},
		"spec": map[string]any{"ca": map[string]string{"secretName": name}},
	}
	secretYAML, err := yaml.Marshal(secret)
	if err != nil {
		return err
	}
	issuerYAML, err := yaml.Marshal(issuer)
	if err != nil {
		return err
	}
	manifest := append(append(secretYAML, []byte("---\n")...), issuerYAML...)
	if err = a.runner.RunIO(ctx, nil, bytes.NewReader(manifest), a.stdout, a.stderr, "kubectl", "--context", env.Options.KubeContext, "apply", "-f", "-"); err != nil {
		return err
	}
	env.ExternalPKI = &e2eExternalPKI{IssuerName: name, SecretName: name, CAPEM: caPEM}
	if err = a.runner.Run(ctx, nil, "kubectl", "--context", env.Options.KubeContext, "wait", "--for=condition=Ready",
		"clusterissuer/"+name, "--timeout=2m"); err != nil {
		return fmt.Errorf("external E2E ClusterIssuer did not become Ready: %w", err)
	}
	return nil
}

func (a *App) deleteE2EExternalIssuer(ctx context.Context, env *E2EEnvironment) error {
	if env.ExternalPKI == nil {
		return nil
	}
	issuerErr := a.runner.Run(ctx, nil, "kubectl", "--context", env.Options.KubeContext, "delete", "clusterissuer",
		env.ExternalPKI.IssuerName, "--ignore-not-found=true", "--wait=true")
	secretErr := a.runner.Run(ctx, nil, "kubectl", "--context", env.Options.KubeContext, "-n", "cert-manager", "delete", "secret",
		env.ExternalPKI.SecretName, "--ignore-not-found=true", "--wait=true")
	return errors.Join(issuerErr, secretErr)
}

func generateE2ERootCA(commonName string) (string, string, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}
	now := time.Now().UTC()
	template := &x509.Certificate{SerialNumber: randomPositiveSerial(), Subject: pkix.Name{CommonName: commonName},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(48 * time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign, MaxPathLenZero: false}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return "", "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})), nil
}

func randomPositiveSerial() *big.Int {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	value, err := rand.Int(rand.Reader, limit)
	if err != nil {
		panic(err)
	}
	if value.Sign() == 0 {
		return big.NewInt(1)
	}
	return value
}

func (a *App) runTLSScenario(ctx context.Context, env *E2EEnvironment) error {
	if env.Options.BootstrapTLS != "strict" || (env.Options.PKIMode != "managed" && env.Options.PKIMode != "existing-cluster-issuer") {
		return errors.New("TLS E2E requires managed or existing-cluster-issuer with strict bootstrap")
	}
	if _, err := a.prepareP4EnterpriseAccess(ctx, env); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(a.stdout, "TLS %s: enterprise authorization ready\n", env.Options.PKIMode)
	target, err := a.createP4Target(ctx, env, "strict", "198.51.100.31", p4NetworkOpen)
	if err != nil {
		return err
	}
	if _, err = a.execP4Target(ctx, env, target, "/bin/bash", "-lc",
		"set +e; curl -sS --connect-timeout 5 -o /dev/null "+env.EnterpriseOrigin()+"; code=$?; [ \"$code\" -eq 60 ]"); err != nil {
		return fmt.Errorf("strict target unexpectedly trusted the Argus CA before provisioning: %w", err)
	}
	_, _ = fmt.Fprintf(a.stdout, "TLS %s: target rejects the untrusted serving certificate\n", env.Options.PKIMode)
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	architecture := strings.TrimPrefix(env.ImagePlatform, "linux/")
	preview, err := client.JSON(ctx, "tls-host-preview", "enterprise", http.MethodPost,
		"/enterprise/hosts/actions/preview-create", http.StatusCreated, map[string]any{
			"name": "tls-strict-host", "platform": "linux", "architecture": architecture, "role": "managed_host",
			"control_path": "direct", "install_method": "manual", "ssh_path": "none",
			"environment": "production", "labels": map[string]string{"suite": "tls", "pki": env.Options.PKIMode},
		}, enterpriseHeaders(env, "tls-host-preview"))
	if err != nil {
		return err
	}
	actionRef, err := stringField(preview, "action_ref")
	if err != nil {
		return err
	}
	confirmed, err := a.confirmPendingAction(ctx, env, "tls-host-confirm", actionRef)
	if err != nil {
		return err
	}
	hostID, err := stringField(confirmed, "resource_ref", "resource_id")
	if err != nil {
		return err
	}
	result, ok := confirmed["one_time_result"].(map[string]any)
	if !ok {
		return errors.New("strict Host onboarding omitted its one-time instruction")
	}
	command, err := validateStrictInstallCommand(result, confirmed)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(a.stdout, "TLS %s: strict instruction generated without a TLS bypass\n", env.Options.PKIMode)
	if _, err = a.execP4Target(ctx, env, target, "/bin/bash", "-lc", command); err == nil {
		return errors.New("strict bootstrap unexpectedly succeeded before its serving CA was trusted")
	}
	_, _ = fmt.Fprintf(a.stdout, "TLS %s: first command failed before CA provisioning as expected\n", env.Options.PKIMode)
	bundle, err := env.Kube.Client.CoreV1().ConfigMaps(env.SystemNS).Get(ctx, env.ReleaseID+"-trust-bundle", metav1.GetOptions{})
	if err != nil || strings.TrimSpace(bundle.Data["ca.crt"]) == "" {
		return fmt.Errorf("read installed Trust Bundle: %w", err)
	}
	if env.ExternalPKI != nil && strings.TrimSpace(bundle.Data["ca.crt"]) != strings.TrimSpace(env.ExternalPKI.CAPEM) {
		return errors.New("existing ClusterIssuer install published a different Trust Bundle")
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(bundle.Data["ca.crt"]))
	trustedCommand := "umask 077; printf '%s' '" + encoded + "' | base64 -d > /tmp/argus-serving-ca.pem; " +
		"export CURL_CA_BUNDLE=/tmp/argus-serving-ca.pem; " + command
	if _, err = a.execP4Target(ctx, env, target, "/bin/bash", "-lc", trustedCommand); err != nil {
		return fmt.Errorf("strict bootstrap with provisioned CA: %w", err)
	}
	_, _ = fmt.Fprintf(a.stdout, "TLS %s: the same command completed after CA provisioning\n", env.Options.PKIMode)
	if err = a.waitPostgresValue(ctx, env,
		"SELECT h.status || '|' || h.connection_status || '|' || c.status || '|' || o.status || '|' || o.stage FROM hosts h JOIN connectors c ON c.id=h.connector_id JOIN host_onboarding_operations o ON o.host_id=h.id WHERE h.id='"+hostID+"';",
		"active|online|online|succeeded|completed", 5*time.Minute); err != nil {
		return err
	}
	if err = a.verifyP4HostOnboardingTimeline(ctx, env, hostID); err != nil {
		return err
	}
	evidence := fmt.Sprintf("{\"pki_mode\":%q,\"bootstrap_tls_mode\":\"strict\",\"untrusted_fetch\":\"rejected\",\"trusted_fetch\":\"completed\"}\n", env.Options.PKIMode)
	return writePrivate(filepath.Join(env.Options.Artifacts, "tls-bootstrap.json"), []byte(evidence))
}

func validateStrictInstallCommand(result, confirmation map[string]any) (string, error) {
	if result["schema_version"] != "argus.action_one_time_result/v3" || result["result_kind"] != "connector_install_command" {
		return "", errors.New("unexpected strict one-time result envelope")
	}
	confirmedID, _ := nestedString(confirmation, "execution", "execution_id")
	executionID, _ := result["execution_id"].(string)
	sets, _ := result["instruction_sets"].([]any)
	for _, raw := range sets {
		instruction, ok := raw.(map[string]any)
		if !ok || !strings.HasPrefix(fmt.Sprint(instruction["platform"]), "linux_") {
			continue
		}
		command, _ := instruction["command"].(string)
		if instruction["shell"] != "posix_sh" || instruction["privilege"] != "system" || instruction["bootstrap_tls_mode"] != "strict" ||
			strings.Contains(command, "--insecure") || !strings.Contains(command, "sha256sum -c -") || executionID == "" || executionID != confirmedID {
			return "", errors.New("strict instruction weakened its bootstrap or execution binding")
		}
		return command, nil
	}
	return "", errors.New("strict Linux installation instruction is unavailable")
}
