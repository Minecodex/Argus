export const modelGovernanceZh = {
  modelGovernance: {
    title: "模型治理",
    open: "查看治理仪表盘",
    model: "模型",
    month: "月份",
    unavailable: "模型治理仪表盘暂不可用",
    managedHint: "系统内置只读仪表盘，内部模型指标仅供此仪表盘使用。",
    basis:
      "按企业时区 {{timezone}} 统计自然月；数据更新时间：{{cutoff}}。金额为结算分析数据，额度判断以结算服务为准。",
    delayed: "模型指标正在同步",
    cacheIncomplete: "缓存 Token 用量不完整",
    quotaBasis:
      "调用与金额按完成时间归月；额度占用按预留所属月份统计活动预留和已结算金额，并对照当前部门限额。历史月份不重建历史限额。",
    pending: "还有 {{count}} 条内部记录待同步，当前结果尚不完整。",
    unavailableMetrics:
      "工具调用与结构化输出失败尚无可靠分类，显示不可用。缓存 Token 是输入 Token 的子集；额度图展示当前已配置且大于零的部门限额，无配置表示无限，零限额不计算比率。",
    panels: {
      requests: "调用数",
      tokens: "实报 Token",
      cached: "缓存输入 Token",
      amount: "结算金额",
      success: "成功率",
      latency: "平均调用延迟",
      errors: "失败调用",
      cancelled: "取消调用",
      compaction: "压缩调用",
      requests_trend: "每日调用数",
      tokens_trend: "每日实报 Token",
      amount_trend: "每日结算金额",
      p95: "每日 P95 延迟",
      models: "模型金额排行",
      departments: "部门金额排行",
      users: "用户金额排行",
      quota: "所选月份额度占用（当前限额）",
      tool_failures: "工具调用失败（不可用）",
      structured_failures: "结构化输出失败（不可用）",
    },
  },
};
export const modelGovernanceEn = {
  modelGovernance: {
    title: "Model governance",
    open: "View governance dashboard",
    model: "Model",
    month: "Month",
    unavailable: "Model governance dashboard is unavailable",
    managedHint:
      "Read-only built-in dashboard. Internal model metrics are accessible only through this dashboard.",
    basis:
      "Calendar month in enterprise timezone {{timezone}}. Data updated: {{cutoff}}. Amounts are settlement analytics; quota enforcement uses the settlement service.",
    delayed: "Model metrics are synchronizing",
    cacheIncomplete: "Cached token usage is incomplete",
    quotaBasis:
      "Calls and amounts use completion month. Quota usage uses the reservation billing month, including active holds and settled amounts, against current department limits. Historical limits are not reconstructed.",
    pending:
      "{{count}} internal records are awaiting projection. Results are incomplete.",
    unavailableMetrics:
      "Tool calling and structured output failures lack reliable classification and show unavailable. Cached tokens are included in input tokens. Quota charts show current positive department limits; missing limits are unlimited and zero limits have no ratio.",
    panels: {
      requests: "Calls",
      tokens: "Reported tokens",
      cached: "Cached input tokens",
      amount: "Settled amount",
      success: "Success rate",
      latency: "Average call latency",
      errors: "Failed calls",
      cancelled: "Cancelled calls",
      compaction: "Compaction calls",
      requests_trend: "Daily calls",
      tokens_trend: "Daily reported tokens",
      amount_trend: "Daily settled amount",
      p95: "Daily P95 latency",
      models: "Amount by model",
      departments: "Amount by department",
      users: "Amount by user",
      quota: "Selected month quota usage (current limits)",
      tool_failures: "Tool calling failures (unavailable)",
      structured_failures: "Structured output failures (unavailable)",
    },
  },
};
