# Task 01：Dashboard Workbench、Query Runtime 与资源入口

2026-09-28：实现与功能验收完成，最终环境清理与原有资源核对通过。正式页面 32 个场景由 f 轮 26 项与 g 轮 6 项组成；h 轮 Runtime、六项真实故障及工具协议通过。完整范围与证据见 [验收结论](./completion-review-20260928.md)。

本轮追加：Q27～Q31 明确保留 KQL、OTLP/ClickHouse，增强展示/自由布局并建设完整 APM，明确打开链路时可读取另有授权的 B/C。详细事实与待决范围见 [能力讨论](./03-observability-depth-discussion.md)，现有 KQL/Trace 子集不能视为最终能力承诺。

Q35/Q36 确定按 Panel 绑定采集来源，对齐厂商能力与交互，保持 Argus 样式及对象权限。顶部通用过滤、图内局部过滤共同进入服务端执行；来源保存、按来源查询与厂商视图须按 [专项设计](./04-collector-plugins-and-source-views.md) 实现，不能仅增加 UI 选项。

## 1. 目标与基线

在不依赖 AI 的情况下交付真实可用的 Metrics/Logs/Traces 仪表盘。以 [主设计](./01-telemetry-dashboard-and-ai.md) 和 [Q1～Q42 决策记录](./02-confirmed-decisions.md) 为准，未决项按标注继续澄清。

~~~text
创建个人草稿，填写名称/描述/Folder
→ 添加统计图、构建器变量、默认条件和布局
→ 服务端保存草稿，刷新可恢复
→ 统一预览与一次确认
→ 发布不可变 Revision
→ 在 Dashboard 页面查看、筛选和刷新
→ 在 Host/Kubernetes Cluster 页面配置快捷关联并打开
~~~

完成条件包含真实存储、查询、权限与恢复。已有 Telemetry 组件和单查询 Engine 不等于已有 Dashboard Runtime。

## 2. 依赖和非目标

复用 M2 对象授权、Host/Cluster 资源事实、PendingAction/Action Executor、M7/M10 三信号链路、PlanV5 Tool Gateway、Workspace IO/来源授权和前端共享包。

本 Task 不包含 AI 创建和分析、告警、自动修复、任意 SQL、新 Query Engine、实时协同编辑、动态标签绑定。Kubernetes 下级对象绑定、UID 精确过滤、传播与移动端均不属于本期验收。

这里的动态标签绑定指资源页面导航关联，不排除 Q37 的来源类型＋顶部资源范围动态匹配。Q41 的 APM 通过专用统计图与展开详情/全屏承载，不另建独立 APM 一级页面。

Q32 的常用 APM 排查能力纳入本期，Profiling/方法级剖析后续考虑。2026-09-25 用户追加用 Argus 自身验证 SkyWalking/Jaeger，原生 Trace 接收和自检因此纳入本轮，具体协议、版本与验收边界见 [自监控专项](./self-monitoring-native-traces.md)；应用主路径仍为 OTel/OTLP。

## 3. 子任务

### T1.0 遥测能力核实与补齐

在冻结 UI 可配置范围前，以真实数据建立能力矩阵：

| 信号 | 首期构建器交付 |
| --- | --- |
| Metrics | 趋势、当前值、分组聚合、计数器速率、Top N、受控错误率与直方图 P95 |
| Logs | 明细、条件过滤、数量趋势、按字段分组计数 |
| Traces | 列表、错误/慢请求筛选、单条 Trace 详情 |

此表是 Q14 的基础矩阵；Q32 已确定额外首期范围：服务/实例/接口总览、调用量/错误率/时延、服务拓扑、慢错误 Trace、Span 详情和关联日志。不能仅完成此表就宣称整体交付；采样统计按 Q34 展示已接收样本，明确注明口径，不推算全量；独立请求指标另列来源。

必须核实：

