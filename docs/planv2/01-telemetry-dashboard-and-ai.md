# PlanV2：遥测仪表盘、AI 创建与资源绑定

本轮范围提示：Q27～Q31 明确保留 Argus KQL、OTel/OTLP 和 ClickHouse，目标扩展到丰富展示与完整 APM。现有 KQL/Trace GraphQL 子集只是当前代码事实；协议转换、跨批次组装和 APM 分析仍需建设，详细范围见 [能力讨论](./03-observability-depth-discussion.md)。Demo 已增强可视化与自由布局，不代表真实后端完成。

Q35/Q36 最新确认：各 Panel 可分别绑定采集来源，Trace 按厂商能力提供对应查询和视图，保持 Argus 统一样式与对象权限。Dashboard 顶部通用过滤与图内厂商过滤分层，来源和局部条件进入执行范围。详细规则见 [插件与两层过滤设计](./04-collector-plugins-and-source-views.md)。

## 1. 目标与非目标

### 1.1 目标

提供 Metrics、Logs、Traces 混合仪表盘，并交付两条复用同一领域服务的闭环：

~~~text
后台创建个人编辑草稿
→ 配置统计图、变量、布局和默认条件
→ 服务端验证并生成发布预览
→ 用户一次确认，原子发布不可变 Revision
→ 在 Dashboard 页面查看、筛选和刷新
→ 从 Host/Kubernetes Cluster 页面快捷打开并带入资源筛选

Chat 中 @具体仪表盘，或从授权列表中明确选择仪表盘
→ 读取已发布统计图查询定义
→ 根据问题确定时间、资源和变量
→ 执行已发布的展示查询，或显式配置的明细查询
→ 完整结果分片写入会话 Workspace
→ AI 使用 read/grep/bash/脚本分析文件
→ 在 Chat 中给出结论，并简述范围和证据不足
~~~

本轮 Q1～Q42 的结论见 [决策记录](./02-confirmed-decisions.md)。后续澄清取代早期的“统计图只用构建器”“分析 Dashboard 当前页面视图”“为类似日志界面改用 LogQL”等提议。Q37/Q38/Q39/Q41/Q42 已明确动态来源、显式共享映射、标准下钻、APM 展示位置和归档恢复；Q40 明确重新取数采用最新发布版；Q34 的采样统计仍待澄清。

产品体验：

- 目录使用 Folder + Dashboard，支持根级未分组、组内创建、名称/描述搜索。
- 编辑者逐个添加统计图，积累变量、查询和布局修改，集中预览并发布一次。
- 普通查看始终读取已发布版本，编辑草稿保存在服务端，刷新后可继续。
- 自定义过滤变量只提供构建器；统计图支持构建器或底层已支持的查询语句。
- Host 和 Kubernetes Cluster 页面可以绑定仪表盘；快捷打开时自动选中对应资源。
- AI 分析发生在 Chat 中，查询结果导入会话 Workspace 后由 AI 使用离线文件工具分析。Dashboard 页面是否打开、浏览器当前筛选条件，都不参与 AI 查询。
- 用户只需阅读 Chat 结论，不要求打开图表、分析工作台或额外可视化页面。
- AI 创建仍使用显式 /创建仪表盘 入口，配置通过 Preview/Commit 后才能生效。

### 1.2 非目标

本期不实现：

- Profiling、方法级性能剖析；原生 SkyWalking/Jaeger Receiver 等多协议采集扩展放在后续，本期 APM 通过标准 OTLP 输入验收。
- 告警规则、通知、值班、自动修复；不建设 AI 必须遵守的固定诊断规则或必填阈值。已有可选阈值仅为展示配置，异常由 AI 结合数据与用户问题判断。
- 任意 SQL、ClickHouse 直查、未经支持的查询语法或任意 Collector 配置。
- 新查询引擎、第二套遥测存储、将三信号统一成一个语义 AST。
- 变量的自由 DSL 编辑器；构建器中的任意公式或任意函数嵌套。
- Kubernetes Namespace、Node、Workload、Service、Pod 的独立绑定、后代传播和 UID 精确过滤。
- 动态标签绑定、实时多人协同编辑、模板市场。
- 依靠 Dashboard 绑定授予数据权限，或依靠前端下拉框作为授权边界。
- 未经用户明确选择就自动挑选 Dashboard。
- AI 临时新增、改写查询、删除聚合/limit、反推原始数据或自动展开未配置的 Trace 字段。额外明细仍须有明确的已发布查询，但不为仪表盘增加导出功能或配置。
- 从 Dashboard 页面捕获“当前视图”、随 AI 查询改变页面筛选、专门的 AI 分析工作台。
- 移动端产品体验与移动端 E2E 门禁。

## 2. 现有架构与实施前提

### 2.1 保留的边界

| 能力 | 归属 |
| --- | --- |
| Dashboard、草稿、Revision、绑定、REST | argus-server 领域服务、PostgreSQL、OpenAPI |
| 遥测查询 | argus-telemetry query 的 PromQL、Argus KQL、SkyWalking GraphQL Engine |
| AI 创建和分析 | PlanV5 Agent Harness、自有 Tool Gateway、Run、持久 Workspace、离线 read/write/edit/bash |
| 查询结果文件交付 | 现有 Worker、Workspace IO 与对象存储；新增受控分片导出编排，不新增服务进程 |
| 生效配置确认 | PendingAction、Action Executor、Execution、幂等与审计 |
| 创建预览详情 | Tool 自有模板与宿主单次确认 |
| Dashboard 展示 | @argus/ui、现有 Telemetry 组件、ECharts、Design Tokens |
| 授权 | 功能权限 + 显式对象授权 + AuthorizationVersion |
| 资源入口 | Host/Kubernetes Cluster 领域服务与详情页 |

DashboardFolder 仅组织仪表盘，TelemetryGroup 仅描述 Collector 网络拓扑，两者不复用。

结构化查询配置是每种信号各自的编辑模型。服务端将其生成到既有查询语言，不替换 M10 Engine，不新建统一查询语言。AI 与人工配置使用同一契约与验证服务。

### 2.2 当前代码核查基线

以下是本轮源码核查结果，不代表新的实现已经完成：

| 能力 | 当前事实 | 本期新增工作 |
| --- | --- | --- |
| Dashboard 本体 | 企业门户、API Client、后端尚无完整 Dashboard 领域 | 存储、草稿、授权、发布、运行时、页面 |
| 三信号查询 | 已有原生 Engine、传输、单查询预算及基础展示组件 | 能力补齐、变量适配、Panel 编排、总预算与结果覆盖记录 |
| Catalog | 现有目录描述 Collector 发行版、组件与采集 Profile | 从真实遥测数据发现指标、字段、属性和值 |
| Chat 引用 | 现有 Host/Connector 引用发送时拼入文本 | 稳定 Dashboard ID、结构化引用/选择、持久化与重放 |
| Skill | 已有版本化上下文接口，显式激活尚需接入 | /创建仪表盘 与分析上下文装配、Run 快照 |
| Tool Gateway | 固定分类和名称映射尚未包含 Dashboard | 增加 Dashboard 领域分类、版本化元数据与映射 |
| PendingAction | 可用于没有 Conversation/Run 的后台操作 | Dashboard Action 分派；现有 telemetry.* 分派须避免误按 Collector 计划解析 |
| 数据授权 | user/department/role/service_account 对 Host/Cluster 显式授权，当前代码另有三信号权限 | Dashboard 加入相同对象授权体系；HTTP/Tool/UI/AI 遥测门禁统一收敛到对象访问，不按三信号或查询字段分权 |
| Workspace 文件分析 | 已有受控 result_ref 导入、离线四工具、持久目录、来源授权与配额 | 大结果分片导出、完整性清单、已发布明细来源和文件分析追溯 |

事实入口包括：

- [企业路由](../../web/apps/enterprise/src/router.tsx)、[API Client](../../web/packages/api-client/src/client.ts)。
- [Chat Composer](../../web/apps/enterprise/src/components/chat/composer.tsx)、[Skill Context](../../internal/toolruntime/skill_context.go)。
- [Collector Catalog](../../internal/telemetry/catalog.go)、[Query Coordinator](../../internal/telemetry/queryengine/coordinator.go)。
- [Tool Gateway](../../internal/toolgateway/gateway.go)、[Telemetry Action Extension](../../internal/telemetry/action_extension.go)。
- [授权 SQL](../../internal/storage/postgres/queries/authorization.sql)、[资源权限基线](../02-identity-authorization-and-data-permission.md)。
- [Workspace 结果导入](../../internal/workspace/import.go)、[离线工具](../../internal/workspace/tools.go)、[来源授权](../../internal/workspace/source_authorization.go)、[PlanV5 Workspace 约束](../planv5/task-04-opensandbox-and-mcp.md)。

### 2.3 T1.0 遥测能力核实与补齐

实施前先建立“信号 × 字段 × 运算 × 返回类型 × 图表”能力矩阵，并以真实数据验证，不能仅根据 Parser 或示例目录宣称能力已存在：

