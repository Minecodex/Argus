# Plan8：应用、监控采集与 Kubernetes 工作台

> 本地环境清理：用户已确认保留正式构建且不要求高可用。其他测试运行时、测试卷、旧镜像及 Docker 构建缓存已清理，正式环境健康与业务数据保留核验通过；手动缓存清理状态见[本机正式版记录](../local-formal-deployment-20261026.md)。历史测试资源记录按主环境业务数据保留。

> 2026-10-26 本地发布：已按用户明确指令重新构建并发布 `local-20261026-013254`；20/20 健康检查、10 个运行角色镜像及两台原主机的新鲜采集核验通过，业务数据、卷、Workspace 和本次遥测基线保留检查通过。[本次发布记录](../../artifacts/local-rebuild-20261026/report.md)。旧 P8-A64 缺口与全计划 75/76 状态保持，单独授权的本地发布不等同全计划验收完成。

> 2026-10-26 后续完善：保留校验工具已纳入仓库，12 项专项和真实环境的新基线 30 表采集/恢复写入/复核通过，Writer 已恢复。已更新发布状态及 T16 文档的过期描述。R88 产品代码和运行制品保持原版本，本次新增维护工具与文档修订单独记证，不追溯覆盖 R88 的 75/76 验收结论。见[校验修正及验证](./108-telemetry-preservation-checker.md)。

已完成确认范围内功能实现；当前源码完整业务 **21/21**、原生 **90/90**、Go **92 包**、支持组合 **27** 项和内置能力 **12** 项通过。原 evaluation 部署已更新，原节点、真实指标、门户 **12** 项与标准健康 **20** 项通过。正式基础门禁仍为 **75/76**，不能声明全部验收完成。

剩余 **P8-A64**：全部 30 张遥测事实表审计中，322 条旧指标目录元数据的序列化摘要不一致；旧基线缺少可重新规范化的内容，精确对应的 Kafka 分区 2/offset 4580 已早于 Broker 最早保留的 4582。当前无法证明该部分历史元数据完全保留，也不能仅凭摘要差异断言数据丢失。其他业务表、卷、Workspace 和遥测记录核验结果单列。

[R88 交付、支持表与无法补齐项](../../artifacts/plan8-r88-delivery-and-remaining-limitations.md)。11 类独立监控、SkyWalking、Oracle 已测长密码组合、Windows/ARM 实机与 production 架构边界继续按已确认范围记录。R88 验收源码指纹为 `d6737f4bb7b56ab79d92e6a39b2565ea592b54fbb03fb12054467d3fe9c3f672`；后续维护工具及文档变更不冒充该冻结版本。

本计划覆盖本轮讨论的整体改造：应用与服务、监控目标、多实例采集、监控专用凭据、SDK 接入、日志与 Trace 归属、主机自身监控、Kubernetes 组件监控与在线资源管理、两批服务集成、默认仪表盘、统一查询筛选，以及告警和 Agent 的联动。

用户已认可两批服务目录、配套仪表盘和整体原型，并明确调整新建流程：监控不重复填写环境；应用/服务使用搜索分页列表弹框；普通新建自动建立目标；认证信息在监控表单直接填写并自动托管，不混入平台用途凭据默认目录。这些是本计划的产品基线。

2026-10-20 已继续实现 [Helm 工作台](./51-helm-workbench.md)：安装/升级/回滚/卸载、受保护 Chart/values、逐对象授权、审批队列、逐写许可、历史与共享门户。新增三个原生工具，领域工具合计 23 个；具名 harness 合计 41 组。真实原生 API、数据库/HTTP、受限渲染及四种门户配色/语言有分层证据，正式部署闭环与整体计划仍未完成、未发布。

2026-10-20 接续审计补齐 [DNS、ICMP、Kafka](./53-native-network-and-kafka.md) 的实际目标与默认内容。为消除 0.158 DNS Receiver 空实现，Collector 统一升级 0.159.0/发行标识 0.3.0-plan8，三平台重建并回归已有能力。模板共 27 套，具名检查共 45 组；新增真实查询、故障恢复、只读 Kafka 身份及中英明暗门户证据。旧版或未验证 Collector 不因同名组件而显示为兼容。52 审计保留本轮补充前的时点，当前状态见 progress.json；整体仍未完成、未发布。

