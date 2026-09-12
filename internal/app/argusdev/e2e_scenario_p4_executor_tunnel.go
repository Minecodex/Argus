package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type p4ExecutorTunnelRetryFacts struct {
	PreviousOperationID  string `json:"previous_operation_id"`
	RetryOf              string `json:"retry_of"`
	PreviousConnectorID  string `json:"previous_connector_id"`
	CurrentConnectorID   string `json:"current_connector_id"`
	PreviousTunnelID     string `json:"previous_tunnel_id"`
	CurrentTunnelID      string `json:"current_tunnel_id"`
	PreviousStatus       string `json:"previous_status"`
	PreviousDropReason   string `json:"previous_drop_reason"`
	PreviousLeaseOwner   string `json:"previous_lease_owner"`
	ActivePreviousLeases int    `json:"active_previous_leases"`
	CurrentStatus        string `json:"current_status"`
	CurrentConnector     string `json:"current_connector"`
}

func validateP4ExecutorTunnelRetryFacts(facts p4ExecutorTunnelRetryFacts) error {
	if facts.PreviousOperationID == "" || facts.RetryOf != facts.PreviousOperationID {
		return fmt.Errorf("retry operation omitted its exact predecessor")
	}
	if facts.PreviousConnectorID == "" || facts.CurrentConnectorID == "" || facts.PreviousConnectorID == facts.CurrentConnectorID {
		return fmt.Errorf("retry operation did not rotate the pre-enrollment Connector identity")
	}
	if facts.PreviousTunnelID == "" || facts.CurrentTunnelID == "" || facts.PreviousTunnelID == facts.CurrentTunnelID {
		return fmt.Errorf("retry operation did not create a distinct control tunnel")
	}
	if facts.PreviousStatus != "removed" || facts.PreviousDropReason != "host_onboarding_retry" || facts.PreviousLeaseOwner != "" || facts.ActivePreviousLeases != 0 {
		return fmt.Errorf("previous control tunnel was not fully retired: status=%s reason=%s owner=%q active_leases=%d",
			facts.PreviousStatus, facts.PreviousDropReason, facts.PreviousLeaseOwner, facts.ActivePreviousLeases)
	}
	if facts.CurrentStatus != "established" || facts.CurrentConnector != "online" {
		return fmt.Errorf("retry did not converge: tunnel=%s connector=%s", facts.CurrentStatus, facts.CurrentConnector)
	}
	return nil
}

type p4FailedHostOnboarding struct {
	OperationID string
	ConnectorID string
	TunnelID    string
}

type p4CallbackFailureInjection struct {
	triggerName  string
	functionName string
}