- Metrics：核对标签、资源属性、指标类型、速率、聚合、直方图分位数和资源裁剪。
- Logs：补齐首期所需的数量趋势和分组统计。现有 KQL 聚合主要是 count() by field；Parser、实际日志表及 Writer 的字段支持必须一致，包括 resource_attributes 类字段。
- Traces：验证查询列表、错误/慢请求筛选、单条 Trace 详情；不将现有总时长展示组件当作完整 Span 瀑布图。
- Q27：保留 Argus KQL，补齐时间分桶/聚合、结果类型、稳定分页、字段目录和日志上下文；丰富图型按结果形状适配，不由查询语言名称决定。
- Q28～Q30：延续 OTLP → Ingest/Kafka/Writer → ClickHouse，增加完整 APM 所需的跨批次 Trace 组装、目录、指标和拓扑分析。应用需埋点与上下文传播；Collector 收到数据不等于这些能力已存在。
- Q31：入口列表按 Host A 筛选，用户明确展开同一 Trace 时可读取另有授权的 B/C；不改变全局资源筛选，未授权部分不返回，完整性与实际展开范围明确。查询子查询、详情和缓存均执行当前对象授权；AI 不自动补取未配置明细。
- Q32：首期验收覆盖服务/实例/接口总览、调用量/错误率/时延、服务拓扑、慢错误 Trace、Span 详情和关联日志；Profiling 后续考虑。
- Q33/Q34：后续扩展托管 Collector 的 Receiver，支持 SkyWalking、Jaeger 等原生来源，标准 OTLP 来源复用 OTLP Receiver；统一数据语义、实际字段与受支持能力，沿用 Distribution/Component/Profile 发布机制。接收插件不证明数据完整，采样统计口径尚待澄清，不默认全量或自动估算。
- 目录发现采用真实观测数据；安装了某插件不等于存在某指标。
- 导出完整性需单独补齐：日志稳定分页/续传与显式 limit 区分、Metrics 原生结果与 step、Trace 列表与嵌套 Span 字段及其截断。现有有界结果写文件不等于完整导出。
- workflow.import_result 可复用身份、来源登记和 IO 规则，但整体 JSON 读入内存的路径不能承载任意大导出；需通过有界分片或流式交付复用相同边界。
- 系统资源范围只承诺 Host/Cluster。Kubernetes 下级对象身份采集、UID 归属、存储和过滤留待后续独立设计。

此阶段是 Task 1 的正式前置工作；能力不足必须补齐或明确报告，不能用 mock 返回结果通过验收。

## 3. 领域对象

### 3.1 DashboardFolder 与 Dashboard

~~~text
DashboardFolder
  id / enterprise_id / name / description / sort_order
  status = active | archived
  created_by / updated_by / created_at / updated_at

Dashboard
  id / enterprise_id / folder_id
  name / description / aliases / tags
  active_revision_id
  lifecycle = active | archived
  created_by / updated_by / created_at / updated_at / archived_at
~~~

Folder 不承担授权，不把一个 Dashboard 放入 Folder 就授予任何人访问权。一个 Dashboard 属于至多一个 Folder，空 folder_id 表示根级未分组。

Q42：非空 Folder 必须先迁出所含 Dashboard 再归档，不级联归档内容。Dashboard 归档保留 Revision、审计和个人草稿；归档或编辑者撤权期间禁止发布。对象/权限恢复后重新检查当前授权、Folder 可用性、草稿版本和发布基线，不自动提交旧预览。草稿保留不授予撤权后的读取/编辑能力。

Dashboard 本身使用显式对象授权，不使用没有授权模型支撑的 enterprise/restricted 可见性开关。Folder 下的 Dashboard 按当前用户对象授权过滤；候选与搜索也执行相同过滤。

新建时填写名称、描述和 Folder，先创建编辑者自己的草稿。草稿可在该编辑者的工作台中恢复，不作为他人可见的已发布 Dashboard。首次发布时才原子创建 Dashboard、Revision 和创建者对象授权。允许发布合法空 Spec，查看页显示空态，AI 分析则报告没有适用统计图。

### 3.2 DashboardDraft：服务端个人编辑草稿

~~~text
DashboardDraft
  id / enterprise_id
  dashboard_id?                    # 新建时可为空
  editor_subject_type / editor_subject_id
  base_revision_id?                # 首次发布为空；修改已有对象时必须记录
  draft_version
  status = editing | published | discarded
  published_revision_id?
  name / description / folder_id
  spec_json / proposed_bindings[]
  created_at / updated_at
~~~

- 每位编辑者独立保存草稿，首期不做共享实时协同编辑。
- 已有 Dashboard 的草稿归属为“Dashboard + 编辑者”；新建草稿以 Draft ID 标识。
- 保存草稿无需 PendingAction 确认，不改变 active Revision，不对其他编辑者或普通查看生效。
- 保存仍须校验身份、对象授权、编辑权限、基本 Schema 和大小限制，并记录审计。允许保存尚未完成查询验证的草稿。
- draft_version 防止同一编辑者多个页面互相覆盖；base_revision_id 防止多人发布互相覆盖。
- 发布预览冻结草稿 ID、版本、内容与基线。继续编辑不会改变已冻结计划，但会使旧预览失去提交资格，需要重新预览；新编辑内容保留在草稿中。
- Commit 时若 active_revision_id 已不同于基线，拒绝覆盖，返回可理解的冲突和差异；用户整理后重新预览。
- Commit 同时检查草稿仍为 editing 且版本未变化，原子标记 published 并回填 dashboard_id/published_revision_id。一个新建草稿最多转成一个 Dashboard；同一 Action 重试返回原结果，不同旧预览不能再创建第二个对象。
- 发布基线还覆盖 Dashboard 元数据/生命周期版本；Commit 同事务检查仍 active、目标 Folder 可用和当前对象权限。归档、恢复或元数据变更不能在 active_revision_id 未变时绕过旧预览失效检查。
- 发布后继续编辑从当前发布版建立新的个人编辑草稿，已消费的草稿保留发布来源，不重新作为新建草稿提交。
- 页面刷新后可恢复草稿；只有一次最终确认使整批变更生效。

### 3.3 DashboardRevision 与验证状态

~~~text
DashboardRevision
  id / enterprise_id / dashboard_id / revision_number
  schema_version = argus.telemetry_dashboard/v1
  spec_json / spec_hash
  validation_status = valid
  validation_report_json
  sample_status = success | no_data | unavailable
  sample_report_json / query_catalog_snapshot_json
  created_by / created_at / published_at
~~~

发布的 Revision 及其配置、验证报告不可变。无效配置保留在个人草稿或失败预览中，不成为 active Revision。布局、变量、Panel、默认条件的多次编辑先积累在草稿，最终发布才形成一个新 Revision。

配置验证与样本执行分开：

| 检查 | 发布规则 |
| --- | --- |
| Schema、查询语法、字段/类型、展示查询图表兼容性、明细查询文件 Schema、变量依赖、对象访问/编辑权限、预算 | 硬门禁；失败或无法完成必要硬校验时不能发布 |
| 样本为空、时间窗口无事件 | 显示 no_data，不等于配置错误或系统正常 |
| 样本查询服务临时不可用 | 已通过硬校验时可带 unavailable 警告发布 |
| 样本证明类型不匹配、查询非法或权限不满足 | 属于硬错误，不能当作普通临时警告放行 |

发布报告记录当时的样本状态。运行时新查询状态单独返回，不回写旧 Revision 的不可变报告。只改布局不应因样本服务临时故障而被阻塞，但仍校验配置和当前权限。

普通页面每次加载当前 active Revision。一次查询或 AI 分析开始时由服务端冻结该发布版本；执行期间发布新版本不会混入旧查询。冻结上下文短期有效，归档、撤权与硬性失效仍即时检查。

### 3.4 DashboardSpec

~~~json
{
  "schema_version": "argus.telemetry_dashboard/v1",
  "default_time_range": {"kind": "relative", "seconds": 3600},
  "default_refresh_seconds": 0,
  "variables": [],
  "panels": [],
  "layout": {"columns": 12, "row_height": 8}
}
~~~

资源绑定不存入 Spec。默认变量值是配置的一部分；用户查看时选中的时间、机器、环境等属于运行时上下文，只有主动编辑并发布默认配置才改变 Revision。

### 3.5 Panel、QueryTarget 与唯一编辑来源

一个 Dashboard 可以混排三信号；每个 Panel 的展示查询属于一个 Signal，可有多个同语义 QueryTarget。下钻目标可按 §3.5.1 显式声明其他信号，与展示查询分开执行。

Q36 允许各 Panel 分别绑定来源；每张 Trace Panel 的全部展示 Target 继承同一来源绑定，不自动跨厂商合并。来源绑定不是资源页 DashboardBinding。Q39 的下钻使用单独发布的目标信号、来源和参数映射，跨信号关联不等于将展示查询混合来源。

Q37：source_binding 按来源类型与顶部授权资源范围动态匹配，具体采集实例作为局部筛选。后续查询可纳入新匹配来源；停止采集不删除保留期内历史，重装必须区分新旧来源身份。当前查询时间范围内的历史来源同样参与解析，不能只检查当前已启用组件。每次执行冻结实际 source IDs、来源/配置/能力版本及参数，运行中的分页/导出不自动加入新来源，权限仍按每次访问重验。

