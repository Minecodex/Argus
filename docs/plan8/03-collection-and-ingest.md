# 采集实例、可信摄入与应用数据

## 集成定义与配置编译

`IntegrationDefinitionVersion` 至少声明：业务类型、支持信号、目标身份规则、配置 Schema、可选认证方式、平台/架构、Collector 版本/组件依赖、运行依赖与内置准入、权限、端点/路径约束、基数预算、采集责任、指标/日志 Schema、诊断码及默认模板版本。

配置按以下顺序生成：

1. 读取固定版本的实例、目标、关联、凭据引用与 Collector 能力。
2. 校验配置 Schema、真实网络位置、路径/端点权限、认证方式和目标版本。
3. 展开 CollectionClaim，检测同目标、同信号和范围交集；验证 Host/Node 强身份绑定。
4. 将多个启用实例编译成 Collector Bundle；Receiver 使用稳定的类型/实例命名，不再用一张 `map[profile]bool` 表示同类配置。
5. 公共 Processor/Exporter/Extension 按拓扑组合；在合并/批处理前写入受控来源标识，转发不覆盖原来源。
6. 用目标发行版校验完整配置，生成不含 Secret 原文的 Diff，冻结 target/collector/distribution/credential/binding revisions。
7. 经过现有 Preview/Confirm/Commit 后更新 desired revision，Worker/Connector 执行并回报 effective revision。
8. 观察进程健康、单实例采集结果与实际样本；失败回滚有效配置，保留失败原因。回滚不覆盖其他用户的新版本。

K8s 发布使用 `argus-otelcol-config` / `argus-otelcol-config-next` 两个固定配置槽及对应私有 Secret；Namespace 内的固定 Lease 串行化发布并保存最后有效槽位/版本。候选配置经原生 Collector 校验后再滚动工作负载，Gateway 与 DaemonSet 均就绪才记录生效。运行时权限仅扩展到固定配置槽、校验 Job 和 Lease；ClusterRole/ClusterRoleBinding 仍由安装引导阶段授予，不在 Collector 收敛时创建或扩权。实施中，尚未完成真实集群回滚验收。

组件编译进受控发行版。缺组件时显示需升级/不支持，不在线下载任意插件或把未经验证的模板标为 supported。只采用符合[内置约束](./12-collector-native-support.md)的组件；独立exporter及外部JMX采集进程暂不纳入。系统工具/SDK依赖须明确列出，不能把Receiver名称当作纯进程内证明。外部服务资源指标还须有对应专用采集实现，通用Prometheus/SQL、存储Exporter或消息传输Receiver均不满足准入。

## 状态与责任

| 状态维度 | 示例 | 解释 |
| --- | --- | --- |
| 配置生命周期 | draft/active/deleting/deleted | 领域配置是否存在 |
| 期望采集 | enabled/disabled | 用户要求；不能当作已生效 |
| 应用状态 | pending/validating/applying/applied/failed/rollback/unknown | Collector 对版本的执行结果 |
| 观测健康 | healthy/degraded/error/unavailable | 真实采集、队列、发送结果 |
| 数据状态 | waiting/fresh/stale/no_data | 时间范围内是否有样本；没有数据不等于 0 |

`observed_at`、`last_attempt_at`、`last_sample_at`、desired/effective revision 和错误码独立返回。一个 Receiver 失败不能把同进程其他实例显示为全部失败；进程失联则明确影响其全部分配。

迁移执行位置使用新的 assignment epoch 与来源安装记录；目标和逻辑采集实例 ID 不变。默认同一责任只能有一个 primary。有限双采窗口需要明确主副、有效期和去重/切换规则；不能让 Host/DaemonSet 或两个数据库采集器长期重复计数。

文件轮转/读取 checkpoint、Kafka offset、采集队列不是应用业务状态，但其持久化和迁移规则必须明确。停止/重启不能无说明地从文件开头重放或跳过尚未处理数据。

## 多目标可信摄入