func (a *App) runP4ExecutorTunnelHost(ctx context.Context, env *E2EEnvironment, scenario *p4Scenario) error {
	if _, err := a.execP4Target(ctx, env, scenario.ExecutorHostTarget, "/bin/bash", "-lc",
		"timeout 3 bash -c '</dev/tcp/"+env.Endpoints.IngressIP+"/443'"); err == nil {
		return fmt.Errorf("executor-tunnel Host unexpectedly retained direct platform egress")
	}
	testID, err := a.createP4ConnectionTest(ctx, env, "p4-executor-host-first", scenario.ExecutorHostTarget.ExternalIP, scenario.CredentialID, "")
	if err != nil {
		return err
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	input := p4ExecutorTunnelHostInput(scenario, testID, 0)
	preview, err := client.JSON(ctx, "p4-executor-host-preview", "enterprise", http.MethodPost,
		"/enterprise/hosts/actions/preview-create", http.StatusCreated, input,
		enterpriseHeaders(env, "p4-executor-host-preview"))
	if err != nil {
		return err
	}
	actionRef, err := stringField(preview, "action_ref")
	if err != nil {
		return err
	}
	hostID, err := a.postgresQuery(ctx, env,
		"SELECT resource_id::text FROM pending_actions WHERE action_ref='"+actionRef+"';")
	if err != nil || strings.TrimSpace(hostID) == "" {
		return fmt.Errorf("executor-tunnel preview did not reserve a Host identity: %q, %v", hostID, err)
	}
	hostID = strings.TrimSpace(hostID)
	scenario.ExecutorHostID = hostID
	injection, err := a.installP4CallbackFailureInjection(ctx, env, hostID)
	if err != nil {
		return err
	}
	injectionActive := true
	defer func() {
		if injectionActive {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_ = a.removeP4CallbackFailureInjection(cleanupCtx, env, injection)
		}
	}()
	confirmed, confirmErr := client.JSON(ctx, "p4-executor-host-confirm-failing", "enterprise", http.MethodPost,
		"/enterprise/pending-actions/"+actionRef+"/confirm", http.StatusOK, nil,
		enterpriseHeaders(env, "p4-executor-host-confirm-failing"))
	cleanupErr := a.removeP4CallbackFailureInjection(ctx, env, injection)
	if cleanupErr == nil {
		injectionActive = false
	}
	if confirmErr != nil {
		return confirmErr
	}
	if cleanupErr != nil {
		return cleanupErr
	}
	if confirmedHostID, _ := nestedString(confirmed, "resource_ref", "resource_id"); confirmedHostID != "" && confirmedHostID != hostID {
		return fmt.Errorf("executor-tunnel confirmation changed Host identity from %s to %s", hostID, confirmedHostID)
	}
	failed, err := a.waitP4CallbackFailure(ctx, env, hostID)
	if err != nil {
		return err
	}

	retryTestID, err := a.createP4ConnectionTest(ctx, env, "p4-executor-host-retry", scenario.ExecutorHostTarget.ExternalIP, scenario.CredentialID, "")
	if err != nil {
		return err
	}
	host, err := client.JSON(ctx, "p4-executor-host-resource", "enterprise", http.MethodGet,
		"/enterprise/hosts/"+hostID, http.StatusOK, nil, map[string]string{"Origin": env.EnterpriseOrigin()})
	if err != nil {
		return err
	}
	version, err := numberField(host, "resource_version")
	if err != nil {
		return err
	}
	retryPreview, err := client.JSON(ctx, "p4-executor-host-retry-preview", "enterprise", http.MethodPost,
		"/enterprise/hosts/"+hostID+"/actions/preview-retry", http.StatusCreated,
		p4ExecutorTunnelHostInput(scenario, retryTestID, version),
		enterpriseHeaders(env, "p4-executor-host-retry-preview"))
	if err != nil {
		return err
	}
	retryActionRef, err := stringField(retryPreview, "action_ref")
	if err != nil {
		return err
	}
	if _, err = a.confirmPendingAction(ctx, env, "p4-executor-host-retry-confirm", retryActionRef); err != nil {
		return err
	}
	if err = a.verifyP4HostOnboardingTimeline(ctx, env, hostID); err != nil {
		return err
	}
	facts, currentConnectorID, err := a.loadP4ExecutorTunnelRetryFacts(ctx, env, failed)
	if err != nil {
		return err
	}
	if err = validateP4ExecutorTunnelRetryFacts(facts); err != nil {
		return err
	}
	evidence, err := json.MarshalIndent(facts, "", "  ")
	if err != nil {
		return err
	}
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "p4-executor-tunnel-retry.json"), append(evidence, '\n')); err != nil {
		return err
	}
	if err = a.verifyP4ControlTunnelTakeover(ctx, env, currentConnectorID); err != nil {
		return err
	}
	if output, execErr := a.execP4Target(ctx, env, scenario.ExecutorHostTarget, "/bin/sh", "-ec",
		"systemctl is-active argus-connector.service; test ! -e /var/lib/argus-connector-install/"+failed.ConnectorID); execErr != nil || !strings.Contains(output, "active") {
		return fmt.Errorf("executor-tunnel retry did not leave one active Connector and no failed staging data: %v", execErr)
	}
	return nil
}

func p4ExecutorTunnelHostInput(scenario *p4Scenario, connectionTestID string, expectedVersion int64) map[string]any {
	hostName := scenario.ExecutorHostName
	if hostName == "" {
		hostName = "p4-executor-tunnel-host"
	}
	input := map[string]any{
		"name": hostName, "address": scenario.ExecutorHostTarget.ExternalIP, "port": 22, "platform": "linux",
		"role": "managed_host", "control_path": "executor_tunnel", "install_method": "ssh", "ssh_path": "direct_executor",
		"credential_id": scenario.CredentialID, "username": "root", "connection_test_id": connectionTestID,
		"environment": "production", "labels": map[string]string{"suite": "p4", "mode": "executor-tunnel-host"},
	}
	if expectedVersion > 0 {
		input["expected_version"] = expectedVersion
	}
	return input
}

