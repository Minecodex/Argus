package collectormanager

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/logcollection"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	logsrpc "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricsrpc "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type ownedCoexistLogSink struct {
	logsrpc.UnimplementedLogsServiceServer
	mu    sync.Mutex
	token string
	rows  []map[string]string
}
type ownedCoexistMetricCapture struct {
	*nativeMetricCapture
	allowed map[string]string
}

func (c *ownedCoexistMetricCapture) Export(ctx context.Context, r *metricsrpc.ExportMetricsServiceRequest) (*metricsrpc.ExportMetricsServiceResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	if len(md.Get("x-plan8-native-test")) != 1 || md.Get("x-plan8-native-test")[0] != c.token {
		return nil, status.Error(codes.Unauthenticated, "owned coexist metric capture")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, resource := range r.ResourceMetrics {
		target, kind, source := "", "", ""
		for _, a := range resource.Resource.GetAttributes() {
			switch a.Key {
			case "argus.resource.id":
				target = a.Value.GetStringValue()
			case "argus.resource.type":
				kind = a.Value.GetStringValue()
			case "argus.source.id":
				source = a.Value.GetStringValue()
			}
		}
		if target == "" || source == "" || c.allowed[target] != kind {
			return nil, status.Error(codes.InvalidArgument, "unexpected owned Host/Cluster source identity")
		}
		for _, scope := range resource.ScopeMetrics {
			for _, metric := range scope.Metrics {
				if len(metric.GetGauge().GetDataPoints())+len(metric.GetSum().GetDataPoints())+len(metric.GetHistogram().GetDataPoints())+len(metric.GetExponentialHistogram().GetDataPoints())+len(metric.GetSummary().GetDataPoints()) == 0 {
					continue
				}
				if c.targets[target] == nil {
					c.targets[target] = map[string]bool{}
				}
				c.targets[target][metric.Name] = true
			}
		}
	}
	c.requests = append(c.requests, proto.Clone(r).(*metricsrpc.ExportMetricsServiceRequest))
	return &metricsrpc.ExportMetricsServiceResponse{}, nil
}
func (s *ownedCoexistLogSink) Export(ctx context.Context, r *logsrpc.ExportLogsServiceRequest) (*logsrpc.ExportLogsServiceResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	if len(md.Get("x-plan8-native-test")) != 1 || md.Get("x-plan8-native-test")[0] != s.token {
		return nil, status.Error(codes.Unauthenticated, "owned coexist capture")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, resource := range r.ResourceLogs {
		attrs := map[string]string{}
		for _, a := range resource.Resource.GetAttributes() {
			attrs[a.Key] = a.Value.GetStringValue()
		}
		for _, scope := range resource.ScopeLogs {
			for _, record := range scope.LogRecords {
				s.rows = append(s.rows, map[string]string{"body": record.Body.GetStringValue(), "collection": attrs["argus.collection.id"], "resource": attrs["argus.resource.id"], "pod_uid": attrs["k8s.pod.uid"]})
			}
		}
	}
	return &logsrpc.ExportLogsServiceResponse{}, nil
}

// Actual native Host process and actual DaemonSet share one owned Kind Node.
// The native capture transport is controlled; formal enrollment is separate.
func TestOwnedNodeHostAndDaemonSetHaveDistinctSourcesAndNoContainerLogDoubleRead(t *testing.T) {
	if os.Getenv("ARGUS_PLAN8_NODE_COEXIST") != "1" {
		t.Skip("explicit owned physical coexistence execution required")
	}
	output := os.Getenv("ARGUS_PLAN8_NODE_COEXIST_EVIDENCE")
	if output == "" {
		t.Fatal("specific evidence path required")
	}
	if _, e := os.Stat(output); e == nil {
		t.Fatal("refuse to replace a physical coexistence observation")
	}
	proof := map[string]any{"passed": false, "scope": "actual_same_node_host_process_and_daemonset_native_metrics_logs_with_controlled_capture_not_formal_enrollment", "started_at": time.Now().UTC()}
	complete := false
	t.Cleanup(func() {
		proof["passed"] = complete && !t.Failed() && !t.Skipped()
		proof["finished_at"] = time.Now().UTC()
		body, _ := json.MarshalIndent(proof, "", "  ")
		if e := os.WriteFile(output, body, 0600); e != nil {
			t.Error(e)
		}
	})
	f := newNativeResourceFixture(t)
	nodeName := os.Getenv("ARGUS_PLAN8_COEXIST_NODE")
	contextName := os.Getenv("ARGUS_PLAN8_COEXIST_CONTEXT")
	if nodeName != "plan8-drain-completion-20261021-control-plane" || contextName != "kind-plan8-drain-completion-20261021" {
		t.Fatal("exact owned coexist cluster/node required")
	}
	if label := f.must("", "inspect", "--format", `{{index .Config.Labels "io.x-k8s.kind.cluster"}}`, nodeName); label != "plan8-drain-completion-20261021" {
		t.Fatal("owned Node container changed")
	}
	cfg, e := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: contextName}).ClientConfig()
	if e != nil {
		t.Fatal(e)
	}
	api, e := kubernetes.NewForConfig(cfg)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	node, e := api.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if e != nil {
		t.Fatal(e)
	}
	machine := f.must("", "exec", nodeName, "cat", "/etc/machine-id")
	boot := f.must("", "exec", nodeName, "cat", "/proc/sys/kernel/random/boot_id")
	if machine != node.Status.NodeInfo.MachineID || boot != node.Status.NodeInfo.BootID {
		t.Fatal("actual Host machine/boot identity differs from Kubernetes Node")
	}
	ip := ""
	for _, a := range node.Status.Addresses {
		if a.Type == corev1.NodeInternalIP {
			ip = a.Address
		}
	}
	if net.ParseIP(ip) == nil {
		t.Fatal("actual Node InternalIP unavailable")
	}
	owner := "plan8-coexist-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	prefix := "/opt/" + owner
	ns, e := api.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: owner, Labels: map[string]string{"argus.owner": owner}}}, metav1.CreateOptions{})
	if e != nil {
		t.Fatal(e)
	}
	var roleUID, bindingUID string
	t.Cleanup(func() {
		cleanup, c := context.WithTimeout(context.Background(), 50*time.Second)
		defer c()
		current, e := api.CoreV1().Namespaces().Get(cleanup, owner, metav1.GetOptions{})
		if e == nil && current.UID == ns.UID && current.Labels["argus.owner"] == owner {
			uid := current.UID
			e = api.CoreV1().Namespaces().Delete(cleanup, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}})
			if e != nil {
				t.Error(e)
			}
			if e = wait.PollUntilContextTimeout(cleanup, 200*time.Millisecond, 35*time.Second, true, func(ctx context.Context) (bool, error) {
				_, e := api.CoreV1().Namespaces().Get(ctx, owner, metav1.GetOptions{})
				return apierrors.IsNotFound(e), nil
			}); e != nil {
				t.Error(e)
			}
		} else if !apierrors.IsNotFound(e) {
			t.Error("namespace ownership changed", e)
		}
		for _, kind := range []string{"binding", "role"} {
			if kind == "binding" && bindingUID != "" {
				v, e := api.RbacV1().ClusterRoleBindings().Get(cleanup, owner, metav1.GetOptions{})
				if e == nil && string(v.UID) == bindingUID && v.Labels["argus.owner"] == owner {
					uid := v.UID
					if e = api.RbacV1().ClusterRoleBindings().Delete(cleanup, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
						t.Error(e)
					}
				} else if !apierrors.IsNotFound(e) {
					t.Error("binding ownership changed", e)
				}
			}
			if kind == "role" && roleUID != "" {
				v, e := api.RbacV1().ClusterRoles().Get(cleanup, owner, metav1.GetOptions{})
				if e == nil && string(v.UID) == roleUID && v.Labels["argus.owner"] == owner {
					uid := v.UID
					if e = api.RbacV1().ClusterRoles().Delete(cleanup, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
						t.Error(e)
					}
				} else if !apierrors.IsNotFound(e) {
					t.Error("role ownership changed", e)
				}
			}
		}
		if marker, e := f.command("", "exec", nodeName, "cat", prefix+"/owner"); e == nil && marker == owner {
			_, _ = f.command("", "exec", nodeName, "systemctl", "stop", owner+".service")
			if _, e = f.command("", "exec", nodeName, "rm", "-rf", "--", prefix); e != nil {
				t.Error(e)
			}
		}
	})
	no := false
	grace := int64(1)
	pod, e := api.CoreV1().Pods(owner).Create(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "log-origin", Labels: map[string]string{"argus.owner": owner}}, Spec: corev1.PodSpec{NodeName: nodeName, AutomountServiceAccountToken: &no, TerminationGracePeriodSeconds: &grace, Tolerations: []corev1.Toleration{{Operator: corev1.TolerationOpExists}}, Containers: []corev1.Container{{Name: "main", Image: "alpine:3.21", ImagePullPolicy: corev1.PullNever, Command: []string{"sh", "-c", `n=0; while true; do echo "plan8-coexist-k8s:$n"; n=$((n+1)); sleep 1; done`}}}}}, metav1.CreateOptions{})
	if e != nil {
		t.Fatal(e)
	}
	podFile := fmt.Sprintf("/var/log/pods/%s_%s_%s/main/0.log", owner, pod.Name, pod.UID)
	if e = wait.PollUntilContextTimeout(ctx, 300*time.Millisecond, 45*time.Second, true, func(context.Context) (bool, error) {
		_, e := f.command("", "exec", nodeName, "test", "-f", podFile)
		return e == nil, nil
	}); e != nil {
		t.Fatal("actual owned Pod log file unavailable", e)
	}
	_, e = api.CoreV1().ServiceAccounts(owner).Create(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "collector"}}, metav1.CreateOptions{})
	if e != nil {
		t.Fatal(e)
	}
	role, e := api.RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: owner, Labels: map[string]string{"argus.owner": owner}}, Rules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"nodes/stats"}, Verbs: []string{"get"}}, {APIGroups: []string{""}, Resources: []string{"pods", "namespaces"}, Verbs: []string{"get", "list", "watch"}}}}, metav1.CreateOptions{})
	if e != nil {
		t.Fatal(e)
	}
	roleUID = string(role.UID)
	rb, e := api.RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: owner, Labels: map[string]string{"argus.owner": owner}}, RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: owner}, Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: "collector", Namespace: owner}}}, metav1.CreateOptions{})
	if e != nil {
		t.Fatal(e)
	}
	bindingUID = string(rb.UID)
	_, e = api.RbacV1().Roles(owner).Create(ctx, &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "metadata"}, Rules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list", "watch"}}, {APIGroups: []string{"apps"}, Resources: []string{"replicasets"}, Verbs: []string{"get", "list", "watch"}}, {APIGroups: []string{"batch"}, Resources: []string{"jobs"}, Verbs: []string{"get", "list", "watch"}}}}, metav1.CreateOptions{})
	if e != nil {
		t.Fatal(e)
	}
	_, e = api.RbacV1().RoleBindings(owner).Create(ctx, &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "metadata"}, RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: "metadata"}, Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: "collector", Namespace: owner}}}, metav1.CreateOptions{})
	if e != nil {
		t.Fatal(e)
	}
	ca := f.must("", "exec", nodeName, "cat", "/var/lib/kubelet/pki/kubelet.crt")
	cluster := uuid.NewString()
	listener, e := net.Listen("tcp", "0.0.0.0:0")
	if e != nil {
		t.Fatal(e)
	}
	sink := &ownedCoexistLogSink{token: f.capture.token}
	metricSink := &ownedCoexistMetricCapture{nativeMetricCapture: f.capture, allowed: map[string]string{f.host: "host", cluster: "kubernetes_cluster"}}
	server := grpc.NewServer()
	metricsrpc.RegisterMetricsServiceServer(server, metricSink)
	logsrpc.RegisterLogsServiceServer(server, sink)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	hostIP := strings.Fields(f.must("", "exec", nodeName, "getent", "ahostsv4", "host.docker.internal"))[0]
	if net.ParseIP(hostIP) == nil {
		t.Fatal("test capture address unavailable")
	}
	endpoint := net.JoinHostPort(hostIP, fmt.Sprint(listener.Addr().(*net.TCPAddr).Port))
	hostMetric, hostLog, nodeMetric, nodeLog := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	dsCollector := uuid.NewString()
	fileConfig, _ := json.Marshal(logcollection.Config{Scope: "system", Include: []string{prefix + "/input/*.log"}, StartAt: "beginning"})
	workloadConfig, _ := json.Marshal(logcollection.Config{Scope: "component", Workload: &logcollection.Workload{NamespaceID: uuid.New(), Namespace: owner, NamespaceUID: string(ns.UID), Type: "pod", Name: pod.Name, UID: string(pod.UID), Containers: []string{"main"}}, StartAt: "beginning"})
	prepareConfig := func(collector, resource, kind, placement, state string, collections []configbundle.Collection) []byte {
		raw, e := configbundle.Render(configbundle.RenderInput{CollectorID: collector, ResourceID: resource, ResourceType: kind, Role: "direct", Platform: "linux_amd64", SourceGeneration: uuid.NewString(), ConfigRevision: 1, RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: "https://control.test/enroll", IngestGRPCEndpoint: "grpcs://ingest.test:4317", IngestHTTPEndpoint: "https://ingest.test", Collections: collections})
		if e != nil {
			t.Fatal(e)
		}
		value, e := configbundle.Extract(raw, placement)
		if e != nil {
			t.Fatal(e)
		}
		var config map[string]any
		if json.Unmarshal(value, &config) != nil {
			t.Fatal("bundle JSON unavailable")
		}
		service := config["service"].(map[string]any)
		pipelines := service["pipelines"].(map[string]any)
		receivers, processors := map[string]any{}, map[string]any{}
		for name, v := range pipelines {
			if !strings.Contains(name, "/collection_") {
				delete(pipelines, name)
				continue
			}
			p := v.(map[string]any)
			p["exporters"] = []string{"otlp/owned"}
			for _, v := range p["receivers"].([]any) {
				n := v.(string)
				receivers[n] = config["receivers"].(map[string]any)[n]
			}
			for _, v := range p["processors"].([]any) {
				n := v.(string)
				processors[n] = config["processors"].(map[string]any)[n]
			}
		}
		for name, v := range receivers {
			if strings.HasPrefix(name, "kubeletstats/") {
				r := v.(map[string]any)
				r["endpoint"] = "https://" + net.JoinHostPort(nodeName, "10250")
				r["ca_file"] = "/kubelet-ca.pem"
				r["auth_type"] = "serviceAccount"
				r["insecure_skip_verify"] = false
			}
		}
		config["receivers"], config["processors"] = receivers, processors
		config["extensions"] = map[string]any{"file_storage": map[string]any{"directory": state, "create_directory": true}}
		service["extensions"] = []string{"file_storage"}
		config["exporters"] = map[string]any{"otlp/owned": map[string]any{"endpoint": endpoint, "tls": map[string]bool{"insecure": true}, "headers": map[string]string{"x-plan8-native-test": f.capture.token}}}
		body, _ := json.Marshal(config)
		return body
	}
	collection := func(id, key, kind, target string, config []byte) configbundle.Collection {
		c := configbundle.Collection{ID: id, Integration: key, TargetType: kind, TargetID: target, Config: config, Enabled: true, Revision: 1, AssignmentEpoch: 1}
		if key == "k8slog" {
			c.NamespaceUID = string(ns.UID)
		}
		return c
	}
	hostConfig := prepareConfig(f.collector, f.host, "host", "host", prefix+"/host-state", []configbundle.Collection{collection(hostMetric, "host_metrics", "host", f.host, []byte(`{"interval":"10s"}`)), collection(hostLog, "filelog", "host", f.host, fileConfig)})
	nodeConfig := prepareConfig(dsCollector, cluster, "kubernetes_cluster", "kubernetes_agent", "/checkpoint", []configbundle.Collection{collection(nodeMetric, "kubernetes_node_metrics", "kubernetes_cluster", cluster, []byte(`{"interval":"10s"}`)), collection(nodeLog, "k8slog", "kubernetes_cluster", cluster, workloadConfig)})
	f.must("", "exec", nodeName, "mkdir", "-p", prefix+"/input")
	f.must(owner, "exec", "-i", nodeName, "sh", "-c", "cat > "+prefix+"/owner")
	f.must("plan8-coexist-host-local\n", "exec", "-i", nodeName, "sh", "-c", "cat > "+prefix+"/input/host.log")
	f.must("", "exec", nodeName, "ln", "-s", podFile, prefix+"/input/from-k8s.log")
	f.must("", "cp", f.binary, nodeName+":"+prefix+"/collector")
	f.must("", "exec", nodeName, "chmod", "755", prefix+"/collector")
	f.must(string(hostConfig), "exec", "-i", nodeName, "sh", "-c", "cat > "+prefix+"/host-config.json")
	f.must("", "exec", nodeName, "systemd-run", "--unit="+owner, "--collect", prefix+"/collector", "--config="+prefix+"/host-config.json")
	_, e = api.CoreV1().ConfigMaps(owner).Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "collector"}, Data: map[string]string{"config.json": string(nodeConfig), "kubelet-ca.pem": ca}}, metav1.CreateOptions{})
	if e != nil {
		t.Fatal(e)
	}
	fileType := corev1.HostPathFile
	dirType := corev1.HostPathDirectory
	readOnly := true
	ds, e := api.AppsV1().DaemonSets(owner).Create(ctx, &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "collector", Labels: map[string]string{"argus.owner": owner}}, Spec: appsv1.DaemonSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "collector"}}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "collector"}}, Spec: corev1.PodSpec{ServiceAccountName: "collector", HostAliases: []corev1.HostAlias{{IP: ip, Hostnames: []string{nodeName}}}, NodeSelector: map[string]string{"kubernetes.io/hostname": nodeName}, Tolerations: []corev1.Toleration{{Operator: corev1.TolerationOpExists}}, TerminationGracePeriodSeconds: &grace, Containers: []corev1.Container{{Name: "main", Image: "alpine:3.21", ImagePullPolicy: corev1.PullNever, Command: []string{"/collector", "--config=/config.json"}, Env: []corev1.EnvVar{{Name: "K8S_NODE_NAME", Value: nodeName}}, SecurityContext: &corev1.SecurityContext{ReadOnlyRootFilesystem: &readOnly, Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}}, VolumeMounts: []corev1.VolumeMount{{Name: "binary", MountPath: "/collector", ReadOnly: true}, {Name: "config", MountPath: "/config.json", SubPath: "config.json", ReadOnly: true}, {Name: "config", MountPath: "/kubelet-ca.pem", SubPath: "kubelet-ca.pem", ReadOnly: true}, {Name: "logs", MountPath: "/var/log/pods", ReadOnly: true}, {Name: "checkpoint", MountPath: "/checkpoint"}}}}, Volumes: []corev1.Volume{{Name: "binary", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: prefix + "/collector", Type: &fileType}}}, {Name: "config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "collector"}}}}, {Name: "logs", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/log/pods", Type: &dirType}}}, {Name: "checkpoint", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}}}}}}, metav1.CreateOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if e = wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, 75*time.Second, true, func(context.Context) (bool, error) {
		m := f.capture.snapshot()
		sink.mu.Lock()
		defer sink.mu.Unlock()
		hostCount, k8sCount := 0, 0
		for _, r := range sink.rows {
			if r["collection"] == hostLog && strings.Contains(r["body"], "plan8-coexist-k8s:") {
				return false, fmt.Errorf("Host generic filelog double-read actual Kubernetes file")
			}
			if r["collection"] == hostLog && strings.Contains(r["body"], "plan8-coexist-host-local") {
				hostCount++
			}
			if r["collection"] == nodeLog && strings.Contains(r["body"], "plan8-coexist-k8s:") && r["pod_uid"] == string(pod.UID) {
				k8sCount++
			}
		}
		return len(m[f.host]) > 0 && len(m[cluster]) > 0 && hostCount >= 1 && k8sCount >= 3, nil
	}); e != nil {
		pods, _ := api.CoreV1().Pods(owner).List(ctx, metav1.ListOptions{LabelSelector: "app=collector"})
		for _, p := range pods.Items {
			b, _ := api.CoreV1().Pods(owner).GetLogs(p.Name, &corev1.PodLogOptions{Container: "main", TailLines: func() *int64 { v := int64(15); return &v }()}).DoRaw(ctx)
			t.Log(strings.ReplaceAll(string(b), f.capture.token, "[REDACTED]"))
		}
		t.Fatal("actual physical coexistence failed", e)
	}
	current, e := api.AppsV1().DaemonSets(owner).Get(ctx, ds.Name, metav1.GetOptions{})
	if e != nil || current.Status.NumberReady != 1 || current.Status.DesiredNumberScheduled != 1 {
		t.Fatal("actual single-node DaemonSet not ready", e)
	}
	proof["node_name"], proof["node_uid"], proof["machine_id"], proof["boot_id"], proof["provider_id"], proof["internal_ip"], proof["system_uuid"] = node.Name, string(node.UID), machine, boot, node.Spec.ProviderID, ip, node.Status.NodeInfo.SystemUUID
	proof["host_collector_id"], proof["daemonset_collector_id"], proof["host_resource_id"], proof["cluster_resource_id"] = f.collector, dsCollector, f.host, cluster
	proof["same_actual_machine_and_boot_identity"] = true
	proof["actual_native_host_metrics"] = f.capture.snapshot()[f.host]
	proof["actual_native_node_metrics"] = f.capture.snapshot()[cluster]
	proof["actual_metrics_payloads"] = f.capture.payloads()
	sink.mu.Lock()
	proof["actual_log_observations"] = append([]map[string]string{}, sink.rows...)
	sink.mu.Unlock()
	proof["actual_owned_pod_uid"] = string(pod.UID)
	proof["host_canonical_symlink_did_not_double_read_kubernetes_logs"] = true
	proof["ordinary_host_file_observed"] = true
	proof["kubernetes_log_retained_actual_pod_uid"] = true
	complete = true
}