2026-10-20 继续补齐 [Helm 仓库与 OCI 来源](./55-helm-repository-and-oci.md)：三种来源共用原生预览/审批/执行、固定摘要和加密临时输入，源认证在预览后移除，执行不再下载。协议更新到 argus.kubernetes_helm/v2，新增来源分组后完整驱动注册 48 组。54 中该实现缺口已补；其他功能与完整部署/模型/发布验收继续进行。

## 阅读与执行入口

当前维护补充：[108 遥测保留校验修正、运行方法与实测范围](./108-telemetry-preservation-checker.md)、[109 监控列表筛选与仪表盘工具栏](./109-monitoring-filters-and-dashboard-toolbar.md)。

| 文档 | 用途 |
| --- | --- |
| [01-scope-and-decisions.md](./01-scope-and-decisions.md) | 已确认需求、实现边界、旧设计变化与技术前件 |
| [02-domain-data-and-authorization.md](./02-domain-data-and-authorization.md) | 应用/服务/目标/来源身份、存储、环境归属和数据权限 |
| [03-collection-and-ingest.md](./03-collection-and-ingest.md) | 多实例采集、Collector 配置、可信身份、日志、Trace 与 APM |
| [04-credentials-and-api-contracts.md](./04-credentials-and-api-contracts.md) | 监控凭据、SDK Key、API、分页、工具和敏感输入 |
| [05-host-and-kubernetes.md](./05-host-and-kubernetes.md) | 主机自身采集、K8s 组件和在线资源管理 |
| [06-dashboard-scope-and-integrations.md](./06-dashboard-scope-and-integrations.md) | 仪表盘筛选、模板、默认告警和跨入口范围 |
| [07-integration-catalog.md](./07-integration-catalog.md) | 39 个候选入口、支持组合及内置能力清单 |
| [grafana-references.md](./grafana-references.md) | 42 条已核对的 Grafana 参考及采集依赖 |
| [08-implementation-plan.md](./08-implementation-plan.md) | 16 个任务、依赖图、交付顺序与统一完成定义 |
| [09-acceptance-matrix.md](./09-acceptance-matrix.md) | 真实组件、权限、故障、模型和桌面 E2E 门禁 |
| [10-rollout-and-status.md](./10-rollout-and-status.md) | 迁移、数据保留、发布/回退、环境前件和状态 |
| [11-grilling-decisions-and-upstream.md](./11-grilling-decisions-and-upstream.md) | 问答结论、SDK 身份事实、开源选型与缺项规则 |
| [12-collector-native-support.md](./12-collector-native-support.md) | 内置组件约束、39候选路线、暂缓项与运行依赖 |
| [13-direct-receiver-review.md](./13-direct-receiver-review.md) | 专用采集插件复查：11资源暂缓，通用抓取不算资源支持 |
| [14-implementation-log.md](./14-implementation-log.md) | 实际实现、分层验证证据及未完成项 |
| [15-sdk-admission-findings.md](./15-sdk-admission-findings.md) | SDK 原生协议验证与 SkyWalking 新增暂缓 |
| [16-namespace-log-query.md](./16-namespace-log-query.md) | Namespace 选择授权、实际数据身份、来源过滤与运维日志验证 |
| [17-managed-defaults-and-sdk-admission.md](./17-managed-defaults-and-sdk-admission.md) | 受管默认模板、真实原生指标查询、个人副本与严格 SDK 来源准入 |
| [18-kubernetes-mutation-privileges.md](./18-kubernetes-mutation-privileges.md) | K8s 主机特权、服务账户身份、RBAC 委派与写入协议 v2 |
| [19-kubernetes-yaml-editor.md](./19-kubernetes-yaml-editor.md) | 共享 YAML/JSON 清单编辑、敏感字段与资源版本冻结 |
| [20-collector-control-and-admission.md](./20-collector-control-and-admission.md) | 空 Profile、配置应用窗口、历史来源围栏与摄入重试 |
| [21-native-log-backend-pipeline.md](./21-native-log-backend-pipeline.md) | 原生系统/业务日志经 mTLS、Kafka、Writer、ClickHouse 的联合恢复验证 |
| [22-filelog-runtime-path-protection.md](./22-filelog-runtime-path-protection.md) | 文件日志的运行目录排除、软链接检查与来源字段保护 |
| [23-fixed-own-metric-instances.md](./23-fixed-own-metric-instances.md) | Host/K8s 基础自身实例、原生生命周期及 Host/Collector 默认模板 |
| [24-journald-and-native-log-durability.md](./24-journald-and-native-log-durability.md) | 原生 Journald、同步发射默认值、持久队列与实际故障恢复 |
| [25-kubernetes-own-metric-samples.md](./25-kubernetes-own-metric-samples.md) | 集群/Kubelet/API Server/CoreDNS 原生指标、独立启停和 24 项默认查询 |
| [26-kubernetes-storage-jobs-and-allocations.md](./26-kubernetes-storage-jobs-and-allocations.md) | PVC 扩容前件、Job 实际就绪与 Service/PVC 自动分配摘要 |
| [27-application-observability-and-time-boundaries.md](./27-application-observability-and-time-boundaries.md) | 应用七视图、服务身份、入口授权与三信号精确时间窗口 |
| [28-windows-testing-deferral.md](./28-windows-testing-deferral.md) | 用户暂缓 Windows 实机测试、保留实现及未验证范围 |
| [29-kubernetes-workload-rollback.md](./29-kubernetes-workload-rollback.md) | 三类工作负载的历史修订、受控模板回滚和分层验证 |
| [30-native-sdk-pipeline-and-signal-privacy.md](./30-native-sdk-pipeline-and-signal-privacy.md) | 真实 SDK 三信号链路、应用授权、队列前脱敏与指标后缀修复 |
| [31-cronjob-manual-run.md](./31-cronjob-manual-run.md) | 冻结 CronJob 模板、独立手动 Job、写协议 v4 与分层验收 |
| [32-implementation-audit.md](./32-implementation-audit.md) | 16 项任务完成度、实际开发缺口、真实验证及暂缓范围 |
| [33-native-domain-tools.md](./33-native-domain-tools.md) | 20 个新增原生工具、分类发现、敏感输入及带类型结果授权 |
| [34-host-process-and-service-metrics.md](./34-host-process-and-service-metrics.md) | 主机进程/系统服务、三平台内置组件、Linux 实际状态和默认查询 |
| [35-acceptance-harness.md](./35-acceptance-harness.md) | 具名验收入口、源码/Collector 指纹、拒绝 Skip 抵扣及六组实际执行 |
| [36-kubernetes-control-metric-availability.md](./36-kubernetes-control-metric-availability.md) | 三个控制面固定实例、默认模板和真实不可达/拒绝/启停验证 |
| [37-postgres-and-network-probes.md](./37-postgres-and-network-probes.md) | PostgreSQL/HTTP/TCP/证书期限真实链路、14 个查询及认证失败零值修复 |
| [38-workspace-typed-source-revocation.md](./38-workspace-typed-source-revocation.md) | 应用/目标/Namespace 撤权对结果、文件、导出及 Workspace 的约束 |
| [39-cache-and-web-targets.md](./39-cache-and-web-targets.md) | Memcached/Apache/HAProxy 实际目标、认证/生命周期与 18 个默认查询 |
| [40-rabbitmq-and-zookeeper-targets.md](./40-rabbitmq-and-zookeeper-targets.md) | RabbitMQ/ZooKeeper 实际只读目标、认证/生命周期和 12 个默认查询 |
| [41-implementation-followup-status.md](./41-implementation-followup-status.md) | 审计后新增实现、分层真实证据及当前剩余开发/验收事项 |
| [42-completion-audit.md](./42-completion-audit.md) | 第二轮实施前的 16 项任务复核，保留历史时点 |
| [43-node-drain.md](./43-node-drain.md) | drain/PDB、取消、权限和恢复、原生双节点与门户证据 |
| [44-control-metrics-authentication.md](./44-control-metrics-authentication.md) | 受保护组件直接 HTTPS、证书信任、安装权限与十八个真实面板 |
| [45-followup-delivery.md](./45-followup-delivery.md) | 第二轮交付、当前数量、剩余开发及正式验收边界 |
| [46-completion-audit.md](./46-completion-audit.md) | 日志会话交付前的 16 项任务复核，保留历史时点 |
| [47-pod-log-sessions.md](./47-pod-log-sessions.md) | UID/容器代次绑定、短期票据、当前/上次增量日志、撤权、清退及真实 Pod 验证 |
| [48-completion-audit.md](./48-completion-audit.md) | 日志会话交付后的历史复核，区分开发、已有证据和正式验收 |
| [49-kubernetes-stream-sessions.md](./49-kubernetes-stream-sessions.md) | 独立终端/转发会话、权限与凭据、门户/CLI、受限真实集群和分层验证 |
| [50-completion-audit.md](./50-completion-audit.md) | 终端/转发交付后的历史逐任务复核、源码指纹与剩余范围 |
| [51-helm-workbench.md](./51-helm-workbench.md) | 原生 Helm 生命周期、敏感材料/受限渲染、逐对象授权与分层证据 |
| [52-completion-audit.md](./52-completion-audit.md) | Helm 上传入口交付后的历史复核，仓库/OCI 遗漏由 54 补充 |
| [53-native-network-and-kafka.md](./53-native-network-and-kafka.md) | Collector 统一升级、DNS/ICMP/Kafka 原生目标、默认模板与回归 |
| [54-completion-audit.md](./54-completion-audit.md) | DNS/ICMP/Kafka 交付后的历史复核，其中 Helm 仓库/OCI 缺口已由 55 补齐 |
| [55-helm-repository-and-oci.md](./55-helm-repository-and-oci.md) | 仓库/OCI 来源、私有认证、固定制品、原生/数据库/门户分层证据 |
| [56-completion-audit.md](./56-completion-audit.md) | Helm 仓库/OCI 交付后的最新逐任务复核、实现缺口与正式验收/发布边界 |
| [57-completion-delivery.md](./57-completion-delivery.md) | 全量补齐执行、当前证据和最终限制记录（执行中） |
| [58-query-scope-and-file-view.md](./58-query-scope-and-file-view.md) | 大来源集合二进制传输、空范围不扩大、Linux 文件日志与 systemd 视图 |
| [59-collector-outcome-reconciliation.md](./59-collector-outcome-reconciliation.md) | 断线未知投影、当前连接核验旧类型化结果、私密保存与正式故障门禁 |
| [60-bounded-retry-and-delivery-gaps.md](./60-bounded-retry-and-delivery-gaps.md) | 五分钟有限重试、私密控制报告、可靠 Outbox、告警及真实故障证据 |
| [61-verified-route-recovery.md](./61-verified-route-recovery.md) | 配置证明、当前隧道租约和凭据共同核验后的逻辑路由恢复 |
| [62-external-read-transaction-boundary.md](./62-external-read-transaction-boundary.md) | K8s 外部预检与短提交事务、私有快照绑定及正式冲突修复 |
| [63-kubernetes-gateway-persistent-state.md](./63-kubernetes-gateway-persistent-state.md) | Gateway 专属 PVC、校验任务隔离、Pod 替换与卷回收边界 |
| [64-writable-and-observed-log-config.md](./64-writable-and-observed-log-config.md) | 日志配置输入与只读观测分离、统一客户端转换和独立启停 |
| [65-sdk-durable-acknowledgement.md](./65-sdk-durable-acknowledgement.md) | SDK 确认前持久入队、60,000 点出口故障与强杀恢复验证 |
| [66-formal-foundation-and-network-policy.md](./66-formal-foundation-and-network-policy.md) | 14 个功能阶段通过、NetworkPolicy 校验失败及隔离集群后续验收 |
| [67-outcome-idempotency-and-model-run.md](./67-outcome-idempotency-and-model-run.md) | 状态重放版本幂等、r35 失效范围和隔离镜像前件核验 |
| [68-pinned-upstream-crd-hook.md](./68-pinned-upstream-crd-hook.md) | 安装工具镜像摘要冻结与上游 CRD Hook 保持原行为 |
| [69-model-business-gates-and-driver-corrections.md](./69-model-business-gates-and-driver-corrections.md) | 八场真实模型业务、确认幂等和公开工具分类修正，以及实际网络/HPA 证据 |
| [70-typed-apm-agent-queries.md](./70-typed-apm-agent-queries.md) | 真实模型 APM 失败、类型化查询与安全反馈，以及实际 SDK 查询回归 |
| [71-kubernetes-rbac-live-acceptance.md](./71-kubernetes-rbac-live-acceptance.md) | 四类 RBAC 真实批准变更、平台范围和目标 API 提权/绑定拒绝 |
| [72-crd-deletion-and-stable-fault-prerequisites.md](./72-crd-deletion-and-stable-fault-prerequisites.md) | CRD 删除依赖、共享 Helm 保护、内部协议升级及稳定故障前件 |
| [73-native-description-and-targeted-revocation.md](./73-native-description-and-targeted-revocation.md) | r49 四个真实模型、统一描述门禁与精准授权失效 |
| [74-live-integration-admission-and-queue-gate.md](./74-live-integration-admission-and-queue-gate.md) | 真实服务版本、完整平台准入路径、逐目标授权及排除上游重采的队列门禁 |
| [75-query-delivery-and-evidence-verification.md](./75-query-delivery-and-evidence-verification.md) | 六模型结果、告警权威契约、自动查询交付、配额及完整证据校验 |
| [76-formal-model-completion-and-focused-repair.md](./76-formal-model-completion-and-focused-repair.md) | 八场真实模型完成、逐服务结果、剩余失败及局部复验入口 |
| [77-authentication-boundaries-and-exact-artifacts.md](./77-authentication-boundaries-and-exact-artifacts.md) | r52 定向结果、Oracle 认证边界、活跃查询窗及实际物料绑定 |
| [78-scoped-aggregation-and-admission-diagnostics.md](./78-scoped-aggregation-and-admission-diagnostics.md) | PG正式准入通过、聚合范围证明和分层OTLP诊断 |
| [79-unverified-namespace-attribution.md](./79-unverified-namespace-attribution.md) | Namespace 证据缺失的 Cluster 保留、身份矛盾拒绝与查询覆盖说明 |
| [80-action-batch-confirmation-recovery.md](./80-action-batch-confirmation-recovery.md) | 真实模型预览/读取同批停顿、受限批次收尾及崩溃恢复 |
| [81-source-and-status-report-binding.md](./81-source-and-status-report-binding.md) | 实现严格冻结、三个状态报告独立留摘要与原始验收合同约束 |
| [82-live-selectors-and-config-admission.md](./82-live-selectors-and-config-admission.md) | 80应用/80服务的真实浏览器验证、稳定ID提交及逐服务CONFIG负向门禁 |
| [83-host-kubernetes-log-responsibility.md](./83-host-kubernetes-log-responsibility.md) | 同节点日志重复采集修复、实际进程回滚、身份故障注入及门户会话前件 |
| [84-bounded-sdk-acceptance.md](./84-bounded-sdk-acceptance.md) | SDK长场景分组、总进程时限修复及保持不变的容量／重试预算 |
| [85-crd-foreground-gc-observation.md](./85-crd-foreground-gc-observation.md) | 新 CRD 前台删除的控制器发现时序与原工作流期限 |
| [86-native-picker-name-and-key-search.md](./86-native-picker-name-and-key-search.md) | 名称／标识搜索、确定性碰撞与归档目标的实际浏览器断言 |
| [87-install-completion-and-portal-session.md](./87-install-completion-and-portal-session.md) | 安装完成记录与真实门户会话传递 |
| [88-kubernetes-log-owner-resync.md](./88-kubernetes-log-owner-resync.md) | 原生元数据缓存重同步与实际日志恢复 |
| [89-existing-enterprise-registry-upgrade.md](./89-existing-enterprise-registry-upgrade.md) | 已有企业内置权限目录升级、并发与原子回滚 |
| [90-local-forward-verification.md](./90-local-forward-verification.md) | 本地集群别名、显式转发 mTLS 验证、生产边界及 SDK 诊断结果 |
| [91-model-record-transactions.md](./91-model-record-transactions.md) | 模型记录事务并发修复、实际 PostgreSQL 回归及 Workspace 定向验收 |
| [92-selected-series-scan.md](./92-selected-series-scan.md) | 默认视图扫描预算、已选序列索引裁剪和真实 ClickHouse 回归 |
| [100-query-completeness-and-structured-conclusions.md](./100-query-completeness-and-structured-conclusions.md) | 实际指标取数、覆盖说明、严格 JSON 结论及模型专项范围 |
| [101-installation-tool-boundary.md](./101-installation-tool-boundary.md) | 独立安装工具、真实 ServiceAccount Hook 与远程连接边界 |
| [102-connector-rotation-identity-consistency.md](./102-connector-rotation-identity-consistency.md) | 原目标发现的轮换 CSR 缓存缺陷、完整身份提交与修正版本重新验收 |
| [progress.json](./progress.json) | 可机读任务状态、集成状态、依赖与验收状态 |

