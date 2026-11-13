package dashboard

const operationsName = "Kubernetes 运维大盘 / Operations overview"

func operationsOverviewTemplate() TemplateDefinition {
	spec := EmptySpec()
	spec.SchemaVersion = SchemaVersionV2
	spec.DefaultRefreshSeconds = 30
	native := func(id, title, kind, view string, x, y, w, h int, types, states []string) Panel {
		signal := "resources"
		if kind == "alert_instances" {
			signal = "alerts"
		}
		return Panel{ID: id, Title: title, Type: view, Signal: signal, AuthoringMode: "builder", ApplicableResourceTypes: []string{"kubernetes_cluster", "kubernetes_namespace"}, SourceBinding: SourceBinding{SourceType: kind, CapabilityVersion: "v1"}, DataSource: &OperationalSource{Kind: kind, ResourceTypes: types, States: states, Limit: 20}, LocalFilters: []LocalFilter{}, Targets: []Target{}, DetailQueryTargets: []Target{}, Drilldowns: []Drilldown{}, Thresholds: []Threshold{}, Layout: Rectangle{X: x, Y: y, W: w, H: h, MinW: 2, MinH: 12}}
	}
	spec.Panels = []Panel{
		native("health", "资源状态 / Resource status", "kubernetes_inventory", "resource_summary", 0, 0, 9, 28, nil, nil),
		native("active_alerts", "活跃告警 / Active alerts", "alert_instances", "alert_summary", 9, 0, 3, 28, nil, nil),
		native("nodes", "节点与 Pod 分布 / Nodes and Pods", "kubernetes_inventory", "node_allocation", 0, 28, 6, 42, nil, nil),
		native("distribution", "资源状态构成 / Resource distribution", "kubernetes_inventory", "resource_distribution", 6, 28, 6, 42, nil, nil),
	}
	metrics := metricTemplate("operations.trends", "Resource usage", "kubernetes_node_metrics", "", "", []nativeTemplateMetric{
		{"cpu", "sum by (resource_k8s_namespace_name) (k8s_pod_cpu_usage)", "cores", "timeseries"},
		{"memory", "sum by (resource_k8s_namespace_name) (k8s_pod_memory_usage)", "bytes", "timeseries"},
	})
	for i, p := range metrics.Spec.Panels {
		p.ID = []string{"cpu", "memory"}[i]
		p.Title = []string{"CPU 使用 / CPU usage", "内存使用 / Memory usage"}[i]
		p.SourceBinding = SourceBinding{SourceType: "kubeletstats", CapabilityVersion: "v1"}
		p.ApplicableResourceTypes = []string{"kubernetes_cluster", "kubernetes_namespace"}
		p.Layout = Rectangle{X: i * 6, Y: 70, W: 6, H: 52, MinW: 4, MinH: 24}
		spec.Panels = append(spec.Panels, p)
	}
	requests := metricTemplate("operations.requests", "Resource requests", "kubernetes_cluster_metrics", "", "", []nativeTemplateMetric{{"cpu_requests", "sum by (resource_k8s_namespace_name) (k8s_container_cpu_request)", "cores", "timeseries"}, {"memory_requests", "sum by (resource_k8s_namespace_name) (k8s_container_memory_request)", "bytes", "timeseries"}})
	for i, p := range requests.Spec.Panels {
		p.SourceBinding = SourceBinding{SourceType: "k8s_cluster", CapabilityVersion: "v1"}
		p.ID = []string{"cpu_requests", "memory_requests"}[i]
		p.Title = []string{"CPU 请求 / CPU requests", "内存请求 / Memory requests"}[i]
		p.ApplicableResourceTypes = []string{"kubernetes_cluster", "kubernetes_namespace"}
		p.Layout = Rectangle{X: i * 6, Y: 122, W: 6, H: 52, MinW: 4, MinH: 24}
		spec.Panels = append(spec.Panels, p)
	}
	spec.Panels = append(spec.Panels,
		native("relations", "资源关系 / Resource relationships", "kubernetes_inventory", "resource_graph", 0, 174, 12, 62, nil, nil),
		native("events", "近期警告事件 / Recent warning events", "kubernetes_inventory", "resource_events", 0, 236, 6, 34, []string{"event"}, []string{"warning"}),
		native("attention", "需要关注的资源 / Resources needing attention", "kubernetes_inventory", "resource_table", 6, 236, 6, 34, []string{"pod", "deployment", "statefulset", "daemonset", "job", "pvc"}, []string{"failed", "warning", "pending", "unknown"}),
		native("alerts", "活跃告警 / Active alerts", "alert_instances", "alert_list", 0, 270, 12, 34, nil, nil))
	return TemplateDefinition{Key: OperationsOverviewTemplate, Version: 3, Name: operationsName, Description: "当前授权资源的状态、分布、关系和告警；历史趋势来自真实采集。Current authorized resource state and relationships, with real telemetry trends.", IntegrationKeys: []string{}, Spec: spec, References: []TemplateReference{}}
}