~~~text
Panel
  id / title / description
  type                              # 来源能力注册的图型，含基础图型与 Q41 专用 APM 图型
  signal = metrics | logs | traces
  authoring_mode = builder | dsl
  applicable_resource_types[]        # host / kubernetes_cluster，非空
  source_binding                    # 来源类型 + 动态资源范围，Trace Panel 必需；实例在局部条件选择
  local_filters[]                   # 局部过滤定义、类型、候选、默认值与参数绑定
  targets[]
  detail_query_targets[]?          # 可选的普通明细查询定义，非导出配置
  drilldowns[]?                     # 标准下钻入口、detail_query_ref、输入映射与范围策略
  layout = {x, y, w, h, min_w, min_h}
  unit / decimals / thresholds / legend / display_options / links

QueryTarget
  id / language = promql | kql | skywalking_graphql
  source_definition                 # 由 Panel 的 authoring_mode 确定结构
  signal / source_binding?           # 展示目标继承 Panel；下钻目标显式声明目标信号和来源
  parameter_bindings
  query_mode / min_step_seconds / budget
  range_step_policy                  # 固定秒数，或服务端按目标点数自动计算
  compiled_query / compiler_version / query_hash   # 服务端派生
~~~

authoring_mode 决定整张统计图的唯一编辑来源。一个 Panel 可有多个 Target，但全部普通查询和明细查询 Target 使用该模式对应的 source_definition，不混合维护两套可独立修改的构建器配置与查询文本。模式切换须对整张图无损成立，否则保持原模式。

applicable_resource_types 是发布配置中明确声明、由服务端校验的资源类型契约，全部 Target 继承。构建器可根据所选数据源预填，DSL 同样需要声明；不能单凭原生语句、是否有样本、Binding 或模型临时判断适用性。从 Host 打开时 Cluster-only Panel 可返回 not_applicable；没有授权或查询失败不能借此伪装成不适用。分析上下文中的适用类型来自这些已验证配置。

- builder 保存对应信号的结构化配置，服务端生成既有原生查询。
- dsl 保存原生查询定义：PromQL expression、KQL expression/pipeline 或只读 GraphQL document/operation/参数契约，按语言分别定义严格 Schema。
- 构建器可以转换为查询语句。
- 查询语句只有能完整、无损解析时才能转回构建器；不支持的表达式保留语句模式，不近似转换、不丢条件。
- compiled_query 是服务端派生的执行产物，不是第二个可编辑事实来源；客户端不能通过上传派生字段覆盖验证结果。
- query_hash 覆盖原生查询及参数契约等执行语义，布局修改不改变查询哈希。明细查询独立生成 detail_query_hash 并纳入 Spec Hash。
- 来源绑定、局部过滤定义和参数映射纳入执行身份与发布校验；运行态局部选值纳入请求/缓存，不作为第二份可编辑查询。来源所需字段、查询运算及厂商详情逐项声明支持，现有 skywalking_graphql 名称不证明所有厂商原生 API 兼容。
- UI 与 AI 创建支持相同的两种编辑来源；分析时只执行已发布的展示或明细查询定义，不能上传替代查询文本。

### 3.5.1 平台标准下钻与已发布明细

- Q39：平台按来源和图型提供标准下钻，如 Trace 详情、关联日志、接口慢请求；生成明细查询及参数约束，随 Panel 一起预览发布，作者需要时可编辑。
- 编辑器使用“下钻详情”配置区，展示目标、查询、参数映射及实际范围；不新增导出来源、文件格式或下载设置。
- 下钻入口引用已发布 detail_query_ref，并声明输入字段/变量映射、目标信号、目标来源及 scope_policy。生成查询遵守 Panel 的有效 authoring_mode，不维护第二份隐式可编辑查询；生成器/来源升级不直接改写旧 Revision。
- 同信号同来源下钻可显式继承 Panel 绑定；日志关联 Trace、Trace 关联日志等跨信号下钻须声明对应目标来源和查询契约。仅凭名称或同名字段不能自动关联。
- UI 点击和 AI 按问题选择都执行同一已发布定义，使用当前对象授权、系统资源、合法变量与共享预算；只接收声明允许的参数，不接受临时替代表达式。
- 普通下钻继承原有效范围；Q31 的完整 Trace 展开作为专门的范围策略，才可读取另有授权的 B/C。引用其他资源的 ID 不自动授予访问权。
- 缺少必要关联字段时入口显示不可用原因，不伪造关联；查询合法但没有数据按 no_data 处理。明细生成和变更纳入发布 Diff、硬验证及独立样本状态。

### 3.6 首期构建器能力

| 信号 | 首期交付范围 | 类型约束 |
| --- | --- | --- |
| Metrics | 趋势、当前值、分组聚合、计数器速率、Top N、受控错误率与直方图 P95 | 根据 Gauge/Counter/Histogram 等实际类型提供运算；P95 不能从平均延迟直接推导 |
| Logs | 日志明细、条件过滤、数量趋势、按字段分组计数 | 字段发现、查询生成、Pipeline、返回类型与图表一致 |
| Traces | Trace 列表、错误/慢请求筛选、单条 Trace 详情 | 单 Trace 详情与多 Trace 总时长条区分；不隐式承诺复杂 Trace 统计 |

任意跨查询公式、任意函数嵌套和复杂 Trace 聚合不进入首期构建器。DSL 模式允许当前底层已支持且通过校验的语法，但不能使用未知字段、突破预算或返回不适合该 Panel 的类型。首期构建器矩阵是交付要求，不是对现有代码能力的完成声明。

该表为基础编辑能力，不排除 Q32/Q41 的平台预定义 APM 能力。首期增加服务/实例/接口总览、调用量/错误率/时延、服务拓扑和慢错误 Trace 等专用统计图，提供展开详情与全屏视图；按厂商组织信息、复用 Argus 组件及权限，不另建独立 APM 一级页面。每种图型声明实际查询、结果形状、过滤与下钻能力；不能只注册名字而仍渲染成通用列表。采样统计口径仍按 Q34 待澄清项处理。

### 3.7 系统资源筛选与自定义变量

两类过滤承担不同职责：

1. 系统资源筛选：Host/Cluster 选项来自当前主体授权和资源事实，服务端强制应用到所有适用 Panel，不依赖查询是否显式写了 $host 或 $cluster。
2. 自定义变量：环境、服务等由作者在构建器中配置选项查询，仅作用于显式使用该变量的 Panel 查询位置。

~~~text
DashboardVariable
  key / label
  kind = query | interval
  signal = metrics | logs | traces       # query 变量
  options_builder                       # 该信号的结构化发现/过滤配置
  value_field / text_field / value_type
  dependencies[]                        # 从类型化引用派生
  selection = single | multi       # 全部是系统保留的运行态选项
  default_values / max_values
  refresh = on_data_refresh | manual # 仅控制同一上下文内的额外刷新
  source_provenance?
  discovery_scope / value_identity   # 明确候选发现来源与值身份，不能从某一 Panel 隐式借用
~~~

- 变量配置只提供构建器，不开放变量的自由 DSL。保留三信号、链式依赖、单/多选、默认值和刷新能力。
- 主要从已有数据目录选择字段和值，不专门建设无数据预建字段流程。
- 下游变量在构建器中引用上游变量，服务端由引用派生依赖图，拒绝未定义引用、循环与不兼容类型。
- Panel 构建器选择类型化变量；DSL 模式通过语言适配后的参数绑定使用这些变量。
- PromQL matcher、KQL predicate、GraphQL 原生 variables 分别适配。不能简单全局扫描/替换 $variable，不能把 GraphQL 原生参数误当作 Dashboard 依赖。
- 多选值按语义编码，重验长度、数量、类型和允许字段。All 指当前授权、前置条件与适用范围内的全部匹配值，不等于下拉框已加载的有限候选，也不能移除系统资源范围。
- 目录和自定义变量候选使用当前请求时间范围；未出现的具体值不再列出。重新取值后，已有选中值消失或不再完全有效时，该变量自动回退为“全部”（All），并重新计算依赖变量。
- 首次加载以及时间、前置变量、资源选择或对象授权变化时，候选必须失效并重新获取。refresh 只控制同一上下文内随数据刷新或手动刷新，不能阻止上下文变化后的必要重载。
- 单选、多选都支持此回退；没有任何具体候选时仍保留“全部”状态，查询没有数据则返回 no_data。界面和 Chat 查询上下文记录实际回退后的值，不继续使用已消失的旧选择。
- All 只解除当前自定义变量的值限制，仍受当前对象授权、已选系统资源和上游有效条件约束。该规则不用于把未授权 Host/Cluster、非法类型或伪造参数转换成全部资源。
- 自动回退属于运行态，不修改发布版默认变量，也不额外要求发布确认。
- 三信号变量可以串联，但相同显示名称不代表相同业务实体；实际使用哪个字段由明确的配置决定。
- Q38：环境等具有统一业务语义的条件可共享，厂商服务/实例默认放在各图；作者显式配置跨来源共享时，声明候选发现范围、稳定值身份和每个 Target 的参数映射。候选不按名称隐式合并，执行不将一个厂商的原生 ID 自动传给另一个厂商。
- 编辑界面展示变量链、配置、依赖、预览与 Panel 引用；宽屏弹框固定 Header/Footer，仅正文承担纵向滚动。

