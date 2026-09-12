package argusdev

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// TestInstalledExecutorTunnelE2E is an opt-in destructive integration test for
// a designated test enterprise on an existing Argus installation. The supplied
// account must already own the Host, Secret/Credential and Pending Action
// permissions required by the scenario. The test never enrolls MFA, changes
// role bindings, or invokes cleanupE2E against the installed release.
//
// Required environment:
//
//	ARGUS_INSTALLED_TUNNEL_E2E=1
//	CONFIG_PATH=<the exact installed Argus config>
//	ENTERPRISE_USERNAME=<dedicated test-enterprise account>
//	PASSWORD=<account password>
//	FIXTURE_IMAGE=<systemd target image already available on every node>
//
// TOTP_SECRET is required only when the existing account already has MFA.
// ARGUS_TUNNEL_E2E_ARTIFACTS optionally keeps redacted evidence in a caller
// selected directory; without it, Go's temporary test directory is used.
func TestInstalledExecutorTunnelE2E(t *testing.T) {
	if os.Getenv("ARGUS_INSTALLED_TUNNEL_E2E") != "1" {
		t.Skip("set ARGUS_INSTALLED_TUNNEL_E2E=1 to test a designated enterprise on an existing installation")
	}
	t.Log("using an existing installation: CONFIG_PATH and credentials must belong to a designated disposable test enterprise")

	configPath := installedTunnelRequiredEnv(t, "CONFIG_PATH")
	username := installedTunnelRequiredEnv(t, "ENTERPRISE_USERNAME")
	password := installedTunnelRequiredEnv(t, "PASSWORD")
	fixtureImage := installedTunnelRequiredEnv(t, "FIXTURE_IMAGE")
	totpSecret := strings.TrimSpace(os.Getenv("TOTP_SECRET"))
	absoluteConfig, err := filepath.Abs(configPath)
	if err != nil {
		t.Fatal(err)
	}
	config, err := loadInstalledTunnelConfig(absoluteConfig)
	if err != nil {
		t.Fatal(err)
	}
	root, err := findRepoRoot("")
	if err != nil {
		t.Fatal(err)
	}
	runID := kubernetesNameForDev("installed-tunnel-" + time.Now().UTC().Format("20060102-150405") + "-" + uuid.NewString()[:8])
	artifacts, err := installedTunnelArtifacts(t)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{root: root, stdout: io.Discard, stderr: io.Discard,
		runner: Runner{Dir: root, Stdout: io.Discard, Stderr: io.Discard}}
	kube, err := NewE2EKube(config.Spec.KubeContext, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	architecture, err := kube.NodeArchitecture(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	env := &E2EEnvironment{
		Options: E2EOptions{Suite: "p4", KubeContext: kube.Context, RunID: runID, Artifacts: artifacts},
		Root:    root, ConfigPath: absoluteConfig, Profile: config.Spec.Profile, ReleaseID: config.Spec.ReleaseID,
		SystemNS: config.Spec.Namespaces.System, SandboxNS: config.Spec.Namespaces.Sandbox, ObservNS: config.Spec.Namespaces.Observability,
		ImagePlatform: "linux/" + architecture, Kube: kube, State: NewScenarioState(runID),
	}
	env.State.FixtureImages["systemd"] = fixtureImage
	if err = app.resolveE2EAccess(t.Context(), env); err != nil {
		t.Fatal(err)
	}
	env.State.HTTP = NewDomainScenarioHTTP(env)
	mfaAuthenticated, err := loginInstalledTunnelEnterprise(t.Context(), env, username, password, totpSecret)
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := snapshotInstalledExecutor(t.Context(), env)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
		defer cancel()
		if restoreErr := restoreInstalledExecutor(cleanupCtx, env, snapshot); restoreErr != nil {
			t.Errorf("restore installed Direct Executor: %v", restoreErr)
		}
	})
	if err = app.patchP4DirectExecutor(t.Context(), env); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		for index := len(env.ManagedNamespaces) - 1; index >= 0; index-- {
			if namespace := env.ManagedNamespaces[index]; namespace != "" {
				if cleanupErr := env.Kube.DeleteNamespace(cleanupCtx, namespace); cleanupErr != nil {
					t.Errorf("delete owned target namespace %s: %v", namespace, cleanupErr)
				}
			}
		}
	})
	target, err := app.createP4Target(t.Context(), env, runID, "198.51.100.28", p4NetworkRootTunnel)
	if err != nil {
		t.Fatal(err)
	}
	if err = installTargetIngressBlock(t.Context(), app, env, target); err != nil {
		t.Fatal(err)
	}

	owned := &installedTunnelResources{}
	scenario := &p4Scenario{ExecutorHostTarget: target, ExecutorHostName: runID}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
		defer cancel()
		if cleanupErr := cleanupInstalledTunnelResources(cleanupCtx, app, env, scenario, owned, username, password, totpSecret, mfaAuthenticated); cleanupErr != nil {
			t.Errorf("clean installed executor-tunnel test resources: %v", cleanupErr)
		}
	})
	if err = createInstalledTunnelCredential(t.Context(), env, owned); err != nil {
		t.Fatal(err)
	}
	scenario.CredentialID = owned.CredentialID
	if err = app.runP4ExecutorTunnelHost(t.Context(), env, scenario); err != nil {
		t.Fatal(err)
	}
}

