# 新增、修改工具与契约草案

以下为 Plan11 候选实现的协议设计。JSON 中的尖括号值是占位符，不是可执行请求。正式输入 Schema 来自生产注册，输出类型和权限复用领域服务；候选代码通过不代表正式环境已升级，验收见实施记录。

主要修改契约由原6项增加为8项后，D57补 `dashboard/catalog` 的来源分项、量值语义和查询配方输出，以及 `draft.validate` / `publish.preview` 的确定性显示单位校验行为，共11项。三信号查询、仪表盘列表、采集预览、旧目录采样语义及显式资源确认继续保留。公开业务工具总数仍为133。

## 1. 三元工具：输入结构不变

| 元工具        | 必填输入                  | Plan11 改动                                       |
| ------------- | ------------------------- | ------------------------------------------------- |
| tool.search   | category                  | 枚举增加 observe，更新分类说明/版本               |
| tool.describe | category、name            | 枚举增加 observe，返回新增导航元数据与业务 Schema |
| tool.invoke   | category、name、arguments | 增加 observe 路由，执行新版业务 Schema            |

三者继续 additionalProperties=false。resource_types、signals、operation_kind、result_kind **不是三者的顶层输入**。tool.search 不接受 query、limit、cursor；资源/指标目录的 search、limit、cursor 是对应业务工具 arguments 内的字段。

```json
{ "category": "observe" }
```

Search 返回当前类别完整授权目录后，下一步可以 Describe：

```json
{ "category": "observe", "name": "resources.list" }
```

得到成功的 Schema 响应后，才构造 Invoke：

```json
{
  "category": "observe",
  "name": "resources.list",
  "arguments": {
    "resource_types": ["monitoring_target"],
    "integrations": ["mysql"],
    "limit": 20
  }
}
```

调用链继续约束主体、Run、授权版本、分类、规范名称与工具版本。提示中的工具引用只提供 category/name，不代替 Describe；不允许同批次 Describe/Invoke 同一工具。

## 2. 工具描述元数据

| 字段           | 草案类型 | 约定                                                                                                                  |
| -------------- | -------- | --------------------------------------------------------------------------------------------------------------------- |
| resource_types | string[] | 适用业务对象类型；源自共享类型词表，允许 dashboard/connector 等非遥测业务对象；不能直接作为 QueryScope 的类型枚举     |
| signals        | string[] | 取 metrics/logs/traces；无遥测语义的工具为空数组                                                                      |
| operation_kind | string   | discover/query/prepare/preview/control/deliver，描述主要操作，不替代 Risk/ReadOnly/确认策略                           |
| result_kind    | string[] | 允许多形态结果；如 timeseries/instant_vector/scalar、resource_list、metric_catalog、pending_action、file、interaction |

保留讨论中的 result_kind 字段名，明确类型为数组，避免 metric.query 被错误限制成只返回 timeseries。prepare 适用于草稿与计划准备，control 适用于取消/恢复；四个最初示例枚举不足以覆盖全部 130 个工具。

拟增加在版本化 Discovery/Manifest；Search 返回紧凑枚举，Describe 返回完整说明。完整数组按元数据投影保留，不按普通业务结果裁剪。空数组表示该维度不适用，不表示所有类型或全部权限；未知值在注册/生成阶段拒绝。

```json
{
  "category": "metric",
  "name": "query",
  "resource_types": [
    "host",
    "kubernetes_cluster",
    "kubernetes_namespace",
    "application",
    "monitoring_target"
  ],
  "signals": ["metrics"],
  "operation_kind": "query",
  "result_kind": ["timeseries", "instant_vector", "scalar"]
}
```

这些元数据不设置 UI 模板、不执行路由、不读取数据库、不授予权限。已有 Risk、Required、ExecutionMode、输入/输出 Schema、Template Binding 继续独立生效。签名同时包含元数据与分类导航，修改后 Describe 缓存、Catalog Revision、Run 快照按既有版本边界失效或受控恢复。

## 3. 新增 observe/resources.list

### 输入

| 字段           | 语义                                                                                                    |
| -------------- | ------------------------------------------------------------------------------------------------------- |
| resource_types | 可选类型数组；遥测选择身份为 host/kubernetes_cluster/kubernetes_namespace/application/monitoring_target |
| families       | 可选家族数组；对有明确家族的对象筛选，未知或不适用不得默认匹配全部                                      |
| integrations   | 可选集成数组，使用当前目录的稳定 key                                                                    |
| search         | 可选普通名称搜索，非工具关键词检索                                                                      |
| limit、cursor  | 服务端分页；建议默认 20、最大 100，游标绑定完整查询条件                                                 |

