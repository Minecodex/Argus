package argusdev

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// M6 validates remote access over the outbound Host Connector. Windows/RDP is
// exercised by the dedicated Windows VM suite rather than a Linux simulation.
func (a *App) runM6Scenario(ctx context.Context, env *E2EEnvironment) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	if err = a.refreshEnterpriseLogin(ctx, env); err != nil {
		return err
	}
	hostID, credentialID := env.State.Values["m3_direct_host_id"], env.State.Values["m3_credential_id"]
	if hostID == "" || credentialID == "" {
		return fmt.Errorf("M6 Host Connector topology is incomplete")
	}
	account, err := client.JSON(ctx, "m6-managed-account", "enterprise", http.MethodPost, "/enterprise/managed-accounts", http.StatusCreated,
		map[string]any{"host_id": hostID, "username": "root", "privilege_level": "sudo", "credential_id": credentialID, "allowed_protocols": []string{"shell", "ssh"}},
		enterpriseHeaders(env, "m6-managed-account"))
	if err != nil {
		return err
	}
	accountID, err := stringField(account, "id")
	if err != nil {
		return err
	}
	profile, err := client.JSON(ctx, "m6-session-profile", "enterprise", http.MethodPost, "/enterprise/session-profiles", http.StatusCreated,
		map[string]any{"name": "m6-required-recording", "description": "M6 E2E immutable session controls", "max_session_seconds": 3600,
			"idle_timeout_seconds": 600, "recording_mode": "required", "command_audit_mode": "required", "clipboard_mode": "disabled",
			"file_upload_mode": "disabled", "file_download_mode": "disabled", "port_forward_mode": "disabled", "session_share_mode": "disabled",
			"retention_days": 90, "status": "draft"}, enterpriseHeaders(env, "m6-session-profile"))
	if err != nil {
		return err
	}
	profileID, _ := stringField(profile, "id")
	if _, err = client.JSON(ctx, "m6-enable-session-profile", "enterprise", http.MethodPost,
		"/enterprise/session-profiles/"+profileID+"/enable?expected_version=1", http.StatusOK, nil, enterpriseHeaders(env, "m6-enable-session-profile")); err != nil {
		return err
	}
	rule, err := client.JSON(ctx, "m6-access-rule", "enterprise", http.MethodPost, "/enterprise/remote-access-rules", http.StatusCreated,
		map[string]any{"name": "m6-require-mfa", "description": "M6 E2E MFA rule", "priority": 100, "protocols": []string{"shell", "ssh"},
			"actions": []string{"terminal"}, "source_cidrs": []string{}, "time_windows": []any{}, "effects": []string{"require_mfa"},
			"session_profile_id": profileID, "status": "draft"}, enterpriseHeaders(env, "m6-access-rule"))
	if err != nil {
		return err
	}
	ruleID, _ := stringField(rule, "id")
	validFrom := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	validUntil := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	grant, err := client.JSON(ctx, "m6-host-connector-grant", "enterprise", http.MethodPost, "/enterprise/remote-access-grants", http.StatusCreated,
		map[string]any{"subject_type": "user", "subject_id": env.State.Values["admin_user_id"], "host_ids": []string{hostID},
			"managed_account_ids": []string{accountID}, "protocols": []string{"shell", "ssh"}, "actions": []string{"terminal"},
			"valid_from": validFrom, "valid_until": validUntil, "status": "draft"}, enterpriseHeaders(env, "m6-host-connector-grant"))
	if err != nil {
		return err
	}
	grantID, _ := stringField(grant, "id")
	if _, err = client.JSON(ctx, "m6-enable-grant", "enterprise", http.MethodPost,
		"/enterprise/remote-access-grants/"+grantID+"/enable?expected_version=1", http.StatusOK, nil, enterpriseHeaders(env, "m6-enable-grant")); err != nil {
		return err
	}
	baselineRequest, err := client.JSON(ctx, "m6-grant-without-rule-request", "enterprise", http.MethodPost, "/enterprise/remote-access-requests", http.StatusCreated,
		map[string]any{"host_id": hostID, "managed_account_id": accountID, "protocol": "ssh", "action": "terminal", "reason": "M6 E2E grant baseline without rules"},
		enterpriseHeaders(env, "m6-grant-without-rule-request"))
	if err != nil || baselineRequest["status"] != "authorized" {
		return fmt.Errorf("M6 grant-only request did not authorize: %#v, %v", baselineRequest, err)
	}
	baselineRequestID, _ := stringField(baselineRequest, "id")
	if leaseID, leaseErr := a.findM6Lease(ctx, env, baselineRequestID); leaseErr != nil || leaseID == "" {
		return fmt.Errorf("M6 grant-only request did not issue a lease: %w", leaseErr)
	}
	if _, err = client.JSON(ctx, "m6-enable-access-rule", "enterprise", http.MethodPost,
		"/enterprise/remote-access-rules/"+ruleID+"/enable?expected_version=1", http.StatusOK, nil, enterpriseHeaders(env, "m6-enable-access-rule")); err != nil {
		return err
	}
	if err = a.verifyM6MFAResume(ctx, env, hostID, accountID); err != nil {
		return err
	}
	if err = a.verifyM6ApprovalFlow(ctx, env, profileID, hostID, accountID); err != nil {
		return err
	}
	if err = a.stepUpEnterprise(ctx, env); err != nil {
		return err
	}
	sshLeaseID, err := a.verifyM6RemoteSession(ctx, env, m6RemoteCase{name: "ssh", hostID: hostID, accountID: accountID, protocol: "ssh", command: "echo argus-e2e-ok", expect: "argus-e2e-ok"})
	if err != nil {
		return err
	}
	if _, err = a.verifyM6RemoteSession(ctx, env, m6RemoteCase{name: "shell", hostID: hostID, accountID: accountID, protocol: "shell", command: "whoami", expect: "argus-connector"}); err != nil {
		return err
	}
	if err = a.verifyM6TerminatedTicket(ctx, env, sshLeaseID); err != nil {
		return err
	}
	if err = a.verifyM6ObjectStoreFailClosed(ctx, env, sshLeaseID); err != nil {
		return err
	}
	connectorID, err := a.postgresQuery(ctx, env, "SELECT connector_id FROM hosts WHERE id='"+hostID+"';")
	if err != nil || connectorID == "" {
		return fmt.Errorf("M6 Host Connector identity is unavailable: %w", err)
	}
	if err = a.verifyM6CrossGatewayDrain(ctx, env, hostID, accountID, connectorID); err != nil {
		return err
	}
	if err = a.verifyM6ControlPlaneRecovery(ctx, env); err != nil {
		return err
	}
	if err = a.runPlaywright(ctx, env, "e2e/m6-real.spec.ts", map[string]string{
		"ARGUS_M6_E2E": "1", "ARGUS_M6_ENTERPRISE_USERNAME": env.State.Values["enterprise_username"],
		"ARGUS_M6_ENTERPRISE_PASSWORD": env.State.Values["enterprise_password"], "ARGUS_M6_HOST_ID": hostID,
	}); err != nil {
		return err
	}
	_, err = client.JSON(ctx, "m6-disable-grant", "enterprise", http.MethodPost,
		"/enterprise/remote-access-grants/"+grantID+"/disable?expected_version=2", http.StatusOK, nil, enterpriseHeaders(env, "m6-disable-grant"))
	return err
}
