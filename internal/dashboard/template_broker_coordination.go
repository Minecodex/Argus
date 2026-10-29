package dashboard

func brokerCoordinationTemplates() []TemplateDefinition {
	rabbitmq := metricTemplate("rabbitmq.overview", "RabbitMQ", "rabbitmq", "https://grafana.com/grafana/dashboards/10991-rabbitmq-overview/", "10991", []nativeTemplateMetric{
		{"memory", `rabbitmq_node_mem_used`, "bytes", "timeseries"},
		{"disk", `rabbitmq_node_disk_free`, "bytes", "timeseries"},
		{"descriptors", `rabbitmq_node_fd_used`, "short", "timeseries"},
		{"uptime", `rabbitmq_node_uptime / 1000`, "s", "stat"},
		{"messages", `rabbitmq_message_current`, "short", "timeseries"},
		{"consumers", `rabbitmq_consumer_count`, "short", "timeseries"},
		{"published", `rate(rabbitmq_message_published[5m])`, "ops", "timeseries"},
		{"delivered", `rate(rabbitmq_message_delivered[5m])`, "ops", "timeseries"},
		{"acknowledged", `rate(rabbitmq_message_acknowledged[5m])`, "ops", "timeseries"},
	})
	rabbitmq.Version = 2
	return []TemplateDefinition{
		rabbitmq,
		metricTemplate("zookeeper.overview", "ZooKeeper", "zookeeper", "https://grafana.com/grafana/dashboards/10465-zookeeper/", "10465", []nativeTemplateMetric{
			{"connections", `zookeeper_connection_active`, "short", "timeseries"},
			{"znodes", `zookeeper_znode_count`, "short", "stat"},
			{"watches", `zookeeper_watch_count`, "short", "timeseries"},
			{"requests", `zookeeper_request_active`, "short", "timeseries"},
			{"latency", `zookeeper_latency_avg`, "ms", "timeseries"},
			{"packets", `rate(zookeeper_packet_count[5m])`, "ops", "timeseries"},
		}),
	}
}