- 字段、数据类型、聚合、结果形状与图表兼容性；P95 需要可计算分位数的数据，不能基于平均延迟伪造。
- KQL Parser、Writer 和实际日志表支持一致，补齐日志数量趋势及首期所需聚合。
- Trace 列表、单条详情和总时长展示语义分开，不把现有组件当作完整 Span 瀑布图。
- 保留 KQL，补时间分桶、聚合结果模型、字段探索、稳定分页与上下文读取；不引入 LogQL/Loki 作为丰富展示的前置条件。
- 完整 APM 需跨批次/跨主机拼接、去重、迟到更新、真实层级与完整性状态，及实体目录、RED 指标和依赖分析；计算可复用 Collector 组件或后端实现，但须定义唯一统计口径。
- OTel/OTLP 为主入口，ClickHouse 继续存储。标准 OTLP 验证应用内部 APM，新增 SkyWalking/Jaeger Receiver 验证原生 SDK 自检数据；Collector 发行清单和 Catalog 共同声明能力。Span Metrics/Service Graph Connector 未作为本轮 APM 统计来源，不因 UI 出现选项就宣称所有厂商能力已接入。
- 接收来源与统计口径分开：数据目录从实际观测字段建立，不从已安装插件推定数据存在或全量；收到的 Trace 样本按 Q34 展示样本调用量/错误率/时延，不采用全量估算或隐式切换指标来源。
- Q31 区分入口 A 的筛选和明确详情展开：仅扩展到当前另有授权的 B/C，不扩大 Dashboard 全局选择，不返回未授权明细；实例/标签筛选子查询和详情缓存同样授权。
- 目录基于实际观察数据；Collector distribution/component/profile 是安装能力目录，不能冒充指标字段目录。
- Host/Cluster 范围过滤在三信号中一致；空范围不能退化成无过滤。
- 统一现有三信号/字段细分门禁到对象访问与编辑控制，HTTP、Tool、前端、角色及来源指纹一致调整。
- 导出需补稳定分页/读取边界、续传、去重和完整性；区分显式 limit/聚合与系统分页，不能把有界原结果写文件称为完整导出。

交付：经真实数据验证的字段/运算/图表矩阵、受限目录查询能力和底层补齐项。未完成的能力不得用 mock 替代验收。

### T1.1 契约、编辑来源与状态机

固化 DashboardFolder、Dashboard、DashboardDraft、DashboardRevision、Panel、QueryTarget、DashboardVariable、DashboardBinding 的版本化 Schema、错误码、OpenAPI 和 ADR。

关键规则：

- 普通查看读取已发布版本，个人草稿与发布版分离。
- 每张统计图只有一种有效编辑模式，builder 或 dsl；多 Target 也遵守该模式，不维护两套可编辑事实。
- Builder 保存对应信号的结构化定义，服务端生成原生查询；DSL 保存底层支持的查询定义。
- Builder 可转 DSL；DSL 只有完整、无损可解析时才能转回，否则继续使用语句模式。
- 服务端派生 compiled_query、compiler_version 和 query_hash，不接受客户端派生字段作为执行事实。
- 变量只提供构建器，支持三信号选项查询、依赖、单/多选、All、默认值和刷新；不提供变量 DSL。
- 依赖来自类型化引用，拒绝循环、未定义变量和不兼容类型。
- PromQL、KQL、GraphQL 参数按各自语义适配；禁止统一文本替换和混淆 GraphQL 原生参数。
- Host/Cluster 系统资源过滤作用于所有适用 Panel；环境/服务等自定义变量仅作用于使用它的查询。
- Panel 声明非空 applicable_resource_types（host/kubernetes_cluster），全部 Target 继承，服务端发布时校验。类型不适用、没有授权、无数据和查询失败分别返回状态，不能用样本为空或 Binding 推断不适用。
- Trace Panel 的展示 Target 继承同一来源绑定；Dashboard 可组合不同来源的独立 Panel。Q37 按来源类型与顶部资源动态解析授权采集实例，具体实例是局部条件，不隐式混合不同厂商。
- 来源停采仍可查询保留期内历史，重装区分新旧来源身份；每次执行冻结实际 source IDs 与配置/能力版本，执行中的分页和导出不动态加新成员，访问权限仍实时重验。
- Q38 的共享变量独立声明候选发现范围和值身份，并对各 Target 显式映射；不按服务/实例名称合并，默认保留厂商局部服务/实例过滤。
- Q39 的标准下钻按来源/图型生成，使用 drilldown_ref 引用已发布明细查询和参数映射，生成后可编辑并统一预览发布。展示与下钻目标分开建模；跨信号目标显式声明 Signal、来源与查询，不要求继承展示图的 Signal。
- Panel 局部过滤定义包含稳定 ID、类型、候选依赖、默认值和参数映射，纳入版本化契约和发布校验；运行态值按 Panel ID 传递。顶部系统条件、显式引用的共享变量和局部条件共同约束查询。
- Binding 仅是资源页面导航关联；不作为查询范围上限，不提供 all_bound_targets 模式。
- 查询执行、硬验证和样本验证状态各自明确，不能把 no_data 当作语法错误或系统正常。
- 仪表盘和 Panel 页面只配置、发布及查看查询，不提供导出按钮、下载页、export_source 或独立导出配置区。
- AI 数据查询工具复用已发布查询，并在内部交付完整结果文件；额外明细仍需明确的已发布查询定义，不允许临时拆聚合、删 limit 或添加原始字段。文件 Schema 属于工具交付协议，不是用户配置。