未给类型只枚举授权的可查询对象，不执行任何业务查询。相同维度多值为 OR，不同维度为 AND；先应用授权、再筛选、再分页，不能先取一页然后在内存里冒充全目录过滤。服务名搜索不创建新的 service 授权根；返回 application 引用和已授权的服务限定信息，再使用 application/service.list 获取详细身份。

### 输出

items 包含 ref={type,id}、name、environment（如有）、family/integration 信息、授权允许公开的 parent_ref。包含 has_more、next_cursor、complete、observed_at；服务端不能可靠提供 total 时不返回伪总数。名称相同返回多个候选，不自动选择首条。

底层复用 observationscope.ResourceDirectory；按集成/家族筛选如需扩展查询，应在同一领域实现供门户和 Agent 使用。不要把完整 Host/Collection 凭据配置放入此目录。

## 4. 新增 observe/capabilities.get

### 输入

- resources：1～20 个明确的 {type,id} 引用；首版上限需经性能测量确认，不能由模型扩大。
- signals：可选 metrics/logs/traces 子集。
- sections：可选 collection_state/sources/dashboards/next_tools；默认紧凑摘要。
- cursor：有界节的后续读取游标，绑定资源、节、主体、授权版本和配置版本。

不提供模糊名称或 all_authorized；应先解析目标。没有实时采样请求参数，不自动连接数据库、SSH、K8s，也不因查看能力创建采集实例。

### 输出

| 区域             | 内容                                                  | 真实性规则                                                                   |
| ---------------- | ----------------------------------------------------- | ---------------------------------------------------------------------------- |
| resource         | 当前稳定身份、名称、类型                              | 资源读权限不足则拒绝，不返回跨权限名称                                       |
| collection_state | 配置/生效 Collector ID、归属核对状态、期望启用、版本、健康、新鲜度及观察时间 | Collector ID 不等于已核对 Host/Cluster；配置与生效不混同，缺失明确未知；几个状态不得合并成“正常” |
| sources          | source_type、source_key、集成/版本、signals、采集范围 | 来源存在不代表已收到数据，不返回凭据或任意接入地址                           |
| dashboards       | 有权发现的适用仪表盘引用、模板版本、适用原因          | 无 dashboard.read 时该节标记 not_permitted，不导致其他授权节失效             |
| next_tools       | category/name、用途与缺少的前件                       | 只引用当前可用工具，不返回填好的可执行脚本或私有 Token                       |
| completeness     | 每节 complete/has_more/原因及 observed_at             | 服务故障和截断不能转换为空列表后称“没有能力”                                 |

对象本身未授权时不降级返回。可选节权限不足时不泄露该节数量/身份；对象已授权但子节服务失败可保留其他节，并明确 partial。首次响应不承诺跨模块事务一致，分别记录 observation/config revision；调用业务工具时仍重新验证。

## 5. 新增 observe/catalog.query

复用 [ObservationCatalogInput/Result](../../internal/telemetry/observation_catalog.go) 与 [ObservationRecipes](../../internal/dashboard/observation_recipes.go)。工具适配器只负责主体、严格 Schema、结果投影与 Presentation。

| 输入                   | 内容                                                                       |
| ---------------------- | -------------------------------------------------------------------------- |
| scope                  | 现有 QueryScope：mode、resources、resource_types、source_type、source_keys |
| signal                 | metrics/logs/traces                                                        |
| kind                   | metrics/fields/values/sources；metrics 只配 metrics 信号                   |
| metric、field、filters | 目录上下文；字段值查询必须带 field；字段归属按服务契约验证                 |
| from、to               | 必填冻结的绝对时间，使用服务端时钟，沿用现有时间窗口上限                   |
| search、limit、cursor  | 业务目录的有界搜索和分页                                                   |
| observed_only          | 默认只看当前范围内已观测；用户需要时可查看支持但未观测条目                 |
| selected_values        | 复用当前候选存在性核对，不能绕过值数量/授权限制                            |

模型首版只暴露 scope.mode=explicit；复用同一 QueryScope 领域实现，工具 Schema 不开放 all_authorized，门户既有模式保持。若明确请求“所有有权资源”，先完成授权目录分页，再冻结明确对象集合和成本；超过查询上限或目录无法完整获取时分批或说明受限，不能静默截断后声称全量。空集合不能扩大范围，资源目录列出全部候选也不自动执行全范围采样。