任务文件由 [08-implementation-plan.md](./08-implementation-plan.md) 进入；任务 ID 为 `P8-T01`～`P8-T16`，验收 ID 为 `P8-Axx`，集成 ID 为 `P8-Ixx`。

## 当前基线与交付边界

- 代码检查基线 HEAD：`122609b41893d0f6e689fa2b5d453a2a301616b7`；编制时工作区还有本轮监控原型及文档改动。后续实施必须重新记录源码和部署版本。
- [交互原型](../plans/monitoring-workspace/demo.html)及[原型说明](../plans/monitoring-workspace/README.md)保留原位置。本轮未重写 Demo，其中多环境应用、SDK 预选服务、Cluster-only 和有限 K8s 操作仍是旧假设；最新语义以本计划为准。
- 原型主流程 34 条检查、选择器与凭据专项 10 项通过；证据位于 `artifacts/monitoring-workspace-forms-20261018/`。它们只证明模拟交互，不证明生产采集、凭据加密、授权或 Kubernetes 操作已经完成。
- 现有 Collector、三信号链路、查询引擎、Dashboard、告警、PendingAction、Secret Broker 和远程执行能力可以复用，但要通过 Plan8 专项验证才能计入本计划的完成状态。
- 已进入实施，见[实施记录](./14-implementation-log.md)；尚未通过生产验收，不因文档或单层测试通过而标全量完成。

