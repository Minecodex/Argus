# Plan8 完成度复核：Helm 交付后

复核日期：2026-10-20。结论：**尚未全部实现，正式验收与统一发布也未完成。剩余工作包含实际功能缺口，不能仅补测试或修改状态后宣布完成。**

本次按 [16 项任务](./08-implementation-plan.md)、[验收矩阵](./09-acceptance-matrix.md)、当前源码、目录及原始证据复核。[50 审计](./50-completion-audit.md)保留终端/转发交付后的历史时点；其中“Helm 尚未实现”已由 [51 Helm 工作台](./51-helm-workbench.md)补齐，不能继续列作当前缺项。

## 复核基线与证据边界

- HEAD：`122609b41893d0f6e689fa2b5d453a2a301616b7`；审核对象包括当前未提交工作区，而非只看该提交。
- 本次更新文档前，重新枚举源码集合并计算 SHA256：[Helm 冻结记录](../../artifacts/plan8-helm-delivery-manifest.json)中的 **3780 个路径全部一致，无新增未记录路径**，包括生成物、配置及记录的 Collector 制品。结果见[当前对照记录](../../artifacts/plan8-audit-20261020-after-helm-source-comparison.json)。文档更新后的差异单独记录，不扩大既有测试的覆盖范围。
- [工程验证汇总](../../artifacts/plan8-helm-final-checks.json)保留 Go 87 个包、前端 1410 个单元测试、类型/lint/双语/契约检查，以及 16 个选定门户 mock 回归的通过记录；Helm 七组、21 个必需根测试及受限 Linux 渲染有分层证据。
- **本次未重新运行测试、未部署、未清理资源。** 指纹核对说明既有证据对应当前被测代码；组件、受控适配器及 mock 门户的通过仍不等于正式安装后的端到端通过。

## 已有实现与当前实际缺口

### 资源集成与默认内容尚未齐套

[专用集成目录](../../internal/monitoringcatalog/catalog.go)和[原生 Receiver 配置](../../internal/monitoringcatalog/native_receivers.go)已有 SQL Server、Oracle、MongoDB、Kafka、Elasticsearch、DNS、ICMP 等配置适配；不能把这些类型描述成完全没有代码。目录仍采用候选/未验证状态，版本、平台、认证和拓扑组合尚未冻结为支持矩阵。

当前 [默认模板目录](../../api/contracts/dashboard-templates.json)有 24 套模板，**尚无 SQL Server、Oracle、MongoDB、Kafka、Elasticsearch、DNS、ICMP、IIS 的对应默认模板**。这些服务不能仅因能够生成 Receiver 配置就标为完整支持；需补真实目标链路、默认内容、检测/诊断及支持组合的五类门禁。IIS 的 Windows 实机验证按用户决定暂缓。

MySQL 双实例、Redis、Nginx、PostgreSQL、RabbitMQ、HTTP/TCP/TLS、Memcached、Apache、HAProxy、ZooKeeper 已有实际组件和默认查询证据。它们仍需要正式安装、门户、权限、启停及故障恢复的完整产品链路验收。

### Namespace 指标与持续元数据仍未闭环

[来源范围 SQL](../../internal/storage/postgres/queries/observation_scopes.sql)已有 Namespace 日志来源分支，并约束应用及运维日志权限；[工作台读取](../../internal/kubernetesworkbench/read.go)会在可信读取时登记 Namespace UID、退休重建对象的旧身份。这些实现不能替代独立的持续元数据同步，以及指标数据按 Namespace UID 进入统一授权范围的完整链路。

当前 Namespace 日志路径已有验证；Namespace 指标的权威归属、UID 补全/同步和查询、仪表盘、告警、Agent 各入口的一致性仍需完成。不能以开放集群全部指标来抵扣 Namespace 指标支持。

### 采集责任与迁移需要完成整个生命周期

[采集实例动作](../../internal/monitoring/collection_actions.go)已有执行分配变更；[运行时](../../internal/telemetry/collection_runtime.go)支持迁移时配置两个 Collector；[控制面 SQL](../../internal/storage/postgres/queries/telemetry_control.sql)已有 Claim 迁移完成、回滚和过期操作。

