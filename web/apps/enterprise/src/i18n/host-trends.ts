export const hostTrendsZh = {
  hostTrends: {
    cpu: "CPU",
    memory: "内存",
    storage: "磁盘",
    loading: "正在读取指标",
    window: "最近 30 分钟 · 每分钟更新，隐藏时暂停",
    time: "时间",
    sampledAt: "采样时间",
    used: "使用量",
    capacity: "容量",
    utilization: "使用率",
    filesystem: "文件系统",
    storageHint:
      "展示最新使用率最高的一个文件系统的完整趋势，不重复累加挂载点。",
    scopeHint: "按实际采集的主机可见范围计算；共享内核环境不代表容器限额。",
    states: {
      no_data: "暂无样本",
      stale: "数据过期",
      partial: "读取不完整",
      forbidden: "无权限",
      not_collected: "无采集来源",
      query_failed: "读取失败",
    },
    hints: {
      no_data: "最近 30 分钟没有此主机的实际指标样本。",
      stale: "最近实际采样已超过 5 分钟，请检查采集状态。",
      partial: "查询达到预算或结果不完整，不将部分结果显示为精确使用率。",
      forbidden: "当前身份没有读取指标的权限。",
      not_collected: "当前范围没有已授权的主机指标采集来源。",
      query_failed: "指标读取失败，主机列表和操作仍可使用。",
    },
  },
};
export const hostTrendsEn = {
  hostTrends: {
    cpu: "CPU",
    memory: "Memory",
    storage: "Disk",
    loading: "Loading metrics",
    window: "Last 30 minutes · Updates every minute, paused while hidden",
    time: "Time",
    sampledAt: "Sample time",
    used: "Usage",
    capacity: "Capacity",
    utilization: "Utilization",
    filesystem: "Filesystem",
    storageHint:
      "Shows the complete trend of the filesystem with the highest latest utilization. Mounts are not added together.",
    scopeHint:
      "Uses the host scope visible to the collector. Shared-kernel capacity does not represent a container limit.",
    states: {
      no_data: "No samples",
      stale: "Stale",
      partial: "Incomplete",
      forbidden: "No access",
      not_collected: "No source",
      query_failed: "Read failed",
    },
    hints: {
      no_data: "No actual metric samples for this host in the last 30 minutes.",
      stale:
        "The latest actual sample is over 5 minutes old. Check collection health.",
      partial:
        "The query reached its budget or is incomplete. Partial results are not displayed as precise utilization.",
      forbidden: "The current identity cannot read metrics.",
      not_collected: "No authorized host metric source exists in this scope.",
      query_failed:
        "Metrics could not be read. The host list and actions remain available.",
    },
  },
};
