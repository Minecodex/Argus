package argusdev

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type p4Scenario struct {
	CredentialID       string
	DistributionID     string
	HostProfiles       []string
	SelfTarget         p4Target
	ReplayTarget       p4Target
	CommandTarget      p4Target
	RelayHostTarget    p4Target
	DirectHostTarget   p4Target
	ExecutorHostTarget p4Target
	DirectTarget       p4Target
	TunnelTarget       p4Target
	MemberTarget       p4Target
	TunnelInstall      p4InstallResult
	CommandInstall     p4InstallResult
	RootCollectorID    string
	DirectHostID       string
	ExecutorHostID     string
	ExecutorHostName   string
	MemberHostID       string
	DirectInstall      p4InstallResult
}

func (a *App) runP4Scenario(ctx context.Context, env *E2EEnvironment) error {
	for _, workload := range []string{"argus-telemetry-ingest", "argus-telemetry-writer", "argus-telemetry-query"} {
		if err := env.Kube.WaitDeployment(ctx, env.ObservNS, workload, 5*time.Minute); err != nil {
			return err
		}
	}
	if err := a.patchP4DirectExecutor(ctx, env, true); err != nil {
		return err
	}
	credentialID, err := a.prepareP4EnterpriseAccess(ctx, env)
	if err != nil {
		return err
	}
	distributionID, profiles, _, err := a.verifyM7Catalog(ctx, env)
	if err != nil {
		return err
	}
	scenario := &p4Scenario{CredentialID: credentialID, DistributionID: distributionID, HostProfiles: profiles}
	targets := []struct {
		name    string
		ip      string
		network p4TargetNetwork
		set     func(p4Target)
	}{
		{"self", "198.51.100.20", p4NetworkOutboundOnly, func(value p4Target) { scenario.SelfTarget = value }},
		{"token-replay", "198.51.100.21", p4NetworkOpen, func(value p4Target) { scenario.ReplayTarget = value }},
		{"bastion-command", "198.51.100.22", p4NetworkOpen, func(value p4Target) { scenario.CommandTarget = value }},
		{"relay-host", "198.51.100.26", p4NetworkMember, func(value p4Target) { scenario.RelayHostTarget = value }},
		{"direct-host", "198.51.100.27", p4NetworkOpen, func(value p4Target) { scenario.DirectHostTarget = value }},
		{"executor-host", "198.51.100.28", p4NetworkRootTunnel, func(value p4Target) { scenario.ExecutorHostTarget = value }},
		{"bastion-direct", "198.51.100.23", p4NetworkOpen, func(value p4Target) { scenario.DirectTarget = value }},
		{"bastion-tunnel", "198.51.100.24", p4NetworkRootTunnel, func(value p4Target) { scenario.TunnelTarget = value }},
		{"bastion-member", "198.51.100.25", p4NetworkMember, func(value p4Target) { scenario.MemberTarget = value }},
	}
	for _, item := range targets {
		target, createErr := a.createP4Target(ctx, env, item.name, item.ip, item.network)
		if createErr != nil {
			return createErr
		}
		item.set(target)
	}
	for _, target := range []p4Target{scenario.SelfTarget, scenario.CommandTarget, scenario.DirectTarget} {
		if _, err = a.execP4Target(ctx, env, target, "/bin/bash", "-lc",
			"set +e; curl -sS --connect-timeout 5 -o /dev/null "+env.EnterpriseOrigin()+"; code=$?; [ \"$code\" -eq 60 ]"); err != nil {
			return fmt.Errorf("P4 target %s unexpectedly trusted the Argus managed CA or did not reach its TLS endpoint: %w", target.Name, err)
		}
	}
	if _, err = a.execP4Target(ctx, env, scenario.TunnelTarget, "/bin/bash", "-lc",
		"set +e; curl -sS --connect-timeout 5 -o /dev/null "+env.EnterpriseOrigin()+"; code=$?; [ \"$code\" -ne 0 ]"); err != nil {
		return fmt.Errorf("P4 no-egress Bastion unexpectedly reached the Argus TLS endpoint: %w", err)
	}
	if err = a.runP4SelfEnrollment(ctx, env, scenario); err != nil {
		return fmt.Errorf("p4-self-enroll: %w", err)
	}
	if err = a.runP4CommandBastion(ctx, env, scenario); err != nil {
		return fmt.Errorf("p4-bastion-command: %w", err)
	}
	if err = a.runP4ManualRelayHost(ctx, env, scenario); err != nil {
		return fmt.Errorf("p4-host-command-via-bastion: %w", err)
	}
	if err = a.verifyP4CredentialRotationInvalidatesPreview(ctx, env, scenario); err != nil {
		return fmt.Errorf("p4-credential-rotation: %w", err)
	}
	if err = a.runP4DirectHost(ctx, env, scenario); err != nil {
		return fmt.Errorf("p4-host-direct-ssh: %w", err)
	}
	if err = a.runP4ExecutorTunnelHost(ctx, env, scenario); err != nil {
		return fmt.Errorf("p4-host-executor-tunnel: %w", err)
	}
	if err = a.runP4DirectBastion(ctx, env, scenario); err != nil {
		return fmt.Errorf("p4-bastion-direct-install: %w", err)
	}
	if err = a.runP4TunnelBastion(ctx, env, scenario); err != nil {
		return fmt.Errorf("p4-bastion-control-tunnel: %w", err)
	}
	if err = a.runP4MemberTunnel(ctx, env, scenario); err != nil {
		return fmt.Errorf("p4-bastion-tunnel: %w", err)
	}
	if err = a.verifyP4TunnelAudits(ctx, env); err != nil {
		return err
	}
	if err = a.verifyP4NoSensitivePersistence(ctx, env); err != nil {
		return err
	}
	if err = a.runPlaywright(ctx, env, "e2e/p4-real.spec.ts", map[string]string{
		"ARGUS_P4_E2E": "1", "ARGUS_P4_ENTERPRISE_USERNAME": env.State.Values["enterprise_username"],
		"ARGUS_P4_ENTERPRISE_PASSWORD": env.State.Values["enterprise_password"],
	}); err != nil {
		return err
	}
	return a.verifyP4SSHRemovals(ctx, env, scenario)
}