func installedTunnelArtifacts(t *testing.T) (string, error) {
	t.Helper()
	configured := strings.TrimSpace(os.Getenv("ARGUS_TUNNEL_E2E_ARTIFACTS"))
	if configured == "" {
		return t.TempDir(), nil
	}
	directory, err := filepath.Abs(configured)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	return directory, nil
}

type installedTunnelConfig struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		Profile     string `json:"profile"`
		KubeContext string `json:"kubeContext"`
		ReleaseID   string `json:"releaseId"`
		Namespaces  struct {
			System        string `json:"system"`
			Sandbox       string `json:"sandbox"`
			Observability string `json:"observability"`
		} `json:"namespaces"`
	} `json:"spec"`
}

func loadInstalledTunnelConfig(path string) (installedTunnelConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return installedTunnelConfig{}, err
	}
	var config installedTunnelConfig
	if err = yaml.Unmarshal(data, &config); err != nil {
		return installedTunnelConfig{}, err
	}
	if config.Spec.ReleaseID == "" {
		config.Spec.ReleaseID = config.Metadata.Name
	}
	if config.Spec.ReleaseID == "" || config.Spec.Namespaces.System == "" || config.Spec.Namespaces.Observability == "" {
		return installedTunnelConfig{}, fmt.Errorf("installed config must define releaseId and system/observability namespaces")
	}
	return config, nil
}

func installedTunnelRequiredEnv(t *testing.T, name string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		t.Fatalf("%s is required when ARGUS_INSTALLED_TUNNEL_E2E=1", name)
	}
	return value
}

func loginInstalledTunnelEnterprise(ctx context.Context, env *E2EEnvironment, username, password, totpSecret string) (bool, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return false, err
	}
	client.Reset("enterprise")
	login, err := client.JSON(ctx, "installed-tunnel-enterprise-login", "enterprise", http.MethodPost,
		"/enterprise/auth/login", http.StatusOK, map[string]any{"username": username, "password": password},
		map[string]string{"Origin": env.EnterpriseOrigin()})
	if err != nil {
		return false, err
	}
	mfaCompleted := false
	csrf, authenticated := nestedString(login, "authenticated_session", "csrf_token")
	if !authenticated {
		challenge, challengeOK := nestedString(login, "mfa_challenge", "challenge_id")
		if !challengeOK {
			return false, fmt.Errorf("enterprise login returned neither a session nor an MFA challenge")
		}
		if totpSecret == "" {
			return false, fmt.Errorf("TOTP_SECRET is required because the existing account requested MFA")
		}
		code, codeErr := waitForNextTOTP(totpSecret, "")
		if codeErr != nil {
			return false, codeErr
		}
		completed, completeErr := client.JSON(ctx, "installed-tunnel-enterprise-mfa", "enterprise", http.MethodPost,
			"/enterprise/auth/mfa/complete", http.StatusOK, map[string]any{"challenge_id": challenge, "code": code},
			map[string]string{"Origin": env.EnterpriseOrigin()})
		if completeErr != nil {
			return false, completeErr
		}
		csrf, err = stringField(completed, "csrf_token")
		if err != nil {
			return false, err
		}
		env.State.Values["enterprise_mfa_secret"] = totpSecret
		env.State.Values["enterprise_mfa_last"] = code
		authenticated = true
		mfaCompleted = true
	}
	env.State.Values["enterprise_username"] = username
	env.State.Values["enterprise_password"] = password
	env.State.Values["enterprise_csrf"] = csrf
	return mfaCompleted, nil
}

