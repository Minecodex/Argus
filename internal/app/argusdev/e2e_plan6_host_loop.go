package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

type plan6HostLoop struct {
	Docker                                                       *plan6DockerFixture
	HostID, HostName, ScopeID, RootHostID, CollectorID, SourceID string
	DashboardID, PolicyID, ModelID                               string
	CPUExpression, MemoryExpression                              string
	BaselineCPU, TriggerCPU                                      float64
	Channels                                                     []string
	ConditionIDs                                                 []string
	Cases                                                        []map[string]any
}

func (a *App) runPlan6HostLoop(ctx context.Context, env *E2EEnvironment) (failure error) {
	if env.Options.RealModel == nil || env.State.Values["p6_backend_build"] != "production" {
		return fmt.Errorf("Docker Host loop requires normal backend and a real model")
	}
	f, err := a.loadPlan6DockerFixture(ctx, env.Options.Plan6HostFixtures)
	if err != nil {
		return err
	}
	h := &plan6HostLoop{Docker: f, HostName: "Plan6 Docker monitored Host", CPUExpression: `100 * (1 - avg by (argus_resource_id,argus_source_id) (system_cpu_utilization{state="idle"}))`, MemoryExpression: `100 * system_memory_utilization{state="used"}`}
	defer func() {
		data, _ := json.MarshalIndent(map[string]any{"scope": "real Docker Linux Host -> Bastion -> managed Collector -> Query -> dashboard/alerts -> real protocol receivers -> GLM", "passed": failure == nil, "host_id": h.HostID, "bastion_scope_id": h.ScopeID, "collector_id": h.CollectorID, "source_id": h.SourceID, "dashboard_id": h.DashboardID, "policy_id": h.PolicyID, "model_id": h.ModelID, "cases": h.Cases, "shared_linux_kernel": true, "synthetic_metric_injection": false, "external_groups_mailbox": false}, "", "  ")
		failure = errors.Join(failure, writePrivate(filepath.Join(env.Options.Artifacts, "plan6-host-loop.json"), data))
	}()
	for _, step := range []struct {
		name string
		run  func(context.Context, *E2EEnvironment, *plan6HostLoop) error
	}{
		{"docker-onboarding", a.plan6HostOnboarding},
		{"real-hostmetrics-dashboard", a.plan6HostMetricsDashboard},
		{"alert-handling-noise-notifications", a.plan6HostAlertLoop},
		{"real-glm-host-alert-dashboard", a.plan6HostModelLoop},
		{"real-recovery-history", a.plan6HostRecovery},
	} {
		_, _ = fmt.Fprintln(a.stdout, "Plan6 Docker Host stage", step.name, "started")
		if failure = step.run(ctx, env, h); failure != nil {
			return fmt.Errorf("%s: %w", step.name, failure)
		}
		h.Cases = append(h.Cases, map[string]any{"name": step.name, "passed": true})
		_, _ = fmt.Fprintln(a.stdout, "Plan6 Docker Host stage", step.name, "passed")
	}
	return a.plan6HostBrowser(ctx, env, h)
}

