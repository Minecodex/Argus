package toolruntime

import "strings"

// These definitions supply both the Agent's navigation and native tool schema.
// Categories describe potential capabilities; each listing applies current scope.
var nativeCategories = []struct{ name, description string }{
	{"host", "主机查询、接入与组件维护、主机采集器维护 / Host inventory, onboarding, components and host collectors."},
	{"k8s", "集群与 Kubernetes 对象查询、Pod 日志、集群采集器和节点主机绑定 / Kubernetes clusters, objects, Pod logs, collectors and node-host bindings."},
	{"metric", "PromQL 指标查询与遥测信号概览 / PromQL metric queries and telemetry signal overview."},
	{"trace", "调用链、慢请求和 Span 查询 / Distributed traces, slow requests and spans."},
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