返回指标名、类型、单位、描述、来源及版本、标签、observed、推荐表达式及依赖指标；其他 kind 返回字段/值/来源。保留 complete、has_more、next_cursor、warnings。同名但类型/单位/来源版本冲突的指标分项返回，不能名称去重。

推荐语句只能来自当前版本化模板/合法语言契约；保留配方 ID、版本、适用来源、所需指标、结果单位和缺样本规则。元数据缺失标未知；Counter/Gauge、经典/Native Histogram、分母与采样窗口分别验证。支持但未观测条目不能作为健康证据。

D50补充已观测条目的可选query_methods：由实际类型给出representation及count_expression/sum_expression/mean_expression、count_unit/mean_unit和局限。histogram使用经典_count/_sum家族，exponential_histogram使用原生函数，summary不发明桶。未知/支持但未观测条目不填。表达式仅为方法，仍需在已授权source_keys、标签和固定时间窗中调用正常metric/query；还需检查后缀同名冲突。count为观测数，mean保留实际测量单位，不能把count标为秒或mean当P95。observe/catalog.query输出v2、发现revision2；输入和三元工具不变，公开OpenAPI/Go/TS同步。

资源发现、采集配置、指标目录和真实样本有不同权限。目录按资源读能力与显式授权工作，不要求 Dashboard 创建/管理权限；不得通过这种复用降低原 dashboard.catalog 的权限要求。

D62当前目录补充：observe.catalog.query输出argus.observe_result/v4、发现revision5；page_counts只计算已授权当前工具返回页的条目/去重名称/来源ID/版本及字段/值数量，不增加扫描或代表全局总数。筛选目录的complete=false不等于传输裁剪，实际数值仍需使用已发现方法调用正常查询；result_ref不是Workspace路径。三元输入、业务查询参数及权限不变。

## 6. 修改三信号查询

| 公共身份     | 内部注册 ID                | 修改                                                                      |
| ------------ | -------------------------- | ------------------------------------------------------------------------- |
| metric/query | telemetry.promql.query     | 新增 scope，保留 PromQL 与预算；返回真实范围和来源                        |
| log/query    | telemetry.kql.query        | 新增 scope，保留 KQL 与 pipeline；明确应用日志/基础设施运维日志的授权路径 |
| trace/query  | telemetry.skywalking.trace | 新增 scope，保留 GraphQL/application_view 互斥规则、service_ids、RED 窗口 |

scope 与 resource_ids 必须恰有一种；模型 scope 仅开放 explicit。两者同时给出、类型与 ID 不匹配、非法来源、explicit 空集合均受控失败。旧 UUID 路径如保留，复用 ResolveQueryInput，不能增加第二套来源选择实现。应用 service_ids 必须属于已选且授权的 Application，K8s Namespace 需验证当前 UID 与可信归属。

共享返回查询上下文：有效 typed resources、选中来源、from/to、实际采样/聚合单位、预算与 complete/partial/no_data 原因、结果引用和服务端时间。与现有 Data/ModelSummary/QueryFile/Presentation 共同版本化，不把大结果直接塞入模型上下文。Workspace 不可用时沿用明确不可用状态，不悄悄回退另一条无审计取数通道。

## 7. 修改 dashboard/list

保留 search/offset/limit；增加 resource_types、resources、integrations、signals，可选仅看 managed。各筛选交集及多值 OR 必须写入正式 Schema。对象条件先做身份/授权核验，不接受展示名称充当资源引用。

items 在已有 id/name/description 之外增加 managed_template_key/version、applicable_resource_types、integrations、signals、对当前请求的 suitability/match_reasons、data_state。data_state 为 not_checked，sampling_performed=false，附明确原因：本次目录没有执行采样检查，不能解释为无样本或登记滞后。列表不逐面板执行。只返回授权公开元数据，不泄露私有对象和查询语句。

适用性采用确定性规则：模板/定义支持的资源类型、集成、来源与信号相交，并说明全部适用/部分适用；部分适用不能默默排除用户选定对象。无合适仪表盘时返回明确匹配结果，模型可回到 catalog.query。名称搜索仍可用，但不能作为类型判断的唯一依据。

Server/Worker 使用同一构造与授权发现路径；可共享注册/领域工厂，不要求调用启动基础设施的整段应用函数。模型治理保留专用排除。catalog.resources 和 catalog 的旧业务入口不删除，只收敛公共领域实现与权限适配。

## 8. 错误、版本与成本

