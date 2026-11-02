# 候选实现：工具与观测目录矩阵

2026-10-30 从 internal/nativetools、monitoringcatalog、builtincollection 与 dashboard.BuiltinTemplates 导出。133 个业务工具，导航元数据完整；此表证明源码注册和展示分配，不把未执行的集成测试或模型任务标记为通过。基线 130 项保留在第 09 篇。

| 内部工具 ID | 用途（中英标题） | 资源类型 | 信号 | 操作 | 返回形态 | Chatbox 交付 |
| --- | --- | --- | --- | --- | --- | --- |
| alert.channel.list | 通知渠道列表 / List notification channels | alert |  | discover | resource_list | template / alert-settings-list |
| alert.event.acknowledge.preview | 确认处理告警预览 / Preview: Acknowledge alert | alert |  | preview | pending_action | template / change-preview |
| alert.event.assign.preview | 分配告警负责人预览 / Preview: Assign owner to alert | alert |  | preview | pending_action | template / change-preview |
| alert.event.comment.preview | 添加告警备注预览 / Preview: Add comment to alert | alert |  | preview | pending_action | template / change-preview |
| alert.event.get | 告警详情 / Get alert | alert |  | query | resource_detail | template / alert-event-detail |
| alert.event.query | 告警列表 / List alerts | alert |  | query | resource_list | template / alert-events |
| alert.grouping.list | 告警合并设置列表 / List notification grouping settings | alert |  | discover | resource_list | template / alert-settings-list |
| alert.history.query | 告警历史查询 / Query alert history | alert |  | query | resource_list | template / alert-history |
| alert.inhibition.create.preview | 创建告警抑制规则预览 / Preview: Create alert inhibition rule | alert |  | preview | pending_action | template / change-preview |
| alert.inhibition.delete.preview | 删除告警抑制规则预览 / Preview: Delete alert inhibition rule | alert |  | preview | pending_action | template / change-preview |
| alert.inhibition.disable.preview | 停用告警抑制规则预览 / Preview: Disable alert inhibition rule | alert |  | preview | pending_action | template / change-preview |
| alert.inhibition.enable.preview | 启用告警抑制规则预览 / Preview: Enable alert inhibition rule | alert |  | preview | pending_action | template / change-preview |
| alert.inhibition.list | 告警抑制规则列表 / List alert inhibition rules | alert |  | discover | resource_list | template / alert-settings-list |
| alert.inhibition.update.preview | 修改告警抑制规则预览 / Preview: Update alert inhibition rule | alert |  | preview | pending_action | template / change-preview |
| alert.notification.query | 告警通知查询 / Query alert notification | alert |  | query | resource_list | template / alert-notifications |
| alert.notification.resend.preview | 补发告警通知预览 / Preview: Resend alert notification | alert |  | preview | pending_action | template / change-preview |
| alert.notification.test.preview | 测试告警通知预览 / Preview: Test alert notification | alert |  | preview | pending_action | template / change-preview |
| alert.policy.create.preview | 创建告警策略预览 / Preview: Create alert policy | alert |  | preview | pending_action | template / change-preview |
| alert.policy.delete.preview | 删除告警策略预览 / Preview: Delete alert policy | alert |  | preview | pending_action | template / change-preview |
| alert.policy.disable.preview | 停用告警策略预览 / Preview: Disable alert policy | alert |  | preview | pending_action | template / change-preview |
| alert.policy.enable.preview | 启用告警策略预览 / Preview: Enable alert policy | alert |  | preview | pending_action | template / change-preview |
| alert.policy.get | 告警策略详情 / Get alert policy | alert |  | query | resource_detail | template / alert-policy-detail |
| alert.policy.list | 告警策略列表 / List alert policies | alert |  | discover | resource_list | template / alert-policy-list |
| alert.policy.update.preview | 修改告警策略预览 / Preview: Update alert policy | alert |  | preview | pending_action | template / change-preview |
| alert.route.create.preview | 创建通知路由预览 / Preview: Create notification route | alert |  | preview | pending_action | template / change-preview |
| alert.route.delete.preview | 删除通知路由预览 / Preview: Delete notification route | alert |  | preview | pending_action | template / change-preview |
| alert.route.disable.preview | 停用通知路由预览 / Preview: Disable notification route | alert |  | preview | pending_action | template / change-preview |
| alert.route.enable.preview | 启用通知路由预览 / Preview: Enable notification route | alert |  | preview | pending_action | template / change-preview |
| alert.route.list | 通知路由列表 / List notification routes | alert |  | discover | resource_list | template / alert-settings-list |
| alert.route.update.preview | 修改通知路由预览 / Preview: Update notification route | alert |  | preview | pending_action | template / change-preview |
| alert.settings.list | 告警评估账号列表 / List alert evaluator accounts | alert |  | discover | resource_list | template / alert-settings-list |
| alert.silence.create.preview | 创建告警屏蔽预览 / Preview: Create temporary alert silence | alert |  | preview | pending_action | template / change-preview |
| alert.silence.delete.preview | 删除告警屏蔽预览 / Preview: Delete temporary alert silence | alert |  | preview | pending_action | template / change-preview |
| alert.silence.disable.preview | 停用告警屏蔽预览 / Preview: Disable temporary alert silence | alert |  | preview | pending_action | template / change-preview |
| alert.silence.enable.preview | 启用告警屏蔽预览 / Preview: Enable temporary alert silence | alert |  | preview | pending_action | template / change-preview |
| alert.silence.list | 告警屏蔽列表 / List alert silences | alert |  | discover | resource_list | template / alert-settings-list |
| alert.silence.update.preview | 修改告警屏蔽预览 / Preview: Update temporary alert silence | alert |  | preview | pending_action | template / change-preview |
| alert.template.list | 告警消息模板列表 / List notification message templates | alert |  | discover | resource_list | template / alert-settings-list |
| application.change.preview | 应用配置变更预览 / Preview application changes | application |  | preview | pending_action | template / change-preview |
| application.endpoint.connection | SDK 接入配置 / SDK connection configuration | application |  | query | resource_detail | template / resource-list |
| application.endpoint.list | 应用接入点 / Application endpoints | application |  | discover | resource_list | template / resource-list |
| application.get | 应用详情 / Application details | application |  | query | resource_detail | template / resource-detail |
| application.key.list | 接入 Key 元数据 / Ingest Key metadata | application |  | discover | resource_list | template / resource-list |
| application.list | 应用列表 / Applications | application |  | discover | resource_list | template / resource-list |
| application.service.list | 应用服务列表 / Application services | application |  | discover | resource_list | template / resource-list |
| connector.bastion.create.preview | 创建堡垒机范围 / Create bastion scope | connector |  | preview | pending_action | template / change-preview |
| connector.bastion.delete.preview | 删除堡垒机范围 / Delete bastion scope | connector |  | preview | pending_action | template / change-preview |
| connector.bastion.enrollment_rotate.preview | 轮换堡垒机注册凭据 / Rotate bastion enrollment | connector |  | preview | pending_action | template / change-preview |
| connector.bastion.replacement.preview | 替换堡垒机代理 / Replace bastion connector | connector |  | preview | pending_action | template / change-preview |
| connector.bastion.update.preview | 修改堡垒机范围 / Update bastion scope | connector |  | preview | pending_action | template / change-preview |
| connector.get | 查看Connector 连接代理详情 / Get Connector | connector |  | query | resource_detail | template / connector-detail |
| connector.install.retry.preview | 重试代理安装 / Retry connector installation | connector |  | preview | pending_action | template / change-preview |
| connector.list | 列出Connector 连接代理 / List Connectors | connector |  | discover | resource_list | template / connector-list |
| connector.uninstall.preview | 卸载连接代理 / Uninstall connector | connector |  | preview | pending_action | template / change-preview |
| host.collector.configure.preview | 配置主机采集器 / Configure host collector | host |  | preview | pending_action | template / change-preview |
| host.collector.install.preview | 安装主机采集器 / Install host collector | host |  | preview | pending_action | template / change-preview |
| host.collector.repair.preview | 修复主机采集器 / Repair host collector | host |  | preview | pending_action | template / change-preview |
| host.collector.uninstall.preview | 卸载主机采集器 / Uninstall host collector | host |  | preview | pending_action | template / change-preview |
| host.collector.upgrade.preview | 升级主机采集器 / Upgrade host collector | host |  | preview | pending_action | template / change-preview |
| host.create.preview | 创建主机预览 / Preview create host | host |  | preview | pending_action | template / change-preview |
| host.delete.preview | 删除主机预览 / Preview delete host | host |  | preview | pending_action | template / change-preview |
| host.get | 主机获取 / Get host | host |  | query | resource_detail | template / host-detail |
| host.install.retry.preview | 重试主机安装 / Retry host installation | host |  | preview | pending_action | template / change-preview |
| host.list | 主机列表 / List hosts | host |  | discover | resource_list | template / host-list |
| host.removal.preview | 卸载主机组件 / Remove host components | host |  | preview | pending_action | template / change-preview |
| host.update.preview | 更新主机预览 / Preview update host | host |  | preview | pending_action | template / change-preview |
| kubernetes.cluster.create.preview | 创建Kubernetes 集群预览 / Preview create Kubernetes cluster | kubernetes_cluster, kubernetes_namespace |  | preview | pending_action | template / change-preview |
| kubernetes.cluster.delete.preview | 删除Kubernetes 集群预览 / Preview delete Kubernetes cluster | kubernetes_cluster, kubernetes_namespace |  | preview | pending_action | template / change-preview |
| kubernetes.cluster.get | 查看Kubernetes 集群详情 / Get Kubernetes cluster | kubernetes_cluster, kubernetes_namespace |  | query | resource_detail | template / cluster-detail |
| kubernetes.cluster.list | 列出Kubernetes 集群 / List Kubernetes clusters | kubernetes_cluster, kubernetes_namespace |  | discover | resource_list | template / cluster-list |
| kubernetes.cluster.update.preview | 更新Kubernetes 集群预览 / Preview update Kubernetes cluster | kubernetes_cluster, kubernetes_namespace |  | preview | pending_action | template / change-preview |
| kubernetes.collector.configure.preview | 配置集群采集器 / Configure cluster collector | kubernetes_cluster, kubernetes_namespace |  | preview | pending_action | template / change-preview |
| kubernetes.collector.install.preview | 安装集群采集器 / Install cluster collector | kubernetes_cluster, kubernetes_namespace |  | preview | pending_action | template / change-preview |
| kubernetes.collector.repair.preview | 修复集群采集器 / Repair cluster collector | kubernetes_cluster, kubernetes_namespace |  | preview | pending_action | template / change-preview |
| kubernetes.collector.uninstall.preview | 卸载集群采集器 / Uninstall cluster collector | kubernetes_cluster, kubernetes_namespace |  | preview | pending_action | template / change-preview |
| kubernetes.collector.upgrade.preview | 升级集群采集器 / Upgrade cluster collector | kubernetes_cluster, kubernetes_namespace |  | preview | pending_action | template / change-preview |
| kubernetes.daemonset.list | 列出 Kubernetes 守护进程集 / List daemonsets | kubernetes_cluster, kubernetes_namespace |  | discover | resource_list | template / kubernetes-resource-list |
| kubernetes.deployment.list | 列出 Kubernetes 无状态工作负载 / List deployments | kubernetes_cluster, kubernetes_namespace |  | discover | resource_list | template / kubernetes-resource-list |
| kubernetes.drain.cancel.preview | 取消节点排空预览 / Preview cancelling node drain | kubernetes_cluster, kubernetes_namespace |  | preview | pending_action | template / change-preview |
| kubernetes.helm.change.preview | Helm 回滚与卸载预览 / Preview Helm rollback or uninstall | kubernetes_cluster, kubernetes_namespace |  | preview | pending_action | template / change-preview |
| kubernetes.helm.history | Helm 版本历史 / Helm revision history | kubernetes_cluster, kubernetes_namespace |  | query | resource_list | template / kubernetes-resource-list |
| kubernetes.helm.list | Helm 发布列表 / List Helm releases | kubernetes_cluster, kubernetes_namespace |  | discover | resource_list | template / kubernetes-resource-list |
| kubernetes.namespace.list | 列出 Kubernetes 命名空间 / List namespaces | kubernetes_cluster, kubernetes_namespace |  | discover | resource_list | template / kubernetes-resource-list |
| kubernetes.node.list | 列出 Kubernetes 节点 / List nodes | kubernetes_cluster, kubernetes_namespace |  | discover | resource_list | template / kubernetes-resource-list |
| kubernetes.node_host_binding.confirm.preview | 确认节点主机绑定 / Confirm node host binding | kubernetes_cluster, kubernetes_namespace |  | preview | pending_action | template / change-preview |
| kubernetes.object.change.preview | K8s 对象变更预览 / Preview Kubernetes object changes | kubernetes_cluster, kubernetes_namespace |  | preview | pending_action | template / change-preview |
| kubernetes.object.get | K8s 对象详情 / Kubernetes object details | kubernetes_cluster, kubernetes_namespace |  | query | resource_detail | template / resource-detail |
| kubernetes.object.list | K8s 对象列表 / Kubernetes objects | kubernetes_cluster, kubernetes_namespace |  | discover | resource_list | template / kubernetes-resource-list |
| kubernetes.operation.get | K8s 操作状态 / Kubernetes operation status | kubernetes_cluster, kubernetes_namespace |  | query | resource_detail | template / resource-detail |
| kubernetes.operation.list | K8s 操作记录 / Kubernetes operations | kubernetes_cluster, kubernetes_namespace |  | discover | resource_list | template / kubernetes-resource-list |
| kubernetes.pod.list | 列出 Kubernetes 容器组 / List pods | kubernetes_cluster, kubernetes_namespace |  | discover | resource_list | template / kubernetes-resource-list |
| kubernetes.pod.logs | 读取 Pod 容器日志 / Read pod logs | kubernetes_cluster, kubernetes_namespace | logs | query | logs | template / pod-logs |
| kubernetes.service.list | 列出 Kubernetes 服务 / List services | kubernetes_cluster, kubernetes_namespace |  | discover | resource_list | template / kubernetes-resource-list |
| kubernetes.statefulset.list | 列出 Kubernetes 有状态工作负载 / List statefulsets | kubernetes_cluster, kubernetes_namespace |  | discover | resource_list | template / kubernetes-resource-list |
| kubernetes.workbench.types | K8s 类型目录 / Kubernetes types | kubernetes_cluster, kubernetes_namespace |  | discover | resource_detail | template / resource-list |
| monitoring.builtin.change.preview | 自身监控启停预览 / Preview own-resource collection | monitoring_target, host, kubernetes_cluster, application |  | preview | pending_action | template / change-preview |
| monitoring.builtin.list | 自身监控能力 / Own-resource capabilities | monitoring_target, host, kubernetes_cluster, application |  | discover | resource_list | template / resource-list |
| monitoring.collection.change.preview | 采集实例变更预览 / Preview collection changes | monitoring_target, host, kubernetes_cluster, application |  | preview | pending_action | template / change-preview |
| monitoring.collection.form | 监控安全表单 / Monitoring secure form | monitoring_target, host, kubernetes_cluster, application |  | prepare | interaction | template / resource-list |
| monitoring.collection.get | 采集详情 / Collection details | monitoring_target, host, kubernetes_cluster, application |  | query | resource_detail | template / resource-detail |
| monitoring.collection.list | 采集实例列表 / Collection instances | monitoring_target, host, kubernetes_cluster, application |  | discover | resource_list | template / resource-list |
| monitoring.collector.choices | 可用采集器 / Collector choices | monitoring_target, host, kubernetes_cluster, application |  | discover | resource_detail | template / resource-list |
| monitoring.integration.list | 监控能力目录 / Integration catalog | monitoring_target, host, kubernetes_cluster, application |  | discover | resource_list | template / resource-list |
| monitoring.target.list | 监控目标列表 / Monitoring targets | monitoring_target, host, kubernetes_cluster, application |  | discover | resource_list | template / resource-list |
| observe.capabilities.get | 观测能力与来源 / Observation capabilities and sources | host, kubernetes_cluster, kubernetes_namespace, application, monitoring_target | metrics, logs, traces | discover | resource_capabilities | template / observation-discovery |
| observe.catalog.query | 指标与字段目录 / Metrics and fields catalog | host, kubernetes_cluster, kubernetes_namespace, application, monitoring_target | metrics, logs, traces | discover | metric_catalog | template / observation-discovery |
| observe.resources.list | 发现观测资源 / Find observation resources | host, kubernetes_cluster, kubernetes_namespace, application, monitoring_target | metrics, logs, traces | discover | resource_list | template / observation-discovery |
| pending_action.cancel | 取消待确认操作 / Cancel pending action |  |  | control | operation_status | trace /  |
| pending_action.get | 查看操作状态 / Get action status |  |  | query | operation_status | action /  |
| telemetry.collector.get | 查看采集器详情 / Inspect telemetry collector | host, kubernetes_cluster |  | query | resource_detail | template / collector-detail |
| telemetry.collector.list | 列出遥测采集器 / List telemetry collectors | host, kubernetes_cluster |  | discover | resource_list | template / collector-list |
| telemetry.dashboard.catalog | 遥测数据目录 / Discover telemetry data | dashboard | metrics, logs, traces | discover | metric_catalog, fields, values | trace /  |
| telemetry.dashboard.catalog.resources | 可用分析资源 / List analysis resources | dashboard |  | discover | resource_detail | trace /  |
| telemetry.dashboard.context.candidates | 筛选候选获取 / Get filter candidates | dashboard |  | query | resource_detail | trace /  |
| telemetry.dashboard.context.resolve | 分析条件解析 / Resolve analysis conditions | dashboard |  | query | resource_detail | trace /  |
| telemetry.dashboard.draft.create | 仪表盘草稿创建 / Create dashboard draft | dashboard |  | prepare | dashboard_draft | trace /  |
| telemetry.dashboard.draft.get | 仪表盘草稿获取 / Get dashboard draft | dashboard |  | query | resource_detail | template / dashboard-definition |
| telemetry.dashboard.draft.save | 仪表盘草稿保存 / Save dashboard draft | dashboard |  | prepare | dashboard_draft | trace /  |
| telemetry.dashboard.draft.validate | 仪表盘草稿校验 / Validate dashboard draft | dashboard |  | prepare | dashboard_draft | trace /  |
| telemetry.dashboard.get | 仪表盘获取 / Get dashboard | dashboard |  | query | resource_detail | template / dashboard-definition |
| telemetry.dashboard.list | 仪表盘列表 / List dashboards | dashboard |  | discover | resource_list | template / dashboard-list |
| telemetry.dashboard.publish.preview | 仪表盘发布预览 / Preview dashboard publication | dashboard |  | preview | pending_action | template / change-preview |
| telemetry.dashboard.query | 仪表盘查询 / Query dashboard | dashboard | metrics, logs, traces | query | dashboard_result | template / dashboard-result |
| telemetry.dashboard.query.cancel | 仪表盘查询取消 / Cancel dashboard query | dashboard |  | control | operation_status | trace /  |
| telemetry.dashboard.query.get | 仪表盘查询结果 / Get dashboard query result | dashboard | metrics, logs, traces | query | dashboard_result | template / dashboard-result |
| telemetry.dashboard.query.resume | 仪表盘查询恢复 / Resume dashboard query | dashboard |  | control | operation_status | trace /  |
| telemetry.kql.query | 查询 KQL 日志 / Query KQL logs | host, kubernetes_cluster, kubernetes_namespace, application, monitoring_target | logs | query | logs, table | template / log-query |
| telemetry.overview | 查看遥测覆盖 / View telemetry coverage | host, kubernetes_cluster, kubernetes_namespace, application, monitoring_target | metrics, logs, traces | query | coverage | template / telemetry-summary |
| telemetry.promql.query | 指标查询 / Metric query | host, kubernetes_cluster, kubernetes_namespace, application, monitoring_target | metrics | query | timeseries, instant_vector, scalar | template / metric-query |
| telemetry.skywalking.trace | 查询调用链 / Query distributed traces | host, kubernetes_cluster, kubernetes_namespace, application, monitoring_target | traces | query | traces, apm | template / trace-result |
| workflow.import_result | 导入授权查询结果 / Import authorized tool result |  |  | deliver | file | file /  |
| workflow.plan.propose | 提出实施计划 / Propose implementation plan |  |  | prepare | interaction | interaction /  |
| workflow.publish_file | 发布文件下载 / Publish workspace file |  |  | deliver | file | file /  |

