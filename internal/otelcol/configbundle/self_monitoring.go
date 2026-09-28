package configbundle

// Internal Collector metrics are a separate Prometheus receiver/source from
// managed application endpoints and hostmetrics. The listener is loopback only.
func addSelfMonitoring(config map[string]any) {
	config["receivers"].(map[string]any)["prometheus/collector_self"] = map[string]any{
		"config": map[string]any{"scrape_configs": []map[string]any{{
			"job_name": "argus-collector-self", "scrape_interval": "30s",
			"static_configs": []map[string]any{{"targets": []string{"127.0.0.1:8888"}}},
		}}},
	}
	service := config["service"].(map[string]any)
	service["telemetry"] = map[string]any{"metrics": map[string]any{
		"readers": []map[string]any{{"pull": map[string]any{"exporter": map[string]any{"prometheus": map[string]any{"host": "127.0.0.1", "port": 8888}}}}},
	}}
	exporter := "otlp/argus"
	if config["exporters"].(map[string]any)["otlp/gateway"] != nil {
		exporter = "otlp/gateway"
	}
	service["pipelines"].(map[string]any)["metrics/collector_self"] = map[string]any{
		"receivers": []string{"prometheus/collector_self"}, "processors": []string{"memory_limiter", "resource/argus", "batch"}, "exporters": []string{exporter},
	}
}