交付：互斥编辑模式、变量配置、草稿版本、发布基线、样本状态、Panel 状态与原生结果的契约样例。

### T1.2 存储、个人草稿与对象授权

新增：

~~~text
telemetry_dashboard_folders
telemetry_dashboards
telemetry_dashboard_drafts
telemetry_dashboard_revisions
telemetry_dashboard_bindings
telemetry_dashboard_exports          # 任务、分片和来源元数据
~~~

要求：

- 所有对象按企业隔离；Repository 只承担持久化。
- 新建从个人草稿开始，首次发布原子创建 Dashboard、Revision 和创建者显式授权；合法空 Spec 可发布。
- 个人草稿保存 editor、draft_version、base_revision_id；刷新恢复，其他编辑者不能读写这份草稿。
- 草稿状态为 editing/published/discarded；首次发布回填 Dashboard 与 Revision，一个新建草稿最多创建一个 Dashboard。已有对象每位编辑者至多有一个 editing 草稿。
- 已有 Dashboard 草稿按 Dashboard + 编辑者管理，同一编辑者多页面写入也检查 draft_version。
- 草稿可以不完整，保存检查权限、基本 Schema、大小并审计，不要求每次确认或完成全部查询验证。
- Revision 配置、哈希和验证报告不可变；布局等多次编辑在一次发布中形成新 Revision。
- 非空 Folder 拒绝归档，必须先迁出其 Dashboard；Dashboard 归档保留历史和个人草稿，不级联删除/归档。撤权期间保留草稿存储但不授予读取、编辑或发布能力。
- Binding 唯一键为 dashboard_id + target_type + target_id，类型只有 host/kubernetes_cluster。
- Dashboard 对象授权扩展现有 data_authorization_grants，不另建部门/角色权限系统。
- 同步资源类型约束、同企业存在性校验、授权资源目录、身份加载、OpenAPI/DTO、角色和管理界面。
- 功能权限收敛为 Dashboard 访问与管理，显式对象授权决定能访问哪些对象。可访问者能看查询和默认变量，编辑/发布另受管理能力限制，不新增 inspect/catalog/字段阅读权限。
- 用户授权为直接用户、所属部门和有效角色授权并集；创建者获得新对象授权，其他人通过原有管理入口授权。
- Folder 不授予对象权限；Dashboard 授权不隐含 Host/Cluster 数据授权。Metrics/Logs/Traces 不分权，Signal 只用于查询引擎与数据语义。
- 现有 telemetry.query.metrics/logs/traces 与敏感字段角色门禁统一核查收敛，不在仪表盘做特殊绕过。必要的数据安全处理采用统一规则，非字段级授权体系。

交付：Migration/sqlc、草稿持久化和并发、对象授权全入口、创建事务、归档与审计。

### T1.3 统一发布预览、验证与 Commit

编辑过程中保存草稿；正式发布时一次 Preview/Commit：

1. 检查企业、主体、Dashboard/Folder、编辑及发布功能权限。
2. 检查草稿归属与版本，冻结内容和 base_revision_id。
3. 校验编辑模式、展示及明细原生查询、字段、类型、展示图表或导出文件 Schema、变量依赖。
4. 校验布局、Panel/Target 数、默认条件、时间、刷新和预算。
5. 校验明确请求的 Host/Cluster 关联建议及双方权限。
6. 尝试有界样本查询，分别返回配置硬校验与样本状态。
7. 生成 Spec Hash、Diff、样本报告、警告、公开 action_ref 和过期时间。

