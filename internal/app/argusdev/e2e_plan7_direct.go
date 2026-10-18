package argusdev

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kakj-go/Argus/internal/remoteexecution"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// Use the real cluster API, production Direct Executor and a namespace-scoped
// identity. No alternate dialer, fake public endpoint or Connector is involved.
func (a *App) plan7DirectKubernetes(ctx context.Context, env *E2EEnvironment) error {
	targetNS, err := a.plan7TargetNamespace(ctx, env)
	if err != nil {
		return err
	}
	const name, endpoint = "argus-plan7-direct", "https://kubernetes.default.svc"
	labels := map[string]string{"argus.io/release-id": env.ReleaseID, "app.kubernetes.io/part-of": "argus-e2e"}
	no := false
	if _, err := env.Kube.Client.CoreV1().ServiceAccounts(targetNS).Create(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: targetNS, Labels: labels}, AutomountServiceAccountToken: &no}, metav1.CreateOptions{}); err != nil {
		return err
	}
	rules := []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"pods", "configmaps"}, Verbs: []string{"get", "list", "create", "update", "patch", "delete"}},
		{APIGroups: []string{""}, Resources: []string{"pods/exec"}, Verbs: []string{"create"}},
		{APIGroups: []string{"networking.k8s.io"}, Resources: []string{"networkpolicies"}, Verbs: []string{"get", "list", "create", "update", "delete"}},
	}
	if _, err := env.Kube.Client.RbacV1().Roles(targetNS).Create(ctx, &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: targetNS, Labels: labels}, Rules: rules}, metav1.CreateOptions{}); err != nil {
		return err
	}
	if _, err := env.Kube.Client.RbacV1().RoleBindings(targetNS).Create(ctx, &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: targetNS, Labels: labels}, Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: name, Namespace: targetNS}}, RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: name}}, metav1.CreateOptions{}); err != nil {
		return err
	}
	expiration := int64(3600)
	token, err := env.Kube.Client.CoreV1().ServiceAccounts(targetNS).CreateToken(ctx, name, &authv1.TokenRequest{Spec: authv1.TokenRequestSpec{ExpirationSeconds: &expiration}}, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	ca := env.Kube.Config.CAData
	if len(ca) == 0 && env.Kube.Config.CAFile != "" {
		ca, err = os.ReadFile(env.Kube.Config.CAFile)
		if err != nil {
			return err
		}
	}
	config, err := yaml.Marshal(map[string]any{
		"apiVersion": "v1", "kind": "Config", "current-context": name,
		"clusters": []any{map[string]any{"name": name, "cluster": map[string]any{"server": endpoint, "certificate-authority-data": base64.StdEncoding.EncodeToString(ca)}}},
		"contexts": []any{map[string]any{"name": name, "context": map[string]any{"cluster": name, "user": name, "namespace": targetNS}}},
		"users":    []any{map[string]any{"name": name, "user": map[string]any{"token": token.Status.Token}}},
	})
	if err != nil {
		return err
	}
	defer clear(config)
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	secret, err := client.JSON(ctx, "p7-direct-secret", "enterprise", http.MethodPost, "/enterprise/secrets", 201, map[string]any{"name": name, "type": "kubeconfig", "description": "Plan7 namespace-scoped direct acceptance", "value": string(config)}, enterpriseHeaders(env, "p7-direct-secret"))
	if err != nil {
		return err
	}
	secretID, err := stringField(secret, "id")
	if err != nil {
		return err
	}
	credential, err := client.JSON(ctx, "p7-direct-credential", "enterprise", http.MethodPost, "/enterprise/credentials", 201, map[string]any{"name": name, "protocol": "kubernetes", "secret_id": secretID}, enterpriseHeaders(env, "p7-direct-credential"))
	if err != nil {
		return err
	}
	credentialID, err := stringField(credential, "id")
	if err != nil {
		return err
	}
	probe, err := client.JSON(ctx, "p7-direct-test", "enterprise", http.MethodPost, "/enterprise/kubernetes-clusters/connection-tests", 202, map[string]any{"api_server": endpoint, "connection_mode": "direct", "credential_id": credentialID}, enterpriseHeaders(env, "p7-direct-test"))
	if err != nil {
		return err
	}
	probeID, err := stringField(probe, "id")
	if err != nil {
		return err
	}
	if err = a.waitConnectionTest(ctx, env, probeID); err != nil {
		return err
	}
	preview, err := client.JSON(ctx, "p7-direct-cluster-preview", "enterprise", http.MethodPost, "/enterprise/kubernetes-clusters/actions/preview-create", 201, map[string]any{"name": name, "api_server": endpoint, "connection_mode": "direct", "credential_id": credentialID, "default_namespace": targetNS, "environment": "development", "labels": map[string]string{"plan": "plan7"}, "connection_test_id": probeID}, enterpriseHeaders(env, "p7-direct-cluster-preview"))
	if err != nil {
		return err
	}
	ref, err := stringField(preview, "action_ref")
	if err != nil {
		return err
	}
	confirmed, err := a.confirmPendingAction(ctx, env, "p7-direct-cluster-confirm", ref)
	if err != nil {
		return err
	}
	cluster, err := stringField(confirmed, "resource_ref", "resource_id")
	if err != nil {
		return err
	}
	if err = a.refreshEnterpriseLogin(ctx, env); err != nil {
		return err
	}
	conversation, err := a.p5Conversation(ctx, env, "Plan7 production Direct Executor")
	if err != nil {
		return err
	}
	step := func(script string) map[string]any {
		return map[string]any{"description": "Approved namespace-scoped Direct Executor operation", "target": map[string]any{"resource_type": "kubernetes_cluster", "resource_id": cluster, "namespace": targetNS}, "interpreter": "kubectl", "script": script, "timeout_seconds": 60}
	}
	steps := []p5ToolStep{
		plan7PreviewTool("connect", step("p7_value=41; kubectl create configmap plan7-direct-marker -n "+targetNS+" --from-literal=value=approved; kubectl get configmap plan7-direct-marker -n "+targetNS+" -o json > \"$ARGUS_OUTPUT_DIR/marker.json\"")),
		plan7PreviewTool("connect", step("p7_value=$((p7_value+1)); test \"$p7_value\" = 42; if kubectl get secrets -n kube-system >/dev/null 2>&1; then exit 92; fi; printf '%s' \"$p7_value\" > \"$ARGUS_OUTPUT_DIR/state.txt\"")),
	}
	run, err := a.p5StartRun(ctx, env, conversation, "plan7-direct", steps, []string{})
	if err != nil {
		return err
	}
	for n := 1; n <= 2; n++ {
		view, e := a.plan7WaitRequest(ctx, env, conversation, n, func(v remoteexecution.View) bool { return v.Status == "awaiting_confirmation" }, run)
		if e != nil {
			return e
		}
		if e = a.plan7NoDispatchBeforeApproval(ctx, env, view); e != nil {
			return e
		}
		if e = a.plan7Approve(ctx, env, view); e != nil {
			return e
		}
		result, e := a.plan7WaitRequest(ctx, env, conversation, n, func(v remoteexecution.View) bool {
			return v.ID == view.ID && v.Status != "awaiting_confirmation" && v.Status != "ready" && v.Status != "running"
		})
		if e != nil {
			return e
		}
		if e = a.plan7CompletedEvidence(ctx, env, conversation, fmt.Sprintf("direct-kubernetes-%d", n), result); e != nil {
			return e
		}
	}
	if err = a.waitRunTerminal(ctx, env, run); err != nil {
		return err
	}
	count, err := a.postgresQuery(ctx, env, "SELECT count(*) FROM remote_execution_direct_commands WHERE command_type='remote_execution_batch' AND status='succeeded' AND payload->>'requestId' IN (SELECT id::text FROM remote_execution_requests WHERE run_id='"+run+"');")
	if err != nil || strings.TrimSpace(count) != "2" {
		return fmt.Errorf("Direct Executor receipt count mismatch: %s %v", count, err)
	}
	marker, err := env.Kube.Client.CoreV1().ConfigMaps(targetNS).Get(ctx, "plan7-direct-marker", metav1.GetOptions{})
	if err != nil || marker.Data["value"] != "approved" {
		return fmt.Errorf("Direct Executor effect missing: %v", err)
	}
	return a.waitPostgresValue(ctx, env, "SELECT count(*) FROM connectors WHERE kubernetes_cluster_id='"+cluster+"';", "0", time.Second)
}
