package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The legacy Host's OS lives in the Direct Executor's test sidecar and is
// replaced by the production rollout. Create this scenario's fresh Host using
// actual connection testing, Preview, confirmation and Connector enrollment.
func (a *App) preparePlan6IdentityHost(ctx context.Context, env *E2EEnvironment) (string, error) {
	if env.Options.Suite != "plan6" || env.State.Values["p6_backend_build"] != "production" {
		return "", fmt.Errorf("identity Host requires the normal isolated Plan6 backend")
	}
	ns, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, env.SystemNS, metav1.GetOptions{})
	if err != nil || ns.Labels["argus.io/release-id"] != env.ReleaseID {
		return "", fmt.Errorf("identity Host namespace ownership is not established: %v", err)
	}
	old, enterprise, credential := env.State.Values["m7_host_id"], env.State.Values["enterprise_id"], env.State.Values["m3_credential_id"]
	for _, id := range []string{old, enterprise, credential} {
		if uuid.Validate(id) != nil {
			return "", fmt.Errorf("identity Host prerequisite reference missing")
		}
	}
	previous, err := a.postgresQuery(ctx, env, fmt.Sprintf(`SELECT json_build_object('host_id',h.id,'host_status',h.status,'connection_status',h.connection_status,'connector_id',h.connector_id,'connector_status',c.status,'connection_epoch',c.connection_epoch) FROM hosts h LEFT JOIN connectors c ON c.id=h.connector_id WHERE h.id='%s' AND h.enterprise_id='%s';`, old, enterprise))
	if err != nil {
		return "", err
	}
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan6-identity-host-previous.json"), []byte(strings.TrimSpace(previous)+"\n")); err != nil {
		return "", err
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return "", err
	}
	connection, err := client.JSON(ctx, "p6-identity-host-connection", "enterprise", http.MethodPost, "/enterprise/hosts/connection-tests", http.StatusAccepted,
		map[string]any{"address": "8.8.8.8", "port": 22, "platform": "linux", "onboarding_control_path": "direct", "ssh_path": "direct_executor", "credential_id": credential, "username": "root"}, enterpriseHeaders(env, "p6-identity-host-connection"))
	if err != nil {
		return "", err
	}
	connectionID, err := stringField(connection, "id")
	if err != nil {
		return "", err
	}
	if err = a.waitConnectionTest(ctx, env, connectionID); err != nil {
		return "", err
	}
	preview, err := client.JSON(ctx, "p6-identity-host-preview", "enterprise", http.MethodPost, "/enterprise/hosts/actions/preview-create", http.StatusCreated,
		map[string]any{"name": "Plan6 identity Host " + env.Options.RunID, "address": "8.8.8.8", "port": 22, "platform": "linux", "role": "managed_host", "control_path": "direct",
			"install_method": "ssh", "ssh_path": "direct_executor", "credential_id": credential, "username": "root", "environment": "production",
			"labels": map[string]string{"team": "plan6", "fixture": "source-identity"}, "connection_test_id": connectionID}, enterpriseHeaders(env, "p6-identity-host-create"))
	if err != nil {
		return "", err
	}
	ref, err := stringField(preview, "action_ref")
	if err != nil {
		return "", err
	}
	confirmed, err := a.confirmPendingAction(ctx, env, "p6-identity-host-confirm", ref)
	if err != nil {
		return "", err
	}
	host, err := stringField(confirmed, "resource_ref", "resource_id")
	if err != nil || uuid.Validate(host) != nil {
		return "", fmt.Errorf("identity Host confirmation omitted its resource: %v", err)
	}
	if err = a.waitPostgresValue(ctx, env, "SELECT h.connection_status || '|' || c.status || '|' || o.status FROM hosts h JOIN connectors c ON c.id=h.connector_id JOIN host_onboarding_operations o ON o.host_id=h.id WHERE h.id='"+host+"' AND h.enterprise_id='"+enterprise+"';", "online|online|succeeded", 5*time.Minute); err != nil {
		return "", err
	}
	if err = a.refreshEnterpriseLogin(ctx, env); err != nil {
		return "", err
	}
	env.State.Values["p6_identity_host_id"] = host
	report, _ := json.MarshalIndent(map[string]any{"passed": true, "scope": "normal backend, real HTTP onboarding and controlled SSH Host", "namespace_uid": ns.UID,
		"old_host_state": json.RawMessage(strings.TrimSpace(previous)), "new_host_id": host, "connection_test_id": connectionID, "action_ref": ref,
		"execution": confirmed["execution"], "model_acceptance": false}, "", "  ")
	return host, writePrivate(filepath.Join(env.Options.Artifacts, "plan6-identity-host.json"), report)
}
