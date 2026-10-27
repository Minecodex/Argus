package configbundle

// Existing Kubernetes component endpoints only; no exporter is installed.
func kubernetesCoreDNSReceiver(interval string) map[string]any {
	httpConfig := func() map[string]any {
		return map[string]any{
			"authorization": map[string]any{"type": "Bearer", "credentials_file": "/var/run/secrets/kubernetes.io/serviceaccount/token"},
			"tls_config":    map[string]any{"ca_file": "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", "server_name": "kubernetes.default.svc", "insecure_skip_verify": false},
		}
	}
	job, discovery := httpConfig(), httpConfig()
	discovery["api_server"], discovery["role"] = "https://kubernetes.default.svc", "pod"
	discovery["namespaces"] = map[string]any{"names": []string{"kube-system"}}
	discovery["selectors"] = []map[string]any{{"role": "pod", "label": "k8s-app=kube-dns"}}
	job["job_name"], job["scrape_interval"], job["scheme"] = "argus-kubernetes-coredns", interval, "https"
	job["kubernetes_sd_configs"] = []map[string]any{discovery}
	job["sample_limit"] = 100000
	// Explicit read-only pods/proxy authority in kube-system is a prerequisite.
	// The API proxy retains component endpoint isolation and API TLS/auth.
	job["relabel_configs"] = []map[string]any{
		{"source_labels": []string{"__meta_kubernetes_pod_phase"}, "regex": "Running", "action": "keep"},
		{"source_labels": []string{"__meta_kubernetes_pod_container_port_number"}, "regex": "9153", "action": "keep"},
		{"source_labels": []string{"__meta_kubernetes_pod_name"}, "target_label": "__metrics_path__", "replacement": "/api/v1/namespaces/kube-system/pods/$$1:9153/proxy/metrics"},
		{"target_label": "__address__", "replacement": "kubernetes.default.svc:443"},
		{"source_labels": []string{"__meta_kubernetes_pod_name"}, "target_label": "k8s_pod_name"},
		{"source_labels": []string{"__meta_kubernetes_pod_uid"}, "target_label": "k8s_pod_uid"},
	}
	job["metric_relabel_configs"] = []map[string]any{{"source_labels": []string{"__name__"}, "regex": "coredns_dns_requests_total|coredns_dns_responses_total|coredns_dns_request_duration_seconds_(bucket|sum|count)|coredns_cache_(hits_total|requests_total|entries)|process_resident_memory_bytes", "action": "keep"}}
	return map[string]any{"config": map[string]any{"scrape_configs": []map[string]any{job}}}
}
