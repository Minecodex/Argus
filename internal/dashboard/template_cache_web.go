package dashboard

func cacheWebTemplates() []TemplateDefinition {
	return []TemplateDefinition{
		metricTemplate("memcached.overview", "Memcached", "memcached", "https://grafana.com/grafana/dashboards/11527-memcached-overview/", "11527", []nativeTemplateMetric{
			{"memory", `memcached_bytes`, "bytes", "timeseries"},
			{"connections", `memcached_connections_current`, "short", "timeseries"},
			{"items", `memcached_current_items`, "short", "stat"},
			{"evictions", `rate(memcached_evictions[5m])`, "ops", "timeseries"},
			{"commands", `rate(memcached_commands[5m])`, "ops", "timeseries"},
			{"network", `rate(memcached_network[5m])`, "B/s", "timeseries"},
		}),
		metricTemplate("apache.overview", "Apache HTTP Server", "apache", "https://grafana.com/grafana/dashboards/3894-apache/", "3894", []nativeTemplateMetric{
			{"uptime", `apache_uptime`, "s", "stat"},
			{"connections", `apache_current_connections`, "short", "timeseries"},
			{"workers", `apache_workers`, "short", "timeseries"},
			{"requests", `rate(apache_requests[5m])`, "reqps", "timeseries"},
			{"traffic", `rate(apache_traffic[5m])`, "B/s", "timeseries"},
			{"scoreboard", `apache_scoreboard`, "short", "timeseries"},
		}),
		metricTemplate("haproxy.overview", "HAProxy", "haproxy", "https://grafana.com/grafana/dashboards/12693-haproxy/", "12693", []nativeTemplateMetric{
			{"bytes_in", `rate(haproxy_bytes_input[5m])`, "B/s", "timeseries"},
			{"bytes_out", `rate(haproxy_bytes_output[5m])`, "B/s", "timeseries"},
			{"sessions", `haproxy_sessions_count`, "short", "timeseries"},
			{"requests", `rate(haproxy_requests_total[5m])`, "reqps", "timeseries"},
			{"connections_errors", `rate(haproxy_connections_errors[5m])`, "ops", "timeseries"},
			{"requests_queued", `haproxy_requests_queued`, "short", "timeseries"},
		}),
	}
}