### 3.7.1 顶部通用过滤与 Panel 局部过滤

- 顶部提供时间、授权 Host/Cluster 和作者配置的共享变量；时间/系统资源约束全部适用 Panel，环境、服务等自定义变量继续只作用于显式引用它们的查询。
- Panel 标明自己的来源，并按其能力提供服务、实例、接口/操作、状态、标签或时延等局部选项；选项仅影响本图，不反向修改顶部或其他图。相同显示名不表示跨厂商实体相同。
- 局部条件叠加于已发布查询、来源绑定及实际继承的顶部条件，不能覆盖系统资源扩大范围。有效条件无匹配时返回 no_data，不自动放宽查询。
- 局部候选查询继承当前时间、资源、来源、引用的共享变量及局部前置条件。候选型选值消失时沿用 Q22 回退 All，仅解除本项；手输关键字、Trace ID、时延范围不能因无结果被静默清除。
- 局部过滤有稳定 ID、类型、默认值和查询参数映射，按原生语言/厂商能力在服务端绑定并校验；不能只筛当前页或拼接未发布的自由查询文本。
- 来源、过滤项和默认值修改进入个人草稿统一发布；查看时调整选值是运行态，不修改发布版本。清除局部条件仍保留顶部约束和来源绑定。
- 建议常用条件就近展示，高级项折叠，展示有效条件摘要；小尺寸 Panel 可展开筛选，避免大量表单占据图表。
- Q31 的明确完整链路展开保留为单独操作，允许读取另有授权的 B/C；普通局部筛选不承担这项扩大详情范围的行为，也不改变顶部选择。

### 3.8 OTLP Catalog

本节的实际数据发现规则继续有效；Q35/Q36 要求补充采集插件/集成入口、Panel 来源绑定和厂商能力映射。Collector 安装目录不能直接代替数据目录；QueryTarget、局部过滤与相关变量候选继承 Panel 已确定的来源范围。

目录按“信号 → 实际观察到的指标/字段/属性 → 有界的值或范围”组织：

- Metrics：指标名称、类型、Label、有限值和类型适用的运算。
- Logs：允许字段、字段类型、有限值、受控文本检索。
- Traces：允许的服务、Span/资源属性、状态和时延范围；与当前查询接口能力保持一致。
- Host/Cluster 选项直接复用授权资源目录。
- 资源、服务、instrumentation scope 或可靠的 Collector 来源可作为筛选。没有采集到来源血缘时，不猜测某条数据属于哪个插件。
- 已安装的 Collector/Profile、真实观察到的数据、当前窗口有无样本分别标识，不能混用。
- 目录返回受对象授权资源范围、查询语言允许字段、统一数据安全处理、数量、长度和时间预算约束。Signal 用于选择数据源/解析器，不作为权限维度。
- 显示名与底层实际字段显式映射；选择目录内容只生成草稿，发布与执行仍经过 Parser/Validator。

### 3.9 DashboardBinding

~~~text
DashboardBinding
  id / enterprise_id / dashboard_id
  target_type = host | kubernetes_cluster
  target_id
  status = active | stale | revoked
  created_by / created_at / revoked_at / last_verified_at
~~~

绑定由 Host/Kubernetes Cluster 页面配置，负责快捷入口与打开时预选资源。Dashboard 可以被多个资源关联，未绑定也可以从目录打开或在 Chat 中分析。

绑定不限制 Dashboard 的可查询资源全集，不参与运行时有效资源集合的求交；不保留 all_bound_targets 作为默认查询或权限边界。改变绑定不修改统计图查询。

资源删除或关联失效使入口不可用；当前用户撤权只影响该用户的可见性，不能把其他人仍有效的全局绑定改为 revoked。查看入口时同时检查目标资源和 Dashboard 对象权限。

本期不提供下级 Kubernetes 对象、UID、传播规则或动态成员选择；以后另行设计。

### 3.10 仪表盘查询与 AI 工具文件交付的边界

仪表盘本身不提供导出功能：没有导出按钮、下载页、导出来源选择器或独立导出配置区。用户只配置、发布和查看统计图查询。

AI 在 Chat 中查询某个仪表盘的数据时，由数据查询工具读取其已发布查询、执行取数、在内部完成结果导出，并把文件交付到当前会话 Workspace；AI 再使用文件工具分析。

- 默认交付已发布统计图查询的完整结果，包含查询本来就返回的日志原文或 Trace 详情，无需额外配置“导出来源”。
- 若需要统计图现有结果之外的原始日志或 Trace 明细，仍需有明确、已发布的明细查询定义。它属于正常查询契约，不是用户操作的导出功能，工具不能临时删除聚合或拼出未发布查询。
- 普通查询与明细查询使用同一时间、对象授权、系统资源和变量参数规则。工具只能选择当前 Revision 允许的查询引用，不接受替代表达式。
- 明细查询由 Q39 的标准下钻生成并随 Panel 发布，人工和 AI 共用。跨信号目标和参数映射按 §3.5.1 显式声明；工具交付结果文件不另造一套明细定义。
- 聚合图交付聚合结果，显式 limit 100 保留 100 条的查询语义；Metrics 保留 instant/range/step；Trace 保留已发布字段选择。
- 页面查询类型由图表契约验证；工具内部将结果写成文件时另外校验序列化 Schema，不增加用户需要配置的文件格式或导出参数。
- 查询修改继续走草稿与发布；对已有查询结果进行内部文件交付不修改 Dashboard，不触发用户发布确认。

## 4. Dashboard Query Runtime

### 4.1 统一执行

~~~text
ExecuteDashboard(ctx, DashboardExecutionRequest)
  → 读取当前已发布 Revision，或校验本次执行冻结的 Revision
  → 校验 Dashboard 对象权限及当前授权版本
  → 解析时间、系统资源选择和自定义变量
  → 计算适用且有权查询的 Host/Cluster 范围
  → 按保存的查询及参数绑定生成执行请求
  → 调用三种既有 Engine
  → 汇总预算、状态、脱敏、审计和证据
~~~

请求携带 Dashboard、服务端执行上下文、时间、资源选择、变量、Panel IDs、来源和预算；普通执行不接受查询文本覆盖。草稿的样本执行使用独立的已授权预览路径，不伪装成已发布查询。

Q36 增加按 Panel ID 区分的局部参数。服务端从 Revision 读取来源绑定与过滤定义，将对象范围、来源范围、顶部条件及局部条件组成执行请求；客户端不能用改写来源或私传未知过滤字段绕过发布配置。

~~~text
有效资源范围 =
当前用户授权的 Host/Cluster
∩ 当前请求选择（未指定时为全部授权且适用资源）
∩ 当前 Panel 的适用资源类型与查询条件
~~~

Dashboard 对象访问及 Host/Cluster 对象数据授权是前置检查；不按 Metrics/Logs/Traces 划分查询权限。绑定不进入上述公式。空授权或空适用范围返回明确状态，绝不能将空 ResourceIDs 当作“不加资源过滤”传给底层。

从资源页面打开时自动选中当前 Host/Cluster，此后用户可切换到其他有权限且适用的资源。资源选择无论来自页面、URL、Chat 还是 AI Tool，都执行同样的服务端授权。

### 4.2 时间、变量与刷新

- UI 支持相对/绝对时间、立即刷新、关闭/30 秒/1 分钟/5 分钟/15 分钟自动刷新。
- 页面不可见暂停刷新；请求不重入；时间、变量变化取消旧请求，防止旧结果覆盖新结果。
- 顶部时间/系统资源变化更新全部适用 Panel；共享变量仅更新引用它的 Panel。局部过滤变化仅更新本图与内部依赖，缓存与取消按各 Panel 实际条件区分。
- Range step 来自已发布 range_step_policy：固定步长按原配置执行，自动模式按实际时间、目标点数和 min_step_seconds 在服务端确定并记录。UI 仅在自动模式允许时提供图表尺寸建议；Chat 导出使用已发布的服务端目标点数策略，不依赖浏览器尺寸。超预算明确报错或不完整，不能悄悄改变显式固定步长。
- Chat 首次查询采用仪表盘默认时间和默认变量；用户明确条件覆盖默认值，未指定资源时使用当前主体全部有权限且适用的资源。
- 连续追问继承上一次明确选择的仪表盘、资源和变量，按用户要求修改条件；继承的自定义变量选值因新候选失效时按 §3.7 自动回退全部。没有历史明确选择才使用缺省规则，实际条件作为结构化会话事实持久化。
- 没有浏览器“当前视图”输入，也不自动修改 Dashboard 页面条件或默认配置。
- Chat 的局部过滤使用已发布默认值与用户明确条件，按 Panel ID 记录和继承；条件指向不明时澄清，不把一个厂商的同名字段自动套给其他图。
- 单次执行将相对时间解析为绝对区间，冻结 Revision、参数和范围，权限每次调用仍重新验证。
- 已保存的自定义变量默认选值因候选变化消失时同样回退全部。非法类型、未授权对象或不能恢复的必要查询参数仍返回错误/可澄清状态，不将此类请求当作未指定资源。