- 字段错误复用 TOOL_INPUT_INVALID 的路径提示；未知分类、工具、范围和授权遵循现有受控错误，不返回底层凭据或目标响应。
- Unsupported、no_data、stale、partial、unavailable 通过结构化结果区分；鉴权失败不能伪装成无数据。
- 分页游标绑定主体、授权版本、过滤、时间、来源和目录语义；撤权或上下文变化拒绝旧游标。
- 跨次描述缓存允许复用有效版本，但每次调用重新校验实际权限和 Scope。类型/来源/输出元数据变更必须进入版本签名。
- 资源目录、能力与指标目录的数据库/后端成本计入 Run 预算，查询/重试/分页不能绕过额度。预算限额在真实规模测试前记录为候选值，调整保留依据。

2026-10-31 实测补正：`queryengine.ErrBudget` 与共享 Run 的 `telemetry.ErrQueryBudget` 统一转换为 `TOOL_QUERY_BUDGET_EXCEEDED`，保留底层哨兵供原有领域处理。单次执行超限提示先核对本轮余额、保留用户范围并优先查目录；本轮预算不可用时明确 `budget_scope=run`、`retryable=false`。失败且实际成本未知仍保守扣留，不恢复额度。

## 9. 三元前件与 Workspace 交付补正

三元输入仍为 Search 的 category、Describe 的 category/name、Invoke 的 category/name/arguments。版本 7 将持久化 Search 前件提前到 Describe 返回 Schema 之前检查，并在缓存命中前检查本 Run、主体、步骤、模型轮次、完整列表中的名称与版本；Invoke 继续检查完整链路。`TOOL_SEARCH_REQUIRED` 只返回可恢复的分类/名称说明，不返回未取得前件的 Schema，也不放宽最后的执行授权。

四个 Workspace 工具继续作为顶层工具，分别说明读行、完整保存、替换文本、离线执行的职责。write/edit 返回本次操作的 `file_saved=true`、`delivery_created=false`，并提供仅在用户要求导出/下载时使用的 workflow/publish_file 身份引用；该引用仍需 Search/Describe 和当前授权。文件保存不自动发布中间产物。契约版本与 Workspace RPC 运行身份分开，旧 Run 不静默获得新契约。

以上为既有三个元工具和四个 Workspace 工具的协议/说明补充，未增加顶层工具或第 134 项业务工具。数值助手原件校验见第 11 篇。

原生日志输入再补充同编译器前置验证：在授权范围已经解析、计量后端尚未调用时，使用执行端相同的 KQL CompileQuery 拒绝不支持的文档。TOOL_QUERY_INVALID 只返回固定支持语法、字段目录引用与 query_validation 标记，不透传 SQL、内部表或编译错误原文，也不预留 Run 查询预算。支持的 Argus DSL 以 `*` 或谓词起始，聚合用 stats，不把 Syslog/summarize 等任意上游 KQL 当作已支持。真实执行仍重新编译并执行范围与来源限制。

PromQL 同样在进入计量后端前使用执行端相同版本及 Options 的解析器，拒绝无效语法、类型错误，以及范围查询不支持的顶层 matrix/string；合法 instant matrix 继续支持。错误使用固定消息和 `budget_reserved=false`，不透传表达式中的私有文本。指标工具发现修订升至 6，进入原有签名和快照版本处理。Trace 专家文档已由既有 `ValidateToolDocument` 在调用后端前验证。上述验证不改变授权、资源/来源、时间或预算上限；实际执行或失联请求成本未知时仍保守扣留。

r25 轨迹补充既有采集工具的身份说明：`collection.get.id` 使用采集实例 ID，取自 collection.list 项 id 或 capabilities 的 collection_state 项 id；目标、Host、Cluster、Collector ID 仍是不同身份，不增加自动转换或别名。collection.list 的 search 只匹配采集实例名称；可按 integration 分页后精确匹配 item.target.kind/id，也可用目标引用查询 capabilities。两项发现修订升至 2，字段结构不变。

capabilities 发现修订升至 2，明确 `sampling_performed=false/data_state=not_checked`、采集配置/健康元数据与历史遥测可用性的区别；停用、stopped、freshness=no_data 不能单独证明历史数据不存在。next_tools 是后续工具身份引用，不保证每种信号都有配置、来源或样本。验收使用正式 sampling_performed 字段，不再另造易混淆的 observed_data_checked。


## r29 仪表盘条件与根查询范围补正

