# Plan8 完成度复核（Pod 日志会话交付前）

核对日期：2026-10-19。**尚未全部实现，仍有实际开发缺口，也没有完成正式验收和统一发布。**

本次审计后已新增 [47 Pod 日志会话](./47-pod-log-sessions.md)，并补上原生/数据库/HTTP/门户分层验证。下表保留审计时点，日志会话不再按“完全未实现”处理；其他开发与正式门禁仍未完成。

日志会话交付后的最新逐任务复核见[48](./48-completion-audit.md)。

本次对照 [16 项任务](./08-implementation-plan.md)、[正式验收矩阵](./09-acceptance-matrix.md)、当前工作区、[第二轮交付](./45-followup-delivery.md)及下列原始测试记录。仅做源码和已有证据复核，未重新执行测试、部署或清理资源；历史测试只证明其记录的版本、环境和范围。[42](./42-completion-audit.md)保留第二轮实施前的审计时点。

## 总体状态

| 对象 | 当前事实 |
| --- | --- |
| 16 项任务 | 全部 in_progress；均有实现，但尚无任务达到其全部完成标准 |
| 正式基础验收 | 76 项均为 not_run；不代表没有局部测试，也不能用局部测试直接抵扣 |
| 内置能力正式门禁 | 12 项能力共 60 个分项均为 not_run；与运行时 12 个固定指标定义分别统计 |
| 候选集成 | 39 项中 11 类资源与 SkyWalking 原生协议暂缓，其余 27 项尚未冻结正式支持组合 |
| 默认仪表盘 | 24 套实现；23 套有原生/后端成功取数证据，Windows 服务模板尚无实机正向证据；全部正式组合仍待验收 |
| 领域工具与工作台 | 20 个新增可发现工具、32 个内置 K8s 对象类型；不包含尚未实现的 Helm/会话或 drain 独立取消工具 |
| 验收驱动 | full 注册 23 组检查；新增 drain 与 control-positive 分套件各三组通过，尚无当前完整 23 组的同次冻结执行 |
| 支持及发布 | support_matrix_status=not_locked、production_acceptance=not_run、deployment/release=not_started |

## 按任务复核

| 任务 | 已有实现或局部验证 | 剩余开发和验收 |
| --- | --- | --- |
| T01 契约与前件 | 领域迁移、OpenAPI/Schema/sqlc/客户端、内部版本与生成检查 | 完整准入版本/平台/拓扑矩阵、容量及可靠性证据、最终制品版本冻结 |
| T02 应用与目标 | 单环境应用、稳定服务身份、目标/实例/关联、SDK 并发发现 | 生命周期、归档依赖、迁移与所有入口的完整流程验收 |
| T03 范围与授权 | 统一范围、显式空集合、应用/目标/Namespace UID 授权、运维日志权限、Workspace 撤权局部验证 | Namespace 指标可信对象身份、持续权威元数据同步、所有入口撤权与后台主体组合 |
| T04 监控凭据 | 直接输入、用途隔离、加密暂存、事务绑定与脱敏预览 | Agent 可信宿主敏感表单；租约/轮换/撤销/重启完整组合 |
| T05 多实例运行时 | 多实例编译、Bundle、四类状态、独立启停和部分故障恢复 | Host/Node Claim、执行分配、迁移与受控双采、完整失败回滚、正式安装链路 |
| T06 可信遥测 | Resource 级多目标/来源/版本核验；实际 mTLS/Kafka/Writer/ClickHouse 局部链路 | 正式注册/颁证轮换、独立部署 Query RPC、Gateway/Agent 分工与迁移闭环 |
| T07 SDK/APM | Go OTLP 三信号、服务发现、Key 轮换撤销、应用视图及 real 门户局部验证 | 其余 SDK/协议、跨来源/重复埋点统计、正式部署接入及 APM 全链路；SkyWalking 按决定暂缓 |
| T08 日志 | 文件/Journald/K8s/SDK/Windows 规则；Linux 实际故障恢复与三信号关联 | 正式安装、重叠规则、跨节点/Owner 变化组合及真实 Workspace 交付；Windows 实测暂缓 |
| T09 自身指标 | 12 个固定定义；Linux 进程/systemd；三个控制面已成功采样、认证失败恢复、独立启停和默认查询 | Claim/Namespace 指标范围、正式安装部署及全部适用门禁；Windows 服务实测未验证 |
| T10 K8s 读取 | 32 个内置对象、动态发现、分页/watch、UID/Owner、Secret 投影与 Namespace 范围 | 全部动态类型、正式 Connector、持续元数据及完整撤权/恢复组合 |
| T11 K8s 操作 | 通用 CRUD、dry-run/Diff、审批/异步状态、扩缩容/重启、三类回滚、CronJob 手动运行、cordon/uncordon、drain/PDB/取消 | Helm 全生命周期；新授权模型下终端/实时 Pod 日志/端口转发；成功 CSI 扩容；全部对象/Connector/入口验收；drain 独立取消模型工具 |
| T12 第一批集成 | MySQL/Redis/Nginx/PostgreSQL/RabbitMQ/HTTP/TCP/证书期限原生目标及默认查询局部证据 | Kafka/MongoDB/Elasticsearch/SQL Server/Oracle 等剩余实际组合和内容；各支持组合五类正式门禁 |
| T13 第二批集成 | Memcached/Apache/HAProxy/ZooKeeper 真实目标、认证/生命周期和查询 | DNS/ICMP 等剩余实际组合和内容；IIS 实测暂缓；平台/拓扑/认证组合尚未锁定 |
| T14 仪表盘 | 24 套模板、多类型范围、分页选择、历史关联/共享去重、默认升级/用户副本；新增控制面十八个查询 | 剩余资源内容、Namespace 指标权威范围、完整下钻/版本及正式部署验收；Windows 服务实机取数 |
| T15 门户/Agent/告警 | 共享 UI、双语主题、20 个新领域工具、带类型来源授权、Workspace 撤权及 drain/证书配置界面 | 敏感宿主表单、缺失 K8s 功能入口、真实模型/Workspace/PVC/来源、后台主体组合及停采/恢复/NoData 告警闭环 |
| T16 全量验收/发布 | 具名检查、源码/Collector 指纹、缺失测试/Skip 拒绝、隔离资源归属与清理记录 | 全部正式门禁、迁移备份恢复、模型业务闭环、完整当前版本冻结执行、镜像/Collector/模板制品、部署/回退与统一发布 |

