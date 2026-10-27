# Plan8 完成度复核：终端与端口转发交付后

核对日期：2026-10-20。**尚未全部实现。仍有实际开发缺口，正式全链路验收和统一发布也未完成。**

本次对照 [16 项任务](./08-implementation-plan.md)、[验收矩阵](./09-acceptance-matrix.md)、[49 会话交付](./49-kubernetes-stream-sessions.md)、当前未提交代码及原始测试记录复核。只做源码、契约、目录和已有证据检查，没有重新执行测试、安装、部署或资源清理。[48 复核](./48-completion-audit.md)保留日志会话交付后的历史时点。

## 本次确认的变化

- **终端与端口转发不再属于尚未开发。** 当前已有独立会话表、领域服务、四个 HTTP 接口、WebSocket、Direct/Connector/Peer 适配、门户与本机 CLI；固定 Namespace/Pod UID、容器代次及 Service 端点，具备独立授权、一次性票据、原子执行认领和元数据审计。
- 核对了[七组冻结记录](../../artifacts/plan8-streams-delivery-20261019-3681301923/manifest.json)：十五个必需具名根测试全部通过，运行期间源码未变；记录明确 `plan_complete=false`、`release_eligible=false`。
- 逐项核验该记录的 3727 个文件。复核开始时仅 `49-kubernetes-stream-sessions.md` 和 `progress.json` 两份文档不同，其余代码、配置与 Collector 产物的 SHA-256 均一致，没有新增代码。见[本次指纹核验](../../artifacts/plan8-audit-20261020-source-comparison.json)。本次更新审计文档后，文档差异单列，产品代码不变。
- [上一轮工程汇总](../../artifacts/plan8-stream-final-checks.json)及原始记录显示：Go 全包、前端 1409 项、类型/lint/样式/i18n、契约检查通过；选定的门户回归 24 场景通过，最后四个终端/转发场景另有复验。此处是复核既有结果，没有重新跑这些测试。
- 原生会话使用真实 Kubernetes、独立 Namespace 和受限 ServiceAccount；HTTP/WebSocket/TCP/mTLS 测试使用受控目标适配器，浏览器使用明确的 mock 适配器。这些证据仍不覆盖正式注册、实际部署中的 Connector/Gateway/Ingress/门户完整物理链路。

## 仍需开发或补齐的范围

| 范围 | 本次核对结论 |
| --- | --- |
| K8s 工作台 | 尚无工作台 Helm 安装、升级、回滚、卸载及渲染后逐对象授权流程；drain 取消已有 API/门户，原生工具目录仍无独立取消入口。CSI 正向扩容、Namespace 写入竞态及全部操作组合仍需收口。 |
| 采集责任与迁移 | Bundle 已表达多实例、revision 和 assignment epoch，已有 Host/Node 绑定与 Claim 基础；新实例责任、执行分配、迁移、受控双采和完整失败回滚尚未完成全部联动与验收，不能因存在旧 Profile Claim 代码而认定已完成。 |
| Namespace 指标与元数据 | 已有 UID 授权、日志来源及查询范围；指标对象的权威身份、持续元数据同步及所有查询/仪表盘/告警入口仍需补齐，不能用 Namespace 资源目录替代指标数据范围。 |
| 专用集成和默认内容 | SQL Server、Oracle、MongoDB、Kafka、Elasticsearch、DNS、ICMP 等有配置适配基础，但未完成实际目标、认证/版本/拓扑组合、默认内容和全产品流程。当前 24 套受管模板没有这些集成的专用默认模板；IIS 也无专用默认模板，Windows 实测继续暂缓。 |
| SDK/APM | Go OTLP 三信号等已有组件证据；其余 SDK/协议组合、重复埋点和跨来源统计、正式接入地址/颁证轮换及部署后完整 APM 仍未收口。 |
| Agent、凭据与告警 | 现有工具明确拒绝模型提交凭据，使用门户输入；Agent 的可信宿主敏感表单桥接仍缺。完整 Workspace/PVC/来源授权、后台主体、真实模型和告警业务流程未完成。 |
| 验收与发布 | 当前完整入口注册 35 组，但没有当前 35 组同次冻结通过记录；正式部署/模型/支持矩阵/迁移恢复/发布阶段仍未实现完整驱动。源码、镜像、Collector、模板的最终发布清单和发布回退记录尚未形成。 |

关键源码接点已核对：

- [工作台工具目录](../../internal/toolgateway/monitoring_workbench_tools.go)、[工作台 API](../../api/openapi/paths/kubernetes-workbench.yaml)与[会话 API](../../api/openapi/paths/kubernetes-stream-sessions.yaml)：对象查询/变更和新会话存在，Helm 工作台入口缺失。平台安装 CLI 使用 Helm SDK 不等于工作台已具备 Helm 生命周期。
- [范围解析](../../internal/observationscope/sources.go)、[资源目录](../../internal/observationscope/resolver.go)与[物理查询范围测试](../../internal/telemetry/namespace_query_scope_test.go)：Namespace 授权、日志来源与物理查询分离已有实现，仍需补齐 Namespace 指标权威身份和持续同步。
- [多实例契约](../../internal/otelcol/configbundle/collections.go)和 [Claim 重建](../../internal/telemetry/action_extension.go)：多目标与责任基础均存在，不能等同完整迁移/双采/回滚业务闭环。
- [集成目录](../../internal/monitoringcatalog/catalog.go)、[原生配置适配](../../internal/monitoringcatalog/native_receivers.go)及[模板发行目录](../../api/contracts/dashboard-templates.json)：候选、适配器和正式产品支持是不同状态；未提供的默认模板不能以通用空看板替代。
- [原生领域工具](../../internal/toolgateway/monitoring_domain_reads.go)和[敏感参数拒绝](../../internal/toolgateway/monitoring_domains.go)：当前允许公共参数预览，凭据仍要求可信门户输入，完整宿主敏感表单流程待实现。

