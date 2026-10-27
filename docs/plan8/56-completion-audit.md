# Plan8 完成度复核：Helm 仓库与 OCI 交付后

2026-10-20。结论：**尚未全部实现，正式验收和统一发布也未完成。剩余事项既有实际开发缺口，也有已实现但尚未完成正式验收的内容。**

本次按 [16 项任务](./08-implementation-plan.md)、[验收矩阵](./09-acceptance-matrix.md)、当前工作区源码、契约、目录及原始测试记录重新核对。[54 审计](./54-completion-audit.md)保留历史时点，其中 Helm 仓库/OCI 的实现缺口已经由 [55](./55-helm-repository-and-oci.md)补齐，不再列作待开发。

## 复核方法与证据范围

- HEAD：`122609b41893d0f6e689fa2b5d453a2a301616b7`。本次包含未提交工作区，不能只用提交号代表被测版本。
- 文档更新前，重新枚举源码、生成物、配置及三平台 Collector 二进制；[最近冻结记录](../../artifacts/plan8-helm-sources-delivery-manifest.json)的 **3805 条文件记录全部一致，文件集合无增减**。指纹为 `68a21bd2b3e5ff68ae613bd8eea8d8e753f3fc7dcfafa362687eb938b76ec210`，见[本次比对](../../artifacts/plan8-audit-20261020-after-helm-sources-source-comparison.json)。
- [最近工程验证](../../artifacts/plan8-helm-sources-final-checks.json)记录 Go 87 包、前端 1412 测试、8 个 mock 门户场景通过，类型/样式/i18n/生成契约检查通过。本次另外核对原始日志：Helm 10 组的 28 个必需根测试均有实际通过事件。
- 当前实现目录为 **27 套默认模板、23 个新增领域工具、12 个固定自身指标定义、48 组具名检查**。Collector 为 `0.159.0`，发行标识 `0.3.0-plan8`。
- **本次没有重跑测试、部署、安装或清理，只进行源码/证据核对及文档状态更新。** 代码与冻结记录一致可确认已有证据的版本适用性，不能扩大已有测试的场景覆盖。

## 已补齐、不能重复列作缺失的内容

Helm 上传、仓库和 OCI 三种来源现已共用原生预览/审批/执行服务。新增精确版本、Manifest/包摘要核验、私有认证/自定义 CA、网络目标校验、来源凭据加密暂存和预览后移除。执行使用冻结材料，不再访问来源；协议为 `argus.kubernetes_helm/v2`。对应 [API](../../api/openapi/components/kubernetes-helm.yaml)、[来源契约](../../internal/kuberneteshelm/source_types.go)、[领域预览](../../internal/kubernetesworkbench/helm_preview.go)和[私有输入](../../internal/secret/kubernetes_helm_input.go)均有实现。

真实 TLS 仓库及官方 OCI Registry、来源离线后的实际 K8s 写入、数据库审批/Worker/逐写授权已有组件证据。实际接入登记和执行路由使用受控前件；门户八场景为 mock。**这些结果没有覆盖已部署 Connector/Gateway/Ingress/门户的完整 A72，也不代表所有 Chart/拓扑组合通过。**

此前补齐的 DNS、ICMP、Kafka 限定组合与默认查询，以及 Pod 日志会话、终端/转发、drain/PDB/取消、工作负载回滚和 CronJob 手动运行也保留已有实现结论。版本、认证、平台和拓扑范围仍以各自证据为准。

## 仍需开发的内容

