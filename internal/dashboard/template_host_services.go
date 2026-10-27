package dashboard

// References supply layout and units only. These expressions use the native
// embedded receivers; none require the exporters used by the reference decks.
func hostServiceTemplates() []TemplateDefinition {
	process := metricTemplate("host.processes", "Host processes", "host_process_metrics", "https://grafana.com/grafana/dashboards/249-named-processes/", "249", []nativeTemplateMetric{
		{"cpu", `sum by (resource_process_executable_name) (rate(process_cpu_time[5m]))`, "cores", "timeseries"},
		{"memory", `sum by (resource_process_executable_name) (process_memory_usage)`, "bytes", "timeseries"},
		{"virtual_memory", `sum by (resource_process_executable_name) (process_memory_virtual)`, "bytes", "timeseries"},
		{"io", `sum by (resource_process_executable_name, direction) (rate(process_disk_io[5m]))`, "B/s", "timeseries"},
	})
	systemd := metricTemplate("host.systemd", "Linux system services", "host_systemd_metrics", "https://grafana.com/grafana/dashboards/1860-node-exporter-full/", "1860", []nativeTemplateMetric{
		{"active", `sum(systemd_unit_state{systemd_unit_active_state="active"})`, "short", "stat"},
		{"failed", `sum(systemd_unit_state{systemd_unit_active_state="failed"})`, "short", "stat"},
		{"cpu", `sum by (resource_systemd_unit_name) (rate(systemd_service_cpu_time[5m])) / 1000000`, "cores", "timeseries"},
		{"restarts", `systemd_service_restarts`, "short", "timeseries"},
	})
	windows := metricTemplate("host.windows_services", "Windows system services", "host_windows_service_metrics", "https://grafana.com/grafana/dashboards/14694-windows-exporter-dashboard/", "14694", []nativeTemplateMetric{
		{"states", `windows_service_status`, "short", "timeseries"},
		{"running", `sum(windows_service_status == bool 4)`, "short", "stat"},
	})
	process.Version, systemd.Version = 2, 2
	return []TemplateDefinition{process, systemd, windows}
}
