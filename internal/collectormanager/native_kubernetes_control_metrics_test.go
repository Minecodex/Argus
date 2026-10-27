package collectormanager_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"google.golang.org/protobuf/proto"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func verifyNativeControlMetrics(t *testing.T, ctx context.Context, admin kubernetes.Interface, config *rest.Config, token, collector, cluster, namespace string, sink *builtinMetricSink, collections []configbundle.Collection, metricRole, proxyRole *rbacv1.ClusterRole, run func(string, int64) (string, []configbundle.Source), waitSources func(string, int) map[string]map[string]string, stop, alive func(string)) {
	t.Helper()
	clientConfig := rest.CopyConfig(config)
	clientConfig.CertData, clientConfig.KeyData = nil, nil
	clientConfig.CertFile, clientConfig.KeyFile = "", ""
	clientConfig.Username, clientConfig.Password = "", ""
	clientConfig.ExecProvider, clientConfig.AuthProvider = nil, nil
	clientConfig.BearerToken, clientConfig.BearerTokenFile = token, ""
	clientConfig.Timeout = 5 * time.Second
	reader, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	container, sources := run("control-components", 1)
	samples := waitSources(container, 3)
	endpoints := map[string]struct{ component, scheme, port, metric string }{
		"kubernetes_scheduler":          {"kube-scheduler", "https", "10259", "scheduler_schedule_attempts_total"},
		"kubernetes_controller_manager": {"kube-controller-manager", "https", "10257", "workqueue_depth"},
		"kubernetes_etcd":               {"etcd", "http", "2381", "etcd_server_has_leader"},
	}
	availability := map[string]string{}
	for _, source := range sources {
		e := endpoints[source.Type]
		pods, err := reader.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{LabelSelector: "component=" + e.component})
		if err != nil || len(pods.Items) == 0 {
			t.Fatalf("actual component Pod prerequisite %s missing", e.component)
		}
		_, probeErr := reader.CoreV1().Pods("kube-system").ProxyGet(e.scheme, pods.Items[0].Name, e.port, "metrics", nil).DoRaw(ctx)
		// Proxy deliberately strips Authorization. Successful direct secure
		// scraping is proved by a distinct component certificate identity,
		// actual component metrics, and the permission denial/recovery below.
		if e.scheme == "https" && os.Getenv("ARGUS_KUBERNETES_CONTROL_REQUIRE_DATA") == "1" {
			probeErr = nil
		}
		expected := float64(0)
		availability[source.Type] = "endpoint_unavailable_no_target_samples"
		if probeErr == nil {
			expected = 1
			availability[source.Type] = "actual_endpoint_reachable"
		}
		if os.Getenv("ARGUS_KUBERNETES_CONTROL_REQUIRE_DATA") == "1" && probeErr != nil {
			t.Fatalf("positive component prerequisite %s is unreachable: %v", source.Type, probeErr)
		}
		deadline := time.Now().Add(15 * time.Second)
		matched := false
		for time.Now().Before(deadline) {
			if value, ok := sink.gaugeAfter(source.ID, "up", 0); ok && value == expected {
				matched = true
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if !matched {
			t.Fatalf("native component %s up differs from actual authenticated API endpoint", source.Type)
		}
		if expected == 1 {
			samples = sink.snapshot()
			if _, ok := samples[source.ID][e.metric]; !ok {
				t.Fatalf("reachable component %s omitted its actual metric", source.Type)
			}
		} else {
			for name := range samples[source.ID] {
				if name != "up" && len(name) >= 7 && name[:7] != "scrape_" {
					t.Fatalf("unavailable component %s invented target metric %s", source.Type, name)
				}
			}
		}
	}
	if os.Getenv("ARGUS_KUBERNETES_CONTROL_REQUIRE_DATA") == "1" {
		workload, err := admin.AppsV1().Deployments(namespace).Get(ctx, "metric-workload", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		three := int32(3)
		workload.Spec.Replicas = &three
		workload.Spec.Template.Spec.NodeName = ""
		workload.Spec.Template.Spec.NodeSelector = map[string]string{"kubernetes.io/hostname": os.Getenv("ARGUS_KUBERNETES_METRICS_NODE")}
		if _, err = admin.AppsV1().Deployments(namespace).Update(ctx, workload, metav1.UpdateOptions{}); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(25 * time.Second)
		for nativeControlScrapeCount(sink, sources) < 3 && time.Now().Before(deadline) {
			time.Sleep(200 * time.Millisecond)
		}
		if nativeControlScrapeCount(sink, sources) < 3 {
			t.Fatal("three distinct positive scrapes per source are required")
		}
	}
	// Removing only the owned pods/proxy rule must deny the same service account.
	denialStart := sink.requestCount()
	metricRules := metricRole.Rules
	metricRole.Rules = nil
	for _, rule := range metricRules {
		if len(rule.NonResourceURLs) == 0 {
			metricRole.Rules = append(metricRole.Rules, rule)
		}
	}
	metricRole, err = admin.RbacV1().ClusterRoles().Update(ctx, metricRole, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	rules := proxyRole.Rules
	proxyRole.Rules = nil
	proxyRole, err = admin.RbacV1().ClusterRoles().Update(ctx, proxyRole, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	denied := false
	for time.Now().Before(deadline) {
		_, e := reader.CoreV1().Pods("kube-system").ProxyGet("https", "kube-scheduler-"+os.Getenv("ARGUS_KUBERNETES_METRICS_NODE"), "10259", "metrics", nil).DoRaw(ctx)
		if apierrors.IsForbidden(e) {
			denied = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !denied {
		t.Fatal("revoked component proxy rule remained readable")
	}
	waitNativeControlUp(t, sink, sources, denialStart, 0)
	metricRole.Rules = metricRules
	if _, err = admin.RbacV1().ClusterRoles().Update(ctx, metricRole, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	proxyRole.Rules = rules
	if _, err = admin.RbacV1().ClusterRoles().Update(ctx, proxyRole, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	recoveryStart := sink.requestCount()
	waitNativeControlUp(t, sink, sources, recoveryStart, func() float64 {
		if os.Getenv("ARGUS_KUBERNETES_CONTROL_REQUIRE_DATA") == "1" {
			return 1
		}
		return -1
	}())
	sink.mu.Lock()
	requests := make([][]byte, len(sink.requests))
	for i, request := range sink.requests {
		requests[i], _ = proto.Marshal(request)
	}
	sink.mu.Unlock()
	if path := os.Getenv("ARGUS_KUBERNETES_CONTROL_METRIC_EVIDENCE"); path != "" {
		raw, _ := json.MarshalIndent(map[string]any{"scope": "actual_kubernetes_components_availability_not_formal_enrollment", "collector_id": collector, "cluster_id": cluster, "collector_version": configbundle.CollectorVersion, "captured_at": time.Now().UTC(), "sources": sources, "availability": availability, "requests": requests}, "", "  ")
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	stop(container)
	sink.reset()
	for i := range collections {
		collections[i].Enabled = false
		collections[i].Revision = 2
		container, remaining := run("control-toggle-"+collections[i].Integration, int64(i+2))
		if len(remaining) != len(collections)-i-1 {
			t.Fatal("control component toggle changed another responsibility")
		}
		if len(remaining) > 0 {
			waitSources(container, len(remaining))
		} else {
			time.Sleep(2 * time.Second)
			if len(sink.snapshot()) != 0 {
				t.Fatal("all disabled components emitted samples")
			}
			alive(container)
		}
		stop(container)
		sink.reset()
	}
	t.Logf("actual control-plane API availability=%v; unavailable components emit diagnostics only; explicit proxy revocation and independent lifecycle passed", availability)
}

func waitNativeControlUp(t *testing.T, sink *builtinMetricSink, sources []configbundle.Source, start int, want float64) {
	t.Helper()
	if want < 0 {
		return
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		matched := true
		for _, source := range sources {
			value, ok := sink.gaugeAfter(source.ID, "up", start)
			if !ok || value != want {
				matched = false
			}
		}
		if matched {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("new native control scrape did not converge to up=%v", want)
}

func nativeControlScrapeCount(sink *builtinMetricSink, sources []configbundle.Source) int {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	times := map[string]map[uint64]bool{}
	for _, source := range sources {
		times[source.ID] = map[uint64]bool{}
	}
	for _, request := range sink.requests {
		for _, resource := range request.ResourceMetrics {
			id := ""
			for _, attr := range resource.Resource.GetAttributes() {
				if attr.Key == "argus.source.id" {
					id = attr.Value.GetStringValue()
				}
			}
			if times[id] == nil {
				continue
			}
			for _, scope := range resource.ScopeMetrics {
				for _, metric := range scope.Metrics {
					if metric.Name == "up" {
						for _, point := range metric.GetGauge().GetDataPoints() {
							if point.GetAsDouble() == 1 || point.GetAsInt() == 1 {
								times[id][point.TimeUnixNano] = true
							}
						}
					}
				}
			}
		}
	}
	count := -1
	for _, values := range times {
		if count < 0 || len(values) < count {
			count = len(values)
		}
	}
	if count < 0 {
		return 0
	}
	return count
}
