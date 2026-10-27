# Grafana 仪表盘参考与适配表

> 当前资源准入：ClickHouse、TiDB、OceanBase、RocketMQ、Pulsar、NATS、Envoy、JVM、Tomcat、Spring Boot、MinIO资源指标入口暂缓。以下模板链接保留研究用途，不表示本期提供对应默认仪表盘；见[专用插件复查](./13-direct-receiver-review.md)。

> 最新准入：表中的exporter是原模板的数据来源，不是Argus部署依赖。仅采用[内置Collector路线](./12-collector-native-support.md)；按实际字段改写模板，不能为复刻图表安装独立exporter。

核对日期：2026-10-18。此表供模板设计与指标契约对照，不表示已经导入、转换或验收通过。这是 Plan8 的实施参考入口；原型中的列表保留本次研究快照。每个集成的实施修订与验证记录以本目录为准。

共 42 条参考，覆盖两批服务、应用、日志、主机与 Kubernetes。大部分为 Grafana 市场具体模板；ClickHouse 与 Pulsar 使用已核对的官方集成或模板集合，正式实施时还需选定具体模板版本。旧版社区模板只用于结构/指标参考，必须验证当前采集来源、目标版本、指标名称、标签、单位和依赖规则。

| 监控对象 | 参考仪表盘 | ID / 来源 | 原始采集依赖 | Argus 适配重点 |
| --- | --- | --- | --- | --- |
| MySQL / MariaDB | [mysql overview](https://grafana.com/grafana/dashboards/14969-mysqld-overview/) | 14969 | mysqld_exporter | 映射 OTel 与 exporter 指标；MariaDB 独立验证。 |
| PostgreSQL | [PostgreSQL Database](https://grafana.com/grafana/dashboards/9628-postgresql-database/) | 9628 | postgres_exporter | 连接、事务、锁、复制；逐项校验指标覆盖。 |
| SQL Server | [SQL Server Monitoring](https://grafana.com/grafana/dashboards/1883-telegraf-sql-server-monitoring/) | 1883 | Telegraf / Prometheus | 参考面板结构；性能计数器与 SQL 查询口径分别适配。 |
| Oracle | [OracleDB Monitoring](https://grafana.com/grafana/dashboards/13555-oracledb-monitoring-performance-and-table-space-stats/) | 13555 | oracledb_exporter | 表空间、会话和权限；RAC 单列验证。 |
| MongoDB | [MongoDB](https://grafana.com/grafana/dashboards/2583-mongodb/) | 2583 | mongodb_exporter / optional node_exporter | 旧版参考；实例与副本集分别建立能力矩阵。 |
| Redis | [Redis Dashboard for Prometheus Redis Exporter 1.x](https://grafana.com/grafana/dashboards/763-redis-dashboard-for-prometheus-redis-exporter-1-x/) | 763 | redis_exporter | 映射 INFO 指标；Cluster / Sentinel 单独验收。 |
| Kafka | [Kafka Exporter Overview](https://grafana.com/grafana/dashboards/7589-kafka-exporter-overview/) | 7589 | kafka_exporter | Topic、分区、消费组；Broker JVM 指标单独补齐。 |
| RabbitMQ | [RabbitMQ-Overview](https://grafana.com/grafana/dashboards/10991-rabbitmq-overview/) | 10991 | rabbitmq_prometheus | 节点、队列、消息速率；不能仅用节点 Receiver 视为完整覆盖。 |
| Nginx | [NGINX exporter](https://grafana.com/grafana/dashboards/12708-nginx/) | 12708 | nginx-prometheus-exporter | 状态指标映射；状态码与时延需要日志或额外指标。 |
| Elasticsearch | [ElasticSearch](https://grafana.com/grafana/dashboards/2322-elasticsearch/) | 2322 | elasticsearch_exporter | 按集群、节点、索引适配；统计范围与权限保持一致。 |
| ClickHouse | [ClickHouse overview / latency / replica](https://grafana.com/docs/grafana-cloud/observe-and-act/monitor-infrastructure/integrations/integration-reference/integration-clickhouse/) | Official integration | ClickHouse integration metrics | 官方集成模板参考；核对 ClickHouse 版本与指标出口。 |
| TiDB | [Overview \| TiDB Cluster](https://grafana.com/grafana/dashboards/12712-overview-tidb-cluster/) | 12712 | TiDB / TiKV / PD metrics | 按组件和版本拆分；不按 MySQL 协议推定兼容。 |
| OceanBase | [OceanBase Metrics](https://grafana.com/grafana/dashboards/15215-oceanbase-metrics/) | 15215 | OBAgent | 保留租户与 OBServer 维度，明确账号可见范围。 |
| Memcached | [Memcached](https://grafana.com/grafana/dashboards/11527-memcached-overview/) | 11527 | memcached_exporter | 命中率、淘汰、内存；验证比率和计数器单位。 |
| Apache HTTP Server | [Apache](https://grafana.com/grafana/dashboards/3894-apache/) | 3894 | apache_exporter | 请求、Worker、连接；与 server-status 能力对应。 |
| IIS | [Windows Exporter IIS Node](https://grafana.com/grafana/dashboards/14532-windows-exporter-iis-node/) | 14532 | windows_exporter / IIS counters | 按站点、应用池和 Windows 权限验证。 |
| HAProxy | [HAProxy](https://grafana.com/grafana/dashboards/12693-haproxy/) | 12693 | HAProxy v2+ Prometheus metrics | Frontend / Backend / Server 维度保持独立。 |
| Envoy | [Envoy Proxy Monitoring gRPC](https://grafana.com/grafana/dashboards/23239-envoy-proxy-monitoring/) | 23239 | Envoy Prometheus stats | 上下游、gRPC 状态、Histogram 与版本映射。 |
| JVM / Spring Boot | [JVM (Micrometer)](https://grafana.com/grafana/dashboards/4701-jvm-micrometer/) | 4701 | Micrometer | JMX 与 Micrometer 不是相同指标名；HTTP 与连接池单独核对。 |
| Tomcat | [Tomcat dashboard](https://grafana.com/grafana/dashboards/8704-tomcat-dashboard/) | 8704 | JMX exporter / Catalina MBeans | 线程池、会话、请求；适配 JMX 映射与容器版本。 |
| RocketMQ | [Rocketmq_dashboard](https://grafana.com/grafana/dashboards/10477-rocketmq-dashboard/) | 10477 | rocketmq_exporter | Broker、Topic、消费组及 4.x/5.x 差异。 |
| Pulsar | [Apache Pulsar monitoring dashboards](https://pulsar.apache.org/docs/4.0.x/deploy-monitoring/) | Official dashboard collection | Pulsar component metrics | 使用官方文档链接的 Grafana 模板集，区分租户、Namespace、Topic。 |
| NATS / JetStream | [NATS JetStream](https://grafana.com/grafana/dashboards/14725-nats-jetstream/) | 14725 | prometheus-nats-exporter | Server 与 JetStream 分层；Stream / Consumer 控制基数。 |
| ZooKeeper | [ZooKeeper by Prometheus](https://grafana.com/grafana/dashboards/10465-zookeeper-by-prometheus/) | 10465 | ZooKeeper 3.6+ metrics | 原生指标与 JVM 指标分别映射。 |
| MinIO | [Minio Overview](https://grafana.com/grafana/dashboards/10946-minio-overview/) | 10946 | MinIO metrics | 历史模板；需按部署版本校验 v2/v3 指标与鉴权。 |
| HTTP / TCP / TLS / DNS / ICMP | [Prometheus Blackbox Exporter](https://grafana.com/grafana/dashboards/7587-prometheus-blackbox-exporter/) | 7587 | blackbox_exporter | 参考可用性与探测耗时；每种协议所需指标分别适配。 |
| Host metrics | [Host Metrics (opentelemetry)](https://grafana.com/grafana/dashboards/24638-host-metrics/) | 24638 | OTel hostmetrics | 核对 scraper、单位、PromQL 名称转换策略。 |
| Host baseline | [Node Exporter Full](https://grafana.com/grafana/dashboards/1860-node-exporter-full/) | 1860 | node_exporter | 参考信息架构，node_* 不能直接等同 OTel system.*。 |
| Windows host | [Windows Server Status Dashboard - Prometheus](https://grafana.com/grafana/dashboards/16523-windows-server-status-dashboard-prometheus/) | 16523 | windows_exporter | Windows 性能计数器、磁盘与网络；不能套用 Linux 标签。 |
| Collector | [OpenTelemetry Collector](https://grafana.com/grafana/dashboards/12553-opentelemetry-collector/) | 12553 | otelcol internal metrics | 历史模板；按当前发行版核对接收、队列、发送指标。 |
| Kubernetes cluster / workloads | [Kubernetes / Compute Resources / Cluster](https://grafana.com/grafana/dashboards/16364-kubernetes-compute-resources-cluster/) | 16364 | kube-state-metrics / cAdvisor / recording rules | 资源 requests/limits 与使用量分开；补齐或改写录制规则。 |
| Kubernetes namespace | [Kubernetes / Compute Resources / Namespace (Workloads)](https://grafana.com/grafana/dashboards/16366-kubernetes-compute-resources-namespace-workloads/) | 16366 | Kubernetes mixin | Namespace 是集群内对象筛选，授权仍按已确认的集群范围。 |
| Kubernetes workload | [Kubernetes / Compute Resources / Workload](https://grafana.com/grafana/dashboards/16369-kubernetes-compute-resources-workload/) | 16369 | Kubernetes mixin | 通过 owner UID 跟随 Pod；不以 Pod 显示名建立长期身份。 |
| Kubernetes pod | [Kubernetes / Compute Resources / Pod](https://grafana.com/grafana/dashboards/16368-kubernetes-compute-resources-pod/) | 16368 | Kubernetes mixin / cAdvisor | 保留 Pod UID、容器与重启代次，串联日志和事件。 |
| Kubernetes Kubelet | [Kubernetes / Kubelet](https://grafana.com/grafana/dashboards/21788-kubernetes-kubelet/) | 21788 | Kubernetes mixin / kubelet | 保留节点与集群筛选，验证指标端点。 |
| Kubernetes API Server | [Kubernetes / API server](https://grafana.com/grafana/dashboards/12654-kubernetes-api-server/) | 12654 | API Server /metrics | 请求速率、错误、延迟；按 Kubernetes 版本验证。 |
| Kubernetes Scheduler | [Kubernetes / Scheduler](https://grafana.com/grafana/dashboards/12663-kubernetes-scheduler/) | 12663 | Scheduler /metrics | 调度时延、队列与端点权限。 |
| Controller Manager | [Kubernetes / Controller Manager](https://grafana.com/grafana/dashboards/16370-kubernetes-controller-manager/) | 16370 | Controller Manager /metrics / Kubernetes mixin | 核对工作队列、控制器指标与模板依赖规则。 |
| CoreDNS | [Kubernetes / System / CoreDNS](https://grafana.com/grafana/dashboards/15762-kubernetes-system-coredns/) | 15762 | CoreDNS Prometheus metrics | 请求、错误、缓存与 Histogram；按版本匹配。 |
| etcd | [etcd](https://grafana.com/grafana/dashboards/16359-etcd/) | 16359 | etcd /metrics | 确认端点可达和凭据权限，再匹配指标版本。 |
| Application / APM | [OpenTelemetry APM](https://grafana.com/grafana/dashboards/19419-opentelemetry-apm/) | 19419 | OTel / Prometheus metrics | 参考服务 RED 视图；Trace 详情、采样口径与来源用 Argus 契约。 |
| Application / infrastructure logs | [Logs / App](https://grafana.com/grafana/dashboards/13639-logs-app/) | 13639 | Loki / LogQL | 参考交互与分组；查询转换到 Argus KQL，保留日志来源和应用归属。 |

Kubernetes 管理体验参考 [KubeSphere 集群管理](https://www.kubesphere.io/docs/v4.1/07-cluster-management/)。Grafana Kubernetes 模板的来源和规则还可对照 [kubernetes-sigs/kubernetes-mixin](https://github.com/kubernetes-sigs/kubernetes-mixin)。
