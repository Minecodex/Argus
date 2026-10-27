package configbundle

import (
	"github.com/kakj-go/Argus/internal/builtincollection"
	"strings"
)

type controlMetricEndpoint struct{ component, port, scheme, metrics string }

var controlMetricEndpoints = map[string]controlMetricEndpoint{
	"kubernetes_scheduler_metrics":  {"kube-scheduler", "10259", "https", "scheduler_schedule_attempts_total|scheduler_scheduling_attempt_duration_seconds_(bucket|sum|count)|scheduler_pending_pods|process_resident_memory_bytes|process_cpu_seconds_total|go_goroutines"},
	"kubernetes_controller_metrics": {"kube-controller-manager", "10257", "https", "workqueue_depth|workqueue_adds_total|workqueue_queue_duration_seconds_(bucket|sum|count)|workqueue_work_duration_seconds_(bucket|sum|count)|process_resident_memory_bytes|process_cpu_seconds_total|go_goroutines"},
	"kubernetes_etcd_metrics":       {"etcd", "2381", "http", "etcd_server_has_leader|etcd_server_leader_changes_seen_total|etcd_server_proposals_failed_total|etcd_server_proposals_committed_total|etcd_disk_wal_fsync_duration_seconds_(bucket|sum|count)|etcd_mvcc_db_total_size_in_bytes|process_resident_memory_bytes"},
}

// Secure components are scraped directly: API proxy removes Authorization.
// Native HTTP etcd metrics retain the authenticated API proxy route. Neither
// route changes customer listeners, RBAC or component authentication settings.
func kubernetesControlMetricReceiver(key string, value builtincollection.Config) map[string]any {
	endpoint := controlMetricEndpoints[key]
	httpConfig := func() map[string]any {
		return map[string]any{"authorization": map[string]any{"type": "Bearer", "credentials_file": "/var/run/secrets/kubernetes.io/serviceaccount/token"}, "tls_config": map[string]any{"ca_file": "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", "server_name": "kubernetes.default.svc", "insecure_skip_verify": false}}
	}
	job, discovery := httpConfig(), httpConfig()
	discovery["api_server"], discovery["role"] = "https://kubernetes.default.svc", "pod"
	discovery["namespaces"] = map[string]any{"names": []string{"kube-system"}}
	discovery["selectors"] = []map[string]any{{"role": "pod", "label": "component=" + endpoint.component}}
	job["job_name"], job["scrape_interval"], job["scrape_timeout"], job["scheme"] = "argus-"+strings.ReplaceAll(key, "_", "-"), value.Interval, "4s", "https"
	job["kubernetes_sd_configs"] = []map[string]any{discovery}
	job["sample_limit"] = 100000
	job["relabel_configs"] = []map[string]any{
		{"source_labels": []string{"__meta_kubernetes_pod_phase"}, "regex": "Running", "action": "keep"},
		{"source_labels": []string{"__meta_kubernetes_pod_container_name"}, "regex": endpoint.component, "action": "keep"},
		{"source_labels": []string{"__meta_kubernetes_pod_container_port_number"}, "regex": "(|" + endpoint.port + ")", "action": "keep"},
		{"source_labels": []string{"__meta_kubernetes_pod_name"}, "target_label": "k8s_pod_name"},
		{"source_labels": []string{"__meta_kubernetes_pod_uid"}, "target_label": "k8s_pod_uid"},
	}
	rules := job["relabel_configs"].([]map[string]any)
	if endpoint.scheme == "https" {
		tls := map[string]any{"ca_file": "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", "server_name": endpoint.component, "insecure_skip_verify": false}
		if value.ComponentTLS != nil {
			if value.ComponentTLS.ServerName != "" {
				tls["server_name"] = value.ComponentTLS.ServerName
			}
			if value.ComponentTLS.CAPEM != "" {
				delete(tls, "ca_file")
				tls["ca"] = value.ComponentTLS.CAPEM
			}
		}
		job["tls_config"], job["metrics_path"] = tls, "/metrics"
		rules = append(rules, map[string]any{"source_labels": []string{"__meta_kubernetes_pod_ip"}, "regex": ".+", "action": "keep"}, map[string]any{"source_labels": []string{"__meta_kubernetes_pod_ip"}, "target_label": "__address__", "replacement": "$$1:" + endpoint.port}, map[string]any{"source_labels": []string{"__meta_kubernetes_pod_ip"}, "regex": "(.+:.*)", "target_label": "__address__", "replacement": "[$$1]:" + endpoint.port})
	} else {
		rules = append(rules, map[string]any{"source_labels": []string{"__meta_kubernetes_pod_name"}, "target_label": "__metrics_path__", "replacement": "/api/v1/namespaces/kube-system/pods/$$1:" + endpoint.port + "/proxy/metrics"}, map[string]any{"target_label": "__address__", "replacement": "kubernetes.default.svc:443"})
	}
	job["relabel_configs"] = rules
	job["metric_relabel_configs"] = []map[string]any{{"source_labels": []string{"__name__"}, "regex": endpoint.metrics, "action": "keep"}}
	return map[string]any{"config": map[string]any{"scrape_configs": []map[string]any{job}}}
}