发布规则：

- 语法、类型、权限、预算等硬错误拒绝发布。
- 样本 no_data 或服务暂时 unavailable 不等于配置错误；硬校验通过时可以带警告发布。
- 已证明的结果类型不匹配不能包装成 unavailable 放行。
- 预览冻结草稿 ID/版本及计划；后续修改保留新内容，但使旧预览不可再提交，需要重新预览。
- Commit 只接收公开 Action Ref，从服务端加载私有计划并重新授权。
- 当前 active Revision 不等于预览基线时返回冲突与差异，由用户整理后重新预览；不能静默覆盖。
- 同事务校验 Dashboard 元数据/生命周期版本、仍 active、Folder 可用与当前权限；归档前旧预览不能发布。对象/权限恢复后重新校验基线并生成新预览，不复用旧确认。
- 原子校验并消费 editing 草稿版本，写入发布版本、active 指针、创建者授权及明确确认的关联。跨 Action 旧预览不能重复创建新对象；后续编辑从当前发布版建立新草稿。
- 幂等、Execution ID 和 ResultUnknown 对账处理响应丢失、重复点击与重启。

Folder 管理、归档和资源关联等生效变更也沿用 Preview/Commit；个人草稿保存/放弃不反复弹确认。调整现有 telemetry.* Action 分派，避免 Dashboard 计划被按 Collector 计划解析。

### T1.4 Query Runtime、变量、Catalog 与预算

统一接口：

~~~text
ExecuteDashboard(ctx, DashboardExecutionRequest)
  → 读取 active Revision 或校验本次执行冻结版本
  → Dashboard 与 Host/Cluster 对象授权
  → 解析时间、系统资源选择、自定义变量
  → 有效 Host/Cluster 范围
  → 恢复服务端保存查询并绑定合法参数
  → 三种既有 Engine
  → 预算、状态、统一数据安全处理、审计和结果
~~~

范围规则：

- 指定 Host/Cluster 时校验并限制到这些对象；未指定时使用当前用户有权限且适用的资源。
- Binding 不参与查询资源求交；无绑定 Dashboard 也能执行。
- 系统资源筛选应用到所有适用 Panel，不依赖是否显式引用资源变量。
- Panel 来源与局部条件在服务端强制应用；局部不能覆盖顶部时间/资源扩大普通查询范围，Q31 明确完整链路展开走独立授权详情路径。
- Q37 按本次时间窗口解析动态来源集合，保留停采/重装历史身份，并将实际集合冻结到上下文。Q39 下钻只接受已发布引用和受约束参数；普通下钻继承有效范围，跨资源完整链路按 Q31 单独授权。
- 空授权/空适用范围返回明确状态，不向底层传递“无过滤”的空 ID 集合。
- 无权或非法的显式 Host/Cluster 请求不能回退为全部资源；自定义变量候选变化后的 All 回退单独按以下规则执行。

Catalog：

- 信号与 Panel 已绑定来源 → 实际指标/字段/属性 → 有限值或范围；来源必须可追溯，不从安装组件猜测每条记录。
- 资源、服务、scope、可信采集实例可进一步筛选，安装能力与当前窗口实际数据分开。
- 配置以已有数据为主，不专门做无数据预建字段流程。
- 类型、字段映射、查询语言白名单、统一数据处理、数量、长度和时间范围均受服务端限制，不按 Signal/字段分权。
- 候选只列当前时间窗口出现的具体值；重载后已有选值消失或不再完全有效时，该自定义变量自动切为 All，并重新计算依赖变量。
- 没有具体候选时保留“全部”状态，不回填历史值。单选、多选都能回退；All 仍受对象授权、已选系统资源和上游条件限制，不修改发布默认值。
- 首次加载、时间/前置变量/资源/对象授权变化强制刷新候选；手动或随数据刷新策略只作用于同一上下文，不能继续使用旧上下文选项。
- All 不等于已分页加载的候选，更不能删除系统资源约束。
- 局部候选继承来源和顶部适用条件；候选值失效可回退局部 All，但保留来源/全局约束。手输 Trace ID、关键字和时延等不能因无数据自动清空。