## 监控目录

candidate 是目录准入候选，AdapterReady 是代码适配状态；都不能代替实际接入、采集、查询和模型证据。

| 集成 | 名称 | 家族 | 信号 | 候选状态 | 适配器 | 目录验证 |
| --- | --- | --- | --- | --- | --- | --- |
| mysql | MySQL | database | metrics | candidate | True | not_run |
| postgresql | PostgreSQL | database | metrics | candidate | True | not_run |
| sqlserver | SQL Server | database | metrics | candidate | True | not_run |
| oracle | Oracle | database | metrics | candidate | True | not_run |
| mongodb | MongoDB | database | metrics | candidate | True | not_run |
| clickhouse | ClickHouse | database | metrics | deferred_no_eligible_embedded_component | False | not_applicable |
| tidb | TiDB | database | metrics | deferred_no_eligible_embedded_component | False | not_applicable |
| oceanbase | OceanBase | database | metrics | deferred_no_eligible_embedded_component | False | not_applicable |
| redis | Redis | cache | metrics | candidate | True | not_run |
| memcached | Memcached | cache | metrics | candidate | True | not_run |
| kafka | Kafka | messaging | metrics | candidate | True | not_run |
| rabbitmq | RabbitMQ | messaging | metrics | candidate | True | not_run |
| rocketmq | RocketMQ | messaging | metrics | deferred_no_eligible_embedded_component | False | not_applicable |
| pulsar | Pulsar | messaging | metrics | deferred_no_eligible_embedded_component | False | not_applicable |
| nats | NATS | messaging | metrics | deferred_no_eligible_embedded_component | False | not_applicable |
| zookeeper | ZooKeeper | messaging | metrics | candidate | True | not_run |
| nginx | Nginx | web | metrics | candidate | True | not_run |
| apache | Apache HTTP Server | web | metrics | candidate | True | not_run |
| iis | IIS | web | metrics | candidate | True | not_run |
| haproxy | HAProxy | web | metrics | candidate | True | not_run |
| envoy | Envoy | web | metrics | deferred_no_eligible_embedded_component | False | not_applicable |
| elasticsearch | Elasticsearch | database | metrics | candidate | True | not_run |
| jvm | JVM | runtime | metrics | deferred_no_eligible_embedded_component | False | not_applicable |
| tomcat | Tomcat | runtime | metrics | deferred_no_eligible_embedded_component | False | not_applicable |
| springboot | Spring Boot | runtime | metrics | deferred_no_eligible_embedded_component | False | not_applicable |
| minio | MinIO | storage | metrics | deferred_no_eligible_embedded_component | False | not_applicable |
| http | HTTP / HTTPS | probe | metrics | candidate | True | not_run |
| tcp | TCP | probe | metrics | candidate | True | not_run |
| tls | TLS | probe | metrics | candidate | True | not_run |
| dns | DNS | probe | metrics | candidate | True | not_run |
| icmp | ICMP | probe | metrics | candidate | True | not_run |
| otlp | OpenTelemetry / OTLP | ingestion | metrics, logs, traces | candidate | False | not_run |
| skywalking | SkyWalking | ingestion | traces | deferred_upstream_defect | False | blocked |
| jaeger | Jaeger | ingestion | traces | candidate | False | not_run |
| filelog | File logs | ingestion | logs | candidate | True | not_run |
| k8slog | Kubernetes logs | ingestion | logs | candidate | True | not_run |
| sdklog | SDK logs | ingestion | logs | candidate | False | not_run |
| journald | Journald | ingestion | logs | candidate | True | not_run |
| eventlog | Windows Event Log | ingestion | logs | candidate | False | not_run |

