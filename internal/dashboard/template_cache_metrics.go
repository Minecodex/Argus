package dashboard

// Aggregate each cache independently and leave an idle ratio unavailable.
// No GET requests is neither a 0% nor a 100% success observation.
func cacheHitRatio(hits, misses string) string {
	h := "sum by (argus_resource_id) (rate(" + hits + "[5m]))"
	m := "sum by (argus_resource_id) (rate(" + misses + "[5m]))"
	total := "(" + h + " + " + m + ")"
	return "(" + h + " / " + total + ") and (" + total + " > 0)"
}

func redisTemplate() TemplateDefinition {
	definition := metricTemplate("redis.overview", "Redis", "redis", "https://grafana.com/grafana/dashboards/763-redis-dashboard-for-prometheus-redis-exporter-1-x/", "763", []nativeTemplateMetric{
		{"uptime", "redis_uptime", "s", "stat"},
		{"clients", "redis_clients_connected", "short", "timeseries"},
		{"memory", "redis_memory_used", "bytes", "timeseries"},
		{"memory_rss", "redis_memory_rss", "bytes", "timeseries"},
		{"commands", "rate(redis_commands_processed[5m])", "ops", "timeseries"},
		{"blocked", "redis_clients_blocked", "short", "timeseries"},
		{"hits", "rate(redis_keyspace_hits[5m])", "ops", "timeseries"},
		{"misses", "rate(redis_keyspace_misses[5m])", "ops", "timeseries"},
		{"hit_ratio", cacheHitRatio("redis_keyspace_hits", "redis_keyspace_misses"), "percent_ratio", "timeseries"},
		{"evictions", "rate(redis_keys_evicted[5m])", "ops", "timeseries"},
		{"expired", "rate(redis_keys_expired[5m])", "ops", "timeseries"},
	})
	definition.Version = 2
	return definition
}

func memcachedTemplate() TemplateDefinition {
	definition := metricTemplate("memcached.overview", "Memcached", "memcached", "https://grafana.com/grafana/dashboards/11527-memcached-overview/", "11527", []nativeTemplateMetric{
		{"memory", "memcached_bytes", "bytes", "timeseries"},
		{"connections", "memcached_connections_current", "short", "timeseries"},
		{"items", "memcached_current_items", "short", "stat"},
		{"evictions", "rate(memcached_evictions[5m])", "ops", "timeseries"},
		{"commands", "rate(memcached_commands[5m])", "ops", "timeseries"},
		{"network", "rate(memcached_network[5m])", "B/s", "timeseries"},
		{"hits", `rate(memcached_operations{operation="get",type="hit"}[5m])`, "ops", "timeseries"},
		{"misses", `rate(memcached_operations{operation="get",type="miss"}[5m])`, "ops", "timeseries"},
		{"hit_ratio", cacheHitRatio(`memcached_operations{operation="get",type="hit"}`, `memcached_operations{operation="get",type="miss"}`), "percent_ratio", "timeseries"},
	})
	definition.Version = 2
	return definition
}