运行时：

- 保持三种原生结果语义；单 Target error 与上层 Panel/Dashboard partial 分开。
- 区分 success、no_data、error、partial、not_applicable、skipped_budget。
- 数据新鲜度分开表示查询时间、最新样本时间与可获得的摄入状态；不能证明完整性时记 unknown。
- 总预算覆盖目录、变量和所有 Panel，分配并发查询额度并支持取消；后续 AI 分批调用共享预算。
- 缓存至少包含企业、主体、Dashboard/Revision/Panel/Target、查询哈希、时间、step、变量、有效对象范围指纹、Signal/语言、统一数据处理版本与授权版本。
- 缓存命中仍重新授权，不以相同授权版本号判定两个主体权限相同。
- 缓存和查询哈希补来源绑定/能力版本、局部过滤定义与有效局部参数；局部更新只刷新当前 Panel，顶部共享变量变化只更新实际依赖者，不允许旧响应覆盖新条件。

AI 数据查询工具内部的文件交付：

- 内部文件交付服务从冻结 Revision 恢复实际普通/明细查询，复用上述执行、对象授权和总预算，不由 Dashboard 页面直接调用。
- 默认保留聚合、显式 limit、Metrics instant/range/step、Trace 字段选择；明细须从发布配置读取，不接受运行时替代查询。
- Metrics 导出步长依据发布的固定或服务端目标点数策略确定，记录实际值，不依赖图表尺寸；固定步长不能因预算不足而被静默改粗。
- 以任务记录状态、读取边界、分片/游标、Hash 和完整性，补齐稳定分页、续传/去重和取消。
- 大结果流式或有界分片写入会话 Workspace；复用既有 IO、来源登记与容量约束，不把数据集塞入一个 Tool Result/内存缓冲。
- 完整表示查询定义内的结果完整；Trace 嵌套详情单独记录，超预算/空间不足/中断标 partial/failed/cancelled。
- Workspace 文件使用独立导出目录，写入前登记来源，分片原子交付，重启/重试不重复追加。服务端原始来源与 Hash 独立保留。
- 文件导出不修改 Dashboard，不要求额外 Preview/Commit；与 AI 的 grep/bash 分析分工明确。
- 持久化、满额、来源撤权、跨会话保护和明确删除沿用 PlanV5，扩展 Dashboard 来源指纹。
- 可选展示阈值不成为发布必填项，不实现必须由服务端计算的 AI 诊断规则。

### T1.5 REST、API Client 与适配器

主设计 §8 固化路径，至少包括：

- Folder/Dashboard 列表、详情、历史发布版本和归档预览。
- 新建、读取、更新、放弃个人草稿；draft_version 乐观并发。
- 草稿发布预览与公共 PendingAction 确认。
- Dashboard 执行、目录发现与关联摘要。AI 查询任务的内部文件交付由工具服务和已有 Run/Workspace 机制承载，不新增 Dashboard 导出 REST 入口。
- Host/Cluster 页面关联读取、创建/解除预览。
- 显式 Dashboard 对象授权管理扩展。

前端使用 @argus/api-client 生成 DTO，mock/real 共用契约。页面不自行扩展字段或拼接执行查询。普通执行不接受 raw expression 覆盖；编辑、样本预览与运行接口边界清晰。

### T1.6 Workbench

页面：

~~~text
/dashboards
/dashboards/:dashboardId
/dashboard-drafts/:draftId
~~~

正式编辑以个人 Draft ID 作为路由身份：新建尚未发布的仪表盘没有 Dashboard ID；已发布页的“编辑”先创建或恢复该编辑者的草稿，再进入同一路由。

