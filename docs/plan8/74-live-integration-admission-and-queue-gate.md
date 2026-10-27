# 真实集成准入与待发送队列门禁

日期：2026-10-22。状态：目标夹具的运行版本与权限已经实测；新增完整平台路径、逐目标授权及队列恢复待正式执行。不提前标为 supported。

## 专用资源的真实目标

继已通过正式链的 MySQL、SQL Server、Redis、Nginx 后，追加 PostgreSQL、Memcached、Apache、HAProxy、ZooKeeper、RabbitMQ、MongoDB、Elasticsearch、Oracle、Kafka。现有 73 个检查组继续保留，具名根测试扩展。服务目标只在本轮客户内部 Docker 网络创建，所有权意图在启动前保存，容器 ID、标签、镜像 ID 和实际版本按阶段保存；清理只作用于确认归属的容器与匿名卷。

| 目标 | 本机实际版本 | 本次证据范围 |
| --- | --- | --- |
| Memcached | 1.6.45 | 真实协议就绪、版本、镜像与清理 |
| Apache | 2.4.69 | 原生 server-status、进程版本、镜像与清理 |
| HAProxy | 3.2.26-3e0f11a | 专用认证 stats、实际 Apache 后端、版本与清理 |
| ZooKeeper | 3.9.6-a355171b081b5b60749db8f19cca1528b0df936f | 真实四字命令与版本、镜像、清理 |
| RabbitMQ | 4.3.6 | monitor 正向读取、禁止队列写入、错误密码 401、运行版本 |
| MongoDB | 7.0.16 | clusterMonitor、明确拒绝业务写入、实际版本 |
| Elasticsearch | 8.17.0 | monitor 权限、禁止建索引、错误密码拒绝、实际版本 |
| Oracle | 23.5.0.24.07 | 13 个显式只读视图、登录、ORA-01031 写入拒绝、实际版本 |
| Kafka | 4.2.1 | SASL 监控身份、元数据 ACL、业务消息/消费偏移、禁止建 Topic、实际版本 |

PostgreSQL 17 系列目标也已实测启动、pg_monitor、禁止建库、实际表数据、版本读取与归属清理；首次临时文件已随测试清理，不能凭镜像标签补写具体服务版本，正式阶段将保留它的 `SHOW server_version` 结果。

轻组件证据：`artifacts/plan8-light-actual-target-fixtures-r2.log`、`plan8-light-actual-target-versions.json`。RabbitMQ：`plan8-rabbit-actual-target-fixture-r3.log`、`plan8-rabbit-actual-target-version.json`。重目标：`plan8-{mongodb,elasticsearch,oracle,kafka}-actual-target.log` 与 `plan8-additional-actual-target-versions/*.json`。Elasticsearch、Oracle、Kafka 顺序运行，暂停原服务的记录在 `artifacts/plan8-heavy-targets-20261022-desktop-pause`；最后恢复 passed、原 server 1/1。

Memcached、ZooKeeper 此组合没有独立认证账号，不将 Receiver 只读统计行为冒充目标端 ACL。Kafka 管理监听只在容器回环，外部监听使用受限 SASL 身份。认证材料走私有 env/config 文件或 stdin，不放进 Docker 参数、模型或公开版本证据。没有引入 exporter、JMX 采集服务或自研资源 Receiver。

## 完整平台准入路径

所有目标复用同一个正式验证方法：平台 API 创建目标和托管凭据、已接入 Collector 应用配置、真实样本经现有 PKI/Kafka/Writer/Query、逐个默认面板取数、独立暂停/恢复/停止。追加阶段位于模型场景之后；按组启动和清理重实例，失败保留并阻断发布。

平台授权使用真实 API 创建最小 `monitoring.read` 用户、临时密码登录/改密和 MFA。逐目标验证未授权记录/查询拒绝，显式授权后得到该目标真实指标，撤权并正常重新认证后再次拒绝，避免把旧 Session 的整体失效冒充目标/查询撤权。失败 finally 撤销本轮临时授权。旧基础阶段的数据库与 Redis/Nginx 在模型之后执行同一独立历史授权门禁，查询使用其真实最后样本时间。

这些新路径目前只是实现并编译，尚须下一轮正式执行。上表的目标准备成功不抵扣五类完整支持组合门禁。

## 排除上游重采的 Gateway 恢复

新增门禁只操作当前运行创建的 Namespace、Collector、Ingest Deployment、DaemonSet 和 PVC：停止 Ingest，产生 16 条真实工作负载日志，观察 Gateway 原生接收计数与待发队列；然后暂时停止准确归属的上游 Agent，等待实际 Pod 全停，再替换 Gateway Pod 并使用同一 PVC。恢复 Ingest 后，上游仍保持停止，必须查到原 16 条日志且队列排空，再恢复 Agent。这样原始记录不能靠上游重发补齐。

上游暂停使用本轮独有 NodeSelector，无节点标签变更；恢复只移除自身键，保留其他字段，并核验 Namespace/对象 UID。Ingest 和 Agent 均有 finally 恢复，失败也保存阶段和原生计数诊断。五分钟重试上限未改变，此专项不声称通过既有冻结容量负载档案。

当前尚未运行该新增正式门禁。旧 Pod 重建账本证明中的 `pending_export_batch_survival_verified=false` 继续保留，不能由本次实现声明替换。单项故障失败不算通过；只在真实主机数据和控制恢复后，才继续无依赖场景，整轮仍非零退出。