### 4.3 结果、partial 与数据新鲜度

~~~text
DashboardPanelResult
  dashboard_id / revision_id / panel_id / target_id / signal
  status = success | no_data | error | partial | not_applicable | skipped_budget
  native_result / query_meta / warnings / evidence_ref
~~~

保留 Prometheus、KQL、GraphQL 各自结果语义。单 Target 的错误遵守 M10 契约，不伪造为成功 partial；Panel 的部分 Target 或整个 Dashboard 的部分 Panel 失败时，由上层聚合返回 partial 并保留成功结果。

新鲜度区分查询完成时间、最新样本事件时间和可获得的采集/摄入状态。无法证明数据完整到达时记为 unknown。无新错误日志不自动等于数据过期或系统正常。

### 4.4 总预算、完整性与缓存

Dashboard 预算覆盖变量发现、Panel 查询、AI 查询结果的内部导出、分页和同一次 AI 分批请求：

- Panel/Target 数、并发、超时、扫描/返回/导出字节、MaxSamples/MaxSeries/MaxRows、Workspace 剩余容量。
- 并发前分配预算，取消与超时传播至查询和导出任务；分批、分页、分片和重试不能重置总预算。
- 目标是完整导出已发布查询结果。预算、空间或故障导致不完整时标为 partial/failed/cancelled，记录缺失范围，不能静默抽样、缩小时间或宣称下载全部。
- 缓存键包含企业、主体、Dashboard/Revision/Panel/Target、查询引用、query_hash/detail_query_hash、绝对时间、step、变量、有效对象范围指纹、Signal/语言、统一数据处理版本及 AuthorizationVersion。
- 来源绑定定义/能力版本、本次冻结的实际来源集合及配置版本、规范化的各 Panel 局部参数和下钻目标/映射同样进入执行指纹、缓存和文件来源清单；动态来源新增或局部条件变化不能复用不同范围的旧结果。
- Signal 是查询语义，不是授权维度。不同主体授权版本相同不能据此共享不同范围的数据，命中前仍重新执行对象授权。
- 导航绑定缓存独立失效；已导出文件只按实际查询身份、时间与完整性复用，同名路径不代表相同数据。

### 4.5 受控结果导出与 Workspace 交付

由已有 Worker 协调 ExportDashboard，复用 Query Runtime 的对象范围、原生 Engine 和预算，经受控 Workspace IO 交付文件。Sandbox 保持无网络，不用 bash 直接访问数据库或下载数据。

~~~text
DashboardExport
  export_id / enterprise_id / conversation_id / run_id / workspace_id
  dashboard_id / revision_id / panel_id / target_id
  source_query_ref                  # 指向已发布普通/明细查询，非页面导出设置
  query_hash / absolute_time_range / variables / effective_resource_ids
  query_mode / step / native_result_schema
  status = queued | running | complete | partial | failed | cancelled
  snapshot_boundary / continuation / completeness
  chunks[] = {file_ref, path, bytes, rows_or_points, content_hash}
  source_ref / created_at / completed_at
~~~

- 冻结查询身份、实际时间/资源/参数和读取边界，补齐稳定排序、游标/快照、续传和去重，避免同时间戳或导出期间新增数据造成重复/遗漏。
- 区分原查询的聚合、显式 limit、字段选择与系统分页。complete 表示完整取得查询定义的结果，不代表取得全部底层原始遥测。
- 原生查询显式保存的分页参数也属于其语义，不能擅自改变。清单分别记录“已定义结果是否交付完整”和“是否覆盖全部匹配数据”；明确记录显式 limit/分页等范围约束，系统预算截断不能伪装成查询本身的限制。
- Trace 嵌套 Span/关系记录各自完整性；Metrics 保留步长、instant/range 和原生数值/Histogram 类型。步长及求值时间网格冻结到导出任务，分片/续传/重试不得重算，文件转换不能改变语义。
- 按信号使用版本化 JSON/JSONL 分片，保留原生字段和类型；清单记录格式、排序、总量（未知则标未知）、实际导出量及完整性。
- 大结果流式或有界分片交付，不装入一个 Tool Result、模型上下文或内存缓冲。小结果可复用 result_ref 导入，大结果复用同一来源校验和 IO 边界。
- 写入前登记会话来源并检查容量，临时分片完成后原子交付；同一 export_id 重试不重复追加或覆盖他人文件。重启可恢复任务状态与受控进度。
- 文件写入当前会话 Workspace 的独立导出目录，返回可信文件引用和清单；目录名不能证明真实字节已经交付。
- 延续 PlanV5 的持久化、容量、计算回收和明确删除规则；满额停止新增写，不自动删除旧文件。
- 文件落地后继续执行来源对象授权；将 Dashboard 纳入来源指纹，不能因为已经下载就绕过对象撤权。
- 服务端 source_ref、原始 Hash、查询身份和状态是可信事实。Workspace 文件可被 AI 修改，可修改的本地 manifest 不能自证原始性，派生结果不替代原始来源。
- 导出是受授权查询的文件交付，不修改发布配置，不另加 Preview/Commit。Workspace/Sandbox 未配置、不可用或配额不足时如实报告，不伪造文件或已完成分析。

## 5. Chat 内的 AI 文件分析

### 5.1 先明确选择 Dashboard

支持两种入口：

1. @具体仪表盘，消息保存稳定 Dashboard ID，label 仅用于展示。
2. 未指定对象而泛问“有没有问题”：列出有权访问的 Dashboard，用户选择一个或多个后分析。

模型不能根据名称自行挑选。已明确选择的对象可以继续使用，无需每次重新 @；列表、选择、恢复和查询均重新检查对象授权。

~~~text
telemetry.dashboard.list
telemetry.dashboard.get
telemetry.dashboard.query
telemetry.dashboard.query.get
telemetry.dashboard.query.cancel
telemetry.dashboard.catalog.*
~~~

list 与 @ resolver 共用授权搜索。服务端保存 mention 或明确选择事实，AI 不得自行增加未选中的 Dashboard。

### 5.2 查询定义、默认条件与追问

get 返回有访问权的已发布配置：

~~~text
dashboard_ref / revision_ref / dashboard_name
default_time_range / variables / applicable_resource_types
panels[]
  panel_id / title / signal / visualization
  query_definitions / detail_query_definitions?
  query_hash / detail_query_hash?
  parameter_bindings / budget
context_ref / context_expiry
~~~

有 Dashboard 访问权即可查看图表、查询语句、默认变量和已发布查询定义，不再划分“能看图但不能看语句”。编辑/发布需要对象管理能力，导出数据仍检查 Host/Cluster 对象授权。

AI 根据问题确定时间、资源和变量，不改普通或明细查询。首次使用默认时间和变量，未指定资源时使用当前用户有权限且适用的资源；追问继承明确条件，并按用户要求更新。候选变化导致已有自定义变量选值消失时，执行自动回退全部并记录实际取数条件。

对象选择和参数作为结构化会话事实保存。每次实际导出固定 Revision、绝对时间和合法参数；解释旧结论可引用原证据，重新查询产生新的导出，不能将旧文件冒充新结果。

Q40：跨轮重新取数使用请求开始时的最新发布版；同次执行冻结 Revision，进行中的分页/导出不混入后来发布的配置。已生成文件保留原 Revision、查询、参数、时间与来源，解释旧结论可读旧证据。仅继承用户明确且契约兼容的旧条件；其余使用新默认值。来源/参数含义改变导致明确条件无法无损迁移时须提示重新明确，不能静默迁移或当作候选消失而回退 All。每张 Dashboard 保存独立上下文，不共用第一张的默认时间/变量。

Chat 不读取或修改 Dashboard 页面。候选变化使继承的自定义变量选值消失时，同样自动回退 All，并记录实际条件；对象授权与系统资源范围保持有效。

### 5.3 统计图覆盖与数据覆盖

- 泛问检查所选仪表盘全部适用统计图，具体问题按需选择相关图。
- 工具逐图执行已发布查询并在内部交付结果文件；没有明确的明细查询定义时，不自动去掉聚合或补出原始日志/Span。
- 导出覆盖与分析覆盖分开：文件生成不代表已分析，每张图执行过也不代表每份数据完整。
- 服务端记录 selected/applicable/export_status/completeness；Agent Run 保留文件读取、命令和分析过程。
- 无数据、失败、撤权、预算/空间不足和未完成分析进入覆盖说明；不能以局部结果宣称整体正常。
- 多 Dashboard 分别保留版本、时间和范围来源，不能混淆。

### 5.4 AI 数据查询工具、内部导出与 provenance