Namespace 指标当前采用[本轮补齐](./57-completion-delivery.md)的 UID 行范围：原生元数据补全、控制面核验、来源选择、聚合/候选/概览及查询签名使用同一 UID；未把 Namespace 转换成整个 Cluster 的授权。内部范围/查询/目录协议分别升级为 v3/v6/v4，仍在完成部署验收。

保留 Collector mTLS、路由和来源注册机制，替换“Collector 的资源 ID 等于所有遥测资源 ID”的假设。

```text
SDK / 本地文件 / 服务端点 / K8s 节点
  → 受控 Receiver + CollectionAssignment + 目标/应用映射
  → Leaf / DaemonSet / Gateway（保留原始身份）
  → Argus Ingest（认证、逐目标校验、配额）
  → Kafka → Writer → 企业 ClickHouse 表
  → Query Scope → Dashboard / 应用 / 告警 / Workspace
```

- 一个 OTLP 请求可以包含多个 ResourceMetrics/ResourceLogs/ResourceSpans；逐资源段解析来源和目标，不把整批归属第一个目标。
- Collector 只能声明控制面已分配的目标/信号/配置版本，SDK 不能借自报 `argus.*`、service.name 或 host.name 改变租户、应用、目标和权限。
- 网关记录原始来源与当前转发跳；未知身份、撤销绑定、错误环境、未批准信号和错误安装代次应失败关闭，并按预算记录诊断。
- 对混批中部分无效记录明确协议策略：默认整批拒绝以避免假成功；若版本明确支持 OTLP partial success，必须准确回报拒绝量/原因，去重与重试测试覆盖有效记录重发。不得静默丢弃后返回完整成功。
- 迟到数据只能按其已登记的采集/凭据版本和有效窗口处理，不套用当前版本的目标或应用归属。
- 业务目标身份不取代授权根。服务数据关联 Application，系统日志关联 Host/Cluster，共享服务指标关联自身目标。

## Trace 与 SDK 接入

用户流程：选择应用（自动带入唯一环境）→ 新建接入 → 地址/协议/Key/配置说明 → SDK 上报 → 自动发现服务/实例 → 健康诊断。新建无需填写或预建服务。地址可由多个逻辑接入共享，不为每个应用创建新端口或新进程。

Key 绑定企业、应用、从应用继承的环境、允许信号和接入配置；默认可在该应用内受控发现服务，也可配置服务限制。用于写入而非查询或平台管理。OTLP/HTTP 与 OTLP/gRPC 分别验证 Header/Metadata；SkyWalking/Jaeger 使用各自协议认证适配。未认证 UDP 不提供共享公网地址＋Key 的虚假支持；需要兼容时使用明确隔离及认证上行的本地方案，逐组合验收。

应用接入点部署在批准的 Leaf/Gateway/平台遥测入口适配层；与 Collector 到 Ingest 的服务凭据分开。外层共享网关不能把多个应用用一个无身份的内部 OTLP Receiver 混合后丢失归属。

同名服务跨应用/环境不合并；同一个已登记服务在多主机/Pod/Collector 上运行可聚合，实例、来源和部署位置仍可展开。换采集器、重启、发布新版本不自动产生新的业务服务。

实施中的 SDK 入口使用原生协议 Receiver、`argus_application_auth` 认证扩展和 `argus_application_identity` Processor。认证扩展通过现有 Collector mTLS 身份向 Ingest 核验 Key，不在 Collector 配置里分发 Key 清单；协议 Processor 在 batch 前固定接入来源。Ingest 以已登记 source/endpoint 和当前 Key 再次核验，批内服务登记在一个事务中完成。服务发现以应用级事务锁约束并发与配额，默认每应用 1000 个服务、每服务 1000 个实例；达到上限明确拒绝。入口和 Key 的创建继续 Preview/Confirm；明文领取只面向创建人的独立门户操作，10 分钟内一次，不进入模型。上述实现仍需真实协议和整个平台验收。

OTLP service.name、Jaeger Process.ServiceName 和 SkyWalking service/serviceInstance 均有现成转换基础，见[上游核查](./11-grilling-decisions-and-upstream.md)。按 application_id＋service.namespace＋service.name 幂等登记；namespace 缺失记空，不猜名称别名，协议不是服务身份。并发首次上报不能重复建服务；名称改变产生新身份，别名/迁移需显式记录。不同协议可统一服务目录，但来源和 Trace/Span 去重独立，不能重复累计双重埋点。