- 目录按对象权限展示 Folder 与 Dashboard，根级未分组不创建实体 Folder。
- 新建名称/描述/Folder 后进入个人草稿，逐个配置 Panel。
- 查看页展示发布版名称、Revision、时间、系统资源、自定义变量、刷新和 Panel 结果。
- 顶部通用过滤与 Panel 厂商过滤分层，各图显示来源和条件摘要；常用局部条件就近展示，高级条件折叠。局部操作只影响本图，默认值修改仍进入草稿发布。
- 编辑页显式标记草稿，布局调整、变量修改与多个 Panel 修改一起保存草稿，最终统一预览和发布。
- 布局保存 x/y/w/h，支持真实拖动位置、右下角任意网格宽高缩放、吸附、最小尺寸、碰撞下推、键盘及刷新恢复；不以宽度预设或顺序按钮替代。
- 增加图型注册与结果适配、图例/提示/缩放/显示计算，以及日志和 Trace 的探索入口。Demo 展示范围不等于真实引擎全部支持；按保留 KQL/ClickHouse 的方向落实 Q32 的 APM 总览、指标、拓扑、Span 与日志关联展示。
- 正式显示配置支持适用图型的样本计算、自动/固定范围、时序折线/面积/柱形、堆叠、平滑和可选展示阈值；草稿、预览、发布和全屏一致，切换 Metrics 图型保留查询模式。能力矩阵及缺失样本、桶差分和单位规则见 [显示契约](./chart-display.md)。
- Panel 编辑负责当前图的标题、Signal、图表类型、builder/DSL 和查询验证，不新增导出来源选择或文件格式配置。
- 提供“下钻详情”配置：展示平台生成的标准入口、目标信号/来源、明细查询与参数映射，允许编辑；新增、编辑、移除均在发布 Diff 中明确展示，不自动改变已发布版本。
- Q41 增加服务/实例/接口总览、RED、拓扑、慢错误 Trace 等专用统计图和展开详情/全屏。每种图型有真实结果适配与验证，不以通用 Trace 列表冒充完整 APM；AI 入口继续留在 Chat。
- 切换编辑模式不丢信息，不允许隐藏的第二份配置覆盖当前来源。
- 变量管理只有构建器，展示依赖和使用位置；固定 Header/Footer，正文唯一纵向滚动。运行时下拉选项随时间变化，已有选值消失时自动回退全部，依赖候选随之更新。
- 首次发布合法空 Dashboard 时显示添加统计图空态；个人草稿不影响他人查看。
- 默认条件的更改必须进入草稿发布；浏览时调整筛选不改变 Revision。
- 使用共享 UI、Design Tokens、模块 i18n、.argus-*，每个代码文件低于 2000 行。
- 双语、深浅色、桌面键盘/读屏纳入验收。[demo.html](./demo.html) 已提供相应状态与变化核对入口，但模拟数据和本地保存不能替代正式验收。

本 Task 不建设 Dashboard 页面的 AI 分析入口、当前视图捕获或页面联动。

### T1.7 Host/Cluster 页面关联

- 在 Host 或 Kubernetes Cluster 页面选择可访问 Dashboard，预览后确认关联。
- 绑定管理校验目标资源管理权限、目标资源授权和 Dashboard 访问权限。
- 快捷入口展示时检查两侧权限；打开自动预选资源。
- 进入后可以改选其他有权且适用资源，关联本身不是范围限制。
- 资源删除、Dashboard 归档、关联撤销或当前用户撤权时入口不可用。
- 用户撤权不能将其他人仍有效的全局关联误标为 revoked。
- 未绑定 Dashboard 可从目录独立访问和执行。
- 本期不建立 Namespace/Workload/Service/Pod/Node 绑定，不验收 UID 漂移和后代传播。

### T1.8 测试、可观测性与门禁

必须覆盖：

