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
		SchemaVersion: "argus.tool_discovery/v1", Revision: "12",
		Title:             "指标查询 / Metric query",
		Description:       "在授权资源和固定正时间窗(from<to)内执行有界PromQL。瞬时查询省略step_seconds，在to评估，但仍保留原from/to；不得将两者都设为终点。来源版本用已发现的scope.source_keys完整选择器，不发明argus_source_revision标签。rate/increase会外推，速率乘窗口不是实际事件计数。Execute bounded PromQL with the original from<to window. Omit step_seconds for an instant value at to; never set both bounds to to. Select an observed source version with scope.source_keys, not invented revision labels. rate/increase are estimators, not observed whole-window counts. Invalid time/syntax/result type fails before Run query budget reservation.",
		Keywords:          []string{"指标", "监控", "CPU", "内存", "时间序列", "趋势", "promql", "metrics", "query", "memory"},
		ResultDescription: "返回Prometheus data/warnings/argus_meta。returned_points是返回评估点的数量及首末时间，不证明原始采样连续覆盖。latest_sample_at为引擎采样事实；query_context.from/to为请求窗，不能代替实际采样时间。Returns Prometheus results and metadata. returned_points counts evaluation points/timestamps, not continuous raw coverage. Distinguish latest_sample_at from requested query_context.from/to; never infer sample positions from point count or request start. " + modelOriginalAnalysisGuidance,
		Preconditions:     []string{"resource_ids 必须来自当前授权资源；from/to 为明确时间范围，query 为支持的 PromQL。 Use authorized resource_ids, an explicit time range and supported PromQL."},
		Examples:          []mcp.DiscoveryExample{{Request: "分析最近一小时 CPU 指标趋势。 / Analyze CPU metrics over the last hour.", Guidance: "先用 observe/catalog.query 在授权资源和固定窗口内发现真实指标名、来源与单位；再使用观察到的名称和标签构造 query。目录不可用时才用有界 {__name__=~\".+\"} 回退，保留正常预算。共享 Run 预算耗尽后停止重试，不猜测其他集成的指标名或改动用户固定窗口。Prefer observe/catalog.query for actual names, sources and units in the authorized scope and fixed window. Query the selected observed name/labels with the requested step. Only when that catalog is unavailable, use a bounded {__name__=~\".+\"} fallback with normal budgets. Stop retries when the shared Run budget is exhausted; never guess another integration's names or change the user's fixed window."}},
	},
	"telemetry.kql.query": {
		SchemaVersion: "argus.tool_discovery/v1", Revision: "4",
		Title:             "查询 KQL 日志 / Query KQL logs",
		Description:       modelLogSyntaxGuidance + " 在授权范围执行 Argus 日志 DSL；不是任意上游 KQL。首段为 * 或谓词，管道支持 parse/where/unwrap/stats/sort/limit/context。不支持 Syslog 表名或 summarize；字段先查 observe/catalog.query。Use the supported Argus log DSL: * or a predicate, then parse/where/unwrap/stats/sort/limit/context. Do not use upstream table names or summarize. Discover actual fields through observe/catalog.query.",
		Keywords:          []string{"日志", "错误日志", "检索", "过滤", "聚合", "kql", "logs", "search", "errors"},
		ResultDescription: "返回 KQL 表格或聚合 data、schema_version、warnings、partial 和 meta。severity_number 使用 OpenTelemetry 级别：1–4 TRACE、5–8 DEBUG、9–12 INFO、13–16 WARN、17–20 ERROR、21–24 FATAL；不要把9写成ERROR。未查询的级别含义不要猜。 Returns KQL data with schema, warnings, partial status and metadata. severity_number follows OpenTelemetry severity ranges; 9 is INFO, 17 is ERROR. " + modelOriginalAnalysisGuidance,
		Preconditions:     []string{"提供已授权 resource_ids、from/to 和支持的 KQL query；结果受预算约束。 Provide authorized resources, a time range and supported KQL."},
		Examples:          []mcp.DiscoveryExample{{Request: "查找所选主机最近十分钟的错误日志。 / Find recent error logs for selected hosts.", Guidance: "先发现实际字段和来源，再用 * | limit 100 或 severity_number >= 17 | stats count() by severity_text 等支持的语法。保留资源与时间；不猜测 SeverityLevel 等外部字段。 Discover actual fields and sources, then use supported forms such as * | limit 100 or severity_number >= 17 | stats count() by severity_text. Preserve resource/time scope and never guess external field names."}},
	},
	"telemetry.skywalking.trace": {
		SchemaVersion: "argus.tool_discovery/v1", Revision: "6",
		Title:             "查询调用链 / Query distributed traces",
		Description:       "查询授权资源的 Trace 或应用性能。应用 APM 使用 application_view=services/instances/endpoints/red/topology/traces，无需编写 GraphQL；与门户使用同一真实样本口径。也接受 Argus 支持的 document；queryBasicTraces 使用平铺参数，不支持上游的 condition/queryDuration/segmentId 词汇。 Query received Trace/APM samples; prefer typed application_view for performance and coverage.",
		Keywords:          []string{"应用性能", "APM", "RED", "拓扑", "链路", "调用链", "慢调用", "慢请求", "跨度", "trace", "tracing", "skywalking", "graphql", "latency"},
		ResultDescription: "返回 GraphQL data、errors 和 extensions，保留查询预算及部分结果信息。Trace/Span的duration及APM的duration*Ms均为毫秒(ms)，不是微秒；例如1000ms=1s。 Returns GraphQL data/errors/extensions and execution metadata. Trace/span duration and APM duration*Ms fields are milliseconds (ms), not microseconds. Native query results are automatically imported into the Workspace file path in the projection before the next model round. No publish action is needed." + modelOriginalAnalysisGuidance,
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
