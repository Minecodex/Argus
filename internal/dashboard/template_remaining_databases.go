package dashboard

// Native metric names and units come from the locked Collector metadata.
// A definition is not a support claim: unavailable observations remain empty,
// and each platform/authentication/topology combination needs its own evidence.
func remainingDatabaseTemplates() []TemplateDefinition {
	return []TemplateDefinition{
		metricTemplate("mongodb.overview", "MongoDB", "mongodb", "https://grafana.com/grafana/dashboards/2583-mongodb/", "2583", []nativeTemplateMetric{
			{"connections", `mongodb_connection_count`, "short", "timeseries"},
			{"memory", `mongodb_memory_usage`, "bytes", "timeseries"},
			{"data", `mongodb_data_size`, "bytes", "timeseries"},
			{"storage", `mongodb_storage_size`, "bytes", "timeseries"},
			{"operations", `rate(mongodb_operation_count[5m])`, "ops", "timeseries"},
			{"receive", `rate(mongodb_network_io_receive[5m])`, "B/s", "timeseries"},
		}),
		metricTemplate("elasticsearch.overview", "Elasticsearch", "elasticsearch", "https://grafana.com/grafana/dashboards/2322-elasticsearch/", "2322", []nativeTemplateMetric{
			{"health", `elasticsearch_cluster_health`, "short", "stat"},
			{"nodes", `elasticsearch_cluster_nodes`, "short", "stat"},
			{"documents", `elasticsearch_index_documents`, "short", "timeseries"},
			{"heap", `jvm_memory_heap_used`, "bytes", "timeseries"},
			{"disk", `elasticsearch_node_fs_disk_available`, "bytes", "timeseries"},
			{"operations", `rate(elasticsearch_index_operations_completed[5m])`, "ops", "timeseries"},
		}),
		metricTemplate("sqlserver.overview", "SQL Server", "sqlserver", "https://grafana.com/grafana/dashboards/1883-telegraf-sql-server-monitoring/", "1883", []nativeTemplateMetric{
			{"connections", `sqlserver_user_connection_count`, "short", "timeseries"},
			{"batches", `sqlserver_batch_request_rate`, "reqps", "timeseries"},
			{"cache", `sqlserver_page_buffer_cache_hit_ratio`, "%", "timeseries"},
			{"page_life", `sqlserver_page_life_expectancy`, "s", "timeseries"},
			{"lock_wait", `sqlserver_lock_wait_rate`, "reqps", "timeseries"},
			{"compilations", `sqlserver_batch_sql_compilation_rate`, "ops", "timeseries"},
		}),
		metricTemplate("oracle.overview", "Oracle", "oracle", "https://grafana.com/grafana/dashboards/13555-oracledb-monitoring-performance-and-table-space-stats/", "13555", []nativeTemplateMetric{
			{"sessions", `oracledb_sessions_usage`, "short", "timeseries"},
			{"cpu", `rate(oracledb_cpu_time[5m])`, "cores", "timeseries"},
			{"pga", `oracledb_pga_memory`, "bytes", "timeseries"},
			{"tablespace", `oracledb_tablespace_size_usage`, "bytes", "timeseries"},
			{"tablespace_limit", `oracledb_tablespace_size_limit`, "bytes", "timeseries"},
			{"commits", `rate(oracledb_user_commits[5m])`, "ops", "timeseries"},
		}),
		metricTemplate("iis.overview", "IIS", "iis", "https://grafana.com/grafana/dashboards/14532-windows-exporter-iis-node/", "14532", []nativeTemplateMetric{
			{"connections", `iis_connection_active`, "short", "timeseries"},
			{"requests", `rate(iis_request_count[5m])`, "reqps", "timeseries"},
			{"network", `rate(iis_network_io[5m])`, "B/s", "timeseries"},
			{"queued", `iis_request_queue_count`, "short", "timeseries"},
			{"oldest_request", `iis_request_queue_age_max`, "ms", "timeseries"},
			{"pool_state", `iis_application_pool_state`, "short", "stat"},
		}),
	}
}
