export const logCollectionZh = {
  logCollection: {
    workload: "工作负载",
    chooseWorkload: "选择工作负载",
    workloadType: "工作负载类型",
    workloadHint:
      "绑定对象 UID，并跟随该对象的 Pod 与滚动发布；同名重建后需要重新配置。",
    exactNameHint: "名称搜索使用完整对象名，留空时分页浏览。",
    containers: "采集容器",
    chooseContainers: "选择容器",
    containerName: "容器名称",
    containerKind: "容器类型",
    containerKinds: {
      containers: "业务容器",
      initContainers: "初始化容器",
      ephemeralContainers: "临时容器",
    },
    clusterInfrastructureHint:
      "该规则的日志归属所选集群及 Namespace，无需关联业务应用。",
    clusterCollectorHint: "使用集群的节点收集器读取所选工作负载的容器日志。",
    description:
      "按来源创建独立日志规则。业务日志关联应用和服务，系统与组件日志归属执行主机。",
    scope: "日志归属",
    scopes: {
      business: "业务日志",
      system: "系统日志",
      component: "基础设施组件日志",
    },
    application: "所属应用",
    serviceHint:
      "选择已发现或已声明的服务。仅接入日志的服务可在应用详情中创建。",
    infrastructureHint: "该规则的日志归属所选主机，无需关联业务应用。",
    collectorHint: "使用目标主机上的收集器读取本机日志。",
    include: "采集路径",
    exclude: "排除路径",
    patternHint:
      "每行一个绝对路径；支持 *、?、**。启用规则不能重复覆盖同一文件。K8s 容器日志请使用 Kubernetes 日志入口；主机规则自动排除 /var/log/pods 和 /var/log/containers，并校验符号链接的真实路径。",
    encoding: "文件编码",
    format: "日志格式",
    text: "文本",
    multiline: "多行首行表达式",
    multilineHint: "匹配一条新记录的起始行，例如 ^2026-；留空时按行读取。",
    maxBytes: "单条日志大小上限（字节）",
    units: "Systemd 服务单元",
    journalHint: "每行一个单元名；主机需要系统自带的 journalctl。",
    channel: "Windows 事件通道",
    startAt: "首次读取位置",
    end: "从新增日志开始",
    beginning: "从已有日志开始",
    checkpointHint:
      "首次读取位置仅在没有读取记录时生效；重启后按持久化读取位置继续。",
  },
};
export const logCollectionEn = {
  logCollection: {
    workload: "Workload",
    chooseWorkload: "Choose a workload",
    workloadType: "Workload type",
    workloadHint:
      "Bind the object UID and follow its Pods and rolling updates. A recreated object requires a new rule.",
    exactNameHint:
      "Search with the complete object name, or leave blank to browse pages.",
    containers: "Collected containers",
    chooseContainers: "Choose containers",
    containerName: "Container name",
    containerKind: "Container type",
    containerKinds: {
      containers: "Application container",
      initContainers: "Init container",
      ephemeralContainers: "Ephemeral container",
    },
    clusterInfrastructureHint:
      "These logs belong to the selected cluster and Namespace and do not require an application.",
    clusterCollectorHint:
      "Read the selected workload's container logs with the cluster's node Collectors.",
    description:
      "Create an independent log rule. Business logs belong to an application and service; system and component logs belong to the executing host.",
    scope: "Log ownership",
    scopes: {
      business: "Business logs",
      system: "System logs",
      component: "Infrastructure component logs",
    },
    application: "Application",
    serviceHint:
      "Choose a discovered or declared service. Declare a log-only service in application details.",
    infrastructureHint:
      "These logs belong to the selected host and do not require an application.",
    collectorHint:
      "Read local logs with the Collector installed on the target host.",
    include: "Include paths",
    exclude: "Exclude paths",
    patternHint:
      "One absolute path per line; supports *, ? and **. Enabled rules cannot cover the same files. Use Kubernetes logs for container logs; host rules exclude /var/log/pods and /var/log/containers and verify resolved symlink paths.",
    encoding: "File encoding",
    format: "Log format",
    text: "Text",
    multiline: "Multiline start expression",
    multilineHint:
      "Match the first line of a new record, for example ^2026-. Leave blank to read line by line.",
    maxBytes: "Maximum record size (bytes)",
    units: "Systemd units",
    journalHint:
      "One unit per line. The host must provide the system journalctl command.",
    channel: "Windows event channel",
    startAt: "Initial read position",
    end: "New logs only",
    beginning: "Include existing logs",
    checkpointHint:
      "The initial position applies only without a saved checkpoint. Restarting resumes from the persisted position.",
  },
};