func (a *App) installP4CallbackFailureInjection(ctx context.Context, env *E2EEnvironment, hostID string) (p4CallbackFailureInjection, error) {
	parsed, err := uuid.Parse(hostID)
	if err != nil {
		return p4CallbackFailureInjection{}, fmt.Errorf("invalid scoped failure Host ID: %w", err)
	}
	suffix := strings.ReplaceAll(parsed.String(), "-", "")[:12]
	injection := p4CallbackFailureInjection{
		triggerName:  "argus_e2e_callback_" + suffix,
		functionName: "argus_e2e_fail_callback_" + suffix,
	}
	statement := "CREATE FUNCTION " + injection.functionName + "() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN NEW.enroll_forward_target='127.0.0.1:1'; RETURN NEW; END $$; " +
		"CREATE TRIGGER " + injection.triggerName + " BEFORE INSERT ON connector_control_tunnels FOR EACH ROW " +
		"WHEN (NEW.host_id='" + parsed.String() + "'::uuid) EXECUTE FUNCTION " + injection.functionName + "();"
	if _, err = a.postgresQuery(ctx, env, statement); err != nil {
		return p4CallbackFailureInjection{}, fmt.Errorf("install scoped callback failure injection: %w", err)
	}
	return injection, nil
}

func (a *App) removeP4CallbackFailureInjection(ctx context.Context, env *E2EEnvironment, injection p4CallbackFailureInjection) error {
	if injection.triggerName == "" || injection.functionName == "" {
		return nil
	}
	_, err := a.postgresQuery(ctx, env, "DROP TRIGGER IF EXISTS "+injection.triggerName+" ON connector_control_tunnels; DROP FUNCTION IF EXISTS "+injection.functionName+"();")
	return err
}

func (a *App) waitP4CallbackFailure(ctx context.Context, env *E2EEnvironment, hostID string) (p4FailedHostOnboarding, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return p4FailedHostOnboarding{}, err
	}
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		row, queryErr := a.postgresQuery(ctx, env,
			"SELECT id::text || '|' || connector_id::text FROM host_onboarding_operations WHERE host_id='"+hostID+"' ORDER BY created_at DESC,id DESC LIMIT 1;")
		if queryErr != nil {
			return p4FailedHostOnboarding{}, queryErr
		}
		parts := strings.Split(strings.TrimSpace(row), "|")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			if err = waitP4Tick(ctx); err != nil {
				return p4FailedHostOnboarding{}, err
			}
			continue
		}
		operation, getErr := client.JSON(ctx, "p4-executor-host-failure", "enterprise", http.MethodGet,
			"/enterprise/host-onboarding-operations/"+parts[0], http.StatusOK, nil,
			map[string]string{"Origin": env.EnterpriseOrigin()})
		if getErr != nil {
			return p4FailedHostOnboarding{}, getErr
		}
		status, _ := operation["status"].(string)
		if status == "failed" {
			code, _ := operation["error_code"].(string)
			if operation["stage"] != "probing" || code != "HOST_ONBOARDING_CALLBACK_CONNECT_FAILED" {
				return p4FailedHostOnboarding{}, fmt.Errorf("callback failure did not fail closed during probing: stage=%v code=%q", operation["stage"], code)
			}
			for _, raw := range p4ObjectSlice(operation["events"]) {
				if raw["stage"] == "transferring" {
					return p4FailedHostOnboarding{}, fmt.Errorf("callback failure transferred the Connector artifact before failing")
				}
			}
			tunnelID, tunnelErr := a.postgresQuery(ctx, env,
				"SELECT id::text FROM connector_control_tunnels WHERE connector_id='"+parts[1]+"';")
			if tunnelErr != nil || strings.TrimSpace(tunnelID) == "" {
				return p4FailedHostOnboarding{}, fmt.Errorf("failed onboarding control tunnel is unavailable: %q, %v", tunnelID, tunnelErr)
			}
			return p4FailedHostOnboarding{OperationID: parts[0], ConnectorID: parts[1], TunnelID: strings.TrimSpace(tunnelID)}, nil
		}
		if status == "succeeded" || status == "expired" || status == "cancelled" {
			return p4FailedHostOnboarding{}, fmt.Errorf("callback failure operation unexpectedly ended as %s", status)
		}
		if err = waitP4Tick(ctx); err != nil {
			return p4FailedHostOnboarding{}, err
		}
	}
	return p4FailedHostOnboarding{}, fmt.Errorf("callback failure operation did not fail during probing")
}

