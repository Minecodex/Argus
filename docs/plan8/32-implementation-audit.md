# Plan8 实现完成度核对

核对日期：2026-10-19。结论：**尚未全部实现，也未完成全量验收或统一发布。剩余工作包含实际功能开发。**

本次按 [16 项任务](./08-implementation-plan.md)、[验收矩阵](./09-acceptance-matrix.md)、当前工作区代码和已有测试日志交叉核对。没有重新运行全量测试、执行部署或启动 Windows 管理员准备；下文测试结果是对已有证据的复核。历史实施记录中的“仍待完成”按日期保留，当前判断以本表及对应最新证据为准。

## 状态与统计口径

| 项目 | 核对结果 |
| --- | --- |
| 任务完成状态 | T01～T15 为 in_progress，T16 为 not_started；没有任务达到全项完成标准，不能据此理解为没有代码落地 |
| 正式基础验收 | 76 项均未登记整体通过；已有组件/后端/局部门户验证不能直接替代完整门禁 |
| 内置能力验收 | 12 个能力的 60 个分项尚未形成完整通过记录 |
| 候选集成 | 共 39 项，11 个资源监控及 SkyWalking 原生协议已决定暂缓；其余 27 项尚未冻结支持组合 |
| 服务/探测配置 | 已有 20 类专用 Receiver 的配置适配；配置可生成或通过原生校验不等于相应资源已完整支持 |
| 固定指标定义 | 当前 6 个：Host、Collector、集群/工作负载状态、Node/Container、API Server、CoreDNS |
| 受管默认仪表盘 | 当前 9 套：MySQL、Redis、Nginx、Host、Collector、K8s Cluster、Nodes、API Server、CoreDNS；应用 APM 查询视图另计 |
| 发布 | 尚未冻结统一发布清单、镜像与完整模板版本，未发布 |

## 逐任务核对

“已实现部分”只描述核对到的代码和局部证据，不表示本行任务已验收完成。

