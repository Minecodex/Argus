# Plan8 完成度复核：DNS、ICMP、Kafka 交付后

2026-10-20。结论：**尚未全部实现，正式验收及统一发布也未完成。剩余事项包含实际开发缺口，不能只补测试或调整状态后收尾。**

本次重新对照 [16 项任务](./08-implementation-plan.md)、[76 项基础门禁](./09-acceptance-matrix.md)、当前源码/契约、目录和原始证据。与 [52 审计](./52-completion-audit.md)相比，[53](./53-native-network-and-kafka.md)已补齐 DNS、ICMP、Kafka 的限定组合与默认内容，不能再将它们完整列作“未实现”。本次另确认 Helm 仓库及 OCI Chart 接入仍缺失。

## 复核方法与已有证据

- HEAD：`122609b41893d0f6e689fa2b5d453a2a301616b7`；复核包括未提交工作区，不能仅用提交号代替实际源码版本。
- 文档更新前，重新枚举文件集合并核验 SHA256：[最新冻结记录](../../artifacts/plan8-network-delivery-manifest.json)的 **3790 条记录全部一致，文件集合无增减**，包括记录的生成物、配置和三平台 Collector 二进制。见[本次对照](../../artifacts/plan8-audit-20261020-after-network-source-comparison.json)。
- 当前 Collector 为 `0.159.0`，发行标识 `0.3.0-plan8`。当前目录有 **27 套默认模板、23 个新增领域工具、12 个固定自身指标定义、45 组具名检查**。
- [上一轮验证汇总](../../artifacts/plan8-network-kafka-final-checks.json)记录 Go 87 包、前端 1411 测试、17 个数据库领域根测试、8 个 mock 门户场景，以及原有 110 个目标面板查询通过。DNS/ICMP 九个正向面板、Kafka 八个面板实际取数；完全丢包时四个 RTT 面板即时结果为空。
- 升级后的主机/进程/systemd、文件/Journald、K8s API/Kubelet/CoreDNS、原有数据库/缓存/Web、真实 SDK 三信号/Key 轮换及日志恢复均有组件回归证据。实际 SDK 管道仍使用受控接入/CA/登记前件；mock 门户不等于正式部署门户。
- **本次只核对源码和既有证据，没有重新运行测试、部署或清理。** 既有组件证据可与当前被测代码对应，不扩大到其未覆盖的版本、协议、平台、拓扑或正式流程。

## 尚未齐套的功能

### 服务默认内容和支持组合

[原生配置适配器](../../internal/monitoringcatalog/native_receivers.go)中已有 SQL Server、Oracle、MongoDB、Elasticsearch、IIS 的配置路径；不将它们描述为完全没有实现。但 [默认模板目录](../../api/contracts/dashboard-templates.json)尚无这五类对应模板。前四类的实际目标链路、配套内容和支持组合尚未完整交付；IIS/Windows 实机按用户决定暂缓。

Kafka、DNS、ICMP 已有实际限定组合：Kafka 4.2.1 单节点 KRaft/SASL-PLAIN 元数据；DNS UDP/TCP/A；Linux 非 root ping socket ICMP。其他版本、认证、拓扑、DNS 记录/传输及平台不能由这些结果推定。候选目录仍不等于已冻结的支持矩阵，各支持组合的正式 CONFIG/DATA/VIEW/AUTH/LIFECYCLE 门禁未关闭。

### Namespace 指标与持续元数据

[来源范围 SQL](../../internal/storage/postgres/queries/observation_scopes.sql)的 Namespace 特殊来源关联目前限定日志。[可信对象读取](../../internal/kubernetesworkbench/read.go)会登记 Namespace UID、处理同名重建；这已经提供了部分身份基础，但还不能代替完整持续同步和指标的 Namespace 权威归属。

现有 UID 授权、日志与运维日志范围保留；Namespace 指标及查询/仪表盘/告警/Agent 各入口的一致链路仍需补齐和验收，不能以放开整个 Cluster 指标来抵扣。