func (a *App) verifyP4CredentialRotationInvalidatesPreview(ctx context.Context, env *E2EEnvironment, scenario *p4Scenario) error {
	testID, err := a.createP4ConnectionTest(ctx, env, "p4-stale-credential", scenario.DirectHostTarget.ExternalIP, scenario.CredentialID, "", "direct")
	if err != nil {
		return err
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	preview, err := client.JSON(ctx, "p4-stale-credential-preview", "enterprise", http.MethodPost,
		"/enterprise/hosts/actions/preview-create", http.StatusCreated, map[string]any{
			"name": "p4-stale-credential-host", "address": scenario.DirectHostTarget.ExternalIP, "port": 22, "platform": "linux",
			"role": "managed_host", "control_path": "direct", "install_method": "ssh", "ssh_path": "direct_executor",
			"credential_id": scenario.CredentialID, "username": "root", "connection_test_id": testID,
			"environment": "production", "labels": map[string]string{"suite": "p4", "mode": "stale-credential"},
		}, enterpriseHeaders(env, "p4-stale-credential-preview"))
	if err != nil {
		return err
	}
	actionRef, err := stringField(preview, "action_ref")
	if err != nil {
		return err
	}
	secretFacts, err := a.postgresQuery(ctx, env,
		"SELECT s.id::text || '|' || s.version::text || '|' || c.version::text FROM credentials c JOIN secrets s ON s.id=c.secret_id WHERE c.id='"+scenario.CredentialID+"';")
	if err != nil {
		return err
	}
	parts := strings.Split(strings.TrimSpace(secretFacts), "|")
	if len(parts) != 3 {
		return errors.New("credential Secret version is unavailable")
	}
	version, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return err
	}
	credentialVersion, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return err
	}
	if _, err = client.JSON(ctx, "p4-stale-credential-rotate", "enterprise", http.MethodPost,
		"/enterprise/secrets/"+parts[0]+"/rotate", http.StatusOK,
		map[string]any{"value": p4SSHPassword, "expected_version": version}, enterpriseHeaders(env, "p4-stale-credential-rotate")); err != nil {
		return err
	}
	expectedState := fmt.Sprintf("expired|%d", credentialVersion+1)
	if err = a.waitPostgresValue(ctx, env,
		"SELECT t.status || '|' || c.version::text FROM connection_tests t JOIN credentials c ON c.id=t.credential_id WHERE t.id='"+testID+"';",
		expectedState, time.Minute); err != nil {
		return err
	}
	rejected, err := client.JSON(ctx, "p4-stale-credential-confirm", "enterprise", http.MethodPost,
		"/enterprise/pending-actions/"+actionRef+"/confirm", http.StatusConflict, nil,
		enterpriseHeaders(env, "p4-stale-credential-confirm"))
	if err != nil || rejected["code"] != "PENDING_ACTION_INVALIDATED" {
		return fmt.Errorf("credential rotation did not invalidate the frozen Host preview: %#v, %v", rejected, err)
	}
	evidence := fmt.Sprintf("{\"connection_test_status\":\"expired\",\"credential_version_before\":%d,\"credential_version_after\":%d,\"preview_error_code\":\"PENDING_ACTION_INVALIDATED\"}\n",
		credentialVersion, credentialVersion+1)
	return writePrivate(filepath.Join(env.Options.Artifacts, "p4-credential-rotation.json"), []byte(evidence))
}

