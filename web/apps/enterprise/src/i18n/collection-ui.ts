export const collectionUiZh = {
  collectionUi: {
    host: {
      hostBasic: "采集 CPU、内存、磁盘和网络等主机运行指标。",
      systemLogs: "读取 Linux journald 系统与服务日志，用于排查运行异常。",
      fileLogs: "按采集配置读取应用日志文件，并保留日志来源信息。",
      collectorSelf: "采集收集器自身的运行指标，检查采集与推送健康。",
      prometheus: "抓取配置中的 Prometheus 指标端点。",
      otlp: "接收应用通过 OTLP 上报的指标、日志和链路数据。",
    },
    cluster: {
      nodeContainer: "通过 DaemonSet 同时采集节点、容器指标与容器日志。",
      clusterMetrics: "采集 Kubernetes 集群状态和资源元数据指标。",
      otlpGateway: "通过集群 Gateway 接收应用上报的指标、日志和链路。",
    },
    clusterTitle: {
      nodeContainer: "节点与容器采集",
      clusterMetrics: "集群状态指标",
      otlpGateway: "应用 OTLP 接收",
    },
    emptyCapabilities: "至少开启一项采集能力；停止整个收集器请使用卸载操作。",
  },
};
export const collectionUiEn = {
  collectionUi: {
    host: {
      hostBasic:
        "Collect CPU, memory, disk and network metrics from this host.",
      systemLogs:
        "Read Linux journald system and service logs for troubleshooting.",
      fileLogs:
        "Read configured application log files and preserve their source metadata.",
      collectorSelf:
        "Monitor the Collector itself to check collection and export health.",
      prometheus:
        "Scrape the Prometheus metric endpoints in the collection configuration.",
      otlp: "Receive application metrics, logs and traces over OTLP.",
    },
    cluster: {
      nodeContainer:
        "Collect node and container metrics together with container logs through the DaemonSet.",
      clusterMetrics:
        "Collect Kubernetes cluster state and resource metadata metrics.",
      otlpGateway:
        "Receive application metrics, logs and traces through the cluster Gateway.",
    },
    clusterTitle: {
      nodeContainer: "Node and container collection",
      clusterMetrics: "Cluster state metrics",
      otlpGateway: "Application OTLP receiver",
    },
    emptyCapabilities:
      "Enable at least one collection capability. To stop the entire Collector, use Uninstall.",
  },
};