| 任务 | 已实现部分 | 未完成或待验证范围 |
| --- | --- | --- |
| T01 契约与前件 | 领域迁移、OpenAPI/Schema/sqlc/客户端和内部协议持续更新；当前生成契约检查通过 | 全部候选版本/平台/拓扑及运行依赖尚未冻结；完整容量、可靠性与版本组合门禁未闭合 |
| T02 应用与监控目标 | 单环境应用、稳定服务身份、运行实例、监控目标与关系；SDK 并发登记和同名不同 Namespace 分离已有真实数据库/数据验证 | 全部生命周期、依赖/归档、迁移及跨入口业务流程仍需按任务标准验收 |
| T03 范围与授权 | 统一 ScopeResolver、显式空范围、应用/目标授权、Namespace UID 和运维日志权限；多入口真实权限用例已验证 | Namespace 权威元数据持续同步、全部指标行的下级对象身份及所有入口的撤权闭环未完成；当前不能以 Cluster 数据替代 Namespace 指标授权 |
| T04 监控凭据 | 表单直接输入、加密私有暂存、监控用途隔离、事务绑定及清理；敏感值不进入公开预览 | 全平台租约/轮换/撤销/重启组合、宿主敏感表单及 Agent 路径尚未完整验收 |
| T05 采集运行时 | 多实例编译、Bundle、期望/生效版本及健康/新鲜度分离；独立启停、控制专用配置和原生日志持久恢复已有验证 | 正式接入到执行分配、Claim/Host-Node 共存、迁移/受控双采及完整失败回滚仍需逐项收口 |
| T06 可信遥测 | 按 Resource 校验来源/目标/版本，多类型存储/查询，三信号队列前脱敏；真实 mTLS/Kafka/Writer/ClickHouse 联合链路通过 | 正式注册/颁证/轮换、独立部署 Query RPC、真实 Gateway/Agent 分工及全部历史迁移边界未贯通验收 |
| T07 SDK 与 APM | 真实 Go OTLP Trace/Log/Metric SDK、自动发现、Key 轮换撤销、七类查询文档、服务/实例/RED/拓扑/Trace/日志视图；real 门户读取真实数据 | 其他 SDK/协议组合、重复埋点/跨来源统计、正式监听地址/TLS 接入和部署后完整业务验收；Jaeger 仅有原生协议等局部证据，SkyWalking 已暂缓 |
| T08 日志规则 | 文件、Journald、K8s 容器、SDK 和 Windows Event Log 规则/配置；Linux 文件/Journald/K8s 故障恢复及应用日志关联已有实测 | 正式安装后的完整采集/查询闭环、规则冲突与跨节点/Owner 变化组合、Workspace 交付仍需收口；Windows 实机按用户决定暂缓 |
| T09 自身监控 | 6 个固定指标定义；Host/Collector 和 API/Kubelet/API Server/CoreDNS 原生数据、独立启停及默认查询有证据 | **主机进程/系统服务、Scheduler、Controller Manager、etcd 尚未形成对应固定监控能力**；Host/Node Claim、Namespace 指标及全部正式安装路径未完成 |
| T10 K8s 资源读取 | 31 个内置对象类型、CRD 发现、分页/watch、UID/Owner、Secret 投影和 Namespace 范围 | 完整动态类型、全部对象与正式 Connector 部署路径的读取/撤权/恢复验收未完成 |
| T11 K8s 在线操作 | 通用变更、dry-run/Diff/确认/异步观察，Secret 私有输入，扩缩容/重启、三类工作负载历史回滚、CronJob 手动 Job、cordon/uncordon；部分真实 API/数据库工作流通过 | **Helm 生命周期、drain/PDB/存储影响、按新授权模型贯通的终端/实时日志/端口转发会话未交付**；成功 CSI 扩容、Namespace 写入竞态、全部对象/正式 Connector/门户/Agent 验收仍缺 |
| T12 第一批服务 | 通用新建表单、专用配置及首批 MySQL×2/Redis/Nginx 原生采样和默认面板 | 其余数据库、中间件、HTTP/TCP/TLS 的真实目标、认证/版本/拓扑、生命周期及默认仪表盘未完成；不存在“第一批全部支持”结论 |
| T13 第二批服务 | Memcached、Apache、IIS、HAProxy、ZooKeeper、DNS、ICMP 配置适配和表单 | 相应真实服务数据、模板、权限及生命周期未完成；IIS 实机暂缓；既定 11 项资源暂缓不计为已完成支持 |
| T14 仪表盘 | 应用/多资源/Namespace 筛选、分页选择、显式空范围、历史关联/共享去重；模板不可变版本、默认入口自动升级、个人副本；9 套默认模板 | **其余纳入支持的服务及控制面组件默认内容未交付**；Namespace 指标范围、所有面板/下钻/模板版本组合和正式部署闭环未完成 |
| T15 门户/告警/Agent | 共享列表/选择器/表单/图表、双语主题、应用 real 门户；三信号 HTTP/原生查询一致性、部分告警范围逻辑 | **新应用/监控管理及完整 K8s 工作台的原生工具发现/操作尚未接入**；Workspace 来源/文件交付、真实模型及停采/恢复/NoData 的完整告警业务闭环未验收 |
| T16 全量验收与发布 | 已有隔离原生测试驱动、浏览器用例和组件日志；Windows 准备脚本保留且未运行 Prepare | **计划级具名汇总 harness 尚未实现**；全量矩阵、真实模型、迁移/备份恢复、统一制品冻结、部署与回退演练及发布未完成 |

## 明确仍需开发的功能

