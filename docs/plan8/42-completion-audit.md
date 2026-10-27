# Plan8 完成度复核（第二轮实施前）

核对日期：2026-10-19。结论：**尚未全部实现；既有实际功能缺口，也有已实现但尚未完成正式验收的内容，尚未统一发布。**

本审计后 drain/PDB/取消和受保护控制面正向取数已有新增交付；交付见[45](./45-followup-delivery.md)，当前任务复核见[46](./46-completion-audit.md)。本表保留审计时点，不把已补内容继续作为当前缺失。

本次对照 [16 项任务](./08-implementation-plan.md)、[验收矩阵](./09-acceptance-matrix.md)、当前工作区源码、[最新实施进展](./41-implementation-followup-status.md)及现有测试产物复核。未重新运行测试、部署、清理资源或启动 Windows 管理员准备。测试结论仅沿用下列证据的实际范围。[32](./32-implementation-audit.md)保留此前时点，不作为当前数量或缺口的依据。

## 当前数量与状态

| 项目 | 当前结果 |
| --- | --- |
| 16 项任务 | 全部 in_progress；T16 已有验收驱动实现，不能继续写为 not_started，也没有任务达到全项完成标准 |
| 正式基础验收 | 76 项均为 not_run；局部测试不直接抵扣正式门禁 |
| 内置能力 | 12 项能力的 60 个分项仍为 not_run；运行时已有 12 个固定指标定义，两者统计对象不同 |
| 候选集成 | 共 39 项；11 类资源及 SkyWalking 原生协议暂缓；其余 27 项准入/支持组合尚未冻结 |
| 默认仪表盘 | 已实现 24 套；Windows 服务及三个控制面模板缺成功目标取数证明 |
| Agent 工具 | 已接入 20 个应用/监控/通用工作台可发现工具；不等于缺失的 Helm 或会话操作已交付 |
| 支持与发布 | support_matrix_status=not_locked；production_acceptance=not_run；release=not_started |

## 按任务复核