func (a *App) loadP4ExecutorTunnelRetryFacts(ctx context.Context, env *E2EEnvironment, previous p4FailedHostOnboarding) (p4ExecutorTunnelRetryFacts, string, error) {
	current, err := a.postgresQuery(ctx, env,
		"SELECT id::text || '|' || connector_id::text || '|' || coalesce(retry_of::text,'') FROM host_onboarding_operations "+
			"WHERE retry_of='"+previous.OperationID+"' ORDER BY created_at DESC,id DESC LIMIT 1;")
	if err != nil {
		return p4ExecutorTunnelRetryFacts{}, "", err
	}
	currentParts := strings.Split(strings.TrimSpace(current), "|")
	if len(currentParts) != 3 || currentParts[0] == "" || currentParts[1] == "" {
		return p4ExecutorTunnelRetryFacts{}, "", fmt.Errorf("retry operation facts are unavailable: %q", current)
	}
	previousTunnel, err := a.postgresQuery(ctx, env,
		"SELECT status || '|' || coalesce(last_drop_reason,'') || '|' || coalesce(lease_owner,'') FROM connector_control_tunnels WHERE id='"+previous.TunnelID+"';")
	if err != nil {
		return p4ExecutorTunnelRetryFacts{}, "", err
	}
	previousTunnelParts := strings.Split(strings.TrimSpace(previousTunnel), "|")
	if len(previousTunnelParts) != 3 {
		return p4ExecutorTunnelRetryFacts{}, "", fmt.Errorf("previous tunnel retirement facts are unavailable: %q", previousTunnel)
	}
	currentTunnel, err := a.postgresQuery(ctx, env,
		"SELECT id::text || '|' || status FROM connector_control_tunnels WHERE connector_id='"+currentParts[1]+"';")
	if err != nil {
		return p4ExecutorTunnelRetryFacts{}, "", err
	}
	currentTunnelParts := strings.Split(strings.TrimSpace(currentTunnel), "|")
	if len(currentTunnelParts) != 2 {
		return p4ExecutorTunnelRetryFacts{}, "", fmt.Errorf("retry tunnel facts are unavailable: %q", currentTunnel)
	}
	leaseCount, err := a.postgresQuery(ctx, env,
		"SELECT count(*) FROM credential_leases WHERE operation_ref='connector_control_tunnel:"+previous.TunnelID+"' AND status='active';")
	if err != nil {
		return p4ExecutorTunnelRetryFacts{}, "", err
	}
	activeLeases, err := strconv.Atoi(strings.TrimSpace(leaseCount))
	if err != nil {
		return p4ExecutorTunnelRetryFacts{}, "", fmt.Errorf("invalid previous tunnel lease count %q", leaseCount)
	}
	connectorStatus, err := a.postgresQuery(ctx, env,
		"SELECT status FROM connectors WHERE id='"+currentParts[1]+"';")
	if err != nil {
		return p4ExecutorTunnelRetryFacts{}, "", err
	}
	facts := p4ExecutorTunnelRetryFacts{
		PreviousOperationID: previous.OperationID, RetryOf: currentParts[2],
		PreviousConnectorID: previous.ConnectorID, CurrentConnectorID: currentParts[1],
		PreviousTunnelID: previous.TunnelID, CurrentTunnelID: currentTunnelParts[0],
		PreviousStatus: previousTunnelParts[0], PreviousDropReason: previousTunnelParts[1], PreviousLeaseOwner: previousTunnelParts[2],
		ActivePreviousLeases: activeLeases, CurrentStatus: currentTunnelParts[1], CurrentConnector: strings.TrimSpace(connectorStatus),
	}
	return facts, currentParts[1], nil
}

func p4ObjectSlice(value any) []map[string]any {
	items, _ := value.([]any)
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if object, ok := item.(map[string]any); ok {
			result = append(result, object)
		}
	}
	return result
}
