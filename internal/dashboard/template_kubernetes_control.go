package dashboard

func kubernetesControlTemplates() []TemplateDefinition {
	return []TemplateDefinition{
		metricTemplate("kubernetes.scheduler", "Kubernetes Scheduler", "kubernetes_scheduler_metrics", "https://grafana.com/grafana/dashboards/24353-kubernetes-system-kube-scheduler/", "24353", []nativeTemplateMetric{
			{"up", `up`, "short", "stat"},
			{"attempts", `sum by (result) (rate(scheduler_schedule_attempts_total[5m]))`, "ops", "timeseries"},
			{"p95", `histogram_quantile(0.95, sum by (le) (rate(scheduler_scheduling_attempt_duration_seconds_bucket[5m]))) >= 0`, "s", "timeseries"},
			{"pending", `scheduler_pending_pods`, "short", "timeseries"},
			{"memory", `process_resident_memory_bytes`, "bytes", "timeseries"},
			{"goroutines", `go_goroutines`, "short", "timeseries"},
		}),
		metricTemplate("kubernetes.controller_manager", "Kubernetes Controller Manager", "kubernetes_controller_metrics", "https://grafana.com/grafana/dashboards/24352-kubernetes-system-kube-controller/", "24352", []nativeTemplateMetric{
			{"up", `up`, "short", "stat"},
			{"depth", `workqueue_depth`, "short", "timeseries"},
			{"adds", `sum by (name) (rate(workqueue_adds_total[5m]))`, "ops", "timeseries"},
			{"queue_p95", `histogram_quantile(0.95, sum by (le, name) (rate(workqueue_queue_duration_seconds_bucket[5m]))) >= 0`, "s", "timeseries"},
			{"memory", `process_resident_memory_bytes`, "bytes", "timeseries"},
			{"goroutines", `go_goroutines`, "short", "timeseries"},
		}),
		metricTemplate("kubernetes.etcd", "Kubernetes etcd component", "kubernetes_etcd_metrics", "https://grafana.com/grafana/dashboards/22236-etcd/", "22236", []nativeTemplateMetric{
			{"up", `up`, "short", "stat"},
			{"leader", `etcd_server_has_leader`, "short", "stat"},
			{"changes", `rate(etcd_server_leader_changes_seen_total[5m])`, "ops", "timeseries"},
			{"proposals_failed", `rate(etcd_server_proposals_failed_total[5m])`, "ops", "timeseries"},
			{"fsync_p95", `histogram_quantile(0.95, sum by (le) (rate(etcd_disk_wal_fsync_duration_seconds_bucket[5m]))) >= 0`, "s", "timeseries"},
			{"database", `etcd_mvcc_db_total_size_in_bytes`, "bytes", "timeseries"},
		}),
	}
}