query 复用 Query Runtime，并在内部调用 §4.5 的文件交付服务；只接受上下文、Panel 选择、已发布查询引用和合法参数：

~~~json
{
  "context_ref": "analysis_context_01",
  "dashboard_id": "db_01",
  "panel_ids": ["latency", "errors"],
  "time_range": {"from": "...", "to": "..."},
  "resource_selection": {"kind": "explicit", "host_ids": ["host_01"], "cluster_ids": []},
  "variables": {"environment": ["production"]}
}
~~~

实际查询从冻结 Revision 恢复；需要明细时只能选择 get 返回的已发布查询引用，不允许输入替代表达式。context_ref 不是授权凭证，每次导出、续传、读取和取消都检查当前对象授权及会话归属。

- 返回 export_id、状态、source_ref、Workspace 身份、文件清单和完整性，不把全部数据复制进模型上下文。
- query.get/cancel 只操作当前主体有权访问且属于本次分析的取数任务；内部导出重试复用确定身份，不成为独立页面功能。
- provenance 关联 Dashboard/Revision/Panel/Target、实际来源查询哈希、导出 ID、主体、资源、变量、绝对时间、文件 Hash 和完整性。
- 工具内部默认交付完整查询结果；明细数据仅来自明确的已发布查询。两类查询共用对象权限、预算和统一数据处理。
- 不能通过通用遥测工具绕过已选对象或来源限制，未配置明细不能临时拆解聚合。
- 服务端投影负责来源、状态和文件交付元数据，不预先替 AI 判断日志模式或异常。

### 5.5 Workspace 分析与 AI 自主结论

~~~text
导出结果落入当前会话 Workspace
→ AI 查看可信清单与实际文件
→ 使用 read、grep、bash 或预装脚本分析日志/指标/Trace
→ 根据用户问题自行判断
→ Chat 输出结论、依据、范围和未完成部分
~~~

- 复用 PlanV5 离线四工具、持久 Workspace、来源授权和配额；服务端导出，Sandbox 不开网络。
- AI 可自由筛选、统计、关联、编写脚本和生成派生文件；文件计算不等于允许新增服务端查询或扩大数据范围。
- 不新增必填阈值、固定诊断规则或必须由服务端计算的异常判定；可选展示阈值只是上下文，异常由 AI 自行判断。
- 结论基于实际数据，区分事实、模型判断与缺失信息；导出成功不等于业务正常。
- 模型接收小型清单及工具输出，通过分段读取/脚本处理大文件，不用固定样本摘要替代文件分析。
- 工作副本与派生文件可变化，服务端不可变来源和 Hash 保留追溯，修改后的文件不自动成为原始数据。
- 用户主要阅读 Chat 结论；需要交付分析产物时复用受控文件发布，不要求额外图表或工作台。
- 来源撤权复用既有 Workspace 访问/计算限制；文件持久化与明确删除遵循 PlanV5，不能通过拷贝、改名规避。

## 6. 人工编辑、AI 创建与发布

### 6.1 AI 创建入口

显式命令内部 ID 为 telemetry.dashboard.create，显示文案可本地化。

Skill 收集名称、Folder、Panel、变量、默认条件、正常查询需求和 Host/Cluster 关联建议，并通过受授权 Catalog 确认实际字段。候选歧义时询问用户。

AI 可以生成 builder 或 dsl 的 Panel 配置及同模式的明细查询；变量只能生成构建器配置。展示查询验证图表类型，明细查询验证文件 Schema，权限均按对象访问/编辑规则检查。生成受版本化 Schema 约束的 JSON Draft，不生成前端代码。模型的“已验证”声明没有发布效力。

create.preview 支持首次无 Draft ID 的内联配置，由服务端按当前主体及幂等键创建个人草稿并返回 draft_id/draft_version，再执行预览校验。后续修改携带该 ID 和预期草稿版本；也可直接引用已保存草稿生成预览。update.preview 对已有 Dashboard 使用相同草稿流程并校验对象授权与发布基线。首次创建尚无 Dashboard 对象授权时检查创建权限和新对象同企业归属，不要求提供一个尚不存在的对象授权。

### 6.2 草稿、Preview 和 Commit 的职责

~~~text
UI/AI 提交 Draft
→ 保存编辑者个人草稿（不反复确认）
→ 发布预览冻结内容、base_revision_id 和 Spec Hash
→ 配置硬校验 + 独立样本报告 + Diff
→ 用户一次确认
→ Action Executor 重新检查权限和发布基线
→ 原子发布 Dashboard/Revision/创建者授权及明确确认的关联变更
~~~

- 草稿不是发布配置，保存不会创建 active Dashboard 或替换 active Revision。
- 所有影响生效配置的创建、发布、归档、Folder 管理和关联变更使用 Preview/Commit。
- Commit 只接收公开 action_ref；私有 Token、完整提交计划由服务端保管，不由浏览器或模型重传。
- 预览可返回经授权的配置和查询详情；“私有计划不可外泄”不意味着禁止编辑者或创建 AI 看见自己提交的公开配置。
- Commit 校验并消费相应草稿版本；预览后草稿修改、放弃或已发布时拒绝旧预览，跨 Action 的重复创建也由草稿事务约束阻止。
- 更新必须冻结 expected_active_revision_id；基线冲突返回差异，不能按后提交者覆盖。
- AI 和 UI 共用相同 Schema、验证、授权、样本、幂等与发布领域服务。
- 重复确认、响应丢失和 Worker 重启通过 Execution ID、幂等键和 ResultUnknown 对账，不能产生重复对象或 Revision。

### 6.3 工具与宿主确认

~~~text
telemetry.dashboard.create.preview / .commit
telemetry.dashboard.update.preview / .commit
telemetry.dashboard.archive.preview / .commit
telemetry.dashboard_folder.create.preview / .commit
telemetry.dashboard_folder.update.preview / .commit
telemetry.dashboard.binding.attach.preview / .commit
telemetry.dashboard.binding.detach.preview / .commit
~~~

.commit 只进入隐藏 Action Catalog。宿主 PendingAction 控件拥有单次确认入口，Tool 模板展示创建/更新业务详情，不恢复 Card、Slot/Binding 或模板 Catalog。

新增 Dashboard 工具接入 PlanV5 自有三个元工具与版本化元数据，不建立另一套模型工具网关。调整 telemetry.* Action 分派时，不能将 Dashboard 发布当作 Collector 操作。

## 7. 资源关联入口

- 绑定配置位于 Host 和 Kubernetes Cluster 页面，可选择当前用户有权访问的 Dashboard。
- 建立/解除关联同时校验资源管理权限、目标资源授权及 Dashboard 访问权限；不通过关联获得 Dashboard 或遥测数据授权。
- 快捷入口只展示当前主体对两侧均有访问权且未失效的关联。
- 打开 Dashboard 时预选当前 Host/Cluster；服务端系统资源过滤应用到所有适用 Panel。
- 用户在仪表盘内可选择其他有权限且适用的资源，绑定不会固定其查询范围。
- Dashboard 页面可显示关联摘要，资源页面是关联管理入口。
- AI 创建中的可选关联建议只限 Host/Cluster，必须在预览中明确展示并复用同一关联服务。
- Kubernetes 下级对象的绑定、UID、传播和精确过滤不进入本期 API、UI 或 E2E 完成条件。

## 8. REST 与数据流草案