func (a *App) prepareP4EnterpriseAccess(ctx context.Context, env *E2EEnvironment) (string, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return "", err
	}
	secret, err := client.JSON(ctx, "p4-secret-create", "enterprise", http.MethodPost, "/enterprise/secrets", http.StatusCreated,
		map[string]any{"name": "p4-root-password", "type": "ssh_password", "description": "PlanV4 E2E write-only secret", "value": p4SSHPassword},
		enterpriseHeaders(env, "p4-secret-create"))
	if err != nil {
		return "", err
	}
	if _, exposed := secret["value"]; exposed {
		return "", fmt.Errorf("P4 Secret response exposed write-only value")
	}
	secretID, err := stringField(secret, "id")
	if err != nil {
		return "", err
	}
	credential, err := client.JSON(ctx, "p4-credential-create", "enterprise", http.MethodPost, "/enterprise/credentials", http.StatusCreated,
		map[string]any{"name": "p4-root-ssh", "protocol": "ssh", "username": "root", "secret_id": secretID},
		enterpriseHeaders(env, "p4-credential-create"))
	if err != nil {
		return "", err
	}
	credentialID, err := stringField(credential, "id")
	if err != nil {
		return "", err
	}
	roles, err := client.JSON(ctx, "p4-roles", "enterprise", http.MethodGet, "/enterprise/roles", http.StatusOK, nil,
		map[string]string{"Origin": env.EnterpriseOrigin()})
	if err != nil {
		return "", err
	}
	role, err := findItem(objectItems(roles), func(item map[string]any) bool {
		return item["builtin_key"] == "resource_admin" && item["builtin"] == true
	})
	if err != nil {
		return "", err
	}
	roleID, err := stringField(role, "id")
	if err != nil {
		return "", err
	}
	if _, err = client.JSON(ctx, "p4-admin-binding", "enterprise", http.MethodPost, "/enterprise/role-bindings", http.StatusCreated,
		map[string]any{"subject_type": "user", "subject_id": env.State.Values["admin_user_id"], "role_id": roleID},
		enterpriseHeaders(env, "p4-admin-binding")); err != nil {
		return "", err
	}
	if err = a.refreshEnterpriseLogin(ctx, env); err != nil {
		return "", err
	}
	return credentialID, nil
}

