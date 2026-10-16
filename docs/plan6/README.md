# Plan6：告警中心

创建日期：2026-10-07。2026-10-08 按用户确认修订入口归属：告警中心六 Tab、会话工作台 Tool Hub、组织与权限。历史 required 门禁与228组视觉已通过，l01同组源码/镜像于2026-10-12正式revision10发布、恢复及清理完成。当前[正式主机监控与GLM配置](./formal-host-monitoring-20261012.md)已完成用户请求；本次新发现的ServiceAccount直接授权和长上下文压缩问题仍开放，不以历史门禁或其他成功路径替代它们的修复。各门禁与历史接点见 [implementation-status.md](./implementation-status.md)。

## 本轮目标

企业门户新增一个「告警中心」菜单，以六个业务 Tab 承载告警列表、历史、通知记录、策略、屏蔽抑制和通知配置。D04/D05/D06 的 AI 能力接入已有会话工作台，由模型经 Tool Hub 的工具查询、工具详情、工具调用链路自行使用；D08 的权限与告警审计归入「组织与权限」。后台持续评估与通知独立于会话运行，各入口共用领域服务。创建告警策略时可一次提交多个指标规则；模型提出变更，用户通过会话宿主 PendingAction 确认一次。

原型基线为实施计划、任务文件和 [demo.html](./demo.html)；当前实现与验收分别记录，不以演示数据或原型交互代表后端、真实模型、实际通知或 Kubernetes 验收完成。

2026-10-12用户追加的[真实Docker主机与堡垒机验收](./host-bastion-acceptance.md)已在h14整轮通过：实际hostmetrics、双图数据、告警/四协议/处理/降噪/恢复/历史、GLM-5.3主机告警和Workspace文件分析、真实浏览器3项，以及最终环境/镜像/清理/恢复核验。同一源码随后l01完整核心all通过并发布至revision10；本次常驻主机、仪表盘、告警及会话配置为正式业务配置，准确范围与开放问题见上述配置记录。

## 已确认需求与支撑范围

| 编号 | 已确认能力 | 对应入口 |
| --- | --- | --- |
| D01 | 当前告警列表、筛选、状态与详情 | 当前告警 |
| D02 | 告警历史、现场值、规则快照、状态时间线与导出 | 告警历史 |
| D03 | 通知记录、失败原因、重试与受控补发 | 通知记录 |
| D04 | AI 创建告警策略及多个指标规则，Preview/Commit | 会话工作台 → Tool Hub；人工配置位于告警策略 |
| D05 | AI 查询指标、规则是否存在、当前与历史告警 | 会话工作台 → Tool Hub |
| D06 | AI 修改、启停、屏蔽与解除屏蔽，Preview/Commit | 会话工作台 → Tool Hub；人工操作位于告警策略、屏蔽与抑制 |
| D07 | 人工确认、认领、负责人和评论 | 当前告警与历史详情 |
| D08 | 企业隔离、功能权限、显式资源授权与审计 | 组织与权限，复用现有角色、资源授权和审计服务 |

原始需求仍纳入：自定义指标、告警设置、多指标规则、屏蔽、抑制、合并、钉钉/企业微信/飞书机器人 Webhook 和邮件。基础实现包含规则评估频率、持续时间、分级、恢复防抖、NoData/Error、去重、通知路由和投递可靠性。此前建议中的周期维护、模板库、值班、升级通知、日志/APM 告警、动态阈值、SLO、外部告警接入和自动处置没有在本轮追加为已确认范围。

## 文档与任务

| 文件 | 内容 |
| --- | --- |
| [01-product-and-ux.md](./01-product-and-ux.md) | 告警中心六个 Tab、会话工作台、组织与权限、需求映射与界面参考 |
| [02-architecture-and-contracts.md](./02-architecture-and-contracts.md) | 领域、状态、授权、查询、工具与可靠性边界 |
| [03-implementation-plan.md](./03-implementation-plan.md) | 阶段、依赖、交付顺序与完成标准 |
| [task-01-domain-contracts.md](./task-01-domain-contracts.md) | 领域模型、存储、接口与授权契约 |
| [task-02-evaluation.md](./task-02-evaluation.md) | 多规则评估与告警生命周期 |
| [task-03-noise-and-handling.md](./task-03-noise-and-handling.md) | 屏蔽、抑制、合并、人工处理与历史 |
| [task-04-notifications.md](./task-04-notifications.md) | 四类渠道、路由与可靠投递 |
| [task-05-agent-tools.md](./task-05-agent-tools.md) | Tool Gateway、两阶段变更、只读查询与宿主展示 |
| [task-06-enterprise-portal.md](./task-06-enterprise-portal.md) | 正式门户、共享 UI、双语、主题与真实 API |
| [host-bastion-acceptance.md](./host-bastion-acceptance.md) | 追加Docker Linux主机/堡垒机、真实系统指标、告警与GLM仪表盘闭环验收 |
| [local-release.md](./local-release.md) | 保留数据的本地升级、恢复核对与管理员配置指南 |
| [task-07-permissions-and-audit.md](./task-07-permissions-and-audit.md) | IAM 接点、撤权、聚合范围、审计与敏感信息 |
| [task-08-e2e-and-rollout.md](./task-08-e2e-and-rollout.md) | 全链路、真实模型、临时 K8s Namespace 与部署验收 |
| [acceptance-matrix.md](./acceptance-matrix.md) | D01–D08 和故障门禁逐项验收 |
| [progress.json](./progress.json) | 可机读实施状态，原型与生产实施分别记录 |
| [prototype-verification.md](./prototype-verification.md) | 本次原型验证与实际能力边界 |

## 查看原型

可以直接打开 `demo.html`。HTML、CSS、JavaScript 同目录，配色、字体、间距和圆角引用仓库共享 `web/packages/design-tokens/src/tokens.css`，无需下载第三方资源。浏览器偏好与演示业务状态分别存储；「重置演示」只清理本原型命名空间。

原型侧栏提供「告警中心」「会话工作台」「组织与权限」三个可切换入口。告警中心仅有六个业务 Tab；在会话工作台输入自然语言后查看模型的 Tool Hub 调用记录与宿主确认；告警权限与审计从组织与权限进入。

需要 HTTP 预览时，在仓库根目录运行 `python -m http.server 4186 --bind 127.0.0.1`，访问 `http://127.0.0.1:4186/docs/plan6/demo.html`。

## 既有设计接点

- [系统不变量](../00-decisions-and-invariants.md)：enterprise_id、四个服务端程序、PostgreSQL 权威状态与显式资源授权。
- [M10 Query Engine](../adr/telemetry-query-engine.md)：复用 PromQL，不新增独立查询引擎。
- [对象查询授权](../planv2/object-query-authorization.md)：遵循现有 ResourceQueryActor、Scope 和授权版本。
- [PlanV5 原生工具](../planv5/01-agent-tool-template-architecture.md)：三个元工具、隐藏 Commit 与宿主确认。
- [门户重设计](../plans/frontend-redesign/README.md)：正式页面使用 HeroUI v3 与 @argus/ui。

新增告警领域扩展业务范围，不新增服务进程、租户层级、IAM 系统或模型执行权限。PlanV2/M10 原有未覆盖记录保留历史含义，告警实施与验收由本计划独立追踪。