因此缺口不是“没有迁移代码”，而是 Host/Node 共存时的执行责任与放置、正式接入后的责任迁移、重复采集控制及配置失败后完整回滚尚未形成已验收的业务闭环。旧 Profile 责任与多实例目标责任仍需全局核对，不能用一个成功配置应用代替整个迁移过程。

### K8s 主要操作已有实现，仍有局部功能和实际组合待补

通用变更、工作负载回滚、CronJob 手动执行、drain/PDB/取消、Pod 日志、终端、Pod/Service TCP 转发及 **Helm 原生生命周期均已有实现和分层组件证据**。

- [drain 取消领域接口](../../internal/kubernetesworkbench/drain_cancel.go)及门户已实现；独立的 Agent/model 取消工具仍未注册。
- [PVC 扩容校验](../../internal/kubernetesinventory/pvc_resize.go)已有禁止缩容、StorageClass 能力检查；已有实际拒绝场景不能替代真实 CSI 成功扩容和最终容量确认。缺少正向证据的能力不能宣称已完成验收。
- Namespace 写入期间 UID/授权变化的完整竞态组合、全部支持对象/Chart/拓扑，以及正式 Connector/Gateway/Ingress/门户/Agent 路径仍需完整验收。

### SDK、敏感输入与模型业务闭环仍有缺口

应用单环境、稳定服务身份、应用 Endpoint/Key、服务自动发现、应用七视图及真实 Go SDK 三信号链路已有实现。其余纳入范围的 SDK/协议组合、重复埋点与跨来源统计仍未完整闭环；SkyWalking 原生 Receiver 按用户决定暂缓，不能用 Trace 查询能力替代该协议接入。

[监控工具参数](../../internal/toolgateway/monitoring_domain_reads.go)排除了模型提交凭据，[敏感参数检查](../../internal/toolgateway/monitoring_domains.go)要求经可信门户输入。加密托管、用途隔离及 Helm 受保护输入已有实现，但尚不能据此认定 Agent 对话宿主的完整敏感表单交互已交付。真实模型取数、Workspace 文件及来源授权、仪表盘/告警的完整闭环仍未验收。

## 逐任务结论

以下“已有实现”不表示该任务全部验收标准已满足。16 项继续为 `in_progress`。

| 任务 | 已有实现或组件证据 | 尚未完成的主要范围 |
| --- | --- | --- |
| T01 契约/前件 | Migration 42、API/OpenAPI、sqlc、内部协议及生成客户端 | 支持版本矩阵、容量/可靠性和组合验证、最终版本冻结 |
| T02 应用/目标 | 单环境应用、稳定服务/目标、自动发现与资源关联 | 完整生命周期、依赖/归档/迁移和跨入口验收 |
| T03 范围/授权 | 显式范围、空集合、Namespace UID、运维日志权限 | Namespace 指标与持续同步、全入口一致性验收 |
| T04 凭据 | 表单直接托管、加密/租约/用途隔离、受保护临时输入 | Agent 宿主敏感表单、完整轮换/撤销/重启组合 |
| T05 采集运行时 | 多实例 Bundle、分离状态、分配和迁移基础机制 | Host/Node Claim、放置、正式接入及迁移/回滚闭环 |
| T06 可信数据 | 每 Resource 身份、来源范围、真实多目标及三信号组件链路 | 正式接入/PKI 轮换、部署 Query RPC 和 Gateway-Agent |
| T07 SDK/APM | Endpoint/Key、服务自动发现、Go 三信号、七视图 | 其余 SDK/协议、重复埋点/跨来源统计和正式部署 |
| T08 日志 | 文件/Journald/K8s/SDK/Windows 规则、Linux 持久恢复及日志会话 | 正式运行时与节点/Owner/重叠规则组合、Workspace；Windows 实测暂缓 |
| T09 自身指标 | 12 个固定定义、进程/systemd、受保护控制面真实采样与查询 | Host/Node 责任、Namespace 指标、正式安装及全部适用组合 |
| T10 K8s 读取 | 32 个内置类型、发现/分页/watch、UID/Owner 和 Secret 投影 | 持续同步、完整动态类型和部署 Connector 撤权/恢复 |
| T11 K8s 操作 | 通用写入、回滚、CronJob、drain、日志/终端/转发、原生 Helm | drain 取消模型工具、CSI 正向、完整 Namespace 竞态和部署组合 |
| T12 第一批集成 | MySQL/Redis/Nginx/PostgreSQL/RabbitMQ/HTTP/TCP/TLS 真实组件链路及查询 | 其余数据库/中间件、默认内容、支持组合及五类门禁 |
| T13 第二批集成 | Memcached/Apache/HAProxy/ZooKeeper 真实组件链路及查询 | DNS/ICMP、其余组合及默认内容；IIS 实测暂缓 |
| T14 仪表盘 | 24 套模板、多类型范围、关联/去重、版本和用户副本 | 剩余服务默认内容、Namespace 指标、完整版本/下钻/部署验证 |
| T15 门户/Agent/告警 | 共享 UI、双语/主题、23 个新领域工具和工作台门户 | 敏感宿主表单、缺失操作工具、真实模型/Workspace/后台身份/告警闭环 |
| T16 验收/发布 | 41 组具名入口、源码指纹、拒绝 Skip 抵扣、选定分组证据 | 当前 full、全部正式门禁、迁移恢复、制品冻结、统一发布和回退 |

