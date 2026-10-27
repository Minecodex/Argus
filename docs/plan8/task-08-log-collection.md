# P8-T08：主机、K8s 与 SDK 日志规则

状态：in_progress。前件：P8-T04、P8-T05、P8-T06、P8-T07、P8-T10。

## 设计与接点

- [03-collection-and-ingest.md](./03-collection-and-ingest.md)
- [05-host-and-kubernetes.md](./05-host-and-kubernetes.md)

已有/拟扩展接点：`internal/telemetry`、`internal/otelcol/configbundle`、`internal/kubernetesreader`、`internal/collectormanager`、`internal/telemetry/queryengine/kql`、`web/apps/enterprise/src`。新领域包和具体接口在 T01 固化；路径是定位入口，不代表本任务已实现。

## 实施清单

- [ ] SDK日志在受信应用内发现服务；文件/工作负载规则使用受控服务映射。Job/CronJob Owner链与实时/持久日志的运维替代授权都需贯通。
- [ ] 评估文件日志、Journald、Windows Event Log、K8s与SDK日志来源，实现满足内置约束的独立配置；journalctl依赖单列核验，不能默认视为纯进程内。
- [ ] 按业务/系统/组件/数据库来源区分归属；业务日志入库需解析服务身份，SDK可自动发现，文件/工作负载规则使用映射；基础设施日志无需伪造业务应用。
- [ ] 支持允许路径、编码、多行、解析、起始读取、大小限制、轮转/checkpoint 与敏感字段处理。
- [ ] 使用 DaemonSet 节点采集和 UID/Owner/container 映射跟随工作负载变化，Gateway 不直接读取其他节点文件。
- [ ] 规范化路径/选择器范围，检测重复覆盖；多应用 Namespace 按记录来源分流，未匹配映射明确拒绝/诊断。
- [ ] 日志探索、统计、上下文与 Trace 下钻共用 KQL/Scope，缺 Trace ID 时明确时间窗口检索。

## 完成标准

- [ ] 实际文件写入/轮转与 Pod 滚动发布/换节点/重启验证连续性、来源、应用和错误状态。
- [ ] 主机日志、组件日志、业务日志不互相混归属；精确重叠和选择器交集都测试，不只靠字符串相等。
- [ ] 相关存储、接口/生成客户端、权限、门户/工具、审计、配置和文档保持一致；单文件小于 2000 行。
- [ ] 保存测试范围、失败迭代、源码/版本与证据；只完成 mock 或单层测试不计真实验收。

## 验收关联

对应 [验收矩阵](./09-acceptance-matrix.md)：P8-A19、P8-A25、P8-A26、P8-A27、P8-A28、P8-A29。

新增验收关联：P8-A66、P8-A67。

## 实施记录

已加入独立日志规则、路径交集检查、应用/服务映射、持久读取位置以及正式文件日志表单；整体任务尚未验收完成。