### 采集责任、放置与迁移

[执行分配](../../internal/monitoring/collection_actions.go)、[双 Collector 配置运行时](../../internal/telemetry/collection_runtime.go)及 [Claim 完成/回滚/过期 SQL](../../internal/storage/postgres/queries/telemetry_control.sql)已有实现。缺口不是“没有任何迁移代码”。

[Claim 重建](../../internal/telemetry/action_extension.go)仍沿 Profile ID/信号展开。新固定实例/多实例采集与 Host/Node 物理责任、执行放置、迁移期间重复采集控制和完整失败回滚之间，仍需统一并证明完整业务流程，不能用单次配置应用成功代替。

### K8s：Helm 仓库/OCI 仍未实现，其他操作还有缺项

[原始 K8s 范围](./05-host-and-kubernetes.md)和 A72 明确包含仓库及 OCI Chart 接入。当前 [Helm API](../../api/openapi/components/kubernetes-helm.yaml)仅接收受保护的 `chart_archive` 和 values，[原生材料模型](../../internal/kuberneteshelm/types.go)也是 Chart 字节输入；[51](./51-helm-workbench.md)明确未开放远程来源。

因此，**上传打包 Chart 后的安装/升级/回滚/卸载/历史及逐对象授权已有实现和组件证据；Repo/OCI 的受控获取、来源/版本/摘要固定及对应认证入口尚未交付。** 本轮上传入口的限制不等于原计划已经排除仓库/OCI。此前仅写“Helm 已补齐”不足以代表 A72 全部完成，本次纠正该遗漏。

此外，[drain 取消领域服务](../../internal/kubernetesworkbench/drain_cancel.go)及门户已实现，但独立 Agent/model 取消工具未注册。PVC 已有 [扩容前件检查](../../internal/kubernetesinventory/pvc_resize.go)及真实拒绝场景，**真实 CSI 成功扩容及最终容量确认仍缺正向验证**；不能将它写成完全没有扩容校验。Namespace 写竞态、全部对象/Chart/拓扑以及正式 Connector/Gateway/Ingress/门户/Agent 路径还需完整验收。

### SDK、可信输入和模型业务流程

应用 Endpoint/Key、自动发现服务、单环境身份、七视图、Go SDK 三信号及撤销/轮换已有实现。其余纳入范围的 SDK/协议、重复埋点/跨来源统计尚未完整收口；不把 SkyWalking 查询引擎当成已支持其暂缓的原生接入协议。

[原生监控工具](../../internal/toolgateway/monitoring_domain_reads.go)不接收凭据，明确要求可信门户表单。加密托管与用途隔离已经存在，但 Agent 对话宿主内的完整敏感输入流程尚未交付。真实模型创建/修改监控、Workspace 取数及来源授权、仪表盘/告警联动的正式业务验收仍未完成。53 的告警表达式是建议，不是已经部署且验收的生产策略。

## 16 项任务逐项结论

所有任务继续为 `in_progress`；这不代表每项都没有实现，而是没有一项可以仅凭当前证据满足其全部完成标准。