## 正式验收与发布状态

| 对象 | 当前记录与解释 |
| --- | --- |
| 基础正式验收 | 76 项均 `not_run`；不代表组件从未测试 |
| 内置能力正式验收 | 12 项能力，60 个正式分项均 `not_run` |
| 候选集成 | 39 项中 27 项 `in_progress`，11 类资源及 SkyWalking 原生协议暂缓；支持矩阵 `not_locked` |
| 集成正式门禁 | 193 个 `not_run`；SkyWalking 既有 AUTH/LIFECYCLE 失败两项保留在暂缓协议下，不算通过 |
| 当前完整驱动 | 注册 41 组；找到的历史 full 只有 10 组，状态 `incomplete`，不覆盖当前完整驱动 |
| 真实模型/生产验收 | `not_run` |
| 正式部署/统一发布 | `not_started`；未冻结最终镜像、Collector、模板和回退清单 |

[验收驱动源码](../../internal/app/argusdev/e2e_plan8.go)仍显式列出八类缺失正式阶段：正式接入/PKI 轮换、部署 Connector/Gateway/Agent/Query、全部支持组合、全部内置能力、全部 K8s 操作/会话、真实模型/Workspace/告警、迁移备份恢复、冻结发布/部署回退。当前 full 即使组件组成功也返回 `incomplete`。这些阶段需要实现和执行，不能直接改变返回状态。

Windows 11 实机按用户决定暂缓；Windows Server 没有实机通过记录，不作为本期发布前件。ClickHouse、TiDB、OceanBase、RocketMQ、Pulsar、NATS、Envoy、JVM、Tomcat、Spring Boot、MinIO 独立监控及 SkyWalking 原生协议均按决定暂缓。**排除上述暂缓项后，计划仍有实际开发与正式验收缺口。**

## 剩余工作的收敛顺序

1. 补齐未暂缓服务的实际数据与默认内容，逐组合冻结版本、认证、平台及拓扑，明确能支持的范围。
2. 完成 Namespace 指标/持续元数据和 Host/Node 采集责任迁移，验证范围、重复采集、故障回滚及撤权。
3. 补齐剩余 K8s 操作入口、CSI 正向、SDK 统计和可信敏感宿主表单，以实际门户和 Agent 路径串联。
4. 实现缺失正式验收阶段并执行当前完整驱动，完成真实模型、Workspace、告警及迁移恢复。
5. 冻结全套源码/生成物/镜像/Collector/模板，全部纳入范围的门禁通过后统一发布，核验真实样本、保护数据、清理及回退记录。

本次只更新复核文档和机读审计信息，未改变任务、正式门禁、Windows 暂缓或发布状态。