~~~text
GET    /dashboard-folders
POST   /dashboard-folders/preview
GET    /dashboards
GET    /dashboards/{id}
GET    /dashboards/{id}/revisions
POST   /dashboards/{id}/execute
GET    /dashboards/{id}/bindings
GET    /dashboard-drafts
POST   /dashboard-drafts
GET    /dashboard-drafts/{id}
PATCH  /dashboard-drafts/{id}
DELETE /dashboard-drafts/{id}
POST   /dashboard-drafts/{id}/preview
POST   /dashboards/{id}/actions/preview
GET    /dashboards/catalog/*
GET    /hosts/{id}/dashboard-bindings
POST   /hosts/{id}/dashboard-bindings/preview
GET    /kubernetes/clusters/{id}/dashboard-bindings
POST   /kubernetes/clusters/{id}/dashboard-bindings/preview
~~~

不新增 Dashboard 导出/下载的公开页面接口。AI 查询工具内部调用取数与文件交付服务，状态通过既有 Run/Tool/Workspace 机制呈现。

路径是待固化的企业域接口草案。Draft 路径校验归属和 draft_version；发布校验 base_revision_id；通用 PendingAction 接口处理确认。

归档与恢复使用现有 actions/preview，遵守 Q42 的非空分组限制和对象/草稿保留规则；恢复后仍须重新预览发布，不复活旧确认计划。发布同时校验对象元数据/生命周期版本。

前端通过 @argus/api-client 使用生成 DTO，mock 与 real 共用同一契约。HTTP、AI Tool 和 Worker 复用领域服务，不各自拼查询、做权限裁剪或维护另一份发布流程。

## 9. 存储与索引

~~~text
telemetry_dashboard_folders
telemetry_dashboards
telemetry_dashboard_drafts
telemetry_dashboard_revisions
telemetry_dashboard_bindings
telemetry_dashboard_exports          # 任务/来源/分片元数据，数据文件复用既有存储
通用审计索引（不足时再增加 Dashboard 专用索引）
~~~

- 所有对象带 enterprise_id，所有关联检查同企业归属。
- 草稿绑定编辑者，新增草稿独立标识；已有对象按 Dashboard + 编辑者至多维护一个 editing 草稿，使用 draft_version，历史已消费草稿保留发布来源。
- Revision 编号在 Dashboard 内唯一，发布事务更新 active 指针并校验旧指针。
- binding 唯一键为 dashboard_id + target_type + target_id。
- 创建 Dashboard 与创建者显式授权在同一事务完成。
- 归档保留历史发布事实与审计，历史数据访问仍要求当前授权。
- 不在 ClickHouse 新增 Dashboard 专属事实表；目录/查询所需能力复用或补齐现有遥测链路。

## 10. 授权、审计与失效

### 10.1 与 Host/Kubernetes 一致的对象授权

~~~text
对象访问：能访问哪些 Dashboard、Host、Kubernetes Cluster
对象管理：能否创建、编辑、发布、归档或管理关联
数据查询/导出：有 Dashboard 访问权，再按 Host/Cluster 对象授权裁剪
~~~

复用 data_authorization_grants 的 user/department/role/service_account 主体体系，增加 dashboard 类型。用户有效对象为直接、部门和有效角色授权并集；ServiceAccount 使用自身直接及角色授权。

有 Dashboard 访问权即可查看图表、查询配置、默认变量和已发布明细定义，不另加配置字段阅读权限。编辑、发布和归档受对象管理能力控制，Folder 不自动授予 Dashboard 访问权。

功能权限建议收敛为 telemetry.dashboard.read/manage：读取、目录、分析和导出复用对象访问，创建与生效配置修改复用对象管理，不另增 inspect/catalog/字段阅读权限。资源页面关联检查目标 Host/Cluster 管理能力和 Dashboard 可见性。

不按 Metrics/Logs/Traces 或遥测字段分权，Signal 仅用于数据源与引擎选择。必要的凭证屏蔽等统一数据安全规则仍适用，不构成另一套细粒度角色权限。

当前代码存在 telemetry.query.metrics/logs/traces、telemetry.sensitive_fields.read 等细分门禁；本期核查并统一收敛 HTTP、Tool、角色、前端、缓存与来源校验，不能只在 Dashboard 特殊绕过旧权限。

同步数据库约束、同企业验证、授权目录、主体加载、契约、角色管理和 Workspace 来源指纹。遥测 Scope 只接收 Host/Cluster ID，不能混入 Dashboard ID。

首次创建检查创建/管理能力，发布事务授予实际创建者新对象访问权。既有 Workspace 启用、容量、健康及使用能力是文件分析运行前提，不替代对象授权，也不新增三信号权限。

样本报告、目录观测值与导出文件属于数据结果，读取配置不等于绕过 Host/Cluster 范围访问这些结果。

### 10.2 审计与安全投影

审计关联草稿、发布基线、Revision、Panel、查询哈希、调用主体、来源、时间、资源、变量摘要、预算、覆盖情况和结果状态；记录绑定变更、授权变化及确认者。

源码审查与提示词不是安全边界。私有 Token、Secret、未经裁剪的大结果不能进入模型或公开日志；经对象授权的配置与查询定义可用于编辑、预览和分析。原始大数据在受控 Workspace 内，通过文件工具分析；可信来源与可变派生文件明确区分。

### 10.3 撤权与历史事实

每次列表、详情、目录、查询、导出/续传、文件分析、草稿保存、绑定和 Commit 都检查相应权限。撤权使该主体旧缓存和上下文失效，不能回退到默认更大范围。

发布新版本不混改已开始的短期执行快照；归档、撤权、硬性查询失效仍阻止继续执行。查询引擎升级不兼容必须明确报错或限制执行，不能仅给警告后执行未经验证语句。

## 11. 前端与 Chat 交付

~~~text
/dashboards
/dashboards/:dashboardId
/dashboards/:dashboardId/edit
草稿恢复入口（包括首次发布前的新建草稿）
~~~

- 目录按对象授权展示，支持 Folder、未分组、搜索和个人草稿恢复。
- 普通查看读取发布版，具备时间、系统资源、自定义变量、自动刷新与状态展示。
- 编辑页明确草稿状态，Panel 编辑仅管理当前图；变量、布局、默认条件归 Dashboard 编辑会话。
- 构建器和 DSL 互斥编辑来源，切换遵循无损规则；编辑与预览不覆盖普通查看。
- 变量弹框只提供构建器；Header/Footer 固定，正文唯一纵向滚动。时间、资源或前置变量变化后重载候选，当前选值消失自动切换“全部”，依赖变量随之更新。
- 仪表盘及 Panel 编辑页面不提供导出按钮或导出配置区，取数后的文件交付仅由 AI 数据查询工具内部完成。
- Host/Kubernetes Cluster 页面管理关联，快捷打开自动设置系统资源筛选。
- Chat 支持授权列表选择、结构化 @ 引用、明确选中对象的恢复、创建预览与宿主确认。
- AI 通过会话 Workspace 的结果文件分析并输出 Chat 结论，复用已有文件状态和交付，不建设 Dashboard 页面联动或额外分析工作台。
- 使用 @argus/ui、Design Tokens、.argus-*、模块 i18n；zh-CN/en-US、light/dark、桌面键盘和读屏纳入验收，文件低于 2000 行。

[demo.html](./demo.html) 已提供本轮交互原型及二十项前后变化说明。它模拟草稿恢复/发布冲突、变量构建器/自动回退全部、Builder/DSL 无损切换、资源入口、只读身份和 Chat 内部文件分析，并展示丰富图型、日志探索、Span 瀑布和自由布局；无 Dashboard 导出功能。详情见 [原型说明](./demo/README.md)。存储在浏览器、数据及执行均为模拟，不替代正式 API、引擎验证和 Kubernetes E2E。

### 自由布局与可视化扩展

布局应保存 x/y/w/h，提供标题把手拖动、右下角宽高缩放、12 列吸附、最小尺寸、边界约束与碰撞处理；键盘操作、取消拖动及刷新恢复纳入验收。半宽/全宽按钮或仅重排顺序不能视为完成。普通查看不可修改布局，编辑只写个人草稿，统一发布。

展示层应按结果形状注册图型与配置，复用 @argus/ui/ECharts。Demo 展示 Time series、Stat、Gauge、Bar gauge、Bar chart、Pie、Histogram、Heatmap、State timeline、Scatter、Table；正式数据适配、单位、计算口径和支持矩阵仍须验证。日志继续使用 Argus KQL，日志/Trace 需要可检索、展开和下钻的交互。APM 展示覆盖 Q32 的服务/实例/接口总览、指标、拓扑及链路排查清单；采样统计口径见能力讨论的待澄清项，数据能力缺失不能仅以插件已启用掩盖。

## 12. 实施任务与顺序

### Task 1：人工仪表盘闭环

详见 [Task 01](./task-01-dashboard-workbench-and-runtime.md)。

~~~text
T1.0 遥测能力核实与补齐
→ T1.1 契约、双编辑模式、变量与状态机
→ T1.2 存储、个人草稿与对象授权
→ T1.3 发布 Preview/Commit 与并发基线
→ T1.4 Query Runtime、Catalog、总预算
→ T1.5 REST/API Client
→ T1.6 Workbench
→ T1.7 Host/Cluster 关联入口
→ T1.8 测试与临时 Kubernetes Namespace E2E
~~~

### Task 2：AI 创建与 Chat 分析

详见 [Task 02](./task-02-ai-dashboard-create-and-analysis.md)。

~~~text
T2.0 显式命令、结构化引用与 Gateway 接点
→ T2.1/T2.2 AI Draft 与 Catalog
→ T2.3/T2.4 同契约验证与用户确认发布
→ T2.5 授权列表选择及 @ 引用
→ T2.6/T2.7 查询规划、导出任务、Workspace 交付与 provenance
→ T2.8/T2.9 文件分析、覆盖清单与 AI 自主结论
~~~

Task 2 依赖 Task 1 真实人工闭环，不根据历史 M0-M10 验收推定当前分支已完成。

### P2V 检查清单

- [ ] P2V-CONTRACT-01：Folder/Dashboard/Draft/Revision/Panel/QueryTarget/Variable/Binding Schema。
- [ ] P2V-ADR-01：Dashboard 独立领域、单一编辑来源、绑定仅导航、Chat 查询边界。
- [ ] P2V-OPENAPI-01：草稿、发布基线、授权、执行、目录和公开预览契约。
- [ ] P2V-TOOL-01：Gateway 分类、版本化元数据、list/get/query 与隐藏 Commit。
- [ ] P2V-DB-01：Migration、sqlc、个人草稿、对象授权与索引。
- [ ] P2V-DOMAIN-01：草稿/发布版分离与 active 指针原子更新。
- [ ] P2V-VALIDATE-01：硬校验、独立样本状态、双模式无损转换与变量图。
- [ ] P2V-ACTION-01：生效配置的 Preview/Commit、幂等、冲突与恢复。
- [ ] P2V-AUDIT-01：编辑、发布、查询覆盖、授权与关联审计。
- [ ] P2V-EXEC-01：统一 ExecuteDashboard、系统资源筛选与变量适配。
- [ ] P2V-EXEC-02：三种原生结果与首期构建器能力矩阵。
- [ ] P2V-EXEC-03：共享预算、范围指纹缓存、取消、partial 与空范围拒绝。
- [ ] P2V-EXEC-04：UI 查询与 AI 导出共用执行语义，AI 通过 Workspace 文件分析。
- [ ] P2V-EXPORT-01：AI 查询工具内部使用已发布查询、稳定分页/分片/续传与来源哈希/完整性。
- [ ] P2V-EXPORT-02：会话 Workspace 流式交付、容量、撤权、取消与恢复。
- [ ] P2V-AUTH-01：三信号/字段细分门禁统一收敛到对象访问与编辑控制。
- [ ] P2V-CATALOG-01：真实数据目录、字段映射、类型和值发现。
- [ ] P2V-WEB-01：目录、个人草稿恢复、详情、编辑与发布预览。
- [ ] P2V-WEB-02：时间、资源、变量、布局和结果状态。
- [ ] P2V-WEB-03：双语、深浅色、桌面键盘与读屏。
- [ ] P2V-BINDING-01：Host/Cluster 页面配置与解除关联。
- [ ] P2V-BINDING-02：快捷打开预选资源并强制过滤所有适用 Panel。
- [ ] P2V-BINDING-03：资源删除、关联失效、双方撤权和用户切换范围。
- [ ] P2V-AI-01：版本化 Skill 激活、结构化引用、追问继承与 Run 恢复。
- [ ] P2V-AI-02：未指定对象先列授权仪表盘，用户明确选择后执行。
- [ ] P2V-AI-03：文件来源、数据完整性、未知状态与泛问覆盖检查。
- [ ] P2V-AI-04：AI 生成 builder/DSL Panel、构建器变量和严格 Draft JSON。
- [ ] P2V-AI-05：预览、宿主单次确认、隐藏 Commit 与幂等。
- [ ] P2V-AI-06：仅 Host/Cluster 的可选关联建议。
- [ ] P2V-AI-07：从冻结发布版恢复展示/明细查询，记录文件来源，拒绝未配置的原始数据获取。
- [ ] P2V-E2E-01：UI 草稿、恢复、发布、冲突、运行与审计。
- [ ] P2V-E2E-02：AI 两种查询编辑模式创建并确认发布。
- [ ] P2V-E2E-03：列表选择/@、泛问全覆盖、具体问题按需、Chat 结论。
- [ ] P2V-E2E-04：Host/Cluster 快捷入口与无绑定独立访问。
- [ ] P2V-E2E-05：对象/数据双层授权、跨企业、相同授权版本不同主体、统一数据安全处理。
- [ ] P2V-E2E-06：Redis 清空、重启、重复确认、版本变化、partial 和失败恢复。
- [ ] P2V-RELEASE-01：官方 Harness 的归属清理与发布门禁。

## 13. 关键验收场景

| 场景 | 预期 |
| --- | --- |
| 修改多个 Panel、变量和布局 | 自动/手动保存个人草稿，最终确认一次；查看者继续看到旧发布版 |
| 两位编辑者基于 R1 发布 | 第一位发布 R2；第二位收到基线冲突与差异，不能覆盖 |
| Builder 转 DSL 后加入无法反解的语句 | 保留 DSL，不丢表达式、不创建第二份有效编辑来源 |
| 合法查询没有样本或样本后端临时不可用 | 明确警告，可在硬校验通过后发布；不显示数据正常 |
| 未绑定 Dashboard，从目录或 Chat 打开 | 可按对象授权访问，并按用户数据权限查询 |
| 从主机 A 快捷打开 | 所有适用图受 A 的系统资源范围限制，即使未引用 $host |
| 绑定 A 后用户选择有权访问的 B | 可以查询 B；绑定不限制数据范围 |
| 用户只能访问一部分资源 | 下拉、目录、查询、AI 均使用相同裁剪，空范围不扩大 |
| 未指定仪表盘而泛问 | 列出授权候选，等待用户选择；不自行选择或立即查询 |
| 已选仪表盘而泛问 | 检查所有适用图，缺失、失败与超预算明确进入覆盖报告 |
| 具体问题 | 按需导出已选仪表盘内相关图并分析文件，不使用未发布明细来源 |
| 分析期间有人发布新版本 | 同次分析不混用版本，仍实时检查权限与归档状态 |
| Dashboard 页面未打开 | Chat 导出到 Workspace 后分析，使用默认条件、继承的明确条件与问题 |
| 聚合图没有明细来源 | 只导出聚合值，不自动取得原始日志 |
| 已配置明细来源 | 使用发布的明细查询，记录实际范围、Hash 和完整性 |
| 导出超预算/空间不足 | 标记未完成范围，不静默抽样或冒充完整数据 |
| 当前自定义变量选值在新候选中消失 | 自动切到全部并更新依赖，仍保持授权和系统资源范围 |
| Dashboard 查看与编辑页面 | 不出现导出入口/导出来源配置；AI 取数工具内部交付文件 |
| 追问“那昨天呢” | 继承已明确的仪表盘、资源和变量，只按用户要求改时间 |
| K8s 下级对象绑定请求 | 本期不提供该能力，不能以名称过滤假冒 UID 精确绑定 |

## 14. 测试与完成定义

除通用完成定义外，必须覆盖：

- Domain、Repository、授权、Schema、语言适配、无损转换、变量依赖及生命周期测试。
- 配置硬错误与样本 no_data/unavailable 的差异；后者不掩盖前者。
- 服务端个人草稿持久化、隐私、版本并发、发布基线、首次无 ID 创建、旧预览失效、跨预览去重与恢复。
- Host/Cluster 系统资源范围、Dashboard 访问/管理、取消三信号/字段权限细分、缓存命中重新授权。
- 空资源范围拒绝、不同主体相同授权版本、目录截断、All、多值编码和跨信号参数适配。
- 总预算覆盖目录、Panel、明细导出、分页和 Workspace 空间；查询覆盖和文件完整性分层。
- 文件内容/Hash、稳定分页/续传/去重、显式 limit 与分页区分、Metrics instant/range/step、Trace 嵌套完整性。
- AI 实际使用 read/grep/bash/脚本分析文件并自行判断，不依赖固定摘要或必填阈值；派生文件不冒充原始来源。
- 追问继承、选值消失自动回退 All、空候选保留全部状态、Workspace/Sandbox 不可用、满额、中断、来源撤权和跨会话拒绝。
- Chat 结构化选择、没有对象先列清单、已选对象泛问全覆盖、具体问题按需。
- AI 创建可写受支持 DSL，AI 分析仅导出已发布展示/明细来源；文件来源、哈希、上下文与对象权限防伪。
- Playwright mock 与 real 的同 DTO、双语深浅色、桌面交互、无 Dashboard 页面依赖的 Chat 分析。
- 临时 Kubernetes Namespace E2E 覆盖三信号真实数据、重启、Redis 清空、授权变化和失败恢复。
- 成功或失败均按归属清理临时 Namespace、PVC、Topic、Bucket、Lease、测试绑定与临时诊断资源；保留脱敏验收证据，保护正常部署与无关资源。

本计划更新不等于代码实现完成；所有实现与发布复选项保持未完成，须由对应真实证据关闭。

## 15. 文档与架构影响

本轮确认的变化：

- 在同一 PostgreSQL 领域内增加服务端个人草稿，草稿保存不走用户确认，生效配置继续 Preview/Commit。
- Dashboard 加入既有对象授权体系，现有三信号/字段门禁统一收敛为对象访问与编辑；不新建权限系统，不扩大 Host/Cluster 范围。
- 统计图保留 builder/DSL 两种互斥编辑来源，变量只用构建器，继续复用三个 Engine。
- 绑定收敛为 Host/Cluster 导航入口；删除其作为查询集合限制的语义。
- Chat 支持授权列表选择和 @，追问继承明确条件，不依赖 Dashboard 页面。
- 仪表盘没有导出功能或配置。AI 数据查询工具内部将已发布查询结果交付到 Workspace 后分析，额外明细仍需明确的已发布查询；复用既有进程与存储。
- 自定义变量选值在新时段/条件下消失时自动回退全部，不修改默认配置；对象授权、系统资源及上游条件仍生效。
- 明确 Catalog、构建器能力、结构化消息、Skill 激活和 Gateway 分派的新增工作。

实施时同步更新 docs/00、02、04、05、09 及相关 ADR、OpenAPI 和生成契约，区分当前实现与计划扩展，不能提前宣称已有授权类型或接口已经落地。

PlanV5 的 Tool 自有模板、宿主单次确认和 Bridge 限制保持不变。创建预览可以展示业务详情；分析默认输出 Chat 结论，模板 Bridge 不发起查询。
