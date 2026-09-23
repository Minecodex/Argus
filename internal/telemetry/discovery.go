package telemetry

import "github.com/kakj-go/Argus/internal/mcp"

var telemetryDiscovery = map[string]mcp.Discovery{
	"telemetry.collector.list": {
		SchemaVersion: "argus.tool_discovery/v1", Revision: "1",
		Title:             "列出遥测采集器 / List telemetry collectors",
		Description:       "查询当前企业有权查看的采集器及其安装、连接和采集状态。 List authorized collectors and their installation, connectivity and collection state.",
		Keywords:          []string{"采集器", "遥测", "列表", "清单", "状态", "collector", "list", "status"},
		ResultDescription: "返回有界采集器清单及状态。 Returns collector records and their current state.",
		Preconditions:     []string{"使用读取权限和当前资源授权；本工具无输入筛选字段。 Requires read permission and current resource authorization; this tool has no input filter fields."},
		Examples:          []mcp.DiscoveryExample{{Request: "查看有哪些采集器。 / List available collectors.", Guidance: "使用空参数对象 {} 查询，再从结果获取 collector_id。 Invoke with {} and obtain collector_id from the result."}},
	},
	"telemetry.collector.get": {
		SchemaVersion: "argus.tool_discovery/v1", Revision: "1",
		Title:             "查看采集器详情 / Inspect telemetry collector",
		Description:       "读取指定采集器的版本、配置和当前运行状态。 Read the version, configuration and runtime state of a selected collector.",
		Keywords:          []string{"采集器", "详情", "配置", "状态", "collector", "get", "detail"},
		ResultDescription: "返回采集器详情及版本信息。 Returns collector details and version.",
		Preconditions:     []string{"先查询采集器清单取得真实 collector_id。 Obtain a real collector_id from the collector list."},
		Examples:          []mcp.DiscoveryExample{{Request: "查看这个采集器的状态。 / Inspect this collector.", Guidance: "传入清单返回的 collector_id，不猜测身份。 Use the collector_id returned by the list."}},
	},
	"telemetry.promql.query": {
		SchemaVersion: "argus.tool_discovery/v1", Revision: "1",
		Title:             "查询 PromQL 指标 / Query PromQL metrics",
		Description:       "在已授权资源和明确时间范围内执行受控 PromQL 指标查询，支持采样步长、序列数和扫描预算。 Execute bounded PromQL over authorized resources and an explicit time range.",
		Keywords:          []string{"指标", "监控", "CPU", "内存", "时间序列", "趋势", "promql", "metrics", "query", "memory"},
		ResultDescription: "返回 Prometheus 语义的 data、warnings 与 argus_meta，可能包含部分结果和预算信息。 Returns Prometheus data, warnings and Argus execution metadata, including partial/budget information.",
		Preconditions:     []string{"resource_ids 必须来自当前授权资源；from/to 为明确时间范围，query 为支持的 PromQL。 Use authorized resource_ids, an explicit time range and supported PromQL."},
		Examples:          []mcp.DiscoveryExample{{Request: "分析最近一小时 CPU 指标趋势。 / Analyze CPU metrics over the last hour.", Guidance: "用已存在的指标名构造 query，提供 resource_ids、from、to，按需设置 step_seconds；不要编造指标。 Build query from known metric names and supply resource_ids/from/to and optional step_seconds."}},
	},
	"telemetry.kql.query": {
		SchemaVersion: "argus.tool_discovery/v1", Revision: "1",
		Title:             "查询 KQL 日志 / Query KQL logs",
		Description:       "在授权资源和时间范围内运行受控 KQL 日志检索、过滤和聚合，限制扫描及结果大小。 Search, filter and aggregate logs with bounded KQL over authorized resources.",
		Keywords:          []string{"日志", "错误日志", "检索", "过滤", "聚合", "kql", "logs", "search", "errors"},
		ResultDescription: "返回 KQL 表格或聚合 data、schema_version、warnings、partial 和 meta。 Returns KQL data with schema, warnings, partial status and metadata.",
		Preconditions:     []string{"提供已授权 resource_ids、from/to 和支持的 KQL query；结果受预算约束。 Provide authorized resources, a time range and supported KQL."},
		Examples:          []mcp.DiscoveryExample{{Request: "查找所选主机最近十分钟的错误日志。 / Find recent error logs for selected hosts.", Guidance: "根据已有日志字段构造 query，提供资源与时间范围；翻页或换时间通过新的用户消息触发。 Use existing log fields and explicit resources/time; request a new query for another page or time range."}},
	},
	"telemetry.skywalking.trace": {
		SchemaVersion: "argus.tool_discovery/v1", Revision: "1",
		Title:             "查询调用链 / Query distributed traces",
		Description:       "使用支持的 SkyWalking GraphQL 文档查询授权资源的调用链、慢请求及跨度详情。 Query traces, slow requests and spans with supported SkyWalking GraphQL.",
		Keywords:          []string{"链路", "调用链", "慢调用", "慢请求", "跨度", "trace", "tracing", "skywalking", "graphql", "latency"},
		ResultDescription: "返回 GraphQL data、errors 和 extensions，保留查询预算及部分结果信息。 Returns GraphQL data/errors/extensions and execution metadata.",
		Preconditions:     []string{"提供实际 resource_ids、from/to 和 document，不使用任意 GraphQL 服务地址。 Provide actual resources, a time range and a supported document."},
		Examples:          []mcp.DiscoveryExample{{Request: "查看最近的慢调用链。 / Inspect recent slow traces.", Guidance: "依据支持的 GraphQL Schema 编写 document，限定授权资源与时间；使用结果中的真实 trace ID。 Use supported GraphQL, explicit resources/time and returned trace IDs."}},
	},
	"telemetry.overview": {
		SchemaVersion: "argus.tool_discovery/v1", Revision: "1",
		Title:             "查看监控概览 / View telemetry overview",
		Description:       "汇总已授权资源在指定时间范围内的遥测健康和信号概况，作为深入查询的起点。 Summarize telemetry health and signals for authorized resources over a time range.",
		Keywords:          []string{"监控", "概览", "健康", "遥测", "overview", "health", "telemetry"},
		ResultDescription: "返回遥测概览、信号统计和可用状态。 Returns telemetry overview, signal statistics and availability.",
		Preconditions:     []string{"选择当前授权 resource_ids，可用 lookback_seconds 设置回看窗口。 Choose authorized resource_ids and optionally set the lookback_seconds window."},
		Examples:          []mcp.DiscoveryExample{{Request: "汇总这些资源的监控健康状态。 / Summarize telemetry health.", Guidance: "使用 resource_ids 和可选 lookback_seconds 查询概览，再按需要 Describe 指标、日志或链路工具。 Supply resource_ids and optional lookback_seconds, then describe signal-specific tools for follow-up."}},
	},
}
