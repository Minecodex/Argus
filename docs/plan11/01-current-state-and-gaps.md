# 当前事实、缺口与统计口径

核对日期：2026-10-29。源码基线及命令输出摘要保存在 [baseline.json](./baseline.json)。本页是源码与目录核对，不声称核对时的部署、用户授权或真实模型结果与源码完全一致。

## 1. 工具实际组成

| 层次         | 当前组成                                | 数量与条件                     |
| ------------ | --------------------------------------- | ------------------------------ |
| 元工具       | tool.search、tool.describe、tool.invoke | 3                              |
| Workspace    | read、write、edit、bash                 | 0 或 4，依赖运行时、权限与容量 |
| 远程请求     | connect                                 | 0 或 1，依赖执行资格           |
| 用户交互     | request_user_input                      | 0 或 1，依赖会话资格           |
| 客户 MCP     | 当前会话已选择并授权的连接工具          | N，实际还受会话场景过滤        |
| 自有业务目录 | 通过元工具调用，完整启用口径            | 130，不直接注入模型 tools 参数 |

生产 [Worker](../../internal/app/worker/app.go) 与 [Server](../../internal/app/server/app.go) 组装 Gateway、Workspace、RemoteExecution、Interaction、EnterpriseMCP 五个 Provider。工具名称在线路上传输时，三个带点名称映射为 tool_search/tool_describe/tool_invoke，见 [名称映射](../../internal/integration/modelprovider/messages.go)。

执行 `go run ./scripts/tool-labels --catalog` 得到 129 个业务条目；[Interaction 注册](../../internal/interaction/tools.go) 另有 workflow.plan.propose，因此完整业务目录是 130。生成器未包含该交互入口是统计口径差异，不应简单认定缺少模板。隐藏 Commit 和旧隐藏 remote_plan.preview 均不计入公开目录。

| category    | 当前 | 新增后 |
| ----------- | ---: | -----: |
| application |    7 |      7 |
| monitoring  |    9 |      9 |
| host        |   12 |     12 |
| k8s         |   29 |     29 |
| connector   |   11 |     11 |
| metric      |    2 |      2 |
| log         |    1 |      1 |
| trace       |    1 |      1 |
| workflow    |    5 |      5 |
| dashboard   |   15 |     15 |
| alert       |   38 |     38 |
| observe     |    0 |      3 |
| 合计        |  130 |    133 |

## 2. 监控与仪表盘规模

- [监控目录](../../internal/monitoringcatalog/catalog.go) 有 39 项：20 个专用采集目标条目、8 个接入/日志条目、11 个暂缓条目。candidate、AdapterReady、实际发行版准入、操作系统兼容和实测结果必须分别核对；39 不是生产支持数量。
- [内置采集](../../internal/builtincollection/catalog.go) 有 12 项；这是主机/集群自身采集，与外部目标目录分开。
- [生成模板契约](../../api/contracts/dashboard-templates.json) 有 33 个模板、202 个面板，其中模型治理为独立私有域。模板定义、资源上可发现的仪表盘、可执行面板和有数据面板是四种数量。
- 实际指标数量由授权资源、来源、时间与数据保留决定。不能把模板表达式数、面板数、静态组件字段或一次目录分页当成全部指标数量。

## 3. 已确认的代码缺口

| ID  | 发现与证据                                                                                                                                                                                                                      | 影响                                         | 验证边界                                                   |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------- | ---------------------------------------------------------- |
| G01 | [Server](../../internal/app/server/app.go) 的 Dashboard Service 设置 ProvideDefaults=true，[Worker](../../internal/app/worker/app.go) 未设置                                                                                    | 模型与门户的默认仪表盘发现可能不一致         | 装配差异已确认；实际漏项需在资源授权主体上复现             |
| G02 | [ObservationCatalog](../../internal/telemetry/observation_catalog.go)、[推荐表达式](../../internal/dashboard/observation_recipes.go) 已服务门户，未注册对应业务工具                                                             | 普通诊断无法完整复用指标发现能力             | 源码确认                                                   |
| G03 | [dashboard 工具](../../internal/toolgateway/dashboards.go) 的 catalog 限 create 模式                                                                                                                                            | 不能以创建仪表盘作为普通指标发现的前置条件   | 源码确认                                                   |
| G04 | [三信号工具](../../internal/telemetry/tools.go) 使用 resource_ids；门户已有 [类型化 scope](../../internal/telemetry/observation_query_scope.go)                                                                                 | 模型侧缺少同等的来源选择与显式类型契约       | 现有 UUID 路径仍做服务端授权，不将此差异描述成已证明的越权 |
| G05 | [Dashboard Acquire](../../internal/dashboardcontext/automatic.go) 将 none 转成 analyze，[Policy](../../internal/dashboardcontext/policy.go) 收窄业务集合                                                                        | 综合诊断可能在读取仪表盘后无法继续跨领域查询 | 属于现有设计，修改须更新状态和边界                         |
| G06 | [Composer](../../web/apps/enterprise/src/components/chat/composer.tsx) 只有 host/connector/dashboard；[资源汇总](../../web/apps/enterprise/src/components/chat/conversation-resources.ts) 只有 host/cluster/connector/dashboard | 应用、监控目标、Namespace 等入口不足         | 不代表这些资源后端完全不能查询                             |
| G07 | [资源模板](../../internal/toolgateway/templates/resource.html) 对部分新领域使用通用列；[导航](../../web/apps/enterprise/src/components/chat/tool-presentation.tsx) 未处理全部新引用                                             | 关键状态不突出或无法跳转详情                 | 不把存在通用字段展示等同门户体验一致                       |
| G08 | [Discovery](../../internal/mcp/discovery.go) 没有本计划四项导航元数据                                                                                                                                                           | 对象类型、信号和操作意图依赖描述文本判断     | 需要完整工具标注与语义验证                                 |
| G09 | [后端目录检查](../../internal/tooltemplatefixture/catalog_test.go) 为 132 个绑定，前端夹具为 126                                                                                                                                | 新工具没有进入这套模板前端回归               | 差异是 6 个绑定，不是 6 个已证明不可用的功能               |

G09 缺少的条目：application.endpoint.connection、kubernetes.drain.cancel.preview、kubernetes.helm.change.preview、kubernetes.helm.history、kubernetes.helm.list、monitoring.collection.form。宿主表单和计划/询问属于专用宿主路径，应分别覆盖，不能为了凑数量强制改成模板。

## 4. 已有能力必须保留

三信号的查询/图表、应用 APM 六类视图、仪表盘异步取数与文件交付、草稿/发布流程、宿主安全表单、只读封存结果分页和明确变更确认均已有代码。Plan11 以复用和补齐为主。

当前模型治理仪表盘被专门排除在 Agent、Workspace 和普通遥测目录外，见 [模型治理设计](../plans/frontend-redesign/model-governance-dashboard-20261028.md)。这是权限与数据域边界，不纳入 G01 的默认仪表盘修复。

## 5. 证据与验收

本轮会话前序已运行 635 项模板/APM 前端测试与后端注册覆盖测试并通过，但旧前端夹具仍能通过，说明“通过数量”不能替代目录一致性检查。Plan11 实施后须按第 08 篇重新执行，不沿用前序结果作为新代码通过记录。

[Plan9 进度](../plan9/progress.json) 当前真实模型聊天闭环仍未执行；其他批次或旧 Plan7 的真实模型验收只证明各自场景，不能替代本计划的多资源路由评测。当前只读目录核对未查询正式数据库，也未触发真实业务操作。