func (a *App) plan6HostOnboarding(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop) error {
	if err := a.preparePlan6DockerRouting(ctx, env, h.Docker); err != nil {
		return err
	}
	client, _ := scenarioHTTP(env)
	scope, err := a.createM3Bastion(ctx, env, "docker-bastion", "docker-host-loop")
	if err != nil {
		return err
	}
	h.ScopeID = scope
	command := env.State.Values["docker_bastion_install_command"]
	if command == "" {
		return fmt.Errorf("owned Bastion bootstrap command absent")
	}
	// A multi-homed customer Bastion advertises the interface reachable by
	// members. Configure it before enrollment so local state and heartbeat agree.
	setup := "mkdir -p /etc/systemd/system/argus-connector.service.d\n" +
		"printf '%s\\n' '[Service]' 'Environment=ARGUS_BASTION_RELAY_ADVERTISE_ADDRESS=" + h.Docker.BastionIP + "' > /etc/systemd/system/argus-connector.service.d/relay-address.conf\n" +
		"systemctl daemon-reload\n"
	if _, err = a.plan6DockerScript(ctx, h.Docker, h.Docker.Bastion, setup); err != nil {
		return err
	}
	if _, err = a.plan6DockerScript(ctx, h.Docker, h.Docker.Bastion, command+"\n"); err != nil {
		return err
	}
	var root map[string]any
	deadline := time.Now().Add(3 * time.Minute)
	for {
		root, err = client.JSON(ctx, "host-loop-bastion", "enterprise", http.MethodGet, "/enterprise/bastion-scopes/"+scope, 200, nil, enterpriseHeaders(env, ""))
		if err != nil {
			return err
		}
		if root["relay_status"] == "ready" {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("Docker Bastion relay not ready")
		}
		if err = waitContext(ctx, time.Second); err != nil {
			return err
		}
	}
	h.RootHostID, _ = stringField(root, "connector_host_id")
	advertised, _ := root["relay_address"].(string)
	peer, err := a.plan6InspectDocker(ctx, h.Docker.Bastion, h.Docker.Owner)
	if err != nil {
		return err
	}
	ownedAddress := false
	for _, network := range peer.NetworkSettings.Networks {
		ownedAddress = ownedAddress || network.IP == advertised
	}
	if net.ParseIP(advertised) == nil || !ownedAddress || advertised != h.Docker.BastionIP {
		return fmt.Errorf("Bastion did not advertise its configured member-reachable interface")
	}
	secret, err := client.JSON(ctx, "host-loop-ssh-secret", "enterprise", http.MethodPost, "/enterprise/secrets", 201, map[string]any{"name": "Docker Host SSH fixture", "type": "ssh_password", "value": "M3-e2e-ssh-password"}, enterpriseHeaders(env, "host-loop-secret"))
	if err != nil {
		return err
	}
	sid, _ := stringField(secret, "id")
	cred, err := client.JSON(ctx, "host-loop-ssh-credential", "enterprise", http.MethodPost, "/enterprise/credentials", 201, map[string]any{"name": "Docker Host SSH fixture", "protocol": "ssh", "username": "root", "secret_id": sid}, enterpriseHeaders(env, "host-loop-credential"))
	if err != nil {
		return err
	}
	cid, _ := stringField(cred, "id")
	test, err := a.createP4ConnectionTest(ctx, env, "docker-host", h.Docker.HostIP, cid, scope, "bastion_relay")
	if err != nil {
		return err
	}
	preview, err := client.JSON(ctx, "host-loop-create-preview", "enterprise", http.MethodPost, "/enterprise/hosts/actions/preview-create", 201, map[string]any{"name": h.HostName, "address": h.Docker.HostIP, "port": 22, "platform": "linux", "role": "managed_host", "control_path": "bastion_relay", "install_method": "ssh", "ssh_path": "bastion_connector", "credential_id": cid, "username": "root", "bastion_scope_id": scope, "connection_test_id": test, "environment": "development", "labels": map[string]string{"suite": "plan6-host", "run": env.Options.RunID}}, enterpriseHeaders(env, "host-loop-create"))
	if err != nil {
		return err
	}
	ref, _ := stringField(preview, "action_ref")
	result, err := a.confirmPendingAction(ctx, env, "host-loop-create", ref)
	if err != nil {
		return err
	}
	h.HostID, _ = stringField(result, "resource_ref", "resource_id")
	if uuid.Validate(h.HostID) != nil {
		return fmt.Errorf("Docker Host identity invalid")
	}
	if err = a.refreshEnterpriseLogin(ctx, env); err != nil {
		return err
	}
	if err = a.verifyP4HostOnboardingTimeline(ctx, env, h.HostID); err != nil {
		return err
	}
	distribution, profiles, _, err := a.verifyM7Catalog(ctx, env)
	if err != nil {
		return err
	}
	collector, err := a.applyP4HostCollector(ctx, env, "docker-host", h.HostID, distribution, profiles, "bastion_gateway", "bastion_tunnel", "", 14318)
	if err != nil {
		return err
	}
	h.CollectorID, _ = stringField(collector, "id")
	query := fmt.Sprintf("SELECT json_build_object('host_control_path',h.control_path,'host_connector',h.connector_id,'scope',h.bastion_scope_id,'collector',c.id,'collector_status',c.status,'route',r.kind,'transport',r.transport,'gateway',r.gateway_collector_id,'gateway_status',g.status,'gateway_role',g.role,'source',(SELECT id FROM telemetry_sources s WHERE s.collector_id=c.id AND s.source_type='hostmetrics' ORDER BY created_at DESC LIMIT 1)) FROM hosts h JOIN collector_instances c ON c.resource_id=h.id AND c.resource_type='host' JOIN telemetry_routes r ON r.collector_id=c.id JOIN collector_instances g ON g.id=r.gateway_collector_id WHERE h.id='%s' AND h.enterprise_id='%s' AND c.id='%s';", h.HostID, env.State.Values["enterprise_id"], h.CollectorID)
	raw, err := a.postgresQuery(ctx, env, query)
	if err != nil {
		return err
	}
	var facts map[string]any
	if json.Unmarshal([]byte(raw), &facts) != nil || facts["host_control_path"] != "bastion_relay" || facts["route"] != "bastion_gateway" || facts["transport"] != "bastion_tunnel" || facts["gateway_status"] != "converged" || facts["gateway_role"] != "edge_gateway" {
		return fmt.Errorf("Host monitoring route is not the committed Bastion path")
	}
	h.SourceID, _ = facts["source"].(string)
	if uuid.Validate(h.SourceID) != nil {
		return fmt.Errorf("managed host telemetry source missing")
	}
	return writePrivate(filepath.Join(env.Options.Artifacts, "host-collector-route.json"), []byte(strings.TrimSpace(raw)))
}