## 按任务复核

| 任务 | 当前已有内容 | 剩余开发或正式验证 |
| --- | --- | --- |
| T01 契约与前件 | 迁移至 41，OpenAPI/Schema/sqlc/生成客户端、协议与目录 | 准入版本/平台/拓扑组合、容量可靠性和最终制品冻结 |
| T02 应用与目标 | 单环境应用、稳定服务、目标关联、SDK 并发发现 | 生命周期/依赖/归档组合、迁移和跨入口完整验收 |
| T03 范围与授权 | 显式空范围、应用/目标/Namespace UID 授权、运维日志、新会话撤权 | Namespace 指标权威身份、持续元数据与全部查询/后台主体入口 |
| T04 监控凭据 | 直接输入、用途隔离、加密暂存、租约与接收方校验 | Agent 敏感宿主表单、全部轮换/撤销/重启组合 |
| T05 采集运行时 | 多实例 Bundle、状态拆分、独立启停、部分恢复 | Host/Node 责任、分配、迁移/双采、完整回滚和正式安装 |
| T06 可信遥测 | Resource 来源/版本校验、原生组件到存储的分层证据 | 正式注册/轮换、部署 Query RPC/Gateway/Agent 和完整迁移 |
| T07 SDK/APM | 应用接入/Key/发现、Go OTLP 三信号、应用视图 | 其他 SDK/协议、重复埋点/跨来源统计和正式部署 |
| T08 日志 | 文件/Journald/K8s/SDK/Windows 规则及 Linux 恢复；交互日志会话 | 正式运行时、跨节点/Owner/重叠规则与 Workspace；Windows 实测暂缓 |
| T09 自身指标 | 12 个固定定义、Linux 进程/systemd、控制面采样与查询 | Claim/Namespace 指标、正式安装和全部适用组合 |
| T10 K8s 读取 | 32 个内置类型、发现/分页/watch、UID/Owner、Secret 投影 | 动态类型、持续同步、部署 Connector 及完整撤权/恢复 |
| T11 K8s 操作 | 通用变更、回滚、CronJob、drain/PDB/取消、日志、终端和 TCP 转发 | Helm、drain 取消模型工具、CSI 正向/竞态及完整实际部署入口 |
| T12 第一批集成 | MySQL/Redis/Nginx/PostgreSQL/RabbitMQ/HTTP/TCP/TLS 的实际组件及查询证据 | 其余目标、版本/平台/认证/拓扑、默认内容与五类门禁 |
| T13 第二批集成 | Memcached/Apache/HAProxy/ZooKeeper 实际组件及查询 | DNS/ICMP 等其余组合与默认内容；IIS/Windows 实测暂缓 |
| T14 仪表盘 | 24 套模板、范围/关联/去重、版本升级和用户副本 | 剩余集成内容、Namespace 指标范围、完整下钻/版本和部署验证 |
| T15 门户/Agent/告警 | 共享 UI、双语主题、20 个新领域工具、日志及会话门户 | 敏感宿主表单、缺失操作工具、真实模型/Workspace/PVC/告警闭环 |
| T16 验收/发布 | 35 组具名入口、指纹与 skip 拒绝、分组证据和所有权清理 | 当前 full、全部正式门禁、迁移恢复、制品、统一部署和回退 |

## 正式状态与暂缓项

| 对象 | 当前记录 |
| --- | --- |
| 16 项任务 | 均为 `in_progress`；已有实现不等于各项全部完成标准已经满足 |
| 基础正式验收 | 76 项均 `not_run` |
| 内置能力正式分项 | 12 项能力、60 个分项均 `not_run` |
| 候选集成 | 39 项中 27 项 `in_progress`；11 类资源和 SkyWalking 原生协议按决定暂缓；支持组合矩阵 `not_locked` |
| 集成门禁记录 | 193 个 `not_run`；SkyWalking 的 AUTH/LIFECYCLE 两个既有失败保留，归属于已确认暂缓的协议 |
| 完整检查 | 当前注册 35 组；仅找到历史 full 的 10 组通过记录，结果为 `incomplete`，不能覆盖当前完整入口 |
| 模型/正式部署/发布 | `not_run` / `not_started` / `not_started` |

[验收驱动](../../internal/app/argusdev/e2e_plan8.go)在 full 的组件组通过后仍显式返回 `incomplete`：正式部署、模型、支持组合和发布阶段尚未实现。不能通过改返回值或把组件组标成正式门禁来宣布完成。

Windows 实测继续按用户决定暂缓；Windows Server 实机没有通过记录，也不作为本期要求。11 类资源暂缓和 SkyWalking 原生协议暂缓不计支持。**排除这些暂缓项后，其他功能仍有实际开发与验收缺口。** 本次没有修改任何正式验收或发布状态。