Helm SDK 已供平台安装命令使用，不能据此认定 K8s 工作台交付了 Helm 生命周期。旧有远程执行或日志读取也不能代替绑定 Namespace UID、Pod UID/容器及可撤销授权的新会话模型。

## 本次确认已补齐的部分

- [drain 冻结记录](../../artifacts/plan8-drain-final-20261019-3588167696/manifest.json)：三个具名组通过，源码执行期间未变；真实双节点验证 PDB 阻断/恢复、撤权停止、取消及失联恢复。门户/API 已支持取消，尚无独立模型取消工具；已下发批次可能完成，节点保留 cordon。[实现边界](./43-node-drain.md)。
- [控制面冻结记录](../../artifacts/plan8-control-complete-20261019-2550694980/manifest.json)：三个具名组通过，包含实际安装清单 RBAC、原生采样与 Writer/ClickHouse 查询。Scheduler/Controller Manager 使用直接 HTTPS、ServiceAccount 与严格证书校验；etcd 使用认证 API Proxy。[实现边界](./44-control-metrics-authentication.md)。
- 两份记录均明确 plan_complete=false、release_eligible=false。临时集群中的实际 API/后端验证和 mock 门户不等于正式注册、独立部署 Connector 与模型业务链路已经通过。
- [最新工程记录](../../artifacts/plan8-second-followup-final-checks.json)：Go 全量回归、前端 1404 项、类型/lint/样式/i18n、生成契约和 20 个当前 mock 门户场景通过。已检查相应原始 [Go](../../artifacts/plan8-current-all-go.log)、[前端](../../artifacts/plan8-current-web-unit.log)、[门户](../../artifacts/plan8-current-portal-regression.log)及[契约](../../artifacts/plan8-current-contracts.log)日志；本次没有重新运行这些测试。

## 状态记录校正与暂缓范围

本次修正 progress.json 中 T09/T11/T14 仍将控制面成功采样、drain/PDB 列为缺失的旧摘要，并更新集成及验收驱动的描述。历史审计不改写为新证据；正式门禁仍保持未执行。

11 类资源和 SkyWalking 原生协议继续按用户决定暂缓，不计为支持。Windows 实机测试继续 deferred_by_user/not_verified，不执行管理员准备、服务安装或 IIS 启用，也不声明 Windows 11/Server 已通过。**这些暂缓不是整体尚未完成的唯一原因，上表其余开发与验收缺口仍需完成。**

下一步先补 T11 的 Helm/授权会话、Namespace 指标权威范围、剩余集成及可信敏感表单，再对冻结版本执行完整业务验收、迁移恢复及发布回退。不能仅将状态改为通过来完成本计划。
