package dashboard

func networkKafkaTemplates() []TemplateDefinition {
	dns := metricTemplate("dns.overview", "DNS checks", "dns", "https://grafana.com/grafana/dashboards/7587-prometheus-blackbox-exporter/", "7587", []nativeTemplateMetric{
		{"status", `dnscheck_status`, "", "stat"},
		{"duration", `dnscheck_duration`, "ms", "timeseries"},
	})
	for i := range dns.Spec.Panels {
		dns.Spec.Panels[i].SourceBinding.SourceType = "dns_check"
	}
	icmp := metricTemplate("icmp.overview", "ICMP checks", "icmp", "https://grafana.com/grafana/dashboards/7587-prometheus-blackbox-exporter/", "7587", []nativeTemplateMetric{
		{"loss", `ping_loss_ratio`, "percent", "timeseries"},
		// Upstream emits zero RTT when every packet is lost. Suppress that RTT
		// rather than presenting an unreachable destination as a fast reply.
		{"average", `ping_rtt_avg and (ping_loss_ratio < 100)`, "ms", "timeseries"},
		{"minimum", `ping_rtt_min and (ping_loss_ratio < 100)`, "ms", "timeseries"},
		{"maximum", `ping_rtt_max and (ping_loss_ratio < 100)`, "ms", "timeseries"},
		{"jitter", `ping_rtt_stddev and (ping_loss_ratio < 100)`, "ms", "timeseries"},
	})
	for i := range icmp.Spec.Panels {
		icmp.Spec.Panels[i].SourceBinding.SourceType = "icmpcheckreceiver"
	}
	kafka := metricTemplate("kafka.overview", "Kafka", "kafka", "https://grafana.com/grafana/dashboards/7589-kafka-exporter-overview/", "7589", []nativeTemplateMetric{
		{"brokers", `kafka_brokers`, "short", "stat"},
		{"partitions", `kafka_topic_partitions`, "short", "timeseries"},
		{"current_offset", `kafka_partition_current_offset`, "short", "timeseries"},
		{"replicas", `kafka_partition_replicas`, "short", "timeseries"},
		{"in_sync", `kafka_partition_replicas_in_sync`, "short", "timeseries"},
		{"consumer_offset", `kafka_consumer_group_offset`, "short", "timeseries"},
		{"consumer_lag", `kafka_consumer_group_lag`, "short", "timeseries"},
		{"consumer_members", `kafka_consumer_group_members`, "short", "timeseries"},
	})
	for i := range kafka.Spec.Panels {
		kafka.Spec.Panels[i].SourceBinding.SourceType = "kafka_metrics"
	}
	return []TemplateDefinition{dns, icmp, kafka}
}
