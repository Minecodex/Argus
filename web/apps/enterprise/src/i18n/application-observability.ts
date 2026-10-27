export const applicationObservabilityZh = {
  applicationObservability: {
    title: "应用性能",
    basis:
      "按当前授权应用和服务汇总已接收的入口 Span。同一 Trace/Span 身份跨来源只计一次，原始来源保留。统计反映采样数据。",
    denied: "需要 Trace 查询权限才能读取应用性能数据。",
    view: "性能视图",
    range: "时间范围",
    allServices: "选择服务（当前全部）",
    clearService: "清除服务筛选",
    clearDrill: "清除下钻条件",
    approximate: "按时间与服务检索",
    approximateHint:
      "该日志没有 Trace ID，显示当前服务和时间范围内的调用链，不能视为精确关联。",
    traceDetail: "调用链详情",
    refresh: "刷新数据",
    noData: "尚无应用数据",
    noDataHint: "接入 SDK 并上报后显示实际样本。未上报不会生成性能数值。",
    views: {
      services: "服务",
      instances: "运行实例",
      endpoints: "接口",
      red: "请求与耗时",
      topology: "服务拓扑",
      traces: "调用链",
      logs: "应用日志",
    },
    ranges: {
      900: "最近 15 分钟",
      3600: "最近 1 小时",
      21600: "最近 6 小时",
      86400: "最近 24 小时",
    },
  },
};
export const applicationObservabilityEn = {
  applicationObservability: {
    title: "Application performance",
    basis:
      "Aggregate received entry spans within the authorized application and service. Identical trace/span identities count once across sources; original sources remain available. Statistics describe samples.",
    denied:
      "Trace query access is required to read application performance data.",
    view: "Performance view",
    range: "Time range",
    allServices: "Choose service (currently all)",
    clearService: "Clear service filter",
    clearDrill: "Clear drilldown filters",
    approximate: "Search by time and service",
    approximateHint:
      "This log has no trace ID. Traces are searched within the current service and time window; this is not an exact correlation.",
    traceDetail: "Trace details",
    refresh: "Refresh data",
    noData: "No application data yet",
    noDataHint:
      "Actual samples appear after SDK ingestion. Unobserved services have no synthetic performance values.",
    views: {
      services: "Services",
      instances: "Runtime instances",
      endpoints: "Endpoints",
      red: "Requests and duration",
      topology: "Service topology",
      traces: "Traces",
      logs: "Application logs",
    },
    ranges: {
      900: "Last 15 minutes",
      3600: "Last hour",
      21600: "Last 6 hours",
      86400: "Last 24 hours",
    },
  },
};
