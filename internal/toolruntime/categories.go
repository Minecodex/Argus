package toolruntime

import "strings"

// These definitions supply both the Agent's navigation and native tool schema.
// Categories describe potential capabilities; each listing applies current scope.
var nativeCategories = []struct{ name, description string }{
	{"application", "应用、服务、SDK 接入与 Key 元数据、应用关系和变更预览；环境继承应用 / Applications, services, SDK endpoints, Key metadata, relationships and change previews."},
	{"monitoring", "专用监控能力、目标、采集实例、执行器选择及变更预览；凭据由宿主安全表单填写 / Dedicated integrations, targets, collection instances, collector choices and previews; secrets use trusted host forms."},
	{"host", "主机查询、接入与组件维护、主机采集器维护 / Host inventory, onboarding, components and host collectors."},
	{"k8s", "集群与 Kubernetes 对象查询、Pod 日志、集群采集器和节点主机绑定 / Kubernetes clusters, objects, Pod logs, collectors and node-host bindings."},
	{"metric", "指标值和历史趋势使用 query；overview 仅说明信号覆盖，不返回指标数值 / Use query for metric values and history; overview reports signal coverage, not measurements."},
	{"trace", "应用 APM 服务/实例/接口/RED/拓扑及调用链、慢请求和 Span 查询；应用性能优先使用 application_view / Application APM views and distributed traces; prefer typed application_view for performance."},
	{"log", "KQL 日志检索、过滤和聚合；Pod 日志属于 k8s / KQL log search, filtering and aggregation; Pod logs use k8s."},
	{"connector", "连接代理、堡垒机管理与采集器清单状态；客户 MCP 单独接入 / Connectors, bastions and collector inventory/status; customer MCP is separate."},
	{"workflow", "待确认操作状态与取消、授权结果导入 Workspace 和文件下载交付 / Pending-action status/cancellation, authorized result import and file delivery."},
	{"dashboard", "仪表盘发现取数、条件解析、个人草稿准备、校验与发布预览 / Dashboard discovery, queries, conditions, personal drafts, validation and publication previews."},
	{"alert", "告警、历史、通知与已有配置查询；策略、屏蔽、抑制、路由和处理预览 / Alerts, history, notifications and existing configuration; policy, silence, inhibition, route and handling previews."},
}

func NativeCategoryNames() []string {
	names := make([]string, 0, len(nativeCategories))
	for _, category := range nativeCategories {
		names = append(names, category.name)
	}
	return names
}

func NativeCategoryGuide() string {
	var guide strings.Builder
	guide.WriteString("Native tool categories / 自有工具分类:\n")
	for _, category := range nativeCategories {
		guide.WriteString("- " + category.name + ": " + category.description + "\n")
	}
	return guide.String()
}