type installedExecutorSnapshot struct {
	replicas    *int32
	hostAliases []corev1.HostAlias
}

func snapshotInstalledExecutor(ctx context.Context, env *E2EEnvironment) (installedExecutorSnapshot, error) {
	deployment, err := env.Kube.Client.AppsV1().Deployments(env.SystemNS).Get(ctx, "argus-direct-executor", metav1.GetOptions{})
	if err != nil {
		return installedExecutorSnapshot{}, err
	}
	if deployment.Labels["argus.io/release-id"] != env.ReleaseID {
		return installedExecutorSnapshot{}, fmt.Errorf("Direct Executor %s/%s belongs to release %q, expected %q",
			env.SystemNS, deployment.Name, deployment.Labels["argus.io/release-id"], env.ReleaseID)
	}
	snapshot := installedExecutorSnapshot{hostAliases: cloneHostAliases(deployment.Spec.Template.Spec.HostAliases)}
	if deployment.Spec.Replicas != nil {
		value := *deployment.Spec.Replicas
		snapshot.replicas = &value
	}
	return snapshot, nil
}

func restoreInstalledExecutor(ctx context.Context, env *E2EEnvironment, snapshot installedExecutorSnapshot) error {
	patch := map[string]any{"spec": map[string]any{
		"replicas": snapshot.replicas,
		"template": map[string]any{"spec": map[string]any{"hostAliases": snapshot.hostAliases}},
	}}
	if err := env.Kube.PatchDeployment(ctx, env.SystemNS, "argus-direct-executor", patch); err != nil {
		return err
	}
	if err := env.Kube.WaitDeployment(ctx, env.SystemNS, "argus-direct-executor", 5*time.Minute); err != nil {
		return err
	}
	current, err := env.Kube.Client.AppsV1().Deployments(env.SystemNS).Get(ctx, "argus-direct-executor", metav1.GetOptions{})
	if err != nil {
		return err
	}
	if !replicasEqual(current.Spec.Replicas, snapshot.replicas) || !reflect.DeepEqual(current.Spec.Template.Spec.HostAliases, snapshot.hostAliases) {
		return fmt.Errorf("Direct Executor replicas or hostAliases did not return to the installed snapshot")
	}
	return nil
}

func cloneHostAliases(input []corev1.HostAlias) []corev1.HostAlias {
	if input == nil {
		return nil
	}
	result := make([]corev1.HostAlias, len(input))
	for index := range input {
		result[index] = input[index]
		result[index].Hostnames = append([]string(nil), input[index].Hostnames...)
	}
	return result
}

