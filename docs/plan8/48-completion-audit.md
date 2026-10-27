# Plan8 当前完成度复核（Pod 日志会话交付后）

核对日期：2026-10-19。**尚未全部实现：仍有实际开发缺口，正式验收和统一发布也尚未完成。**

本次对照 [16 项任务](./08-implementation-plan.md)、[验收矩阵](./09-acceptance-matrix.md)、当前未提交工作区、[47 最新交付](./47-pod-log-sessions.md)及原始记录复核。仅做源码与已有证据检查，没有重新执行测试、部署或清理资源。[46](./46-completion-audit.md)保留日志会话交付前的审计时点。

## 已有交付与证据边界

- 应用/服务/目标、单环境归属、监控专用凭据、多实例采集、统一范围与默认仪表盘均有主干实现；不能据此将整个领域生命周期和正式接入标为完成。
- 通用 K8s 读取与变更、工作负载回滚、CronJob 手动运行、drain/PDB/取消，以及当前/上次 Pod 日志会话已有实现及分层验证。日志会话不再属于“尚未开发”。
- 日志会话已包含 UID/容器代次、一次性票据、增量游标、暂停授权检查、清退、门户及固定 API 地址；共享内联身份校验也已接入 Direct、Connector 与远程执行。
- 复核了[五组冻结记录](../../artifacts/plan8-pod-logs-identity-20261019-1923360225/manifest.json)：11 个必需具名根测试通过，执行期间源码未变；本次额外核对 13 个日志相关文件及 3 个共享身份调用文件，当前 SHA-256 仍与该记录一致。该记录明确 plan_complete=false、release_eligible=false。
- [最新工程记录](../../artifacts/plan8-pod-log-final-checks.json)及原始 [Go](../../artifacts/plan8-pod-log-all-go-identity.log)、[前端](../../artifacts/plan8-pod-log-web-final.log)、[门户](../../artifacts/plan8-pod-log-portal-final.log)、[契约](../../artifacts/plan8-pod-log-contracts-locked.log)记录显示：Go 全量、前端 1406 项、类型/lint/样式/i18n、生成契约及 24 个 mock 门户场景通过。这里是复核既有记录，没有重新运行。
- 真实 Pod、受控 HTTP/协议、数据库与 mock 门户分别验证；不能把这些分层证据合并为正式注册后、部署中的 Connector/Gateway/Agent/门户全链路通过。

## 仍需开发的主要部分

| 范围 | 实际缺口或尚未交付完整的能力 |
| --- | --- |
| K8s 工作台 | Helm 安装/升级/回滚/卸载及逐渲染对象授权；新授权模型下终端与端口转发；drain 独立取消模型工具 |
| 采集责任与生命周期 | 已有 Node 身份证据和 Claim 基础，但 Host/Node 责任、执行分配、迁移/受控双采与完整失败回滚仍未收口 |
| Namespace 指标 | 当前范围与 UID 授权已有实现；可信指标对象身份、持续权威元数据同步及所有查询入口仍需补齐 |
| 剩余专用集成 | Kafka/MongoDB/Elasticsearch/SQL Server/Oracle、DNS/ICMP 等已有配置适配基础，实际目标、认证/拓扑组合、默认内容及完整平台流程仍未齐备 |
| SDK/APM 与 Agent | 其余 SDK/协议组合、跨来源/重复埋点统计、可信宿主敏感表单，以及完整模型/Workspace/PVC/告警业务流程 |
| 默认内容与查询 | 已有 24 套模板，但剩余资源内容、Namespace 指标权威筛选及完整版本/下钻/部署行为仍需完成 |

源码核对确认：工作台接口目前包含通用对象变更和日志会话，没有 Helm/终端/端口转发的完整新领域入口；`argusctl` 使用 Helm SDK 安装平台依赖，不代表用户工作台已经交付 Helm 生命周期。drain 取消已有 API/门户，当前可发现工具目录尚无独立取消工具。服务配置适配器存在也不等于该资源已经取得真实数据与完整产品支持。

## 按任务复核

