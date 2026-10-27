export const builtinCollectionsZh = {
  builtinCollections: {
    trust: {
      action: "证书信任",
      title: "组件指标证书信任",
      hint: "使用采集器的服务账号直接读取受保护的组件指标。组件未暴露指标端口时，请先由集群管理员提供访问条件。",
      serverName: "证书服务名",
      serverNameHint:
        "留空使用组件名，例如 kube-scheduler。此名称必须匹配服务端证书。",
      ca: "受信任的公开证书（PEM）",
      caHint:
        "留空使用集群 CA；可填写受信 CA 或自签名服务端证书，仅接受 CERTIFICATE，不接受私钥。",
      preview: "预览信任配置",
      invalidServerName: "证书服务名不能超过 253 个字符。",
      invalidCA: "公开证书不能超过 256 KiB。",
    },
    logsLink: "日志与其他收集",
    clusterTitle: "采集实例与运行配置",
    clusterHint: "自身指标按实例独立配置；镜像和拉取凭据仅调整采集器运行部署。",
    title: "自身监控",
    name: "监控内容",
    hint: "固定指标独立启停，保留各自的配置版本、运行健康和样本状态。应用接入与日志使用独立的接入点或规则。",
    notConfigured: "未配置",
    availability: "能力与诊断",
    available: "组件可用",
    hints: {
      host_process_metrics:
        "仅观测采集器身份可读取的进程；不采集完整命令行、路径或用户名。",
      host_systemd_metrics:
        "需要可读的系统 D-Bus；仅观测已加载单元，单元卸载后不补造状态。",
      host_windows_service_metrics: "观测采集器身份可读取的 Windows 服务状态。",
      kubernetes_scheduler_metrics:
        "直接读取受保护的 Scheduler HTTPS 指标，需要可达端口、指标权限和受信证书；仅回环监听的端点无法从网关访问。",
      kubernetes_controller_metrics:
        "需要已有的 Controller Manager 指标端点及读取权限；托管集群可能不开放。",
      kubernetes_etcd_metrics:
        "读取现有 etcd 组件指标端点；不开放监听、不代用成员私钥，缺少端点时显示无数据。",
    },
    diagnostics: {
      COLLECTION_SCRAPE_FAILED: "组件端点抓取失败，请检查端点、证书与读取权限",
      COLLECTION_SCRAPE_PARTIAL_FAILURE: "部分组件端点不可用",
    },
    noAccess: "暂无采集器查看权限",
    preview: "自身监控配置预览",
    names: {
      host_metrics: "主机基础指标",
      host_process_metrics: "主机进程指标",
      host_systemd_metrics: "Linux 系统服务",
      host_windows_service_metrics: "Windows 系统服务",
      collector_metrics: "Collector 自身指标",
      kubernetes_cluster_metrics: "集群与工作负载状态",
      kubernetes_node_metrics: "节点与容器指标",
      kubernetes_apiserver_metrics: "API Server 指标",
      kubernetes_coredns_metrics: "CoreDNS 指标",
      kubernetes_scheduler_metrics: "Scheduler 指标",
      kubernetes_controller_metrics: "Controller Manager 指标",
      kubernetes_etcd_metrics: "etcd 组件指标",
    },
    reasons: {
      collector_not_installed: "尚未安装采集器",
      component_unavailable: "当前分发包或平台暂不可用",
    },
  },
};
export const builtinCollectionsEn = {
  builtinCollections: {
    trust: {
      action: "Certificate trust",
      title: "Component metric certificate trust",
      hint: "Use the Collector service account to read protected component metrics directly. The cluster administrator must expose the component metric port first.",
      serverName: "Certificate server name",
      serverNameHint:
        "Leave empty to use the component name, such as kube-scheduler. This name must match the server certificate.",
      ca: "Trusted public certificates (PEM)",
      caHint:
        "Leave empty to use the cluster CA. Supply trusted CA or self-signed server certificates as CERTIFICATE blocks; private keys are rejected.",
      preview: "Preview trust configuration",
      invalidServerName: "The certificate name must be at most 253 characters.",
      invalidCA: "Public certificates must be at most 256 KiB.",
    },
    logsLink: "Logs and other collections",
    clusterTitle: "Collection instances and runtime settings",
    clusterHint:
      "Configure own metrics independently. Image and pull references change only the Collector deployment.",
    title: "Own resource monitoring",
    name: "Collection",
    hint: "Toggle fixed metrics independently and inspect their own configuration, health and sample states. Application ingestion and logs use separate endpoints or rules.",
    notConfigured: "Not configured",
    availability: "Capability and diagnostics",
    available: "Component available",
    hints: {
      host_process_metrics:
        "Only processes readable by the Collector identity; full command lines, paths and user names are excluded.",
      host_systemd_metrics:
        "Requires readable system D-Bus. Loaded units only; unloaded units do not receive synthetic states.",
      host_windows_service_metrics:
        "Observe Windows service states readable by the Collector identity.",
      kubernetes_scheduler_metrics:
        "Reads protected Scheduler HTTPS metrics directly; requires a reachable port, metric permissions and trusted certificate. Loopback-only endpoints are unreachable from the gateway.",
      kubernetes_controller_metrics:
        "Requires an existing Controller Manager metrics endpoint and read access. Managed clusters may not expose it.",
      kubernetes_etcd_metrics:
        "Reads existing etcd component metrics. Listeners and member keys are unchanged; missing endpoints remain without data.",
    },
    diagnostics: {
      COLLECTION_SCRAPE_FAILED:
        "Component scrape failed; check endpoint, certificate and read access",
      COLLECTION_SCRAPE_PARTIAL_FAILURE:
        "Some component endpoints are unavailable",
    },
    noAccess: "Collector read access is required",
    preview: "Preview own resource monitoring",
    names: {
      host_metrics: "Host metrics",
      host_process_metrics: "Host process metrics",
      host_systemd_metrics: "Linux system services",
      host_windows_service_metrics: "Windows system services",
      collector_metrics: "Collector metrics",
      kubernetes_cluster_metrics: "Cluster and workload state",
      kubernetes_node_metrics: "Node and container metrics",
      kubernetes_apiserver_metrics: "API server metrics",
      kubernetes_coredns_metrics: "CoreDNS metrics",
      kubernetes_scheduler_metrics: "Scheduler metrics",
      kubernetes_controller_metrics: "Controller Manager metrics",
      kubernetes_etcd_metrics: "etcd component metrics",
    },
    reasons: {
      collector_not_installed: "Collector is not installed",
      component_unavailable:
        "Component is unavailable for the current distribution or platform",
    },
  },
};