首次 `dashboard/context.resolve` 必须提供 `changes.resources`（`all=false, ids=[实际授权 ID]` 或明确的 `all=true, ids=[]`）及 `/resources` 证据，或者继承已确认的资源条件。明确且有证据的资源/全部条件重置仍可恢复默认范围；AI 选择不能覆盖已有用户条件。没有资源确认时返回公开 `DASHBOARD_RESOURCE_SCOPE_REQUIRED`，事务不保存半成品。后续 `changes={}` 仍允许继承已确认条件。`dashboard/query` 在执行不可变根 context 前重查该事实，阻止旧隐式全部 context；下钻继续沿原父范围。

`evidence.path` 是条件相对 JSON Pointer：`/time`、`/resources`、`/variables/<name>`、`/local_values/<panel>/<name>`，整体重置为 `/`。路径不包含输入对象的 changes 层。路径错误、证据数量或原话不匹配时返回固定公开修正说明、required_paths 及 condition_evidence 标记；模型错误投影仅允许这些已审查的标记错误，其他内部错误继续隐藏。修正证据必须保留用户资源和时间条件，不能通过删除条件规避校验。

两项业务输入版本升为 v3；context.resolve 发现修订 5，query 修订 4，进入原签名/快照/Describe 缓存版本边界。三元工具和业务工具数量不增加。实现及 PostgreSQL/真实模型复验结果另记，原 r29 失败不改写。

## 原件与解释元数据补正

D16 的 `workspace_file.sha256` 位于 Agent 工具结果投影，明确对应原查询文件字节；`read`、`bash` 等后续调用的 `result_hash` 标识各自结果信封。部分投影记录实际截断或工具声明来源，数据查询本身的完整性继续由其独立元数据表达。严格 Schema 与客户端已同步，不新增业务工具或三元输入。

离线助手的可选 `--metric-type` 来自实际指标目录，作用是保留 Counter/Gauge/Histogram/unknown 的解释边界；它不是工具导航字段或 `tool.search` 筛选参数。`resource_types` 仍表示工具适用的业务对象，静态导航仍只有本篇定义的四个维度。相关计算、报告与证据版本见第11篇。

D17 进一步以 `argus_meta.input_metrics`、完整性/来源及 `result_is_original_metric` 传递实际授权输入序列元数据。`metric/query` 输出版本为 `telemetry.promql.query/v2`，发现修订 7；输入字段与三元参数不变。离线 `--unit/--metric-type` 只作一致性提示，缺少旧文件元数据时类型仍 unknown；输入单位/类型不认证派生表达式输出。Workspace/证据 v6 另记，业务工具数量仍 133。


D20 的 delivery_summary_v2 改变验收提示与最终答复承载方式，不增加任何工具输入。CLI 的 --plan11-delivery-summary 仅接受 Plan8 plan11-compare 下显式 candidate-only；旧新比较和其他阶段拒绝。默认仍为 strict_json_v1。报告原始 Hash/大小、完整 JSON、实际查询与所有面板继续校验；最终引用须对应真实交付，若附 JSON 则完整一致。正式产品默认可用业务摘要与现有证据视图/报告分工，显式最终 JSON 请求仍优先。

D21 的 --locale 是现有离线报告命令参数，不是 tool.search/describe/invoke 或 metric/query 输入。Workspace 契约 v7 描述完整双语报告组装；机器 metric_evidence/v6、dashboard_evidence/v1、授权查询输出 v2 及 133 项业务工具不变。旧 Run 仍按其快照版本处理，新报告语言不改变原始数值或来源。

## 公开展示引用与查询根的区别（D26）

ToolPresentation.resource_refs 与 ConversationResourceRef 的公开导航枚举一致，包含 Host、KubernetesCluster、KubernetesNamespace、Application、MonitoringTarget、Dashboard、Connector。公开展示 DTO 补齐新三类引用，采用 canonical OpenAPI 生成并检查 Go/TypeScript 类型。该补齐不增加业务工具或元工具输入，不扩大具有独立含义的操作 ResourceRef；观测 QueryScope 仍只有五类查询根，Dashboard/Connector 引用不能作为遥测数据授权。

D40 候选输出版本：capabilities.get 为 argus.observe_result/v2、发现 revision3；输入仍为原 resources/signals/sections/cursor。configured_collector_id 与 effective_collector_id 来自当前采集事实，placement_state 为 requires_authorized_resolution 或 unassigned；不增加主机/集群信息或权限，后续仍需实际授权关联读取。Collector 绑定和 assignment epoch 同时纳入节游标签名。