func replicasEqual(left, right *int32) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func installTargetIngressBlock(ctx context.Context, app *App, env *E2EEnvironment, target p4Target) error {
	if net.ParseIP(env.Endpoints.IngressIP).To4() == nil {
		return fmt.Errorf("installed executor-tunnel test requires an IPv4 ingress address for target iptables isolation")
	}
	command := "set -eu; command -v iptables >/dev/null 2>&1 || { echo 'FIXTURE_IMAGE lacks iptables' >&2; exit 127; }; " +
		"iptables -I OUTPUT 1 -p tcp -d " + env.Endpoints.IngressIP + " --dport 443 -m conntrack --ctstate NEW -j REJECT; " +
		"iptables -C OUTPUT -p tcp -d " + env.Endpoints.IngressIP + " --dport 443 -m conntrack --ctstate NEW -j REJECT"
	if _, err := app.execP4Target(ctx, env, target, "/bin/bash", "-lc", command); err != nil {
		return fmt.Errorf("install target-only ingress block; FIXTURE_IMAGE must include iptables and conntrack match support: %w", err)
	}
	return nil
}

type installedTunnelResources struct {
	SecretID          string
	SecretVersion     int64
	CredentialID      string
	CredentialVersion int64
}

func createInstalledTunnelCredential(ctx context.Context, env *E2EEnvironment, owned *installedTunnelResources) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	secret, err := client.JSON(ctx, "installed-tunnel-secret-create", "enterprise", http.MethodPost,
		"/enterprise/secrets", http.StatusCreated,
		map[string]any{"name": "installed-tunnel-" + env.Options.RunID, "type": "ssh_password", "description": "temporary installed executor tunnel E2E credential", "value": p4SSHPassword},
		enterpriseHeaders(env, "installed-tunnel-secret-create"))
	if err != nil {
		return err
	}
	owned.SecretID, err = stringField(secret, "id")
	if err != nil {
		return err
	}
	owned.SecretVersion, err = numberField(secret, "version")
	if err != nil {
		return err
	}
	credential, err := client.JSON(ctx, "installed-tunnel-credential-create", "enterprise", http.MethodPost,
		"/enterprise/credentials", http.StatusCreated,
		map[string]any{"name": "installed-tunnel-" + env.Options.RunID, "protocol": "ssh", "username": "root", "secret_id": owned.SecretID},
		enterpriseHeaders(env, "installed-tunnel-credential-create"))
	if err != nil {
		return err
	}
	owned.CredentialID, err = stringField(credential, "id")
	if err != nil {
		return err
	}
	owned.CredentialVersion, err = numberField(credential, "version")
	return err
}

func cleanupInstalledTunnelResources(ctx context.Context, app *App, env *E2EEnvironment, scenario *p4Scenario, owned *installedTunnelResources,
	username, password, totpSecret string, mfaAuthenticated bool) error {
	var cleanupErrors []error
	cleanupMFA, err := loginInstalledTunnelEnterprise(ctx, env, username, password, totpSecret)
	if err != nil {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("refresh enterprise login: %w", err))
		return errors.Join(cleanupErrors...)
	}
	if scenario.ExecutorHostID != "" && owned.CredentialID != "" {
		if mfaAuthenticated || cleanupMFA {
			if err := app.stepUpEnterprise(ctx, env); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("refresh MFA step-up: %w", err))
			}
		}
		if err := removeInstalledTunnelHost(ctx, app, env, scenario, owned.CredentialID); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return errors.Join(append(cleanupErrors, err)...)
	}
	if owned.CredentialID != "" && owned.CredentialVersion > 0 {
		credential, updateErr := client.JSON(ctx, "installed-tunnel-credential-disable", "enterprise", http.MethodPut,
			"/enterprise/credentials/"+owned.CredentialID, http.StatusOK,
			map[string]any{"status": "disabled", "expected_version": owned.CredentialVersion}, enterpriseHeaders(env, ""))
		if updateErr != nil {
			cleanupErrors = append(cleanupErrors, updateErr)
		} else if status, _ := credential["status"].(string); status != "disabled" {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("temporary Credential did not become disabled"))
		}
	}
	if owned.SecretID != "" && owned.SecretVersion > 0 {
		_, disableErr := client.JSON(ctx, "installed-tunnel-secret-disable", "enterprise", http.MethodDelete,
			fmt.Sprintf("/enterprise/secrets/%s?expected_version=%d", owned.SecretID, owned.SecretVersion), http.StatusNoContent,
			nil, enterpriseHeaders(env, ""))
		if disableErr != nil {
			cleanupErrors = append(cleanupErrors, disableErr)
		}
	}
	return errors.Join(cleanupErrors...)
}