| 任务 | 当前已有内容 | 剩余开发或正式验证 |
| --- | --- | --- |
| T01 契约与前件 | 迁移至 40、OpenAPI/Schema/sqlc/客户端、内部版本与生成检查 | 全部准入组合、容量/可靠性、最终制品冻结 |
| T02 应用与目标 | 单环境应用、稳定服务身份、目标/关联、SDK 并发发现 | 生命周期/依赖/归档、迁移及跨入口完整验收 |
| T03 范围与授权 | ScopeResolver、显式空范围、应用/目标/Namespace UID 授权、运维日志与撤权检查 | Namespace 指标可信身份、持续元数据及所有查询/后台主体入口 |
| T04 监控凭据 | 直接输入、用途隔离、加密暂存、事务绑定和脱敏 | Agent 敏感宿主表单、租约/轮换/撤销/重启完整组合 |
| T05 多实例运行时 | 多实例 Bundle、四类状态、独立启停、部分故障恢复 | Host/Node 责任、分配、迁移/双采、完整回滚与正式安装 |
| T06 可信遥测 | Resource 来源/版本核验、mTLS/Kafka/Writer/ClickHouse 局部链路、共享 K8s 内联身份校验 | 正式注册/颁证轮换、部署后的 Query RPC/Gateway/Agent 及完整迁移 |
| T07 SDK/APM | Go OTLP 三信号、发现/Key、应用视图和 real 门户局部证据 | 其他 SDK/协议、重复埋点/跨来源统计及正式部署；SkyWalking 暂缓 |
| T08 日志采集 | 文件/Journald/K8s/SDK/Windows 规则及 Linux 实际恢复；另有交互 Pod 日志会话 | 持续规则的正式安装、重叠/跨节点/Owner 变化及 Workspace 全流程；Windows 实测暂缓 |
| T09 自身指标 | 12 个固定定义、Linux 进程/systemd、三个控制面成功采样与查询 | Claim/Namespace 指标、正式部署及全部适用门禁；Windows 服务未实测 |
| T10 K8s 读取 | 32 个内置类型、发现/分页/watch、UID/Owner、Secret 投影和范围 | 全动态类型、持续元数据、正式 Connector 及完整撤权/恢复 |
| T11 K8s 操作 | 通用变更、回滚、CronJob、drain/PDB/取消、当前/上次增量日志会话 | Helm、终端/转发、drain 取消模型工具；成功 CSI 扩容、竞态与全部正式入口验收 |
| T12 第一批集成 | MySQL/Redis/Nginx/PostgreSQL/RabbitMQ/HTTP/TCP/TLS 的原生目标及查询局部证据 | 其余实际目标、认证/平台/拓扑、默认内容及每组合五类正式门禁 |
| T13 第二批集成 | Memcached/Apache/HAProxy/ZooKeeper 实际目标、生命周期与查询 | DNS/ICMP 等其余组合；IIS 实测暂缓；全部五类门禁 |
| T14 仪表盘 | 24 套模板、多类型范围、历史关联/去重、升级及用户副本；23 套有正向组件数据证据 | 剩余内容、Namespace 指标范围、全部版本/下钻及正式部署；Windows 服务实测 |
| T15 门户/Agent/告警 | 共享 UI、双语主题、20 个新领域工具、Workspace 撤权及新日志门户 | 敏感宿主表单、缺失操作入口、真实模型/Workspace/PVC/来源与完整告警流程 |
| T16 验收/发布 | 具名入口注册 28 组、源码/Collector 指纹、缺项/Skip 拒绝、所有权清理 | 当前完整冻结执行、全部正式门禁、迁移/备份恢复、制品、部署与回退 |

## 当前正式状态

| 对象 | 复核结果 |
| --- | --- |
| 16 项任务 | 全部 in_progress；不表示没有实现，也不表示每项已达到全部完成标准 |
| 正式基础验收 | 76 项均 not_run，已有局部测试没有直接抵扣 |
| 内置能力正式分项 | 12 项能力共 60 个分项均 not_run |
| 候选集成 | 39 项中 11 类资源与 SkyWalking 原生协议按决定暂缓，其余 27 项仍在实施/验证，正式支持矩阵未锁定 |
| 统一检查 | full 已注册 28 组；尚无当前完整 28 组同次执行通过记录，分套件通过不替代 full |
| 正式模型/部署 | not_run / not_started |
| 发布 | not_started；源码制品清单未冻结，发布镜像/Collector/模板清单尚空 |

成功 CSI 扩容、正式安装/注册/证书轮换、全版本拓扑支持、真实模型/Workspace/告警、迁移备份恢复及发布回退属于仍需补齐的实现与验收范围，不能仅凭单层测试宣布通过。

Windows 实测继续按用户决定暂缓；11 类资源和 SkyWalking 原生协议不计支持。**即使不计算这些暂缓项，其余功能和正式验收仍有缺口。** 本次只更新审计入口和状态摘要，未将任何正式门禁或发布状态改为完成。