func (a *App) runP4SelfEnrollment(ctx context.Context, env *E2EEnvironment, scenario *p4Scenario) error {
	client, _ := scenarioHTTP(env)
	architecture := strings.TrimPrefix(env.ImagePlatform, "linux/")
	preview, err := client.JSON(ctx, "p4-manual-preview", "enterprise", http.MethodPost,
		"/enterprise/hosts/actions/preview-create", http.StatusCreated, map[string]any{
			"name": "p4-manual-host", "platform": "linux", "architecture": architecture, "role": "managed_host",
			"control_path": "direct", "install_method": "manual", "ssh_path": "none",
			"environment": "production", "labels": map[string]string{"suite": "p4", "mode": "manual"},
		}, enterpriseHeaders(env, "p4-manual-preview"))
	if err != nil {
		return err
	}
	actionRef, err := stringField(preview, "action_ref")
	if err != nil {
		return err
	}
	confirmed, err := a.confirmPendingAction(ctx, env, "p4-manual-confirm", actionRef)
	if err != nil {
		return err
	}
	hostID, err := stringField(confirmed, "resource_ref", "resource_id")
	if err != nil {
		return err
	}
	result, ok := confirmed["one_time_result"].(map[string]any)
	if !ok {
		return fmt.Errorf("manual Host onboarding did not return a one-time result")
	}
	command, err := validateP4OneTimeResult(result, "connector_install_command", confirmed)
	if err != nil {
		return err
	}
	executionID, _ := result["execution_id"].(string)
	retry, err := client.JSON(ctx, "p4-self-claim-retry", "enterprise", http.MethodPost,
		"/enterprise/executions/"+executionID+"/one-time-result", http.StatusOK, nil,
		enterpriseHeaders(env, "p4-manual-confirm-one-time-result"))
	if err != nil {
		return err
	}
	retryCommand, retryErr := validateP4OneTimeResult(retry, "connector_install_command", confirmed)
	if retryErr != nil || retryCommand != command {
		return fmt.Errorf("same-idempotency one-time result retry was not stable: %v", retryErr)
	}
	second, err := client.JSON(ctx, "p4-self-claim-rejected", "enterprise", http.MethodPost,
		"/enterprise/executions/"+executionID+"/one-time-result", http.StatusConflict, nil,
		enterpriseHeaders(env, "p4-self-second-claim"))
	if err != nil || second["code"] != "ACTION_RESULT_ALREADY_CONSUMED" {
		return fmt.Errorf("different-idempotency one-time result claim was not rejected: %#v, %v", second, err)
	}
	if _, err = a.execP4Target(ctx, env, scenario.SelfTarget, "/bin/bash", "-lc", command); err != nil {
		return err
	}
	if _, err = a.execP4Target(ctx, env, scenario.SelfTarget, "/bin/bash", "-lc", command); err == nil {
		return fmt.Errorf("consumed token still authorized a new bootstrap download")
	}
	if _, err = a.execP4Target(ctx, env, scenario.ReplayTarget, "/bin/bash", "-lc", command); err == nil {
		return fmt.Errorf("consumed Connector install command worked on a second device")
	}
	query := "SELECT h.status || '|' || h.connection_status || '|' || c.status || '|' || o.status || '|' || o.stage FROM hosts h JOIN connectors c ON c.id=h.connector_id JOIN host_onboarding_operations o ON o.host_id=h.id WHERE h.id='" + hostID + "';"
	if err = a.waitPostgresValue(ctx, env, query, "active|online|online|succeeded|completed", 5*time.Minute); err != nil {
		return err
	}
	if err = a.verifyP4HostOnboardingTimeline(ctx, env, hostID); err != nil {
		return err
	}
	collectorCount, err := a.postgresQuery(ctx, env, "SELECT count(*) FROM collector_instances WHERE resource_type='host' AND resource_id='"+hostID+"';")
	if err != nil || strings.TrimSpace(collectorCount) != "0" {
		return fmt.Errorf("Host first install unexpectedly created a Collector: %q, %v", collectorCount, err)
	}
	env.State.Values["p4_host_id"] = hostID
	return nil
}