## 全量目标

1. 每个监控目标有稳定身份，同一 Collector 可独立采集多个数据库、中间件或服务。
2. 业务 Trace/日志归属应用服务，系统日志归属基础设施；关联、授权、部署位置和采集位置分别表达。
3. 两批候选均完成开源审查；纳入支持的类型/模式交付真实采集、状态、默认仪表盘和告警建议，无合格内置组件者暂缓并列缺项。
4. 主机/K8s 固定自身能力直接启停；文件、工作负载日志和外部服务按实例/规则配置。
5. Kubernetes 工作台贯通查询、详情、变更预览、在线操作、日志、终端和审计。
6. 应用、资源、Dashboard、告警、API 和 Agent 使用同一套服务端范围解析，空范围不扩大，关联不授予权限。
7. 正式门户复用 `@argus/ui`、design tokens、生成 API Client；支持中英、明暗和桌面视口。

## 执行原则

先完成身份/授权/凭据与配置契约，再贯通真实 Collector 数据链路；模板框架就绪后逐项接入两批组件。每项工作同时考虑存储、API、门户、工具、审计、测试和文档，不用前端目录或单条成功样本替代业务闭环。

项目开发阶段允许替换旧契约，不建设长期双轨兼容层；清理和数据保留按[发布方案](./10-rollout-and-status.md)明确对象与所有权。编制计划不授权删除既有业务数据。

现行不变量仍包括企业唯一隔离、PostgreSQL 权威状态、四个服务端程序、控制与遥测分离、资源显式授权、平台业务 Preview/Confirm。Plan8 的具体扩展见[决策清单](./01-scope-and-decisions.md)，不将新计划误写为已部署事实。