缺失或 SDK 默认占位服务名不进入正式目录，返回有界拒绝诊断与配置指导，不生成默认应用/服务。缺实例 ID 不拒绝有效服务数据，显示实例未知。数量/名称/速率配额由接入配置约束，客户端不能自报其他应用或环境。

## 日志采集与归属

| 来源 | 目标选择与执行 | 归属 |
| --- | --- | --- |
| 主机业务文件 | 主机本地 Collector、允许路径、多行/编码/轮转/起始位置 | 必须指定 ApplicationService |
| Journald / Windows 系统事件 | 主机对应平台能力、unit/channel/filter | Host；单独选中的业务服务规则可映射应用 |
| Kubernetes 业务容器日志 | 集群 DaemonSet、本地容器文件、Namespace/Owner/Pod UID/container | 必须映射 ApplicationService |
| Kubernetes 组件日志 | 集群组件与相应节点/工作负载 | Cluster/组件 |
| SDK 日志 | 受认证应用接入 | 应用/环境与 Key 一致，按有效 service.name 发现或匹配服务 |
| 数据库/中间件运行日志 | 对应实例/文件/协议，单独声明支持 | 对应目标；应用关联可选 |

业务日志不能用一个应用 ID 覆盖整个混合 Namespace。工作负载规则跟随可信 owner UID 链和容器身份；Pod 名仅作展示。标签可帮助选择，但应用归属来自管理员保存的受控映射，用户普通标签不能扩权。

选择器覆盖、重复文件路径、多行边界、容器重启、Pod 换节点、滚动更新、丢失源文件、权限拒绝、缓冲满与磁盘满均有明确状态。业务规则没有匹配映射时拒绝该业务范围并记录有界诊断，不静默挂到“默认应用”或别的服务。

K8s 文件读取和元数据仍由原生 `filelog` container operator 与 `k8sattributes` 执行。平台身份校验层 `argus_log_identity` 不采集数据、不调用 Kubernetes API，只检查冻结工作负载 UID、节点、容器及原生元数据是否齐备。元数据尚未同步时返回可重试错误，Receiver 保留读取位置并重试；不能成功丢弃后推进 checkpoint。它和既有应用/网关身份组件属于平台归属校验，不引入自研资源指标采集器。需以原生二进制、真实 Pod 滚动发布和元数据故障验证后才准入。

进入日志正文、Kafka、文件交付前保留统一凭据屏蔽规则。采集范围、单条大小、吞吐、保留期、字段基数和解析错误有独立预算与观测。

## APM 与日志关联

本轮已补齐受信应用服务跨来源汇总及相同 Trace/Span 身份去重，返回完整来源集合；原始来源视图和不同 Span 保留。仅有名称相同不触发跨来源合并，详见[补齐记录](./57-completion-delivery.md)。

- 保留服务/实例/接口、时延分布、错误、慢 Trace、Span 详情和受观测依赖拓扑；应用模块复用现有分析引擎和共享组件。
- Trace/Span ID 是精确日志关联键；没有 ID 时只提供标明范围的同服务/实例邻近时间检索。
- 同一 Trace 可跨应用；跨应用展开根据当前授权取可见 Span，明确缺失/受限，不改变原列表筛选。
- 统计继续基于去重后的 SERVER/CONSUMER 入口样本；不把采样速率推算成全量请求率，不对分位数取平均，不用无样本的 0 表示健康。
- SDK metrics、span-derived metrics 与原生数据库指标各自标明来源/口径，不能在同一默认面板中隐式混算。
- 缺少实例、父 Span、传播上下文或关联字段时返回 coverage，不凭拓扑相邻、服务同名或资源依赖关系补造调用链。

执行任务：[T05](./task-05-collector-runtime.md)、[T06](./task-06-trusted-telemetry.md)、[T07](./task-07-application-ingest-apm.md)、[T08](./task-08-log-collection.md)。