func validateP4OneTimeResult(result map[string]any, kind string, confirmation map[string]any) (string, error) {
	if result["schema_version"] != "argus.action_one_time_result/v3" || result["result_kind"] != kind {
		return "", fmt.Errorf("unexpected one-time result envelope")
	}
	executionID, _ := result["execution_id"].(string)
	confirmedID, _ := nestedString(confirmation, "execution", "execution_id")
	command := ""
	sets, _ := result["instruction_sets"].([]any)
	for _, raw := range sets {
		instruction, ok := raw.(map[string]any)
		if !ok || !strings.HasPrefix(fmt.Sprint(instruction["platform"]), "linux_") || instruction["shell"] != "posix_sh" || instruction["privilege"] != "system" {
			continue
		}
		command, _ = instruction["command"].(string)
		bootstrapSHA, _ := instruction["bootstrap_sha256"].(string)
		installerSHA, _ := instruction["installer_sha256"].(string)
		trustSHA, _ := instruction["trust_bundle_sha256"].(string)
		bootstrapDigest, bootstrapErr := hex.DecodeString(bootstrapSHA)
		installerDigest, installerErr := hex.DecodeString(installerSHA)
		trustDigest, trustErr := hex.DecodeString(trustSHA)
		if instruction["bootstrap_tls_mode"] != "insecure-first-fetch" || strings.Count(command, "--insecure") != 1 ||
			!strings.Contains(command, "X-Argus-Enrollment-Token: ") || !strings.Contains(command, "bootstrap-script?scope=linux-system") ||
			!strings.Contains(command, "sha256sum -c -") || !strings.Contains(command, bootstrapSHA) ||
			bootstrapErr != nil || installerErr != nil || trustErr != nil || len(bootstrapDigest) != 32 || len(installerDigest) != 32 || len(trustDigest) != 32 {
			return "", fmt.Errorf("managed first-fetch policy was not preserved in the Worker-generated instruction")
		}
		break
	}
	expires, _ := result["expires_at"].(string)
	expiresAt, parseErr := time.Parse(time.RFC3339, expires)
	if executionID == "" || executionID != confirmedID || command == "" || parseErr != nil || !expiresAt.After(time.Now()) {
		return "", fmt.Errorf("incomplete one-time result envelope")
	}
	return command, nil
}