| 任务 | 已有实现或局部证据 | 仍需开发或正式验收 |
| --- | --- | --- |
| T01 契约与前件 | 领域迁移、OpenAPI/Schema/sqlc/客户端、版本化内部契约与生成检查 | 冻结全部准入组合、依赖、版本与容量/可靠性证据 |
| T02 应用与目标 | 单环境应用、稳定服务身份、目标/运行实例/关联、SDK 并发登记 | 完整生命周期、归档依赖及全部跨入口流程验收 |
| T03 范围与授权 | ScopeResolver、显式空范围、应用/目标/Namespace UID 授权、运维日志权限；新增工具和 Workspace 撤权验证 | Namespace 指标可信对象身份、持续权威元数据同步及所有入口撤权闭环 |
| T04 监控凭据 | 表单直接输入、监控用途隔离、加密私有暂存、事务绑定与公开预览脱敏 | Agent 可信宿主敏感表单；全部租约/轮换/撤销/重启组合验收 |
| T05 多实例运行时 | 多实例编译、Bundle、期望/生效/健康/新鲜度分离及独立启停 | Host/Node Claim、迁移/受控双采、完整失败回滚及正式执行分配链路 |
| T06 可信遥测 | 按 Resource 核验多目标来源/版本；真实 mTLS、Kafka、Writer、ClickHouse 局部链路 | 正式注册/颁证轮换、独立部署 Query RPC、Gateway/Agent 分工与迁移闭环 |
| T07 SDK/APM | Go OTLP 三信号、服务自动发现、Key 轮换撤销、APM/Trace/日志视图及 real 门户证据 | 其他 SDK/协议组合、重复埋点/跨来源统计与正式部署接入；SkyWalking 已暂缓 |
| T08 日志 | 文件/Journald/K8s/SDK/Windows 规则与配置；Linux 实际故障恢复 | 正式安装链路、规则重叠、跨节点/Owner 变化与完整 Workspace 交付；Windows 实测暂缓 |
| T09 自身指标 | 12 个固定指标定义；新增进程、systemd、Windows 服务和三个控制面实例/模板；Linux 进程与真实服务状态验证 | 三个控制面当前只有实际不可达/拒绝/启停证据，缺成功样本；Windows 服务未实测；Claim/Namespace 范围仍待收口 |
| T10 K8s 读取 | 内置对象、动态发现、分页/watch、UID/Owner、Secret 投影和 Namespace 范围 | 全部动态类型、正式 Connector 及读取/撤权/恢复组合验收 |
| T11 K8s 操作 | 通用 CRUD、dry-run/Diff/异步观察、扩缩容/重启、工作负载回滚、CronJob 手动任务、cordon/uncordon | **Helm 生命周期、drain/PDB/存储影响、按新授权模型实现的终端/实时日志/端口转发会话未交付**；成功 CSI 扩容及全部对象/入口验收仍缺 |
| T12 第一批集成 | MySQL/Redis/Nginx/PostgreSQL/RabbitMQ/HTTP/TCP/证书期限的原生组件、实际目标及默认查询局部证据 | Kafka/MongoDB/Elasticsearch/SQL Server/Oracle 等实际支持组合与配套内容；各类型五类正式门禁 |
| T13 第二批集成 | Memcached/Apache/HAProxy/ZooKeeper 等真实目标、认证/生命周期及默认查询证据 | DNS/ICMP 等剩余实际组合和默认内容；IIS 实测暂缓；平台/拓扑/认证矩阵未锁定 |
| T14 仪表盘 | 24 套默认模板、多类型筛选、分页选择、历史关联/共享去重、默认升级/个人副本 | 其余纳入支持类型内容、未成功取数的四套模板、Namespace 指标范围、全部下钻/版本与部署验收 |
| T15 门户/告警/Agent | 共享 UI、双语主题、20 个新领域工具及 Server/Worker 注册、带类型来源授权与 Workspace 撤权局部验证 | 可信敏感宿主表单、缺失 K8s 操作的入口、真实 Workspace/PVC/模型、停采/恢复/NoData 告警业务闭环 |
| T16 全量验收/发布 | e2e-plan8 具名入口、证据脱敏、源码/Collector 指纹、隔离后端归属与缺失测试检查 | 76 项正式基础门禁、支持组合及内置能力门禁、迁移备份恢复、制品冻结、部署/回退和统一发布 |

## 已有验证及边界

- [最新工程检查](../../artifacts/plan8-followup-final-checks.json)：Go 全量回归、前端 1402 项、类型/样式/i18n/lint、生成契约通过。另有 20 个 mock 门户场景、12 个模板浏览器场景及 4 个 real SDK 门户场景；mock 不能证明生产数据链路。
- [已有 full 执行](../../artifacts/plan8-full-component-audit-20261019-2649792552/manifest.json)：当时已选十组组件通过、源码指纹不变，但 status=incomplete、plan_complete=false、release_eligible=false。之后新增的 resources/cache-web/brokers 有分套件证据，不能把历史 full 当作当前全部套件的同次冻结运行。
- [具名验收入口](./35-acceptance-harness.md)明确未完成正式部署、全支持组合、全部 K8s 操作/会话、真实模型/Workspace/告警、迁移恢复及发布回退。即使现有组件均通过，也不能宣布 Plan8 完成。
- 新增过程与真实原生目标分别见 [33～41 的索引](./README.md)。正向取数、不可达诊断、配置适配器和正式产品支持分别记录。

## 暂缓范围

11 类资源及 SkyWalking 原生协议继续按用户决定暂缓，不计为支持或验收通过。Windows 实机按用户决定暂缓，保留代码与构建，未执行服务安装/IIS 准备，Windows 11 和 Server 实测均未验证。**Windows 暂缓不是整体未完成的唯一原因；其余上述功能和验收缺口仍然存在。**

后续先补 T11、Namespace 指标权威范围、剩余专用集成及可信宿主表单等实际功能，再按同一冻结版本完成正式业务验收和统一发布；不以修改状态或豁免失败代替交付。
