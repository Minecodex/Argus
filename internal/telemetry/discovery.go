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
		SchemaVersion: "argus.tool_discovery/v1", Revision: "3",
		Title:             "指标查询 / Metric query",
		Description:       "在已授权资源和明确时间范围内执行受控 PromQL 指标查询，支持采样步长、序列数和扫描预算。 Execute bounded PromQL over authorized resources and an explicit time range.",
		Keywords:          []string{"指标", "监控", "CPU", "内存", "时间序列", "趋势", "promql", "metrics", "query", "memory"},
		ResultDescription: "返回 Prometheus 语义的 data、warnings 与 argus_meta，可能包含部分结果和预算信息。 Returns Prometheus data, warnings and Argus execution metadata, including partial/budget information. Native results are automatically imported into the source-bound Workspace path in the projection; read that actual JSON before analysis.",
		Preconditions:     []string{"resource_ids 必须来自当前授权资源；from/to 为明确时间范围，query 为支持的 PromQL。 Use authorized resource_ids, an explicit time range and supported PromQL."},
		Examples:          []mcp.DiscoveryExample{{Request: "分析最近一小时 CPU 指标趋势。 / Analyze CPU metrics over the last hour.", Guidance: "用已存在的指标名构造 query，提供 resource_ids、from、to，按需设置 step_seconds；不要编造指标。 未知名称时，在指定授权资源和请求时间窗内使用有界的 {__name__=~\".+\"} 读取真实名称及数值，保留正常预算并检查部分结果。 Build query from observed metric names and supply resource_ids/from/to and optional step_seconds. If names are unknown, a bounded {__name__=~\".+\"} selector within those authorized resources and the requested time window returns actual names and values; retain normal series/sample/scan budgets and inspect partial warnings."}},
	},
	"telemetry.kql.query": {
		SchemaVersion: "argus.tool_discovery/v1", Revision: "2",
		Title:             "查询 KQL 日志 / Query KQL logs",
		Description:       "在授权资源和时间范围内运行受控 KQL 日志检索、过滤和聚合，限制扫描及结果大小。 Search, filter and aggregate logs with bounded KQL over authorized resources.",
		Keywords:          []string{"日志", "错误日志", "检索", "过滤", "聚合", "kql", "logs", "search", "errors"},
		ResultDescription: "返回 KQL 表格或聚合 data、schema_version、warnings、partial 和 meta。 Returns KQL data with schema, warnings, partial status and metadata. Native results are automatically imported into the source-bound Workspace path in the projection; read that actual JSON before analysis.",
		Preconditions:     []string{"提供已授权 resource_ids、from/to 和支持的 KQL query；结果受预算约束。 Provide authorized resources, a time range and supported KQL."},
		Examples:          []mcp.DiscoveryExample{{Request: "查找所选主机最近十分钟的错误日志。 / Find recent error logs for selected hosts.", Guidance: "根据已有日志字段构造 query，提供资源与时间范围；翻页或换时间通过新的用户消息触发。 Use existing log fields and explicit resources/time; request a new query for another page or time range."}},
	},
	"telemetry.skywalking.trace": {
		SchemaVersion: "argus.tool_discovery/v1", Revision: "4",
		Title:             "查询调用链 / Query distributed traces",
		Description:       "查询授权资源的 Trace 或应用性能。应用 APM 使用 application_view=services/instances/endpoints/red/topology/traces，无需编写 GraphQL；与门户使用同一真实样本口径。也接受 Argus 支持的 document；queryBasicTraces 使用平铺参数，不支持上游的 condition/queryDuration/segmentId 词汇。 Query received Trace/APM samples; prefer typed application_view for performance and coverage.",
		Keywords:          []string{"应用性能", "APM", "RED", "拓扑", "链路", "调用链", "慢调用", "慢请求", "跨度", "trace", "tracing", "skywalking", "graphql", "latency"},
		ResultDescription: "返回 GraphQL data、errors 和 extensions，保留查询预算及部分结果信息。 Returns GraphQL data/errors/extensions and execution metadata. Native query results are automatically imported into the Workspace file path in the projection before the next model round. Read that actual JSON file for analysis; no publish action is needed.",
		Preconditions:     []string{"提供实际 resource_ids 与 from/to；application_view 与 document 二选一。limit 是总执行行数预算，默认 5000；APM 列表在预算内最多取 100 行，聚合视图需预留统计信息的一行。service_ids 可省略，若提供需为 1–200 个服务 ID。结果只是已接收样本，不能推算生产全量。 Supply explicit resources/time and one query mode; limit is the total row budget including coverage and relations. Do not extrapolate received samples to unsampled traffic."},
		Examples: []mcp.DiscoveryExample{
			{Request: "对比两个应用的性能与错误样本。 / Compare application performance.", Guidance: "对每个应用使用 application_view=services、明确 resource_ids/from/to 和 limit=100，读取 Workspace 中的实际 rows、coverage、sampleCount 和 errorCount。 For each app query services, then inspect delivered rows/coverage/sampleCount/errorCount."},
			{Request: "查看最近的调用链。 / Inspect recent traces.", Guidance: "使用 application_view=traces；专家 document 示例：query { queryTraces(pageSize:10) { total traces { traceId rootService rootOperation startTime duration spanCount errorCount } } }。真实 trace ID 与 source ID 用于后续详情。 Use returned trace/source IDs for detail."},
		},
	},
	"telemetry.overview": {
		SchemaVersion: "argus.tool_discovery/v1", Revision: "2",
		Title:             "查看遥测覆盖 / View telemetry coverage",
		Description:       "汇总授权资源的信号可用性与采集覆盖，仅用于判断已有何种遥测。它不返回指标数值或历史时序，不能代替 metric/query。 Summarize authorized signal availability and collection coverage only; this is not a measurement or historical time-series query. Use metric/query for actual metric values.",
		Keywords:          []string{"监控", "概览", "覆盖", "遥测", "overview", "coverage", "telemetry"},
		ResultDescription: "返回信号记录或数据点数量、来源及可用状态；这些计数不是 CPU、连接数、延迟等资源指标值。 Returns telemetry record/point counts, sources and availability, not resource measurements such as CPU, connections or latency.",
		Preconditions:     []string{"选择当前授权 resource_ids，可用 lookback_seconds 设置回看窗口。 Choose authorized resource_ids and optionally set the lookback_seconds window."},
		Examples:          []mcp.DiscoveryExample{{Request: "查看这些资源有哪些遥测信号。 / Inspect telemetry signal coverage.", Guidance: "查询概览了解覆盖；若用户要求指标值或趋势，继续 Describe 并调用 metric/query，读取其 Workspace 数据；不能只用采集点数量结束指标分析。 Inspect coverage, then describe/invoke metric/query and read its delivered data when measurements or trends are requested."}},
	}}