func (a *App) runP4CommandBastion(ctx context.Context, env *E2EEnvironment, scenario *p4Scenario) error {
	client, _ := scenarioHTTP(env)
	preview, err := client.JSON(ctx, "p4-command-bastion-preview", "enterprise", http.MethodPost,
		"/enterprise/bastion-scopes/actions/preview-create", http.StatusCreated, map[string]any{
			"name": "p4-command-bastion", "environment": "production", "labels": map[string]string{"suite": "p4", "mode": "a"}, "install_mode": "command",
			"architecture": strings.TrimPrefix(env.ImagePlatform, "linux/"),
		}, enterpriseHeaders(env, "p4-command-bastion-preview"))
	if err != nil {
		return err
	}
	actionRef, err := stringField(preview, "action_ref")
	if err != nil {
		return err
	}
	confirmed, err := a.confirmPendingAction(ctx, env, "p4-command-bastion-confirm", actionRef)
	if err != nil {
		return err
	}
	scopeID, err := stringField(confirmed, "resource_ref", "resource_id")
	if err != nil {
		return err
	}
	result, ok := confirmed["one_time_result"].(map[string]any)
	if !ok {
		return fmt.Errorf("mode A did not return a one-time install command")
	}
	command, err := validateP4OneTimeResult(result, "connector_install_command", confirmed)
	if err != nil {
		return err
	}
	connectorID, err := a.postgresQuery(ctx, env, "SELECT preallocated_connector_id FROM connector_enrollment_tokens WHERE bastion_scope_id='"+scopeID+"' AND status='active' ORDER BY created_at DESC LIMIT 1;")
	if err != nil {
		return err
	}
	connectorID = strings.TrimSpace(connectorID)
	if connectorID == "" {
		return fmt.Errorf("mode A did not reserve a Connector identity")
	}
	if _, err = a.execP4Target(ctx, env, scenario.CommandTarget, "/bin/bash", "-lc",
		"set -eu; nohup socat TCP4-LISTEN:8445,bind=0.0.0.0,reuseaddr,fork EXEC:/bin/cat >/tmp/argus-relay-conflict.log 2>&1 & echo $! >/tmp/argus-relay-conflict.pid; for attempt in $(seq 1 20); do ss -ltn | grep -q ':8445 ' && exit 0; sleep 0.1; done; exit 1"); err != nil {
		return fmt.Errorf("reserve preferred Bastion relay port: %w", err)
	}
	if _, err = a.execP4Target(ctx, env, scenario.CommandTarget, "/bin/bash", "-lc", command); err != nil {
		return err
	}
	if err = a.waitM3ConnectorOnline(ctx, env, connectorID, 1); err != nil {
		return err
	}
	if err = a.waitPostgresValue(ctx, env,
		"SELECT status || '|' || onboarding_mode || '|' || relay_status FROM bastion_scopes WHERE id='"+scopeID+"';", "active|command|ready", 2*time.Minute); err != nil {
		return err
	}
	scope, err := client.JSON(ctx, "p4-command-bastion", "enterprise", http.MethodGet,
		"/enterprise/bastion-scopes/"+scopeID, http.StatusOK, nil, map[string]string{"Origin": env.EnterpriseOrigin()})
	if err != nil {
		return err
	}
	onboarding, ok := scope["onboarding"].(map[string]any)
	if !ok || onboarding["state"] != "registered" {
		return fmt.Errorf("mode A onboarding projection did not converge to registered: %#v", scope["onboarding"])
	}
	hostID, _ := scope["connector_host_id"].(string)
	relayAddress, _ := scope["relay_address"].(string)
	relayHTTPSPort, httpsOK := scope["relay_https_port"].(float64)
	relayGatewayPort, gatewayOK := scope["relay_gateway_port"].(float64)
	relayGeneration, generationOK := scope["relay_port_generation"].(float64)
	if hostID == "" || relayAddress == "" || !httpsOK || !gatewayOK || !generationOK || int(relayGeneration) != 1 {
		return fmt.Errorf("mode A omitted its Bastion root Host or reported relay endpoint")
	}
	if int(relayHTTPSPort) <= 8445 || int(relayHTTPSPort) > 8464 || int(relayGatewayPort) != 9445 {
		return fmt.Errorf("mode A did not report deterministic relay port selection: https=%v gateway=%v", relayHTTPSPort, relayGatewayPort)
	}
	scenario.CommandInstall = p4InstallResult{ScopeID: scopeID, ConnectorID: connectorID, HostID: hostID, RelayAddress: relayAddress,
		RelayHTTPSPort: int(relayHTTPSPort), RelayGatewayPort: int(relayGatewayPort)}
	return nil
}