D41 候选：capabilities.get 发现 revision4、输出 argus.observe_result/v3；根级 sampling_performed=false、data_state=not_checked 和 data_state_reason 明确限定为本次元数据能力查询的活动，不是采集/历史数据结论。输入仍不变。


D43 候选：collection.list/get 发现revision3/输出argus.monitoring_result/v2，返回本次元数据请求活动标记；capabilities发现revision5/输出v4，collection_state节coverage=collection_instances_only与覆盖说明。输入、权限、节枚举不变，SDK/来源仍经既有授权工具核对。


D45 候选：capabilities发现revision6/输出v5，sources项含source_id/entry_kind；sources节page_entry_count/page_source_count仅当前页版本项/去重来源计数，不作全目录总数；保留source_key完整选择器和历史修订。应用next_tools补endpoint.list引用，仍当前权限/范围过滤、Search/Describe前件不变。


D52为ToolResultProjection v1增加可选布尔inline_original_complete（缺失=false），仅框架生成的自有三信号查询可认证，原件上限32768字节、非partial、实际summary与封存JSON严格等价；超限/脱敏/裁剪/精度变化不认证。workspace_file的路径、SHA和required_before_next_model_round交付不变，instructions改为按当前query_evidence.inline_original_in_context选择直接分析或读取原件。此字段是输出事实，不是Search/Describe/Invoke参数，也不是新业务工具。当前上下文认证另结合持久原生投影、当前授权/Workspace身份；旧标记和外部自报无效。

D53的query_evidence新增inline_exposure_verified，为已成功实际provider请求的完整小原件曝光凭据；与当前可见inline_original_in_context、物理read_status分别表达。凭据保存于既有model_calls的Migration52私有有界JSON列，只含查询ID/SHA/大小/实际请求Hash，最多64项；不新增工具参数或公有查询结果字段，不回填历史事件。只有当前授权/Run及可用副本仍匹配时复用；需要缺失细节或大/截断结果仍读原件。

D54新增输出：目录ObservationMetric.semantics为可选的来源限定内置量值定义（quantity_kind、recommended_display_transform、definition_id/version、origin、notes）。原生PromQL argus_meta.input_metric_semantics逐项绑定name/source_key/type/unit及同一semantics，描述输入量值；只有result_is_original_metric等原元数据完整时可用于直接展示，派生表达式须独立分析单位。未知项缺省，不把所有单位1当比率。PromQL Inputv2/Outputv3/Discovery10；observe.catalog Outputv3/Discovery4，Input及Search/Describe/Invoke参数不变。报告新增manifest_checksum_status=not_requested|verified；false旧标记不是mismatch，真实mismatch仍错误。Workspacev8，指标v6的可选字段与仪表盘证据v2格式不改写历史。

D55保持PromQL Input2/Output3/Discovery10、目录Output3/Discovery4；Workspacev9。dashboard_evidence/v2的单序列面板增加first_point_value/last_point_value/value_mean，可选且与实际时间戳绑定，多序列只在series内分别提供端点；Markdown全部逐序列列出。metric_evidence/v6的已核实原始counter增加interpretation_limits（input_basis=original_cumulative_readings，interval_activity=not_established_by_this_input），不替代其他速率/边界证据或改变原始单位。旧证据不回填，元工具输入与业务工具数不变。

D57作者目录：dashboard.catalog输出v2/发现修订3，输入不新增字段，底层名称目录保留分页并补实际来源键，类型/单位冲突明确mixed/未知而不是any(unit)；observe的metric_variants继续提供完整分项。MetricDescriptor新增可选semantics/recipes，复用ObservationMetricSemantics/ObservationRecipe，不从任意单位1猜比率；仅当前已授权Host来源完全匹配时提供内置利用率定义和发布模板配方。原单位、类型、测量值不改。私有MCP、非Host、自定义/未知/冲突和外来键无此认证。既有trace交付不变。capabilities发现7的dashboards.scope_note说明逐资源链接；dashboard.list发现4说明组合匹配，不新增工具或三元参数。

D58的draft.validate/publish.preview模型Inputv2要求parameters中的显式资源选择，不能在修复其他字段时丢失并扩大到全部授权。公开DashboardVersionInput新增可选parameters供API/门户转发，领域PreviewPublish与ScopedPublicationVerifier复用ExecutionInput；shortcut bindings不作数据范围。两工具发现修订4。精确已发布配方的已知输出单位可验证，未知派生单位仍不猜，DashboardCompiler v5撤销旧v4计划。
