package dashboard

// Native metric names and units come from the locked Collector metadata.
// A definition is not a support claim: unavailable observations remain empty,
// and each platform/authentication/topology combination needs its own evidence.
func remainingDatabaseTemplates() []TemplateDefinition {
	mongodb := metricTemplate("mongodb.overview", "MongoDB", "mongodb", "https://grafana.com/grafana/dashboards/2583-mongodb/", "2583", []nativeTemplateMetric{
		// serverStatus connections are instance-wide even when the receiver
		// repeats them with a database dimension. Do not sum those copies.
		{"connections", `max by (argus_resource_id, type) (mongodb_connection_count)`, "short", "timeseries"},
		{"memory", `mongodb_memory_usage`, "bytes", "timeseries"},
		{"data", `mongodb_data_size`, "bytes", "timeseries"},
		{"storage", `mongodb_storage_size`, "bytes", "timeseries"},
		{"operations", `rate(mongodb_operation_count[5m])`, "ops", "timeseries"},
		{"receive", `rate(mongodb_network_io_receive[5m])`, "B/s", "timeseries"},
		{"cache", `rate(mongodb_cache_operations[5m])`, "ops", "timeseries"},
	})
	mongodb.Version = 2
	elasticsearch := metricTemplate("elasticsearch.overview", "Elasticsearch", "elasticsearch", "https://grafana.com/grafana/dashboards/2322-elasticsearch/", "2322", []nativeTemplateMetric{
		{"health", `elasticsearch_cluster_health`, "short", "stat"},
		{"nodes", `elasticsearch_cluster_nodes`, "short", "stat"},
		// Primary and total shard aggregations overlap. Show logical active
		// documents and primary operations without counting replica copies.
		{"documents", `sum by (argus_resource_id, resource_elasticsearch_index_name) (elasticsearch_index_documents{aggregation="primary_shards",state="active",resource_elasticsearch_index_name!="_all"})`, "short", "timeseries"},
		{"heap", `jvm_memory_heap_used`, "bytes", "timeseries"},
		{"disk", `elasticsearch_node_fs_disk_available`, "bytes", "timeseries"},
		{"operations", `sum by (argus_resource_id, resource_elasticsearch_index_name, operation) (rate(elasticsearch_index_operations_completed{aggregation="primary_shards",resource_elasticsearch_index_name!="_all"}[5m]))`, "ops", "timeseries"},
		{"unassigned", `elasticsearch_cluster_shards{state="unassigned"}`, "short", "timeseries"},
	})
	elasticsearch.Version = 2
	// Argus supplies a datasource on every platform. In the locked receiver's
	// SQL path these three rate-named gauges contain cumulative DMV counters;
	// only its separate Windows performance-counter path emits cooked rates.
	sqlserver := metricTemplate("sqlserver.overview", "SQL Server", "sqlserver", "https://grafana.com/grafana/dashboards/1883-telegraf-sql-server-monitoring/", "1883", []nativeTemplateMetric{
		{"connections", `sqlserver_user_connection_count`, "short", "timeseries"},
		{"batches", `rate(sqlserver_batch_request_rate[1m])`, "reqps", "timeseries"},
		{"cache", `sqlserver_page_buffer_cache_hit_ratio`, "%", "timeseries"},
		// Buffer Node overlaps Buffer Manager; use the instance-wide value.
		{"page_life", `sqlserver_page_life_expectancy{performance_counter_object_name=~".*Buffer Manager"}`, "s", "timeseries"},
		{"lock_wait", `rate(sqlserver_lock_wait_rate[1m])`, "reqps", "timeseries"},
		{"compilations", `rate(sqlserver_batch_sql_compilation_rate[1m])`, "ops", "timeseries"},
	})
	sqlserver.Version = 2
	return []TemplateDefinition{
		mongodb,
		elasticsearch,
		sqlserver,
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
