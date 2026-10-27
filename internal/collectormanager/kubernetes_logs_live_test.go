package collectormanager

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/logcollection"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	logsrpc "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/grpc"
	appsv1 "k8s.io/api/apps/v1"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// Uses actual node files, a native Collector, and namespace-only metadata RBAC.
// The authenticated OTLP sink is a component gate, not deployed Argus E2E.
func TestLiveKubernetesLogsRolloutAndCheckpoint(t *testing.T) {
	contextName, nodeContainer := os.Getenv("ARGUS_KUBERNETES_LOGS_CONTEXT"), os.Getenv("ARGUS_KUBERNETES_LOGS_NODE_CONTAINER")
	if contextName == "" || nodeContainer == "" {
		t.Skip("requires explicitly selected disposable Docker-driver Kubernetes context and node")
	}
	binary, err := filepath.Abs(os.Getenv("ARGUS_MONITORING_COLLECTOR_BINARY"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(binary); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	loading := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: contextName})
	rest, err := loading.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	api, err := kubernetes.NewForConfig(rest)
	if err != nil {
		t.Fatal(err)
	}
	docker := func(args ...string) string {
		t.Helper()
		bounded, c := context.WithTimeout(ctx, 45*time.Second)
		defer c()
		out, e := exec.CommandContext(bounded, "docker", args...).CombinedOutput()
		if e != nil {
			t.Fatalf("docker: %v: %s", e, out)
		}
		return strings.TrimSpace(string(out))
	}
	var mounts []struct{ Type, Name, Destination string }
	if err = json.Unmarshal([]byte(docker("inspect", "--format", "{{json .Mounts}}", nodeContainer)), &mounts); err != nil {
		t.Fatal(err)
	}
	nodeVolume := ""
	for _, m := range mounts {
		if m.Type == "volume" && m.Destination == "/var" {
			nodeVolume = m.Name
		}
	}
	if nodeVolume == "" {
		t.Fatal("selected node does not expose its /var volume")
	}
	owner := "plan8-k8s-logs-" + uuid.NewString()[:8]
	namespace, err := api.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: owner, Labels: map[string]string{"argus.owner": owner}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, c := context.WithTimeout(context.Background(), 45*time.Second)
		defer c()
		current, e := api.CoreV1().Namespaces().Get(cleanup, owner, metav1.GetOptions{})
		if apierrors.IsNotFound(e) {
			return
		}
		if e != nil || current.Labels["argus.owner"] != owner {
			t.Errorf("cleanup ownership: %v", e)
			return
		}
		if e = api.CoreV1().Namespaces().Delete(cleanup, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &current.UID}}); e != nil {
			t.Error(e)
			return
		}
		if e = wait.PollUntilContextTimeout(cleanup, 500*time.Millisecond, 40*time.Second, true, func(ctx context.Context) (bool, error) {
			_, e := api.CoreV1().Namespaces().Get(ctx, owner, metav1.GetOptions{})
			return apierrors.IsNotFound(e), nil
		}); e != nil {
			t.Error(e)
		}
	})
	_, err = api.CoreV1().ServiceAccounts(owner).Create(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "metadata-reader"}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = api.RbacV1().Roles(owner).Create(ctx, &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "metadata-reader"}, Rules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list", "watch"}}, {APIGroups: []string{"apps"}, Resources: []string{"replicasets"}, Verbs: []string{"get", "list", "watch"}}, {APIGroups: []string{"batch"}, Resources: []string{"jobs"}, Verbs: []string{"get", "list", "watch"}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = api.RbacV1().RoleBindings(owner).Create(ctx, &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "metadata-reader"}, RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: "metadata-reader"}, Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: "metadata-reader", Namespace: owner}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	expiration := int64(600)
	token, err := api.CoreV1().ServiceAccounts(owner).CreateToken(ctx, "metadata-reader", &authv1.TokenRequest{Spec: authv1.TokenRequestSpec{ExpirationSeconds: &expiration}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	replicas := int32(1)
	grace := int64(1)
	deployment, err := api.AppsV1().Deployments(owner).Create(ctx, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "orders"}, Spec: appsv1.DeploymentSpec{Replicas: &replicas, Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "orders"}}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "orders"}}, Spec: corev1.PodSpec{TerminationGracePeriodSeconds: &grace, Containers: []corev1.Container{{Name: "main", Image: "alpine:3.21", ImagePullPolicy: corev1.PullIfNotPresent, Env: []corev1.EnvVar{{Name: "POD_UID", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.uid"}}}}, Command: []string{"/bin/sh", "-c", `n=0; while true; do echo "plan8-log:${POD_UID}:$n"; n=$((n+1)); sleep 1; done`}}}}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var firstPod corev1.Pod
	if err = wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, 45*time.Second, true, func(ctx context.Context) (bool, error) {
		pods, e := api.CoreV1().Pods(owner).List(ctx, metav1.ListOptions{LabelSelector: "app=orders"})
		if e != nil {
			return false, e
		}
		for _, p := range pods.Items {
			if p.Status.Phase == corev1.PodRunning {
				firstPod = p
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	if firstPod.Spec.NodeName != nodeContainer {
		t.Fatal("selected Kubernetes node and Docker node container differ")
	}
	dir := t.TempDir()
	checkpoint := filepath.Join(dir, "checkpoint")
	if err = os.Mkdir(checkpoint, 0700); err != nil {
		t.Fatal(err)
	}
	caData := rest.CAData
	if len(caData) == 0 && rest.CAFile != "" {
		caData, err = os.ReadFile(rest.CAFile)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(caData) == 0 || rest.Insecure {
		t.Fatal("real API trust root is required")
	}
	serverURL, err := url.Parse(rest.Host)
	if err != nil {
		t.Fatal(err)
	}
	tlsName := serverURL.Hostname()
	if rest.TLSClientConfig.ServerName != "" {
		tlsName = rest.TLSClientConfig.ServerName
	}
	metadataAPI := newPartitionedAPI(t, serverURL.Host)
	serverURL.Host = net.JoinHostPort("host.docker.internal", strconv.Itoa(metadataAPI.listener.Addr().(*net.TCPAddr).Port))
	kubeconfig := clientcmdapi.Config{CurrentContext: "test", Clusters: map[string]*clientcmdapi.Cluster{"test": {Server: serverURL.String(), TLSServerName: tlsName, CertificateAuthorityData: caData}}, AuthInfos: map[string]*clientcmdapi.AuthInfo{"test": {Token: token.Status.Token}}, Contexts: map[string]*clientcmdapi.Context{"test": {Cluster: "test", AuthInfo: "test", Namespace: owner}}}
	kubeconfigRaw, err := clientcmd.Write(kubeconfig)
	if err != nil {
		t.Fatal(err)
	}
	kubeconfigPath := filepath.Join(dir, "kubeconfig")
	if err = os.WriteFile(kubeconfigPath, kubeconfigRaw, 0600); err != nil {
		t.Fatal(err)
	}
	clear(kubeconfigRaw)
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	capture := &nativeLogCapture{token: uuid.NewString()}
	server := grpc.NewServer()
	logsrpc.RegisterLogsServiceServer(server, capture)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	collector, app, service := uuid.NewString(), uuid.NewString(), uuid.NewString()
	rule := logcollection.Config{Scope: "business", StartAt: "beginning", Workload: &logcollection.Workload{NamespaceID: uuid.New(), Namespace: owner, NamespaceUID: string(namespace.UID), Type: "deployment", Name: "orders", UID: string(deployment.UID), Containers: []string{"main"}}}
	ruleRaw, _ := json.Marshal(rule)
	collection := configbundle.Collection{ID: uuid.NewString(), Integration: "k8slog", TargetType: "application", TargetID: app, NamespaceUID: string(namespace.UID), Config: ruleRaw, Enabled: true, Revision: 1, AssignmentEpoch: 1, Service: &configbundle.CollectionService{ID: service, Name: "orders", Environment: "testing"}}
	run := func(revision int) string {
		t.Helper()
		bundle, e := configbundle.Render(configbundle.RenderInput{CollectorID: collector, ResourceID: uuid.NewString(), ResourceType: "kubernetes_cluster", Platform: "linux_amd64", Role: "direct", SourceGeneration: collector, ConfigRevision: 1, RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: "https://control.test/enroll", IngestGRPCEndpoint: "https://ingest.test:4317", IngestHTTPEndpoint: "https://ingest.test", Collections: []configbundle.Collection{collection}})
		if e != nil {
			t.Fatal(e)
		}
		value, e := configbundle.Extract(bundle, "kubernetes_agent")
		if e != nil {
			t.Fatal(e)
		}
		var config map[string]any
		if e = json.Unmarshal(value, &config); e != nil {
			t.Fatal(e)
		}
		config["extensions"] = map[string]any{"file_storage": map[string]any{"directory": "/checkpoint", "create_directory": true}}
		config["service"].(map[string]any)["extensions"] = []string{"file_storage"}
		for name, raw := range config["processors"].(map[string]any) {
			if strings.HasPrefix(name, "k8sattributes/") {
				raw.(map[string]any)["auth_type"] = "kubeConfig"
			}
		}
		exporter := config["exporters"].(map[string]any)["otlp/gateway"].(map[string]any)
		exporter["endpoint"] = fmt.Sprintf("host.docker.internal:%d", listener.Addr().(*net.TCPAddr).Port)
		exporter["tls"] = map[string]bool{"insecure": true}
		exporter["headers"] = map[string]string{"x-plan8-native-test": capture.token}
		public, _ := json.Marshal(config)
		path := filepath.Join(dir, fmt.Sprintf("config-%d.json", revision))
		if e = os.WriteFile(path, public, 0600); e != nil {
			t.Fatal(e)
		}
		id := docker("run", "-d", "--label", "argus.owner="+owner, "--name", fmt.Sprintf("%s-%d", owner, revision), "--memory=512m", "--cpus=1", "--read-only", "--cap-drop=ALL", "--env", "K8S_NODE_NAME="+firstPod.Spec.NodeName, "--env", "KUBECONFIG=/test-kubeconfig", "--mount", "type=bind,source="+binary+",target=/argus,readonly", "--mount", "type=bind,source="+path+",target=/config.json,readonly", "--mount", "type=bind,source="+kubeconfigPath+",target=/test-kubeconfig,readonly", "--mount", "type=bind,source="+checkpoint+",target=/checkpoint", "--mount", "type=volume,source="+nodeVolume+",target=/var,readonly", "--entrypoint", "/argus", "alpine:3.21", "--config=/config.json")
		t.Cleanup(func() {
			bounded, c := context.WithTimeout(context.Background(), 30*time.Second)
			defer c()
			label, e := exec.CommandContext(bounded, "docker", "inspect", "--format", `{{index .Config.Labels "argus.owner"}}`, id).Output()
			if e == nil && strings.TrimSpace(string(label)) == owner {
				_ = exec.CommandContext(bounded, "docker", "rm", "-f", "-v", id).Run()
			}
		})
		return id
	}
	waitSample := func(container, podUID string, min int) {
		t.Helper()
		if e := wait.PollUntilContextTimeout(ctx, 300*time.Millisecond, 40*time.Second, true, func(context.Context) (bool, error) {
			count := 0
			for _, r := range capture.snapshot() {
				if strings.HasPrefix(r.body, "plan8-log:"+podUID+":") {
					count++
				}
			}
			return count >= min, nil
		}); e != nil {
			t.Fatalf("Pod logs not received: %v\n%s", e, docker("logs", "--tail", "25", container))
		}
	}
	first := run(1)
	waitSample(first, string(firstPod.UID), 3)
	metadataAPI.partition(true)
	current, err := api.AppsV1().Deployments(owner).Get(ctx, "orders", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	current.Spec.Template.Annotations = map[string]string{"argus.test/rollout": uuid.NewString()}
	if _, err = api.AppsV1().Deployments(owner).Update(ctx, current, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	var secondPod corev1.Pod
	if err = wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, 45*time.Second, true, func(ctx context.Context) (bool, error) {
		pods, e := api.CoreV1().Pods(owner).List(ctx, metav1.ListOptions{LabelSelector: "app=orders"})
		if e != nil {
			return false, e
		}
		for _, p := range pods.Items {
			if p.UID != firstPod.UID && p.Status.Phase == corev1.PodRunning {
				secondPod = p
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second)
	for _, record := range capture.snapshot() {
		if record.resource["k8s.pod.uid"] == string(secondPod.UID) {
			t.Fatal("metadata partition did not isolate the new Pod")
		}
	}
	metadataAPI.partition(false)
	waitSample(first, string(secondPod.UID), 3)
	capture.setPaused(true)
	before := len(capture.snapshot())
	// Force data into the native persistent sending queue, then kill the
	// process without a graceful flush. Receiver checkpoints alone are not enough.
	time.Sleep(3 * time.Second)
	docker("kill", "--signal", "KILL", first)
	second := run(2)
	capture.setPaused(false)
	if err = wait.PollUntilContextTimeout(ctx, 300*time.Millisecond, 40*time.Second, true, func(context.Context) (bool, error) { return len(capture.snapshot()) >= before+3, nil }); err != nil {
		t.Fatalf("restart did not resume: %v\n%s", err, docker("logs", "--tail", "25", second))
	}
	seen := map[string]map[int]bool{}
	for _, r := range capture.snapshot() {
		parts := strings.Split(r.body, ":")
		if len(parts) != 3 || parts[0] != "plan8-log" {
			t.Fatalf("unexpected log body: %q", r.body)
		}
		if parts[1] != string(firstPod.UID) && parts[1] != string(secondPod.UID) {
			t.Fatal("foreign Pod exported")
		}
		n, e := strconv.Atoi(parts[2])
		if e != nil {
			t.Fatal(e)
		}
		if seen[parts[1]] == nil {
			seen[parts[1]] = map[int]bool{}
		}
		seen[parts[1]][n] = true
		for key, value := range map[string]string{"argus.resource.type": "application", "argus.resource.id": app, "argus.application.id": app, "argus.service.id": service, "argus.namespace.uid": string(namespace.UID), "k8s.deployment.uid": string(deployment.UID), "k8s.pod.uid": parts[1]} {
			if r.resource[key] != value {
				t.Fatalf("trusted %s: %q", key, r.resource[key])
			}
		}
	}
	for pod, sequence := range seen {
		max := 0
		for n := range sequence {
			if n > max {
				max = n
			}
		}
		for n := 0; n <= max; n++ {
			if !sequence[n] {
				t.Fatalf("Pod %s lost line %d", pod, n)
			}
		}
	}
	if strings.Contains(docker("logs", first), "plan8-log:") {
		t.Fatal("Collector failure logging exposed raw log bodies")
	}
	t.Logf("native Collector + real Pod files: two rollout Pod UIDs, first-line continuity through metadata API partition, namespace Role metadata access, application identity, unavailable sink and crash recovery from durable checkpoint/queue verified (%d records)", len(capture.snapshot()))
}