func (a *App) runP4ManualRelayHost(ctx context.Context, env *E2EEnvironment, scenario *p4Scenario) error {
	if scenario.CommandInstall.ScopeID == "" {
		return fmt.Errorf("command-installed Bastion is unavailable")
	}
	client, _ := scenarioHTTP(env)
	preview, err := client.JSON(ctx, "p4-relay-host-preview", "enterprise", http.MethodPost,
		"/enterprise/hosts/actions/preview-create", http.StatusCreated, map[string]any{
			"name": "p4-relay-manual-host", "platform": "linux", "architecture": strings.TrimPrefix(env.ImagePlatform, "linux/"),
			"role": "managed_host", "control_path": "bastion_relay", "bastion_scope_id": scenario.CommandInstall.ScopeID,
			"install_method": "manual", "ssh_path": "none", "environment": "production",
			"labels": map[string]string{"suite": "p4", "mode": "manual-relay"},
		}, enterpriseHeaders(env, "p4-relay-host-preview"))
	if err != nil {
		return err
	}
	actionRef, err := stringField(preview, "action_ref")
	if err != nil {
		return err
	}
	confirmed, err := a.confirmPendingAction(ctx, env, "p4-relay-host-confirm", actionRef)
	if err != nil {
		return err
	}
	hostID, err := stringField(confirmed, "resource_ref", "resource_id")
	if err != nil {
		return err
	}
	result, ok := confirmed["one_time_result"].(map[string]any)
	if !ok {
		return fmt.Errorf("manual relay Host did not return a one-time result")
	}
	command, err := validateP4OneTimeResult(result, "connector_install_command", confirmed)
	if err != nil {
		return err
	}
	dialAddress := net.JoinHostPort(scenario.CommandInstall.RelayAddress, strconv.Itoa(scenario.CommandInstall.RelayHTTPSPort))
	if !strings.Contains(command, "--connect-to") || !strings.Contains(command, dialAddress) {
		return fmt.Errorf("manual relay Host command omitted its frozen Bastion HTTPS path")
	}
	if _, err = a.execP4Target(ctx, env, scenario.RelayHostTarget, "/bin/bash", "-lc", command); err != nil {
		return err
	}
	if err = a.verifyP4HostOnboardingTimeline(ctx, env, hostID); err != nil {
		return err
	}
	if _, err = a.execP4Target(ctx, env, scenario.RelayHostTarget, "/bin/bash", "-lc",
		"timeout 3 bash -c '</dev/tcp/"+env.Endpoints.IngressIP+"/443'"); err == nil {
		return fmt.Errorf("manual relay Host retained direct platform egress")
	}
	return nil
}

func (a *App) runP4DirectHost(ctx context.Context, env *E2EEnvironment, scenario *p4Scenario) error {
	testID, err := a.createP4ConnectionTest(ctx, env, "p4-direct-host", scenario.DirectHostTarget.ExternalIP, scenario.CredentialID, "", "direct")
	if err != nil {
		return err
	}
	client, _ := scenarioHTTP(env)
	preview, err := client.JSON(ctx, "p4-direct-host-preview", "enterprise", http.MethodPost,
		"/enterprise/hosts/actions/preview-create", http.StatusCreated, map[string]any{
			"name": "p4-direct-ssh-host", "address": scenario.DirectHostTarget.ExternalIP, "port": 22, "platform": "linux",
			"role": "managed_host", "control_path": "direct", "install_method": "ssh", "ssh_path": "direct_executor",
			"credential_id": scenario.CredentialID, "username": "root", "connection_test_id": testID,
			"environment": "production", "labels": map[string]string{"suite": "p4", "mode": "direct-host"},
		}, enterpriseHeaders(env, "p4-direct-host-preview"))
	if err != nil {
		return err
	}
	actionRef, err := stringField(preview, "action_ref")
	if err != nil {
		return err
	}
	confirmed, err := a.confirmPendingAction(ctx, env, "p4-direct-host-confirm", actionRef)
	if err != nil {
		return err
	}
	hostID, err := stringField(confirmed, "resource_ref", "resource_id")
	if err != nil {
		return err
	}
	scenario.DirectHostID = hostID
	if err = a.verifyP4HostOnboardingTimeline(ctx, env, hostID); err != nil {
		return err
	}
	collectorCount, err := a.postgresQuery(ctx, env,
		"SELECT count(*) FROM collector_instances WHERE resource_type='host' AND resource_id='"+hostID+"';")
	if err != nil || strings.TrimSpace(collectorCount) != "0" {
		return fmt.Errorf("Direct SSH Host first install unexpectedly created a Collector: %q, %v", collectorCount, err)
	}
	return nil
}
