export const dashboardControlsZh = {
  dashboardControls: {
    viewConditions: "仪表盘查询条件",
    refreshInterval: "刷新间隔",
    refreshOff: "自动刷新关闭",
    refreshEvery: "每 {{count}} 秒",
    settings: "仪表盘设置",
    defaultsHint:
      "固定的时间和刷新配置，与自定义过滤项分开；修改保存到个人草稿，发布后成为默认值。",
    refreshHint: "0 表示关闭；开启时至少 5 秒。查看时可以临时更改。",
    invalidRefresh: "刷新间隔应为 0 或至少 5 秒的整数。",
    resourceHint:
      "仅列出你有权限访问的主机和 Kubernetes 集群。“全部”使用当前授权全集，各统计图再按来源和适用类型匹配；选择不会授予权限。",
    defaultResourcesHint:
      "资源默认使用当前用户的授权范围；从主机或 K8s 快捷入口打开时预选对应资源。该范围不在变量编辑器配置。",
    variablesHint:
      "这里只管理自定义过滤项。时间和刷新在仪表盘设置中；资源范围在查看工具栏中选择。",
    emptyVariables: "添加环境、服务等过滤项，再在需要的统计图查询中引用。",
    selectVariable: "选择过滤项",
    removeVariable: "删除过滤项",
    editVariable: "编辑过滤项",
    querySettings: "候选值查询",
    selectionSettings: "选择与默认值",
    conditions: "候选条件与依赖",
    previewCandidates: "预览候选",
    previewHint: "按仪表盘默认时间和前置变量查询，预览不更改默认选值。",
    previewCount: "已读取 {{count}} 个候选值",
    previewPartial: "仅展示当前页候选；还有更多值。",
    consumers: "引用与依赖",
    noConsumers: "尚无统计图引用此项；配置后不会自动过滤整张仪表盘。",
    consumersHint: "只刷新明确引用此项及其依赖的统计图。",
    fieldHint:
      "例如 resource_attributes.deployment.environment.name；字段须来自所选来源。",
    variableList: "过滤项列表",
    defaultSelection: "默认选择",
    candidateBusy: "正在查询候选…",
    filterManager: "管理过滤项",
  },
};
export const dashboardControlsEn = {
  dashboardControls: {
    viewConditions: "Dashboard query conditions",
    refreshInterval: "Refresh interval",
    refreshOff: "Auto refresh off",
    refreshEvery: "Every {{count}} s",
    settings: "Dashboard settings",
    defaultsHint:
      "Time and refresh settings are separate from custom filters. Changes become defaults after saving the draft and publishing.",
    refreshHint:
      "0 disables refresh; otherwise use at least 5 seconds. Viewers can change it temporarily.",
    invalidRefresh: "Refresh must be 0 or an integer of at least 5 seconds.",
    resourceHint:
      "Only authorized hosts and Kubernetes clusters are listed. All uses your current authorized scope; panels then match their source and resource types. Selection does not grant access.",
    defaultResourcesHint:
      "Resources default to the current user's authorized scope. Host or K8s shortcuts preselect their resource. This scope is separate from custom variables.",
    variablesHint:
      "Manage custom filters here. Time and refresh are in dashboard settings; resource selection is in the viewing toolbar.",
    emptyVariables:
      "Add an environment or service filter, then reference it in the relevant panel queries.",
    selectVariable: "Select filter",
    removeVariable: "Delete filter",
    editVariable: "Edit filter",
    querySettings: "Candidate query",
    selectionSettings: "Selection and defaults",
    conditions: "Candidate conditions and dependencies",
    previewCandidates: "Preview candidates",
    previewHint:
      "Uses the dashboard's default time and upstream variables. Preview does not change the default selection.",
    previewCount: "Loaded {{count}} candidates",
    previewPartial: "This is one candidate page; more values are available.",
    consumers: "References and dependencies",
    noConsumers:
      "No panel references this filter yet. It does not automatically filter the entire dashboard.",
    consumersHint: "Only explicitly dependent panels are refreshed.",
    fieldHint:
      "For example resource_attributes.deployment.environment.name; use a field from the selected source.",
    variableList: "Filter list",
    defaultSelection: "Default selection",
    candidateBusy: "Loading candidates…",
    filterManager: "Manage filters",
  },
};