- [真实 PostgreSQL](../../artifacts/plan8-log-rules-postgres-legacy.log)：业务日志必须关联受权应用及已声明/发现的服务；系统日志归属执行主机。路径支持有限的 `*`、`?`、`**`，排除集合参与交集判断，禁用规则可以保存，启用时重新检查冲突。当前同一物理资源只有一个 Collector 的唯一约束保持生效。
- [真实 Linux Collector 文件读取](../../artifacts/plan8-native-filelog-verified.log)：两个归属加一条多行规则，验证 JSON、多行、字面量 `$` 路径、排除目录、文件轮转、重启后持久 checkpoint 和独立关闭。测试使用认证的 OTLP 测试接收端，属于原生组件门禁，不等于正式平台链路。
- [受信归属与脱敏](../../artifacts/plan8-native-log-trusted-identity.log)：每个 Resource 依据来源版本固定应用、服务和环境；原始日志中的伪造值无法替换。无服务映射的应用日志整批拒绝，不先修改合法部分；Kafka 前屏蔽已识别的凭据内容。
- [桌面门户四场景](../../artifacts/plan8-log-portal-e2e-verified.log)：中英 × 明暗，搜索分页应用/服务/Collector 弹框、环境继承、业务与系统切换、创建、禁用及可访问性通过。属于 mock UI；[中文截图](../../artifacts/plan8-log-rule-zh-light.png)、[英文深色截图](../../artifacts/plan8-log-rule-en-dark.png)保留。
- 原生二进制证实 `filter`、`transform`、`k8s_attributes` 已打包，见[组件清单](../../artifacts/plan8-log-collector-components.yaml)。增加这些组件不代表 K8s 日志规则已经通过。
- [K8s 真实 Pod 与原生 Collector](../../artifacts/plan8-k8s-native-logs-metadata-recovery.log)：读取实际节点 CRI 文件，Namespace Role 限定元数据访问；Deployment 滚动发布两个 Pod 均从首行连续到达。故意切断 Collector 的 Kubernetes TLS API 连接后，新 Pod 日志等待元数据，恢复后补齐；接收端不可用期间强制结束 Collector，重启后从持久队列和 checkpoint 续传。失败诊断未打印正文。此为组件及真实集群门禁，接收端仍是独立认证测试端点。
- [K8s 规则 PostgreSQL](../../artifacts/plan8-k8s-log-rules-postgres-verified.log)：Namespace-only 可创建自身组件日志规则，不能获得 Cluster 全量权限；跨 Namespace、Pod/Deployment 重叠拒绝，同名 Namespace 重建不继承。Cluster 管理员可以停用孤立规则，不能重新启用已退役 Namespace 的旧身份。
- [12 个门户场景](../../artifacts/plan8-log-workspace-e2e.log)覆盖文件日志、K8s 日志和工作台的中英明暗。Namespace、工作负载和容器复用统一列表弹框；[K8s 中文截图](../../artifacts/plan8-kubernetes-log-rule-zh-light.png)、[英文深色截图](../../artifacts/plan8-kubernetes-log-rule-en-dark.png)保留。
- 日志链路移除异步 batch，避免 Receiver 提前确认仍在内存中的记录；K8s 节点使用持久发送队列和 Collector 独立的 HostPath。仅启用节点日志时使用受限 UID 0，HostPath 日志挂载只读；Gateway 保持非 root，不获得节点目录。读取位置目录卸载时保留，不能未经节点所有权确认递归清理。
- 初次 K8s 测试误用了其他 context 的 CA，真实证书校验拒绝连接；改为从选定 context 的实际 REST 配置读取信任根后通过，没有跳过 TLS。失败记录保留。
- [当前后端回归](../../artifacts/plan8-log-runtime-regression-verified.log)、[PostgreSQL 完整分包](../../artifacts/plan8-logs-namespace-postgres-full.log)、[契约检查](../../artifacts/plan8-k8s-log-contracts-check.log)通过。一次并行生成与回归冲突造成生成文件缺失，按先生成后测试的顺序重跑通过，初始失败记录保留。
- 尚未完成：Journald/Windows Event Log 的真实数据、旧主机 Profile 完整迁移、K8s 跨节点连续性、持久日志运维查询授权、日志探索/Trace 关联，以及正式平台的下发、摄入、存储和查询闭环。文件及 K8s 日志未在总支持矩阵提前记为全部通过。

新增[真实后端联合链路](./21-native-log-backend-pipeline.md)：系统/应用业务日志各 12 条经原生 Collector、mTLS Ingest、SCRAM Kafka、Writer、ClickHouse，未生效时重试，强制退出后恢复；实际 Kafka 无私有 canary，业务日志不进入执行 Host 的查询。控制记录与 CA 为隔离夹具，正式接入/门户/Agent 闭环仍待完成。另补[运行目录及来源保护](./22-filelog-runtime-path-protection.md)，通过原生软链接和正文伪造路径测试；未把 Linux 结果扩大成 Windows 实测。

Journald 已开放原生规则入口，实际 info/debug、游标、发送队列及接收端故障后 KILL/恢复通过；上游同步发射开关进入发行默认值和制品指纹。单元过滤、依赖诊断和正式主机链路仍待验证，见[当前证据与边界](./24-journald-and-native-log-durability.md)。包含 Journald、K8s 默认面板入口、YAML 的 12 个桌面场景通过，仍为 mock UI。


SDK 业务日志已通过真实 Go SDK、原生 Collector、mTLS/Kafka/Writer/ClickHouse 与 HTTP/工具的服务筛选和 Trace 关联验证；三信号 Kafka 前脱敏已统一，见 [SDK 与脱敏](./30-native-sdk-pipeline-and-signal-privacy.md)。Windows 实测用户暂缓，正式主机/集群接入与完整日志探索仍待验收。
