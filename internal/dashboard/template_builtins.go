package dashboard

import (
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

// Initial content uses metrics observed from the locked native receivers.
// Grafana references inform layout and dimensions, never exporter queries.
func BuiltinTemplates() []TemplateDefinition {
	result := []TemplateDefinition{
		metricTemplate("mysql.overview", "MySQL", "mysql", "https://grafana.com/grafana/dashboards/7362-mysql-overview/", "7362", []nativeTemplateMetric{
			{"uptime", "mysql_uptime", "s", "stat"}, {"threads", "mysql_threads", "short", "timeseries"}, {"buffer_usage", "mysql_buffer_pool_usage", "bytes", "timeseries"}, {"buffer_limit", "mysql_buffer_pool_limit", "bytes", "stat"}, {"operations", "rate(mysql_operations[5m])", "ops", "timeseries"}, {"row_locks", "mysql_row_locks", "short", "timeseries"},
		}),
		metricTemplate("redis.overview", "Redis", "redis", "https://grafana.com/grafana/dashboards/763-redis-dashboard-for-prometheus-redis-exporter-1-x/", "763", []nativeTemplateMetric{
			{"uptime", "redis_uptime", "s", "stat"}, {"clients", "redis_clients_connected", "short", "timeseries"}, {"memory", "redis_memory_used", "bytes", "timeseries"}, {"memory_rss", "redis_memory_rss", "bytes", "timeseries"}, {"commands", "rate(redis_commands_processed[5m])", "ops", "timeseries"}, {"blocked", "redis_clients_blocked", "short", "timeseries"},
		}),
		metricTemplate("nginx.overview", "Nginx", "nginx", "https://grafana.com/grafana/dashboards/12708-nginx/", "12708", []nativeTemplateMetric{
			{"connections", "nginx_connections_current", "short", "timeseries"}, {"requests", "rate(nginx_requests[5m])", "reqps", "timeseries"}, {"accepted", "rate(nginx_connections_accepted[5m])", "ops", "timeseries"}, {"handled", "rate(nginx_connections_handled[5m])", "ops", "timeseries"},
		}),
	}
	for _, group := range [][]TemplateDefinition{ownMetricTemplates(), postgresProbeTemplates(), cacheWebTemplates(), brokerCoordinationTemplates(), networkKafkaTemplates(), remainingDatabaseTemplates()} {
		result = append(result, group...)
	}
	return result
}

type nativeTemplateMetric struct{ key, expression, unit, chart string }

func metricTemplate(key, name, integration, reference, id string, metrics []nativeTemplateMetric) TemplateDefinition {
	spec := EmptySpec()
	spec.DefaultRefreshSeconds = 30
	sourceType := integration
	if entry, exists := monitoringcatalog.Lookup(integration); exists && entry.AdapterReady && entry.Component != "" {
		sourceType = entry.Component
	}
	for i, metric := range metrics {
		// Grafana's unit identifiers are references, not user-facing Argus units.
		switch metric.unit {
		case "short":
			metric.unit = ""
		case "ops":
			metric.unit = "ops/s"
		case "reqps":
			metric.unit = "req/s"
		}
		mode := "range"
		if metric.chart == "stat" {
			mode = "instant"
		}
		spec.Panels = append(spec.Panels, Panel{ID: metric.key, Title: metric.expression, Description: "", Type: metric.chart, Signal: "metrics", AuthoringMode: "dsl", ApplicableResourceTypes: []string{"monitoring_target"}, SourceBinding: SourceBinding{SourceType: sourceType, CapabilityVersion: "v1"}, LocalFilters: []LocalFilter{}, DetailQueryTargets: []Target{}, Drilldowns: []Drilldown{}, Layout: Rectangle{X: i % 2 * 6, Y: i / 2 * 36, W: 6, H: 36, MinW: 2, MinH: 20}, Unit: metric.unit, Decimals: 2, Legend: true, Thresholds: []Threshold{}, Targets: []Target{{ID: "a", Language: queryengine.LanguagePromQL, SourceDefinition: Definition{DSL: &DSL{Expression: metric.expression}}, QueryMode: mode, RangeStepPolicy: StepPolicy{Kind: "auto", TargetPoints: 300, MinStepSeconds: 1}, ParameterBindings: []ParameterBinding{}}}})
	}
	return TemplateDefinition{Key: key, Version: 1, Name: name, Description: "", IntegrationKeys: []string{integration}, Spec: spec, References: []TemplateReference{{Title: name + " Grafana reference", URL: reference, DashboardID: id}}}
}