| 范围 | 本次确认 | 对应任务 |
| --- | --- | --- |
| 服务默认内容 | SQL Server、Oracle、MongoDB、Elasticsearch、IIS 已有原生配置适配器，但默认模板目录尚无这五类模板。前四类实际目标链路、配套内容和支持组合未齐套；Windows 测试暂缓不等于 IIS 默认内容可省略。 | T12、T13、T14 |
| Namespace 指标与持续元数据 | Namespace UID 授权及读取时登记已实现；来源范围 SQL 的 Namespace 特殊关联仍限定日志。持续元数据同步、Namespace 指标的权威归属和各查询入口一致性仍未完成。 | T03、T09、T10、T14 |
| 采集责任与迁移 | 多实例、执行分配、双 Collector 配置及迁移完成/回滚/过期已有基础。Claim 重建仍以 Profile 展开，需与新固定实例/多实例、Host/Node 物理身份和放置统一，闭合重复采集控制及迁移失败恢复。 | T05、T06、T09 |
| 对话敏感输入与部分操作入口 | 门户凭据直填、加密托管、短期输入及用途隔离已实现；模型工具仍要求敏感内容通过可信门户输入。对话宿主内的敏感表单尚未交付；drain 取消领域服务和门户已有，独立模型工具尚未注册。 | T04、T11、T15 |
| SDK/APM 组合与统计 | Endpoint/Key、单环境、服务自动发现、应用七视图和真实 Go SDK 三信号已有实现；其余纳入范围的协议/SDK，以及重复埋点、跨来源统计规则仍需补齐和验证。 | T07、T15 |

定位依据：

- [原生适配器](../../internal/monitoringcatalog/native_receivers.go)与[默认模板目录](../../api/contracts/dashboard-templates.json)：区分“配置路径已有”和“默认内容/支持组合齐套”。
- [来源范围 SQL](../../internal/storage/postgres/queries/observation_scopes.sql)的 `ListObservationSources`、[Namespace 读取登记](../../internal/kubernetesworkbench/read.go)的 `observeNamespaces`：已有 UID/日志授权不能抵扣 Namespace 指标链路。
- [Claim 重建](../../internal/telemetry/action_extension.go)的 `rebuildClaims`、[多实例运行时](../../internal/telemetry/collection_runtime.go)、[迁移 SQL](../../internal/storage/postgres/queries/telemetry_control.sql)：不是完全没有迁移实现，缺的是新责任模型的完整衔接。
- [模型监控工具](../../internal/toolgateway/monitoring_domain_reads.go)、[Helm 工具](../../internal/toolgateway/kubernetes_helm_tools.go)、[drain 取消服务](../../internal/kubernetesworkbench/drain_cancel.go)：领域/门户能力和模型入口分别核对，敏感输入不得转由模型接收。

## 已有实现但验收尚未闭合

- **K8s 全部操作和会话**：PVC 已有能力/缩容拒绝检查及真实负向场景；缺真实 CSI 成功扩容和最终容量确认。Namespace 写竞态、全部资源类型/Chart/拓扑、撤权和故障恢复仍需完整验证。Helm 仓库/OCI 不再是开发缺项，但正式部署链路仍未验收。
- **正式采集与接入**：多目标及 SDK/日志组件流水线已有真实数据；正式 Host/Collector 注册、PKI 轮换、配置收敛、部署 Query RPC 和 Gateway-Agent 全流程仍未闭合。不能用受控登记/CA/会话夹具替代。
- **支持矩阵**：版本、平台、认证和拓扑尚未冻结；候选目录、同名 Receiver 或单个正向样本不能证明所有组合支持。每个准入组合仍需 CONFIG/DATA/VIEW/AUTH/LIFECYCLE 五类正式门禁。
- **产品业务流程**：真实模型操作、Workspace 来源与撤权、仪表盘下钻/模板更新、告警评估和后台身份组合仍需正式验收。告警表达式建议不代表已部署且通过验收的告警策略。
- **发布和恢复**：迁移/备份恢复、容量/可靠性、最终源码及镜像/Collector/模板冻结、目标部署、清理/业务数据保护和回退尚未完成。当前组件清理记录不能替代全量发布清理结论。

## 16 项任务逐项结论

所有任务保持 `in_progress`。已有实现、组件验证、正式验收和发布分别记录，不把任务状态理解为“完全没有开发”。

