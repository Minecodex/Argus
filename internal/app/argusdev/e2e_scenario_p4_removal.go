package argusdev

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Run after browser and telemetry checks so removal cannot invalidate the
// fixtures those checks still need. Every request goes through Preview/Commit.
func (a *App) verifyP4SSHRemovals(ctx context.Context, env *E2EEnvironment, scenario *p4Scenario) error {
	targets := []struct {
		name, kind, id, hostID, scope string
		target                        p4Target
	}{
		{"direct-host", "managed_host", scenario.DirectHostID, scenario.DirectHostID, "", scenario.DirectHostTarget},
		{"executor-host", "managed_host", scenario.ExecutorHostID, scenario.ExecutorHostID, "", scenario.ExecutorHostTarget},
		{"member-host", "managed_host", scenario.MemberHostID, scenario.MemberHostID, scenario.TunnelInstall.ScopeID, scenario.MemberTarget},
		{"direct-bastion", "bastion_scope", scenario.DirectInstall.ScopeID, scenario.DirectInstall.HostID, "", scenario.DirectTarget},
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if target.id == "" || target.hostID == "" {
			return fmt.Errorf("missing removal fixture %s", target.name)
		}
		path := "/enterprise/hosts/" + target.id
		if target.kind == "bastion_scope" {
			path = "/enterprise/bastion-scopes/" + target.id
		}
		current, err := client.JSON(ctx, "remove-"+target.name+"-resource", "enterprise", http.MethodGet, path, http.StatusOK, nil, map[string]string{"Origin": env.EnterpriseOrigin()})
		if err != nil {
			return err
		}
		version, err := numberField(current, "resource_version")
		if err != nil {
			return err
		}
		query := url.Values{"target_type": {target.kind}, "target_id": {target.id}, "expected_version": {strconv.FormatInt(version, 10)}}
		defaults, err := client.JSON(ctx, "remove-"+target.name+"-saved-connection", "enterprise", http.MethodGet,
			"/enterprise/host-removals/connection-defaults?"+query.Encode(), http.StatusOK, nil, map[string]string{"Origin": env.EnterpriseOrigin()})
		if err != nil {
			return err
		}
		credentialID, _ := defaults["credential_id"].(string)
		username, _ := defaults["username"].(string)
		if defaults["status"] != "available" || credentialID != scenario.CredentialID || username != "root" {
			return fmt.Errorf("%s did not reuse its installation SSH references", target.name)
		}
		for key := range defaults {
			if key != "status" && key != "username" && key != "credential_id" && key != "credential_name" {
				return fmt.Errorf("%s connection defaults exposed an unexpected field %s", target.name, key)
			}
		}
		connectionBody := map[string]any{"address": target.target.ExternalIP, "port": 22, "platform": "linux",
			"ssh_path": "direct_executor", "credential_id": credentialID, "username": username}
		if target.scope != "" {
			connectionBody["ssh_path"], connectionBody["bastion_scope_id"] = "bastion_connector", target.scope
		}
		connection, err := client.JSON(ctx, "remove-"+target.name+"-connection-test", "enterprise", http.MethodPost,
			"/enterprise/hosts/connection-tests", http.StatusAccepted, connectionBody, enterpriseHeaders(env, "remove-"+target.name+"-connection-test"))
		if err != nil {
			return err
		}
		testID, err := stringField(connection, "id")
		if err != nil {
			return err
		}
		if err = a.waitConnectionTest(ctx, env, testID); err != nil {
			return err
		}
		preview, err := client.JSON(ctx, "remove-"+target.name+"-preview", "enterprise", http.MethodPost, "/enterprise/host-removals/actions/preview", http.StatusCreated, map[string]any{"target_type": target.kind, "target_id": target.id, "expected_version": version, "mode": "uninstall", "credential_id": credentialID, "connection_test_id": testID}, enterpriseHeaders(env, "remove-"+target.name+"-preview"))
		if err != nil {
			return err
		}
		ref, err := stringField(preview, "action_ref")
		if err != nil {
			return err
		}
		if _, err = a.confirmPendingAction(ctx, env, "remove-"+target.name+"-confirm", ref); err != nil {
			return err
		}
		if err = a.waitPostgresValue(ctx, env, "SELECT status || '|' || local_cleanup FROM hosts WHERE id='"+target.hostID+"';", "uninstalled|verified", 4*time.Minute); err != nil {
			return fmt.Errorf("%s removal: %w", target.name, err)
		}
		output, err := a.execP4Target(ctx, env, target.target, "/bin/sh", "-ec", "test ! -e /usr/local/bin/argus-connector; test ! -e /var/lib/argus-connector; test ! -e /usr/local/bin/argus-otelcol; systemctl is-active ssh; ! id argus-connector >/dev/null 2>&1")
		if err != nil || !strings.Contains(output, "active") {
			return fmt.Errorf("%s cleanup or SSH preservation failed: %v", target.name, err)
		}
	}
	return nil
}
