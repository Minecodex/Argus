package collectormanager_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	metricsrpc "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	appsv1 "k8s.io/api/apps/v1"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Actual Kubernetes API and kubelet, with short-lived service account credentials.
// The capture endpoint is a component gate, not formal Argus enrollment.
func TestNativeKubernetesOwnMetricsUseRealAPIAndKubelet(t *testing.T) {
	runNativeKubernetesMetrics(t, false)
}

func TestNativeKubernetesControlMetricsKeepUnavailableEndpointsUnobserved(t *testing.T) {
	runNativeKubernetesMetrics(t, true)
}

func TestNativeKubernetesControlMetricsUseReachableEndpoints(t *testing.T) {
	t.Setenv("ARGUS_KUBERNETES_CONTROL_REQUIRE_DATA", "1")
	runNativeKubernetesMetrics(t, true)
}

func runNativeKubernetesMetrics(t *testing.T, controlOnly bool) {
	contextName, node := os.Getenv("ARGUS_KUBERNETES_METRICS_CONTEXT"), os.Getenv("ARGUS_KUBERNETES_METRICS_NODE")
	if contextName == "" || node == "" {
		t.Skip("requires an explicitly selected Docker-driver Kubernetes context and node")
	}
	apiPort := os.Getenv("ARGUS_KUBERNETES_METRICS_API_PORT")
	if apiPort == "" {
		apiPort = "8443"
	}
	port, parseErr := strconv.Atoi(apiPort)
	if parseErr != nil || port < 1 || port > 65535 {
		t.Fatal("invalid explicit API port")
	}
	apiAddress := net.JoinHostPort(node, apiPort)
	network := os.Getenv("ARGUS_KUBERNETES_METRICS_NETWORK")
	if network == "" {
		network = node
	}
	binary, err := filepath.Abs(os.Getenv("ARGUS_MONITORING_COLLECTOR_BINARY"))
	if err != nil {
		t.Fatal(err)
	}
	loading := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: contextName})
	rest, err := loading.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	api, err := kubernetes.NewForConfig(rest)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	d := newPipelineDocker(t)
	namespace, err := api.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: d.owner, Labels: map[string]string{"argus.owner": d.owner}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, c := context.WithTimeout(context.Background(), 45*time.Second)
		defer c()
		current, e := api.CoreV1().Namespaces().Get(cleanup, d.owner, metav1.GetOptions{})
		if apierrors.IsNotFound(e) {
			return
		}
		if e != nil || current.UID != namespace.UID || current.Labels["argus.owner"] != d.owner {
			t.Errorf("namespace cleanup ownership: %v", e)
			return
		}
		if e = api.CoreV1().Namespaces().Delete(cleanup, d.owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &current.UID}}); e != nil {
			t.Error(e)
			return
		}
		if e = wait.PollUntilContextTimeout(cleanup, 500*time.Millisecond, 40*time.Second, true, func(ctx context.Context) (bool, error) {
			_, e := api.CoreV1().Namespaces().Get(ctx, d.owner, metav1.GetOptions{})
			return apierrors.IsNotFound(e), nil
		}); e != nil {
			t.Error(e)
		}
	})
	_, err = api.CoreV1().ServiceAccounts(d.owner).Create(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "metrics"}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	role, err := api.RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: d.owner, Labels: map[string]string{"argus.owner": d.owner}}, Rules: []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"events", "namespaces", "nodes", "persistentvolumes", "persistentvolumeclaims", "pods", "replicationcontrollers", "resourcequotas", "services"}, Verbs: []string{"get", "list", "watch"}},
		{APIGroups: []string{""}, Resources: []string{"nodes/stats"}, Verbs: []string{"get"}},
		{APIGroups: []string{"apps"}, Resources: []string{"daemonsets", "deployments", "replicasets", "statefulsets"}, Verbs: []string{"get", "list", "watch"}},
		{APIGroups: []string{"batch"}, Resources: []string{"jobs", "cronjobs"}, Verbs: []string{"get", "list", "watch"}},
		{APIGroups: []string{"autoscaling"}, Resources: []string{"horizontalpodautoscalers"}, Verbs: []string{"get", "list", "watch"}},
		{APIGroups: []string{"discovery.k8s.io"}, Resources: []string{"endpointslices"}, Verbs: []string{"get", "list", "watch"}},
		{NonResourceURLs: []string{"/metrics"}, Verbs: []string{"get"}},
	}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		e := api.RbacV1().ClusterRoles().Delete(cleanup, role.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &role.UID}})
		if e != nil && !apierrors.IsNotFound(e) {
			t.Error(e)
		}
	})
	binding, err := api.RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: d.owner, Labels: map[string]string{"argus.owner": d.owner}}, RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: role.Name}, Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: "metrics", Namespace: d.owner}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		e := api.RbacV1().ClusterRoleBindings().Delete(cleanup, binding.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &binding.UID}})
		if e != nil && !apierrors.IsNotFound(e) {
			t.Error(e)
		}
	})
	proxyRole, err := api.RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: d.owner + "-dns-proxy", Labels: map[string]string{"argus.owner": d.owner}}, Rules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods/proxy"}, Verbs: []string{"get"}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		if e := api.RbacV1().ClusterRoles().Delete(cleanup, proxyRole.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &proxyRole.UID}}); e != nil && !apierrors.IsNotFound(e) {
			t.Error(e)
		}
	})
	proxyBinding, err := api.RbacV1().RoleBindings("kube-system").Create(ctx, &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: d.owner + "-dns-proxy", Labels: map[string]string{"argus.owner": d.owner}}, RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: proxyRole.Name}, Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: "metrics", Namespace: d.owner}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		if e := api.RbacV1().RoleBindings("kube-system").Delete(cleanup, proxyBinding.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &proxyBinding.UID}}); e != nil && !apierrors.IsNotFound(e) {
			t.Error(e)
		}
	})
	expiration := int64(600)
	token, err := api.CoreV1().ServiceAccounts(d.owner).CreateToken(ctx, "metrics", &authv1.TokenRequest{Spec: authv1.TokenRequestSpec{ExpirationSeconds: &expiration}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tokenPath := d.file("service-account-token", []byte(token.Status.Token))
	ca := rest.CAData
	if len(ca) == 0 && rest.CAFile != "" {
		ca, err = os.ReadFile(rest.CAFile)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(ca) == 0 || rest.Insecure {
		t.Fatal("actual API trust root required")
	}
	caPath := d.file("api-ca.pem", ca)
	kubeletCA := d.file("kubelet-ca.pem", []byte(d.must("exec", node, "cat", "/var/lib/kubelet/pki/kubelet.crt")))
	one := int32(1)
	no := false
	grace := int64(1)
	workload := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "metric-workload"}, Spec: appsv1.DeploymentSpec{Replicas: &one, Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "metric-workload"}}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "metric-workload"}}, Spec: corev1.PodSpec{AutomountServiceAccountToken: &no, NodeName: node, TerminationGracePeriodSeconds: &grace, Containers: []corev1.Container{{Name: "main", Image: "alpine:3.21", ImagePullPolicy: corev1.PullIfNotPresent, Command: []string{"/bin/sh", "-c", "while true; do nslookup host.minikube.internal >/dev/null 2>&1; sleep 1; done"}}}}}}}
	if os.Getenv("ARGUS_KUBERNETES_CONTROL_REQUIRE_DATA") == "1" {
		workload.Spec.Template.Spec.NodeName = ""
		workload.Spec.Template.Spec.NodeSelector = map[string]string{"kubernetes.io/hostname": node}
	}
	deployment, err := api.AppsV1().Deployments(d.owner).Create(ctx, workload, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err = wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, 40*time.Second, true, func(ctx context.Context) (bool, error) {
		v, e := api.AppsV1().Deployments(d.owner).Get(ctx, deployment.Name, metav1.GetOptions{})
		return e == nil && v.Status.AvailableReplicas == 1, e
	}); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	sink := &builtinMetricSink{token: uuid.NewString(), samples: map[string]map[string]string{}}
	server := grpc.NewServer()
	metricsrpc.RegisterMetricsServiceServer(server, sink)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	collector, cluster, generation := uuid.NewString(), uuid.NewString(), uuid.NewString()
	var collections []configbundle.Collection
	keys := []string{"kubernetes_cluster_metrics", "kubernetes_node_metrics", "kubernetes_apiserver_metrics", "kubernetes_coredns_metrics"}
	if controlOnly {
		keys = []string{"kubernetes_scheduler_metrics", "kubernetes_controller_metrics", "kubernetes_etcd_metrics"}
	}
	for _, key := range keys {
		config := json.RawMessage(`{"interval":"5s"}`)
		if os.Getenv("ARGUS_KUBERNETES_CONTROL_REQUIRE_DATA") == "1" && builtincollection.HasComponentTLS(key) {
			config, _ = json.Marshal(builtincollection.Config{Interval: "5s", ComponentTLS: &builtincollection.ComponentTLS{CAPEM: string(ca)}})
		}
		collections = append(collections, configbundle.Collection{ID: uuid.NewString(), Integration: key, TargetType: "kubernetes_cluster", TargetID: cluster, Config: config, Revision: 1, AssignmentEpoch: 1, Enabled: true})
	}
	run := func(name string, revision int64) (string, []configbundle.Source) {
		t.Helper()
		raw, e := configbundle.Render(configbundle.RenderInput{CollectorID: collector, ResourceID: cluster, ResourceType: "kubernetes_cluster", Role: "direct", Platform: "linux_amd64", SourceGeneration: generation, ConfigRevision: revision, RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: "https://control.test/enroll", IngestGRPCEndpoint: "https://ingest.test:4317", IngestHTTPEndpoint: "https://ingest.test", Collections: collections})
		if e != nil {
			t.Fatal(e)
		}
		sources, _ := configbundle.Sources(raw)
		// Both placements run in the owned test process, retaining native auth,
		// TLS verification and source identities; production keeps them separate.
		var combined map[string]any
		for _, target := range []string{"kubernetes_gateway", "kubernetes_agent"} {
			public, _ := configbundle.Extract(raw, target)
			var config map[string]any
			_ = json.Unmarshal(public, &config)
			if combined == nil {
				combined = config
				continue
			}
			for _, key := range []string{"receivers", "processors"} {
				for k, v := range config[key].(map[string]any) {
					combined[key].(map[string]any)[k] = v
				}
			}
			for k, v := range config["service"].(map[string]any)["pipelines"].(map[string]any) {
				pipeline := v.(map[string]any)
				pipeline["exporters"] = []string{"otlp/argus"}
				combined["service"].(map[string]any)["pipelines"].(map[string]any)[k] = pipeline
			}
		}
		pipelines := combined["service"].(map[string]any)["pipelines"].(map[string]any)
		receivers, processors := map[string]any{}, map[string]any{}
		for key, raw := range pipelines {
			if !strings.HasPrefix(key, "metrics/collection_") && key != "metrics/control" {
				delete(pipelines, key)
				continue
			}
			pipeline := raw.(map[string]any)
			for _, item := range pipeline["receivers"].([]any) {
				name := item.(string)
				receivers[name] = combined["receivers"].(map[string]any)[name]
			}
			for _, item := range pipeline["processors"].([]any) {
				name := item.(string)
				processors[name] = combined["processors"].(map[string]any)[name]
			}
		}
		combined["receivers"], combined["processors"] = receivers, processors
		for name, raw := range receivers {
			if strings.HasPrefix(name, "prometheus/") {
				// Outside-cluster Docker transport uses the same API server and
				// certificate identity; only its test network address differs.
				job := raw.(map[string]any)["config"].(map[string]any)["scrape_configs"].([]any)[0].(map[string]any)
				if discovery, ok := job["kubernetes_sd_configs"].([]any); ok {
					for _, entry := range discovery {
						entry.(map[string]any)["api_server"] = "https://" + apiAddress
					}
					for _, raw := range job["relabel_configs"].([]any) {
						rule := raw.(map[string]any)
						if rule["target_label"] == "__address__" && rule["replacement"] == "kubernetes.default.svc:443" {
							rule["replacement"] = apiAddress
						}
					}
				} else {
					job["static_configs"] = []map[string]any{{"targets": []string{apiAddress}}}
				}
			}
		}
		combined["extensions"] = map[string]any{"file_storage": map[string]any{"directory": "/tmp/checkpoint", "create_directory": true}}
		combined["service"].(map[string]any)["extensions"] = []string{"file_storage"}
		combined["exporters"] = map[string]any{"otlp/argus": map[string]any{"endpoint": fmt.Sprintf("host.docker.internal:%d", listener.Addr().(*net.TCPAddr).Port), "tls": map[string]bool{"insecure": true}, "headers": map[string]string{"x-plan8-builtin": sink.token}}, "nop/control": map[string]any{}}
		value, _ := json.Marshal(combined)
		path := d.file(name+".json", value)
		container := d.start(name, "--memory=768m", "--cpus=1", "--read-only", "--tmpfs", "/tmp", "--cap-drop=ALL", "-e", "K8S_NODE_NAME="+node, "-e", "KUBERNETES_SERVICE_HOST="+node, "-e", "KUBERNETES_SERVICE_PORT="+apiPort, "--mount", "type=bind,source="+binary+",target=/argus,readonly", "--mount", "type=bind,source="+path+",target=/config.json,readonly", "--mount", "type=bind,source="+tokenPath+",target=/var/run/secrets/kubernetes.io/serviceaccount/token,readonly", "--mount", "type=bind,source="+caPath+",target=/var/run/secrets/kubernetes.io/serviceaccount/ca.crt,readonly", "--mount", "type=bind,source="+kubeletCA+",target=/var/run/argus-kubelet/pki/kubelet.crt,readonly", "alpine:3.21", "sleep", "infinity")
		d.must("network", "connect", network, container)
		d.must("exec", "-d", container, "/bin/sh", "-c", "echo $$ > /tmp/collector.pid; exec /argus --config=/config.json >/tmp/collector.log 2>&1")
		return container, sources
	}
	waitSources := func(container string, count int) map[string]map[string]string {
		t.Helper()
		deadline := time.Now().Add(35 * time.Second)
		for time.Now().Before(deadline) {
			samples := sink.snapshot()
			if len(samples) == count {
				return samples
			}
			time.Sleep(200 * time.Millisecond)
		}
		logs, _ := d.command("exec", container, "cat", "/tmp/collector.log")
		t.Fatalf("native Kubernetes sources %d want %d: %s", len(sink.snapshot()), count, logs)
		return nil
	}
	if controlOnly {
		verifyNativeControlMetrics(t, ctx, api, rest, token.Status.Token, collector, cluster, d.owner, sink, collections, role, proxyRole, run, waitSources, func(container string) { d.must("stop", "--time", "1", container) }, func(container string) {
			d.must("exec", container, "/bin/sh", "-c", `kill -0 "$(cat /tmp/collector.pid)"`)
		})
		return
	}
	first, sources := run("both", 1)
	samples := waitSources(first, 4)
	for i := 0; i < 8; i++ {
		if _, err = api.AppsV1().Deployments(d.owner).Get(ctx, deployment.Name, metav1.GetOptions{}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Second)
	}
	samples = sink.snapshot()
	for _, source := range sources {
		wanted := []string{"k8s.node.cpu.usage", "k8s.node.memory.usage", "k8s.pod.memory.usage"}
		if source.Type == "k8s_cluster" {
			wanted = []string{"k8s.deployment.available", "k8s.pod.phase", "k8s.node.condition_ready"}
		}
		if source.Type == "kubernetes_apiserver" {
			wanted = []string{"apiserver_request_total", "apiserver_request_duration_seconds", "apiserver_current_inflight_requests", "up"}
		}
		if source.Type == "kubernetes_coredns" {
			wanted = []string{"coredns_dns_requests_total", "coredns_dns_responses_total", "coredns_dns_request_duration_seconds", "up"}
		}
		for _, metric := range wanted {
			if _, ok := samples[source.ID][metric]; !ok {
				t.Fatalf("native %s metric absent: %s", source.Type, metric)
			}
		}
	}
	apiSource := ""
	for _, source := range sources {
		if source.Type == "kubernetes_apiserver" {
			apiSource = source.ID
		}
	}
	metricRule := role.Rules[len(role.Rules)-1]
	role.Rules = role.Rules[:len(role.Rules)-1]
	role, err = api.RbacV1().ClusterRoles().Update(ctx, role, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	waitUp := func(expected float64, after int) {
		t.Helper()
		deadline := time.Now().Add(25 * time.Second)
		for time.Now().Before(deadline) {
			if value, ok := sink.gaugeAfter(apiSource, "up", after); ok && value == expected {
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Fatalf("native API scrape state did not become %v", expected)
	}
	waitUp(0, sink.requestCount())
	role.Rules = append(role.Rules, metricRule)
	role, err = api.RbacV1().ClusterRoles().Update(ctx, role, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	waitUp(1, sink.requestCount())
	sink.mu.Lock()
	requests := make([][]byte, len(sink.requests))
	for i, r := range sink.requests {
		requests[i], _ = proto.Marshal(r)
	}
	sink.mu.Unlock()
	d.must("stop", "--time", "1", first)
	sink.reset()
	collections[2].Enabled = false
	collections[2].Revision = 2
	second, remaining := run("api-disabled", 2)
	waitSources(second, 3)
	for _, source := range remaining {
		if source.Type == "kubernetes_apiserver" {
			t.Fatal("disabled API scrape continued")
		}
	}
	d.must("stop", "--time", "1", second)
	sink.reset()
	collections[3].Enabled = false
	collections[3].Revision = 2
	dnsDisabled, remaining := run("dns-disabled", 3)
	waitSources(dnsDisabled, 2)
	for _, source := range remaining {
		if source.Type == "kubernetes_coredns" {
			t.Fatal("disabled DNS scrape continued")
		}
	}
	d.must("stop", "--time", "1", dnsDisabled)
	sink.reset()
	collections[0].Enabled = false
	collections[0].Revision = 2
	third, remaining := run("node-only", 4)
	waitSources(third, 1)
	if len(remaining) != 1 || remaining[0].Type != "kubeletstats" {
		t.Fatal("cluster toggle altered node collection")
	}
	d.must("stop", "--time", "1", third)
	sink.reset()
	collections[1].Enabled = false
	collections[1].Revision = 2
	fourth, remaining := run("disabled", 5)
	time.Sleep(2 * time.Second)
	if len(remaining) != 0 || len(sink.snapshot()) != 0 {
		t.Fatal("disabled instances exported data")
	}
	if _, e := d.command("exec", fourth, "/bin/sh", "-c", `kill -0 "$(cat /tmp/collector.pid)"`); e != nil {
		logs, _ := d.command("exec", fourth, "cat", "/tmp/collector.log")
		t.Fatalf("control runtime exited: %s", logs)
	}
	if path := os.Getenv("ARGUS_KUBERNETES_METRIC_EVIDENCE"); path != "" {
		value, _ := json.MarshalIndent(map[string]any{"scope": "native_real_kubernetes_component_not_formal_enrollment", "collector_version": configbundle.CollectorVersion, "captured_at": time.Now().UTC(), "collector_id": collector, "host_id": cluster, "cluster_id": cluster, "namespace_uid": namespace.UID, "sources": sources, "metric_names_and_units": samples, "requests": requests}, "", "  ")
		if err = os.WriteFile(path, value, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("actual cluster API and kubelet TLS/service-account metrics; independent off/on; namespace and cluster RBAC removed; owner=%s", strings.TrimSpace(d.owner))
}