func removeInstalledTunnelHost(ctx context.Context, app *App, env *E2EEnvironment, scenario *p4Scenario, credentialID string) error {
	testID, err := app.createP4ConnectionTest(ctx, env, "installed-tunnel-remove", scenario.ExecutorHostTarget.ExternalIP, credentialID, "")
	if err != nil {
		return fmt.Errorf("create cleanup connection test: %w", err)
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	host, err := client.JSON(ctx, "installed-tunnel-host-before-remove", "enterprise", http.MethodGet,
		"/enterprise/hosts/"+scenario.ExecutorHostID, http.StatusOK, nil, map[string]string{"Origin": env.EnterpriseOrigin()})
	if err != nil {
		return err
	}
	version, err := numberField(host, "resource_version")
	if err != nil {
		return err
	}
	preview, err := client.JSON(ctx, "installed-tunnel-remove-preview", "enterprise", http.MethodPost,
		"/enterprise/host-removals/actions/preview", http.StatusCreated,
		map[string]any{"target_type": "managed_host", "target_id": scenario.ExecutorHostID, "expected_version": version,
			"mode": "uninstall", "credential_id": credentialID, "connection_test_id": testID},
		enterpriseHeaders(env, "installed-tunnel-remove-preview"))
	if err != nil {
		return err
	}
	actionRef, err := stringField(preview, "action_ref")
	if err != nil {
		return err
	}
	if _, err = app.confirmPendingAction(ctx, env, "installed-tunnel-remove-confirm", actionRef); err != nil {
		return err
	}
	if err = app.waitPostgresValue(ctx, env,
		"SELECT status || '|' || local_cleanup FROM hosts WHERE id='"+scenario.ExecutorHostID+"';",
		"uninstalled|verified", 4*time.Minute); err != nil {
		return err
	}
	host, err = client.JSON(ctx, "installed-tunnel-host-before-delete", "enterprise", http.MethodGet,
		"/enterprise/hosts/"+scenario.ExecutorHostID, http.StatusOK, nil, map[string]string{"Origin": env.EnterpriseOrigin()})
	if err != nil {
		return err
	}
	version, err = numberField(host, "resource_version")
	if err != nil {
		return err
	}
	deletePreview, err := client.JSON(ctx, "installed-tunnel-delete-preview", "enterprise", http.MethodPost,
		"/enterprise/hosts/"+scenario.ExecutorHostID+"/actions/preview-delete", http.StatusCreated,
		map[string]any{"expected_version": version}, enterpriseHeaders(env, "installed-tunnel-delete-preview"))
	if err != nil {
		return err
	}
	deleteRef, err := stringField(deletePreview, "action_ref")
	if err != nil {
		return err
	}
	if _, err = app.confirmPendingAction(ctx, env, "installed-tunnel-delete-confirm", deleteRef); err != nil {
		return err
	}
	checks := []struct {
		name, query, expected string
	}{
		{"Host deletion", "SELECT status FROM hosts WHERE id='" + scenario.ExecutorHostID + "';", "deleted"},
		{"enrollment token revocation", "SELECT count(*) FROM connector_enrollment_tokens WHERE preallocated_host_id='" + scenario.ExecutorHostID + "' AND status='active';", "0"},
		{"control tunnel retirement", "SELECT count(*) FROM connector_control_tunnels WHERE host_id='" + scenario.ExecutorHostID + "' AND status<>'removed';", "0"},
	}
	for _, check := range checks {
		if err = app.waitPostgresValue(ctx, env, check.query, check.expected, time.Minute); err != nil {
			return fmt.Errorf("%s: %w", check.name, err)
		}
	}
	return nil
}