1. K8s 工作台的 Helm 安装/升级/回滚/卸载、逐渲染对象授权与 hooks，以及 drain/PDB/存储影响处理。
2. 按 Cluster/Namespace/Pod UID/container 固定身份、独立授权及撤权关闭的终端、日志和端口转发会话。现有远程执行代码不等于这些 Plan8 会话已交付。
3. 主机进程/系统服务、Scheduler、Controller Manager、etcd 的内置采集、状态和配套默认仪表盘。
4. 其余候选服务的完整资源支持与对应默认面板；不能把 Receiver 编译入制品或已有配置适配器作为五类门禁通过。
5. 新应用、监控目标/实例和完整 K8s 工作台的 Agent 原生工具、共享 Presentation 与敏感宿主表单流程。Server/Worker 目前注册的仍是已有 Resource/Lifecycle、遥测查询、Dashboard、告警等工具；新领域服务存在不等于已供模型发现和调用。
6. Namespace 级指标的可信对象身份与持续权威元数据同步；禁止通过扩大范围补数据。
7. 计划级验收 harness、统一证据/版本清单，以及迁移恢复和发布回退自动化。

采集迁移/Claim、跨来源统计等范围还需进一步逐条完成实现与验证；本表未将“未验证”自动判成“代码不存在”。

## 已有真实验证及其边界

| 已复核证据 | 说明 |
| --- | --- |
| [当前 Go 回归](../../artifacts/plan8-current-v4-all-go.log) | `go test -p 1 ./...` 通过；默认未启用的真实环境用例不能由此推定通过 |
| [前端单元测试](../../artifacts/plan8-cronjob-all-web-unit-verified.log) | 1,346 项通过，另有 typecheck、lint/styles/i18n 和生成契约检查通过 |
| [实际 SDK 与 real 门户](../../artifacts/plan8-native-sdk-real-portal-desktop-views.log) | 实际三信号 SDK 到真实存储/查询及中英明暗四个浏览器场景，每场景核对三种桌面宽度；注册、颁证与可信会话初始化仍使用明确夹具 |
| [实际 K8s 工作流](../../artifacts/plan8-cronjob-postgres-api-workflow.log) | PostgreSQL 审批/Worker 与实际 Service、Job、Secret、三类工作负载回滚及 CronJob 源版本围栏；未证明部署后 Connector 全链路 |
| [K8s 默认查询](../../artifacts/plan8-kubernetes-core-components-template-queries.log) | API/Kubelet/API Server/CoreDNS 实际样本与默认查询；未覆盖 Scheduler/Controller Manager/etcd |
| [Journald 真实 systemd 单元](../../artifacts/plan8-journald-systemd-units.log) | 已有真实日志过滤证据，不能继续将 Journald 统称为未实现；故障恢复范围见[专项记录](./24-journald-and-native-log-durability.md) |

未完成原因不能统一归为“缺环境”或“只差部署”：上方具名功能确有实现缺口。已通过用例也不能抵扣没有覆盖的协议、资源类型、平台、授权入口或部署拓扑。

## 暂缓范围

- 资源监控 11 项：ClickHouse、TiDB、OceanBase、RocketMQ、Pulsar、NATS、Envoy、JVM、Tomcat、Spring Boot、MinIO；原因与证据见[专用组件复查](./13-direct-receiver-review.md)。本次未重新检索上游。
- SkyWalking 原生协议 1 项：用户已确认因上游缺陷新增暂缓，见[协议验证](./15-sdk-admission-findings.md)。
- Windows：用户暂缓本机 Windows 11 实测，保留实现/构建且未运行管理员 Prepare；Server 仍未验证。暂缓测试不等于验证通过，也不能解释其他尚未实现的功能。

## 进度记录修正

原任务 T01/T02 的“尚未开始”、部分集成/内置能力的 `not_started` 与现有代码和组件证据不一致。本次补充当前实现记录与分层状态，但保留正式 A01～A76、支持组合五类门禁和发布未完成状态，不批量补记 passed。

下一步应先收口上述功能缺口与支持矩阵，再执行正式接入、跨入口/真实模型和全量验收，最后统一发布。当前不提供完成百分比：候选支持组合尚未冻结，按文件数或单元测试数计算会误导。