- Schema、Repository、对象授权、Draft/Revision 生命周期和发布事务。
- 同编辑者多页面写冲突、两位编辑者发布冲突、刷新恢复、草稿隔离、预览后修改和同一新建草稿的跨预览去重。
- Builder/DSL 唯一来源、无损转换失败保留 DSL、语言适配与变量依赖。
- 硬校验失败拒绝与样本 no_data/unavailable 可警告发布。
- 三信号构建器矩阵、类型不匹配、目录真实字段与分页/截断/All。
- 强制资源过滤、没有资源变量的 Panel、无绑定、从 A 进入后切换 B、空范围拒绝。
- Trace 跨批次、乱序、缺 root、迟到、重复和跨主机；从 A 筛选后明确展开到授权 B/C、未授权部分不返回、展开后撤权、子查询范围及缓存隔离。
- 同一 Dashboard 不同厂商 Panel 的来源和候选隔离、局部修改不影响其他图、顶部资源约束不可覆盖、共享变量只影响引用者、局部 All 不解除来源、无数据不清除手输条件、刷新竞争及发布默认值隔离。
- 新匹配来源纳入后续查询、停采后查历史、重装新旧来源区分、同次执行来源集合冻结；不同厂商同名服务不隐式合并，显式共享映射与发现范围正确。
- 标准下钻生成/编辑/发布、跨信号目标和参数校验、缺关联字段、UI/AI 同定义、无导出配置；Q41 专用图型与详情/全屏对应真实数据。
- 非空分组归档拒绝、迁出后归档、Dashboard 历史/草稿保留、撤权/归档阻止旧 Commit、恢复后基线冲突与重新预览。
- Q32 的真实 OTLP 服务/实例/接口发现、调用统计、拓扑关系、慢错误定位、Span 属性/事件及关联日志；插件已启用但无数据、关联字段缺失与采样未知分别展示真实状态。按已确认的 Q34 展示已接收样本统计、不推算全量，独立请求指标另标来源。
- Dashboard 与 Host/Cluster 对象授权分离，配置可读与编辑控制，不再按三信号/字段分权；跨企业及相同授权版本不同主体。
- 缓存、总预算、并发、取消、新鲜度、单 Target 错误与上层 partial。
- 无 Dashboard 导出入口/配置；AI 内部文件交付的已发布查询引用、Schema、limit 与分页、Metrics 时点/步长、Trace 嵌套完整性。
- 当前选值消失回退 All、多选失效、级联变量更新、无具体候选仍为全部、授权资源边界保持及默认配置不变。
- 导出文件真实字节/Hash、分页/续传/去重、满额/中断、重启、来源撤权及跨会话拒绝。
- Playwright mock/real：目录、草稿、编辑、预览、发布、冲突、查看、关联与双语深浅色桌面交互。
- 临时 Kubernetes Namespace：真实三信号数据、服务重启、Redis 清空、重复确认、资源删除和撤权。
- 测试成功/失败均按归属清理 Namespace、PVC、Topic、Bucket、Lease、测试关联及临时诊断资源，保留脱敏证据并保护正常部署。

## 4. 顺序

~~~text
T1.0 能力核实与补齐
→ T1.1 契约
→ T1.2 存储/个人草稿/对象授权
→ T1.3 发布验证与确认
→ T1.4 Runtime/Catalog
→ T1.5 REST/API Client
→ T1.6 Workbench
→ T1.7 Host/Cluster 入口
→ T1.8 真实 E2E
~~~

## 5. 退出标准

- 通过后台创建并发布真实三信号 Dashboard，具备首期构建器能力和受支持 DSL 模式。
- Q32 常用 APM 能力取得真实 OTLP → ClickHouse → 查询 → UI 的证据，覆盖总览、指标、拓扑、慢错误 Trace、Span 详情和日志关联；新增 SkyWalking/Jaeger 原生 Trace 自检必须取得协议接收、来源隔离与正式页面证据。Profiling 和其余未确认多协议扩展不作为本期门禁。
- 变量使用构建器，Panel 具有单一编辑来源及受控转换。
- 草稿能跨刷新恢复，多次修改一次发布，冲突不覆盖已发布内容。
- 对象授权与 Host/Cluster 数据权限一致贯穿列表、目录、执行和关联。
- 绑定只负责导航，从资源页面打开准确带入强制资源筛选。
- 无数据、临时不可用、硬验证错误、运行错误与覆盖不足均有明确状态。
- Dashboard 页面没有导出功能；AI 工具内部可交付已发布查询的完整结果，额外明细仅来自明确发布的查询，文件可追溯。
- 后续 AI 复用 Query Runtime 和导出服务，再用 Workspace 文件工具分析，无需复制权限、查询、预算和发布逻辑。
- 所有退出项都有对应测试证据；文档更新或 mock 演示不视为完成。

## 6. 主计划映射

对应主设计 P2V-CONTRACT/ADR/OPENAPI、DB/DOMAIN/VALIDATE/ACTION/AUDIT、EXEC/CATALOG/EXPORT/AUTH、WEB/BINDING，以及 E2E-01/04/05/06 和 RELEASE 门禁。