## 内置采集

| 能力 | 对象类型 | 来源 | 平台 |
| --- | --- | --- | --- |
| host_metrics | host | hostmetrics | linux_amd64, linux_arm64, windows_amd64 |
| host_process_metrics | host | host_processes | linux_amd64, linux_arm64, windows_amd64 |
| host_systemd_metrics | host | host_systemd | linux_amd64, linux_arm64 |
| host_windows_service_metrics | host | host_windows_services | windows_amd64 |
| collector_metrics | host, kubernetes_cluster | collector_self | linux_amd64, linux_arm64, windows_amd64 |
| kubernetes_cluster_metrics | kubernetes_cluster | k8s_cluster | linux_amd64, linux_arm64 |
| kubernetes_node_metrics | kubernetes_cluster | kubeletstats | linux_amd64, linux_arm64 |
| kubernetes_apiserver_metrics | kubernetes_cluster | kubernetes_apiserver | linux_amd64, linux_arm64 |
| kubernetes_coredns_metrics | kubernetes_cluster | kubernetes_coredns | linux_amd64, linux_arm64 |
| kubernetes_scheduler_metrics | kubernetes_cluster | kubernetes_scheduler | linux_amd64, linux_arm64 |
| kubernetes_controller_metrics | kubernetes_cluster | kubernetes_controller_manager | linux_amd64, linux_arm64 |
| kubernetes_etcd_metrics | kubernetes_cluster | kubernetes_etcd | linux_amd64, linux_arm64 |