| 任务 | 已有实现/证据 | 剩余主要范围 |
| --- | --- | --- |
| T01 契约/前件 | Migration 42、API/生成物、Helm v2、三平台 Collector 0.159 | 支持矩阵、容量/可靠性、最终全套版本冻结 |
| T02 应用/目标 | 单环境应用、稳定服务/目标、发现及关联 | 完整依赖/归档/迁移和跨入口生命周期验收 |
| T03 范围/授权 | 显式范围/空集合、应用/目标/UID、运维日志授权 | Namespace 指标/持续元数据和全入口一致性 |
| T04 凭据 | 直接填写、加密托管/租约、短期输入、Helm 源认证移除 | 对话宿主敏感表单、完整轮换/撤销/重启组合 |
| T05 运行时 | 多实例 Bundle、启停/状态、分配与迁移基础 | Host/Node Claim/放置、重复采集及完整迁移回滚 |
| T06 数据链路 | 每 Resource 来源身份、多目标/三信号实际流水线 | 正式接入/PKI、部署 Query RPC 和 Gateway-Agent |
| T07 SDK/APM | Endpoint/Key、服务发现、Go SDK 三信号和应用视图 | 其他协议/SDK、重复统计、正式部署/模型流程 |
| T08 日志 | 文件/Journald/K8s/SDK 规则、持久恢复、日志会话 | 正式安装、节点/Owner/规则组合与 Workspace；Windows 实测暂缓 |
| T09 自身指标 | 12 个固定定义、主机/进程/systemd、核心/控制面样本 | 责任和 Namespace 指标、正式安装及适用组合 |
| T10 K8s 读取 | 32 类内置对象、分页/watch/UID/Owner/Secret 投影 | 持续同步、完整动态类型、部署 Connector/撤权恢复 |
| T11 K8s 操作 | 通用变更/回滚/CronJob/drain、日志/终端/转发、三种来源 Helm 生命周期 | drain 取消模型工具、CSI 正向、写竞态/全部部署组合 |
| T12 第一批集成 | 多类数据库/缓存/Web/消息及网络目标实际样本和查询 | SQL Server/Oracle/MongoDB/Elasticsearch 内容、支持组合及正式五类门禁 |
| T13 第二批集成 | 缓存/Web/ZooKeeper、DNS/ICMP 限定组合和默认查询 | IIS 默认内容、其他平台/协议/版本和正式门禁；Windows 实测暂缓 |
| T14 仪表盘 | 27 套模板、范围/版本/用户副本及真实查询 | 其他服务默认内容、Namespace 指标、版本/下钻/部署验收 |
| T15 门户/Agent/告警 | 共享 UI、双语主题、23 个新增工具、授权来源与选择器 | 对话敏感输入/操作入口、模型/Workspace/告警/后台身份闭环 |
| T16 验收/发布 | 48 组具名入口、文件指纹、拒绝 Skip 抵扣及组件清理证据 | 当前 full、正式门禁、迁移恢复、最终制品及统一部署/回退 |

## 正式状态与暂缓范围

| 项目 | 当前状态 |
| --- | --- |
| 基础正式门禁 | 76 项 `not_run` |
| 内置能力正式门禁 | 12 类、60 个分项 `not_run` |
| 集成候选 | 39 项中 27 项进行中、11 类资源暂缓、1 个 SkyWalking 原生协议暂缓；支持矩阵未冻结 |
| 集成正式门禁 | 193 个 `not_run`；SkyWalking 的两个历史失败保留，不计通过 |
| 当前完整检查入口 | 48 组；本轮最近 Helm 运行覆盖其中 10 组。只找到旧的 10 组 full，结果为 `incomplete`，不能覆盖当前 full |
| 正式模型/生产验收 | `not_run` |
| 正式部署/统一发布 | `not_started`；未形成完整发布制品清单 |

[验收驱动](../../internal/app/argusdev/e2e_plan8.go)明确保留八类缺失正式阶段；组件组全部成功后 full 仍返回 `incomplete`。需要交付并验证这些阶段，不能改返回值或仅补勾选关闭计划。正式门禁尚未运行不代表没有测试，已有组件证据见本报告开头及各交付记录。

Windows 实机继续按用户决定暂缓；实现及构建保留，Windows Server 实机未验证。11 类资源和 SkyWalking 原生协议按已确认范围暂缓，不能作为通过项，也不重新要求用户确认。**排除这些已暂缓内容后，仍有上表中的实现和验收缺口，所以 Plan8 不能标记完成。**

本次只更新当前复核结论和导航，保留历史审计及全部正式任务/门禁/暂缓/发布状态。文档、链接和状态校验见[本次校验记录](../../artifacts/plan8-audit-20261020-after-helm-sources-documentation-validation.json)。