| 任务 | 已有实现/证据 | 剩余主要范围 |
| --- | --- | --- |
| T01 契约/前件 | Migration 42、API/内部协议/生成物，三平台 Collector 0.159 重建 | 支持组合矩阵、容量/可靠性、最终全套版本冻结 |
| T02 应用/目标 | 单环境应用、稳定服务/目标、自动发现及关联 | 完整依赖/归档/迁移和跨入口生命周期验收 |
| T03 范围/授权 | 显式范围、空集合、UID、应用/目标和运维日志授权 | Namespace 指标/持续元数据及全入口一致性 |
| T04 凭据 | 自动加密托管、用途/租约隔离、受保护暂存 | 对话宿主敏感表单，完整轮换/撤销/重启组合 |
| T05 运行时 | 多实例 Bundle、独立启停/状态、分配和迁移基础 | Host/Node Claim/放置、责任迁移、重复采集和完整回滚 |
| T06 数据链路 | 每 Resource 来源身份、实际多目标/三信号与恢复 | 正式接入/PKI、部署 Query RPC 和 Gateway-Agent 链路 |
| T07 SDK/APM | Endpoint/Key、自动发现、Go SDK 三信号及应用视图 | 其余协议/SDK、重复埋点统计和正式部署/模型流程 |
| T08 日志 | 文件/Journald/K8s/SDK 规则、Linux 持久恢复、日志会话 | 正式安装运行时、节点/Owner/规则组合与 Workspace；Windows 暂缓 |
| T09 自身指标 | 12 个固定定义、主机/进程/systemd、核心与控制面样本 | 责任/Namespace 指标、正式安装、适用组合及版本回归覆盖 |
| T10 K8s 读取 | 32 个内置对象类型、分页/watch/UID/Owner/Secret 投影 | 持续同步、完整动态类型、部署 Connector 和完整撤权/恢复 |
| T11 K8s 操作 | 通用变更/回滚/CronJob/drain、日志/终端/转发、上传 Chart 生命周期 | **Repo/OCI**、drain 取消模型工具、CSI 正向、竞态和全部部署组合 |
| T12 第一批集成 | MySQL/Redis/Nginx/PostgreSQL/RabbitMQ/Kafka/HTTP/TCP/TLS 组件数据及查询 | SQL Server/Oracle/MongoDB/Elasticsearch 内容，其他组合及正式五类门禁 |
| T13 第二批集成 | 缓存/Web/ZooKeeper、DNS/ICMP 限定组合及查询 | 其他平台/协议/版本，IIS 实测暂缓，正式门禁未完成 |
| T14 仪表盘 | 27 套模板、范围/版本/用户副本、真实查询及全丢包空 RTT | 其他服务默认内容、Namespace 指标、完整版本/下钻/部署验收 |
| T15 门户/Agent/告警 | 共享 UI、双语主题、23 个新增工具、兼容 Collector 弹框 | 宿主敏感输入、缺失操作入口、真实模型/Workspace/告警/后台身份组合 |
| T16 验收/发布 | 45 组具名入口、分组指纹、拒绝 Skip 抵扣和组件清理 | 当前 full、正式门禁、迁移备份恢复、最终制品、统一部署/回退 |

## 验收与发布：仍未收尾

| 项目 | 当前状态 |
| --- | --- |
| 基础正式门禁 | 76 项 `not_run`，不是说已有组件从未测试 |
| 内置能力 | 12 类能力的 60 个正式分项 `not_run` |
| 集成 | 39 候选中 27 项进行中，11 类资源及 SkyWalking 原生协议暂缓；支持矩阵未冻结 |
| 集成正式门禁 | 193 个 `not_run`；SkyWalking 两个既有失败保留在已确认暂缓项下，不计通过 |
| 完整检查 | 当前 45 组；找到的历史 full 只有 10 组且为 `incomplete`，不能覆盖当前入口 |
| 正式模型/生产验收 | `not_run` |
| 正式部署/统一发布 | `not_started`；当前组件构建不能替代最终镜像/Collector/模板全套发布冻结与回退 |

[驱动源码](../../internal/app/argusdev/e2e_plan8.go)仍列出八类缺失正式阶段，并在 full 的组件组成功后返回 `incomplete`：正式接入/PKI、部署各链路、全部集成组合、全部内置能力、全部 K8s 操作/会话、真实模型/Workspace/告警、迁移备份恢复、冻结发布/部署回退。需要补齐这些阶段和执行证据，不能改返回值抵扣。

Windows 实测继续按用户决定暂缓，Server 实机没有通过记录。11 类资源及 SkyWalking 原生协议按确认范围暂缓；**即使排除这些项，仍有以上实际功能及验收缺口。** 本次不改任何任务、正式门禁、暂缓或发布状态。