## 仪表盘模板

33 项包含模型治理私有模板；普通工具必须拒绝其私有数据域。

| 模板 | 名称 | 版本 | 集成 |
| --- | --- | --- | --- |
| mysql.overview | MySQL | 1 | mysql |
| redis.overview | Redis | 2 | redis |
| nginx.overview | Nginx | 1 | nginx |
| host.overview | Host | 1 | host_metrics |
| collector.overview | Collector | 1 | collector_metrics |
| kubernetes.cluster | Kubernetes cluster | 1 | kubernetes_cluster_metrics |
| kubernetes.nodes | Kubernetes nodes and containers | 1 | kubernetes_node_metrics |
| kubernetes.apiserver | Kubernetes API server | 1 | kubernetes_apiserver_metrics |
| kubernetes.coredns | Kubernetes CoreDNS | 1 | kubernetes_coredns_metrics |
| host.processes | Host processes | 2 | host_process_metrics |
| host.systemd | Linux system services | 2 | host_systemd_metrics |
| host.windows_services | Windows system services | 1 | host_windows_service_metrics |
| kubernetes.scheduler | Kubernetes Scheduler | 1 | kubernetes_scheduler_metrics |
| kubernetes.controller_manager | Kubernetes Controller Manager | 1 | kubernetes_controller_metrics |
| kubernetes.etcd | Kubernetes etcd component | 1 | kubernetes_etcd_metrics |
| postgresql.overview | PostgreSQL | 1 | postgresql |
| http.overview | HTTP checks | 2 | http |
| tcp.overview | TCP checks | 1 | tcp |
| tls.overview | Certificate expiry | 1 | tls |
| memcached.overview | Memcached | 2 | memcached |
| apache.overview | Apache HTTP Server | 1 | apache |
| haproxy.overview | HAProxy | 1 | haproxy |
| rabbitmq.overview | RabbitMQ | 2 | rabbitmq |
| zookeeper.overview | ZooKeeper | 1 | zookeeper |
| dns.overview | DNS checks | 2 | dns |
| icmp.overview | ICMP checks | 1 | icmp |
| kafka.overview | Kafka | 2 | kafka |
| mongodb.overview | MongoDB | 2 | mongodb |
| elasticsearch.overview | Elasticsearch | 2 | elasticsearch |
| sqlserver.overview | SQL Server | 2 | sqlserver |
| oracle.overview | Oracle | 1 | oracle |
| iis.overview | IIS | 1 | iis |
| argus.model-governance | 模型治理 | 1 |  |

验收层次与业务路径见 [04 业务映射](./04-business-tool-map.md)、[08 验收矩阵](./08-acceptance-and-evaluation.md) 和 [实施记录](./implementation-status.md)。
