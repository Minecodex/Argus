package dashboard

import "github.com/kakj-go/Argus/internal/builtincollection"

func ownMetricTemplates() []TemplateDefinition {
	host := metricTemplate("host.overview", "Host", "host_metrics", "https://grafana.com/grafana/dashboards/1860-node-exporter-full/", "1860", []nativeTemplateMetric{
		{"cpu", `1 - avg by (argus_resource_id) (system_cpu_utilization{state="idle"})`, "percent_ratio", "timeseries"},
		{"memory", `system_memory_utilization{state="used"}`, "percent_ratio", "timeseries"},
		{"memory_bytes", `system_memory_usage{state="used"}`, "bytes", "stat"},
		{"filesystem", `system_filesystem_usage{state="used"}`, "bytes", "timeseries"},
		{"disk_io", `rate(system_disk_io[5m])`, "B/s", "timeseries"},
		{"network_io", `rate(system_network_io[5m])`, "B/s", "timeseries"},
	})
	self := metricTemplate("collector.overview", "Collector", "collector_metrics", "https://grafana.com/grafana/dashboards/12553-opentelemetry-collector/", "12553", []nativeTemplateMetric{
		{"memory", `otelcol_process_memory_rss`, "bytes", "timeseries"},
		{"heap", `otelcol_process_runtime_heap_alloc_bytes`, "bytes", "timeseries"},
		{"uptime", `otelcol_process_uptime`, "s", "stat"},
		{"queue", `otelcol_exporter_queue_size`, "", "timeseries"},
		{"capacity", `otelcol_exporter_queue_capacity`, "", "timeseries"},
		{"sent", `rate(otelcol_exporter_sent_metric_points[5m])`, "ops/s", "timeseries"},
	})
	cluster := metricTemplate("kubernetes.cluster", "Kubernetes cluster", "kubernetes_cluster_metrics", "https://grafana.com/grafana/dashboards/16364-kubernetes-compute-resources-cluster/", "16364", []nativeTemplateMetric{
		{"deployments_available", `k8s_deployment_available`, "", "timeseries"},
		{"deployments_desired", `k8s_deployment_desired`, "", "timeseries"},
		{"daemonsets_ready", `k8s_daemonset_ready_nodes`, "", "timeseries"},
		{"nodes_ready", `k8s_node_condition_ready`, "", "timeseries"},
		{"restarts", `k8s_container_restarts`, "", "timeseries"},
		{"cpu_requests", `k8s_container_cpu_request`, "cores", "timeseries"},
	})
	node := metricTemplate("kubernetes.nodes", "Kubernetes nodes and containers", "kubernetes_node_metrics", "https://grafana.com/grafana/dashboards/21788-kubernetes-kubelet/", "21788", []nativeTemplateMetric{
		{"node_cpu", `k8s_node_cpu_usage`, "cores", "timeseries"},
		{"node_memory", `k8s_node_memory_usage`, "bytes", "timeseries"},
		{"node_filesystem", `k8s_node_filesystem_usage`, "bytes", "timeseries"},
		{"pod_memory", `k8s_pod_memory_usage`, "bytes", "timeseries"},
		{"container_cpu", `container_cpu_usage`, "cores", "timeseries"},
		{"node_network", `rate(k8s_node_network_io[5m])`, "B/s", "timeseries"},
	})
	apiServer := metricTemplate("kubernetes.apiserver", "Kubernetes API server", "kubernetes_apiserver_metrics", "https://grafana.com/grafana/dashboards/12654-kubernetes-api-server/", "12654", []nativeTemplateMetric{
		{"api_up", `up`, "", "stat"},
		{"api_requests", `sum(rate(apiserver_request_total[5m]))`, "req/s", "timeseries"},
		{"api_latency", `sum(rate(apiserver_request_duration_seconds_sum[5m])) / sum(rate(apiserver_request_duration_seconds_count[5m]))`, "s", "timeseries"},
		{"api_p95", `histogram_quantile(0.95, sum by (le) (rate(apiserver_request_duration_seconds_bucket[5m])))`, "s", "timeseries"},
		{"api_inflight", `apiserver_current_inflight_requests`, "", "timeseries"},
		{"api_memory", `process_resident_memory_bytes`, "bytes", "timeseries"},
	})
	dns := metricTemplate("kubernetes.coredns", "Kubernetes CoreDNS", "kubernetes_coredns_metrics", "https://grafana.com/grafana/dashboards/15762-kubernetes-system-coredns/", "15762", []nativeTemplateMetric{
		{"dns_up", `up`, "", "stat"},
		{"dns_requests", `sum(rate(coredns_dns_requests_total[5m]))`, "req/s", "timeseries"},
		{"dns_responses", `sum(rate(coredns_dns_responses_total[5m]))`, "req/s", "timeseries"},
		{"dns_p95", `histogram_quantile(0.95, sum by (le) (rate(coredns_dns_request_duration_seconds_bucket[5m])))`, "s", "timeseries"},
		{"dns_cache_hits", `sum(rate(coredns_cache_hits_total[5m])) / sum(rate(coredns_cache_requests_total[5m]))`, "percent_ratio", "timeseries"},
		{"dns_memory", `process_resident_memory_bytes`, "bytes", "timeseries"},
	})
	result := []TemplateDefinition{host, self, cluster, node, apiServer, dns}
	result = append(result, hostServiceTemplates()...)
	result = append(result, kubernetesControlTemplates()...)
	for i := range result {
		definition, _ := builtincollection.Lookup(result[i].IntegrationKeys[0])
		for j := range result[i].Spec.Panels {
			result[i].Spec.Panels[j].ApplicableResourceTypes = definition.ResourceTypes
			result[i].Spec.Panels[j].SourceBinding.SourceType = definition.SourceType
		}
	}
	return result
}
