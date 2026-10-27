package dashboard

func postgresProbeTemplates() []TemplateDefinition {
	postgres := metricTemplate("postgresql.overview", "PostgreSQL", "postgresql", "https://grafana.com/grafana/dashboards/9628-postgresql-database/", "9628", []nativeTemplateMetric{
		{"backends", `postgresql_backends`, "short", "timeseries"},
		{"connection_limit", `postgresql_connection_max`, "short", "stat"},
		{"database_size", `postgresql_db_size`, "bytes", "stat"},
		{"commits", `rate(postgresql_commits[5m])`, "ops", "timeseries"},
		{"rollbacks", `rate(postgresql_rollbacks[5m])`, "ops", "timeseries"},
		{"blocks", `rate(postgresql_blocks_read[5m])`, "ops", "timeseries"},
	})
	http := metricTemplate("http.overview", "HTTP checks", "http", "https://grafana.com/grafana/dashboards/7587-prometheus-blackbox-exporter/", "7587", []nativeTemplateMetric{
		{"duration", `httpcheck_duration`, "ms", "timeseries"},
		{"success", `sum(httpcheck_status{http_status_class=~"2xx|3xx"})`, "short", "stat"},
		{"client_error", `httpcheck_status{http_status_class="4xx"}`, "short", "timeseries"},
		{"server_error", `httpcheck_status{http_status_class="5xx"}`, "short", "timeseries"},
	})
	tcp := metricTemplate("tcp.overview", "TCP checks", "tcp", "https://grafana.com/grafana/dashboards/7587-prometheus-blackbox-exporter/", "7587", []nativeTemplateMetric{
		{"status", `tcpcheck_status`, "short", "stat"},
		{"duration", `tcpcheck_duration`, "ms", "timeseries"},
	})
	tls := metricTemplate("tls.overview", "Certificate expiry", "tls", "https://grafana.com/grafana/dashboards/7587-prometheus-blackbox-exporter/", "7587", []nativeTemplateMetric{
		{"minimum_days", `min(tlscheck_time_left) / 86400`, "days", "stat"},
		{"remaining_days", `tlscheck_time_left / 86400`, "days", "timeseries"},
	})
	for _, item := range []struct {
		template *TemplateDefinition
		source   string
	}{{&http, "http_check"}, {&tcp, "tcp_check"}, {&tls, "tls_check"}} {
		for i := range item.template.Spec.Panels {
			item.template.Spec.Panels[i].SourceBinding.SourceType = item.source
		}
	}
	return []TemplateDefinition{postgres, http, tcp, tls}
}
