# Task 02：AI 创建仪表盘与 Chat 内分析

2026-09-28 收尾：Task 02 首期检查已关闭，主清单 37/37。h/i/o/p 的 13 个代表性真实 GLM 场景有分轮通过证据；p 轮异常组 4/4、部署检查 20/20、退出码 0。未声称一次最新镜像全量重跑或任意问题质量保证，详见 [真实模型验收记录](./real-model-validation-20260928.md)。下文的“暂不执行”保留此前非模型收尾阶段的决定。

2026-09-28：Chat 选择、条件状态、builder/DSL 工具协议、宿主确认、后台任务、真实文件/PVC、来源授权及故障恢复的非模型实现与功能验收完成，最终环境清理与原有资源核对通过；见 [验收结论](./completion-review-20260928.md)。实际模型的生成、条件理解、策略与结论质量按用户要求暂不执行，原始模型质量门禁不标为通过。实现契约见 [Chat 上下文与工具](./chat-context.md)。

## 1. 目标与用户闭环

在 Task 01 的 Dashboard、个人草稿、Catalog、Query Runtime 和结果导出服务上接入 AI 创建与分析。以 [主设计](./01-telemetry-dashboard-and-ai.md) 和 [Q1～Q42 决策记录](./02-confirmed-decisions.md) 为准。Q32 的 APM 能力在 Task 01 补齐并验收后接入相同契约；Q31 的人工完整链路展开不使 AI 自动改写已发布查询或补取未配置明细。Q34 的多来源回答不等于全量数据保证；Q35/Q36 的来源和局部过滤按 Panel 隔离，继续由 Chat 自己的明确参数驱动。Q37/Q38/Q39 的动态来源、显式映射及标准下钻由人工与 AI 共用；Q40 的重新取数采用最新发布版，单次执行冻结配置。

~~~text
/创建仪表盘
→ 从真实 Catalog 收集字段与资源信息
→ AI 生成 builder/DSL Panel 与构建器变量的 Draft
→ 服务端验证并生成发布预览
→ 用户宿主确认
→ 发布 Dashboard/Revision

Chat 中 @具体仪表盘
或未指定对象时列出授权仪表盘，由用户明确选择
→ 获取已发布统计图查询定义
→ 按默认条件和用户问题确定时间、资源、变量
→ 泛问检查所有适用图，具体问题按需查询
→ 导出已发布展示查询或显式明细来源的完整结果
→ 结果文件进入会话 Workspace
→ AI 用 read/grep/bash/脚本分析
→ Chat 输出自主判断、范围和覆盖不足
~~~

AI 分析不依赖 Dashboard 页面是否打开，不获取页面“当前视图”，不改变页面筛选。用户不需要分析工作台、图表或额外可视化界面。

## 2. 依赖与非目标

### 2.1 前提

- Task 01 的人工三信号闭环、草稿发布、授权、Catalog 和 Runtime 已有真实 API/UI E2E 证据。
- 配置契约区分构建器变量与 builder/DSL Panel。
- 统一 Query Runtime 支持系统资源范围、变量适配、总预算和状态；导出服务具备分片、可信来源与完整性记录。
- 复用 PlanV5 持久 Workspace、受控导入、离线 read/write/edit/bash、来源授权和配额；有界 Tool Result 导入不等于大结果完整导出。
- PendingAction、Action Executor、Tool Result Projection 和 PlanV5 宿主确认可复用。
- 不能假定结构化 Dashboard Mention、显式 Skill 激活、授权 Dashboard 列表及 Gateway 分类已存在，需通过 T2.0 补齐。

### 2.2 非目标

- 未经用户选择自动挑选仪表盘。
- AI 临时改写查询、自动去掉聚合/limit、还原原始点或展开未发布 Trace 字段；额外明细必须有明确的已发布查询，文件交付由 AI 工具内部处理，不是仪表盘功能。
- 模型直接提交 Commit、读取私有提交 Token 或代替服务端宣称验证通过。
- 把摘要写回 Dashboard 当作业务事实。
- 告警、自动修复或生产资源变更；不建设必填阈值或服务端固定诊断规则，AI 自行判断。
- Dashboard 页面分析按钮、浏览器当前视图捕获、自动页面联动或分析工作台。
- 本期 Kubernetes 下级对象绑定、UID 精确过滤或传播。

AI 创建可以生成底层支持的查询语句；“分析不能任意查询”不等于“创建只能生成构建器”。

## 3. 子任务

### T2.0 显式命令、结构化引用与 PlanV5 接点

- 补齐版本化 Skill 激活、ContextSource 注入、Run 配置冻结和恢复。
- 创建命令内部 ID 固定为 telemetry.dashboard.create；显示文案本地化。
- 增加稳定 Dashboard ID 的结构化 Mention 与用户列表选择事实，持久化到消息/Run，不能仅拼入自然语言文本。
- 已有 Host/Connector 文本引用能力不能当作 Dashboard 结构化协议已完成的证据。
- 为 telemetry.dashboard.* 注册正式 Gateway 分类、名称映射、Schema 和同包业务元数据，继续通过 PlanV5 三个元工具使用。
- Dashboard Action 与现有 Collector Action 明确分派，隐藏 Commit 不进入模型 Registry。
- 发布、分析、导出和文件来源校验由服务端执行，不依赖 Skill Prompt。
- 按 Q20/Q21 统一收敛现有三信号/字段细分门禁到对象访问和编辑，HTTP/Tool/角色/前端一致，Workspace 来源指纹加入 Dashboard。

### T2.1 AI 创建 Skill

收集名称、描述、Folder、Panel、变量、默认时间、刷新、正常查询需求及 Host/Cluster 关联建议。

- AI 与人工支持相同配置契约。
- Panel 可生成 builder 配置或受支持的 PromQL/KQL/只读 Trace GraphQL 定义。
- 变量只生成构建器配置，不能绕过变量 Schema 提交自由 DSL。
- 每张统计图只有一个有效编辑来源，展示和明细 Target 都使用该模式；切换要求完整、无损。
- 平台根据来源和图型生成标准下钻，AI 与人工一样可配置或编辑后随 Panel 发布；跨信号明细显式声明目标来源、查询和参数映射，不将其变成运行时自由查询。
- 不生成 export_source 或仪表盘导出配置；只创建正常统计图查询。AI 后续取数时工具内部交付文件，额外明细仍须明确的已发布查询。
- 生成严格版本化 JSON Draft，不生成前端 HTML。
- 正常以已有数据配置，不专门建设无数据预建字段流程。
- 不能根据模型声称正确就跳过类型、权限、预算或样本验证。

### T2.2 Catalog 辅助生成

使用受授权的真实数据目录：

- Metrics：指标名、类型、Label、有限值、运算兼容性。
- Logs：实际支持字段、类型、值、过滤与聚合能力。
- Traces：支持的服务/属性、状态、时延筛选与 Trace 详情能力。
- Resources：实际 Host/Cluster ID 与当前用户可查询范围。
- 来源标识仅在可靠时使用；不将安装插件目录当作已采集指标证据。

存在名称或资源候选歧义时让用户选择。Catalog 是配置来源信息，不替代服务端 Parser、Schema、授权与预算校验。

### T2.3 Draft 接收与服务端验证

复用 Task 01 的 DashboardDraft 和 Panel/Variable 契约，包含：

~~~text
draft_id? / expected_draft_version? # 首次内联创建省略，修改已有草稿时必填
dashboard_id? / base_revision_id?
name / description / folder_id
spec
proposed_bindings[]                 # 仅 Host/Cluster，可选
~~~

工具输入使用严格 Schema，拒绝未知字段与客户端伪造的 compiled_query/query_hash。AI 输出的查询语句只是待验证配置，不是可直接执行的提交凭证。

实现采用 draft.create 接收首次配置，按当前主体及不可变工具调用身份复用个人草稿并返回 ID/版本；draft.save 检查预期版本，publish.preview 引用已保存草稿执行发布校验。这样与人工编辑统一，积累修改期间无需反复生成确认。已有 Dashboard 必须已被用户选择，并检查对象授权与发布基线；新建 Dashboard 不要求预先存在对象授权。

校验：

1. 企业、主体、Draft 归属、Folder、Dashboard 对象权限与功能权限。
2. Panel、Signal、图表类型、唯一编辑模式、明确的适用 Host/Cluster 类型和原生查询语言。
3. 字段、类型、查询参数、变量依赖、未定义引用、循环与默认值。
4. 数量、布局、时间、刷新、运算和预算。
5. Dashboard/Host/Cluster 对象访问/管理、统一数据安全规则及可选关联建议；不按三信号或字段分权。
6. 待发布查询的结果类型与用途验证；文件序列化属于工具内部协议，不增加用户导出配置。
7. 配置硬验证与独立样本执行报告。

语法、类型、权限、预算等硬失败阻止发布。硬校验通过后，样本 no_data/unavailable 可作为明确警告发布；实际类型错误不能伪装成样本服务不可用。

### T2.4 预览、宿主确认与发布

复用：

~~~text
telemetry.dashboard.draft.create / draft.get / draft.save
telemetry.dashboard.draft.drilldowns
telemetry.dashboard.publish.preview
telemetry.dashboard.publish.commit # 仅内部 Action Executor
~~~

预览展示名称/Folder、Panel 配置、变量依赖、默认范围、明确关联建议、配置校验、样本状态、Diff、Spec Hash、公开 action_ref 和过期时间。

- AI 草稿可由所属编辑者继续整理，保存不反复确认。
- 预览冻结草稿 ID/版本、发布基线和内容，后续编辑不改变计划，但使旧预览不可提交，需要重新预览；新编辑内容保留。
- 创建/更新模板只展示业务详情；宿主 PendingAction 控件提供一次确认。
- Action Executor 从服务端私有计划执行，Commit 不接收可变查询、名称或资源参数。
- 更新时 active Revision 与基线不一致则返回冲突与差异；不能覆盖其他编辑者的新发布。
- Execution ID、幂等键和 ResultUnknown 对账覆盖重复确认、网络超时和重启。
- 创建者对象授权与 Dashboard/Revision 在同一事务产生。
- Commit 原子消费仍为 editing 的草稿版本，一个新建草稿最多产生一个 Dashboard；跨 Action 的旧创建预览也不能重复创建。

### T2.5 授权列表、用户选择与 @

实现 telemetry.dashboard.list，并让它与 @ resolver 复用同一个授权搜索服务。

~~~text
用户：有没有问题？
→ 列出当前用户可访问的仪表盘候选
→ 用户选择一个或多个
→ 保存稳定 ID 的明确选择事实
→ 开始分析

用户：@支付服务健康度 最近一小时有没有问题？
→ 直接保存稳定 ID 的引用
→ 开始分析，不再重复要求选择
~~~

要求：

- 未指定对象时不能自动选第一个、全部或相似名称的 Dashboard。
- 已在对话中明确选择对象时可以继续使用，不要求每次重新 @。
- ID 是执行事实，名称只是显示值；歧义时澄清。
- 列表分页、过滤、读取、选择、Run 恢复、查询/导出和文件来源均重新检查对象授权。
- 被归档、撤权或不可见的 Dashboard 返回明确不可用结果，不泄漏未授权对象信息。
- 用户选择来源、Conversation、Run 和分析上下文绑定，模型不能自行增加未选择的 Dashboard。

### T2.6 已发布来源、默认参数、追问与覆盖

get 返回冻结 Revision、普通查询与明细查询定义、来源哈希、变量契约、默认条件、适用资源及预算。可访问 Dashboard 的用户可读完整查询配置与默认值，不新增配置字段阅读权限；编辑另受管理能力控制。

get 同时返回来源类型/能力版本、局部过滤和标准下钻定义；实际 source IDs 与历史配置版本由服务端根据时间和顶部资源解析后冻结，模型不能指定任意替代来源。共享变量候选和值映射按作者配置执行，不按厂商服务名称自动合并。

参数：

- 首次使用默认时间/变量，用户明确条件覆盖默认值；未指定资源时使用当前用户有权且适用资源。
- 各 Panel 的来源绑定与局部过滤定义来自冻结 Revision；局部选值从已发布默认值和用户明确要求解析，按 Panel ID 保存、继承和导出。来源不可被运行时随意替换，不把同名厂商字段当作通用变量；指向不明确时澄清。
- 追问继承上一次明确的仪表盘、资源和变量，按用户要求修改条件；继承的变量选值因候选变化失效时应用上述自动 All 规则。
- 以结构化会话事实保存选择和参数，恢复不能只靠模型自然语言摘要；无历史明确条件才使用默认规则。
- 显式无权/非法条件不能按“未指定”处理，Binding 不参与取数范围求交。
- 候选按当前时间/资源/前置变量重新获取；已有自定义变量选值消失时自动切到 All，随后更新依赖变量，并把实际条件记入会话上下文。
- 无具体候选仍保留 All；回退不改变对象授权、系统资源选择或发布默认值，也不把非法/无权的系统资源请求转换为全部。
- 不捕获 Dashboard 页面状态，每次实际导出固定绝对时间和版本；重新查询不能拿旧文件冒充。
- Range 导出步长来自已发布策略：固定值原样执行，自动策略由服务端按实际区间和目标点数确定，不读取浏览器尺寸；结果清单记录实际步长。
- 解释旧结论可使用原证据；新查询按新的权威请求记录来源，权限和归档状态每次重验。
- Q40：新取数采用请求开始时的最新发布版，并在本次执行中冻结。旧文件保留原 Revision/查询/参数/时间/来源；各 Dashboard 独立保留默认值和明确覆盖条件。兼容条件继承，语义不兼容时提示重新明确，不把旧来源参数套到新查询或把旧文件冒充新结果。

覆盖：

- 泛问处理全部适用统计图，具体问题按需处理。
- 导出状态、数据完整性和 AI 实际分析覆盖分别记录；文件生成不等于已分析。
- 未配置明细的聚合图只能导出聚合结果，不能临时补取原始日志或 Span。
- 失败、无数据、预算/空间不足、撤权或未完成分析须在结论中说明。
- 多 Dashboard 保留各自版本、时间和范围，不混淆事实。
- 同一 Dashboard 中不同来源的 Panel 分别保留来源、局部参数和覆盖状态；AI 可综合已选择图的结果，但不能暗中合并查询范围或替另一厂商补取明细。

### T2.7 AI 数据查询工具内部的文件交付与 provenance

模型经 Gateway 调用：

~~~text
telemetry.dashboard.query
telemetry.dashboard.query.get
telemetry.dashboard.query.cancel
~~~

模型 query/v2 输入只包含已选 Dashboard、context_ref 和可选 Panel IDs，或已发布下钻引用；运行条件由 context.resolve 持久解析，query 不再接受自由参数覆盖。普通/明细查询由服务端从发布配置恢复，不能携带替代表达式或未发布原始数据请求。详细契约见 [条件继承与累计预算](./chat-conditions-budget.md)。

Q39 的明细请求通过已发布 drilldown_ref/query_ref 及其声明的输入参数选择；服务端恢复目标信号、来源映射、范围策略和预算。平台生成的标准下钻与人工修改后的下钻均须先发布，不能因为是标准模板就运行未发布的动态新版本。

执行要求：

1. 复用 Task 01 ExportDashboard、原生 Engine、对象授权与共享预算，冻结实际查询、参数和读取边界。
2. 工具默认完整交付统计图查询结果；需要明细时只能选择已发布的查询引用。保留聚合/limit、Metrics instant/range/step 和 GraphQL 字段选择。
3. 补稳定分页、续传、去重及嵌套详情完整性。系统分页不能改变查询含义，第一页不能冒充全量。
4. 受控流式/分片交付到当前会话 Workspace，写入前登记来源并检查容量，分片原子提交；不把大数据集通过模型或单个 Tool Result 中转。
5. 返回 export_id、status、source_ref、Workspace/file refs、路径、Schema、Hash、实际数量和完整性。总量未知时标未知，不伪造统计。
6. 任务、分片和来源元数据持久化，幂等重试不重复追加；重启恢复、取消、预算耗尽和磁盘满均有权威状态。
7. 每次导出、续传、读状态、文件访问和计算检查对象来源授权及会话归属，拒绝跨企业/会话。
8. 已发布统计图查询和明确的明细查询都属于已发布 Panel；不能改用通用工具绕过来源限制。
9. 下载到 Workspace 是查询交付，不修改配置，不额外要求 Preview/Commit。

provenance 关联 Dashboard/Revision/Panel/Target、实际来源及查询哈希、主体、对象资源、时间、变量、导出 ID、文件 Hash 和完整性。

复用 PlanV5 的持久目录、来源依赖、容量和明确删除规则；满额不自动删旧文件。Workspace/Sandbox 不可用时明确报告能力状态，不伪造下载或已分析结果。

### T2.8 离线文件分析与 AI 自主结论

~~~text
服务端完成/报告导出
→ AI 读取可信清单与实际文件
→ read/grep/bash/预装脚本处理三信号数据
→ AI 根据文件和用户问题判断
→ Chat 输出结论、依据和覆盖不足
~~~

- 数据导出由服务端完成，Sandbox 保持离线；AI 不直接连接遥测存储或在 bash 中下载数据。
- 日志、指标和 Trace 均以实际文件为分析依据；可以分段读取、编写脚本、统计、关联并生成派生文件。
- 沿用已发布结果或明细来源的字段/类型/时间，不把计算后的 Metrics 反称原始采样点，不把聚合计数当日志原文。
- 不用服务端固定摘要或有限 Top N 投影代替文件分析。进入模型上下文的是必要清单、命令输出与模型实际读取的片段。
- 不新增必填阈值、固定诊断规则或服务端确定性异常判定；AI 自行分析。结果不完整、缺失与推断依据仍须说明。
- 服务端不可变 source_ref/Hash/范围是原始事实；Workspace 内的清单和副本可被修改，不能自证原始性。派生文件明确区分，不覆盖审计来源。
- 记录工具实际执行与使用的数据文件，不能以模型声称“已读完”代替执行证据。
- 用户主要阅读 Chat 结论，不要求打开文件或图表；用户需要分析产物时复用 workflow.publish_file 等已有受控交付。
- 来源撤权沿用 Workspace 当前访问/计算限制；目录持久化和删除不另建自动清理策略。

### T2.9 Chat 与创建预览展示

- Chat 支持授权列表、明确选择、稳定 Mention 和自然语言问题。
- AI 创建使用 Tool 自有预览详情及宿主单次确认。
- AI 文件分析默认输出 Chat 文字结论，不强制展示图表或专用分析卡片；复用现有会话文件和任务状态。
- 不创建 Dashboard 页面分析工作台，不读取或同步修改页面筛选。
- Dashboard 页面未打开也能完成全部分析流程。
- Template Bridge 不执行查询，确认不通过模型二次推理。
- 复用现有双语、深浅色和桌面可访问性规范。

## 4. 工具与授权清单

~~~text
只读：
  telemetry.dashboard.list
  telemetry.dashboard.get
  telemetry.dashboard.context.resolve
  telemetry.dashboard.context.candidates
  telemetry.dashboard.budget.get
  telemetry.dashboard.query
  telemetry.dashboard.query.get
  telemetry.dashboard.query.cancel
  telemetry.dashboard.catalog.resources
  telemetry.dashboard.catalog # 创建模式的完整配置 Catalog
  telemetry.dashboard.convert

个人草稿（不发布）：
  telemetry.dashboard.draft.create
  telemetry.dashboard.draft.get
  telemetry.dashboard.draft.validate
  telemetry.dashboard.draft.save
  telemetry.dashboard.draft.drilldowns

模型可见的发布预览：
  telemetry.dashboard.publish.preview

仅隐藏 Action Catalog：
  telemetry.dashboard.publish.commit
~~~

模型可见工具经 PlanV5 自有三个元工具使用，并提供严格版本化 Schema、正式业务元数据和权限检查。隐藏 Commit 仅由 Action Executor 内部调用，不通过模型元工具调用。list/get/query 不能因为模型知道某个 ID 就跳过 Dashboard 对象授权。

Dashboard 功能权限收敛为 read/manage，与 Dashboard/Host/Cluster 对象授权、AuthorizationVersion 分别检查；分析/导出/读取配置不新增三信号或字段级权限。关联不授予数据权限。离线四工具和文件导入/发布复用既有 Workspace 能力入口及来源约束。

## 5. 测试与门禁

### 5.1 创建与发布

- UI/AI 共用契约：builder 和 DSL Panel 均能生成、验证、预览和发布；变量拒绝自由 DSL。
- 拒绝未知字段、互斥来源冲突、伪造派生查询、非法语法、类型、变量依赖和越权资源。
- 普通/明细查询必须明确发布；AI 查询工具内部交付文件，Dashboard 页面和创建表单不出现导出入口/导出来源设置。
- 无损转换失败仍保留 DSL。
- 硬错误阻止 active 发布；合法查询 no_data/unavailable 可带明确警告发布。
- 首次无 Draft ID 的 AI 创建、后续带版本修改、预览后编辑失效及同一新建草稿的跨预览去重。
- 私有计划不泄漏，Commit 不接受可变配置；重复提交、冲突、响应丢失与重启可恢复。

### 5.2 选择、查询与结论

- 未指定对象时先列授权清单并等待用户选择，不能先查询。
- @ 或明确选择后不重复询问对象；持久化和重放使用稳定 ID。
- 多个仪表盘分别检查对象授权，伪造选择或上下文被拒绝。
- 默认时间/变量、用户覆盖条件、未指定资源范围及连续追问继承正确；当前选值在新候选中消失则自动回退 All，并保持授权和系统资源边界。
- 泛问覆盖全部适用图，具体问题按需查询。
- 失败、无数据、预算不足和未完成项被准确记录并影响结论。
- 展示/明细来源不可临时改写，未配置原始数据获取和通用 Tool 绕过被拒绝。
- 导出分页/分片不重置总预算，显式 limit 与传输分页区分；同授权版本不同主体不共享越权缓存。
- 三信号真实文件交付、Hash/Schema、内容与完整性一致；空数据、部分文件和完整结果不混淆。
- AI 实际使用 read/grep/bash/脚本分析文件，自主给结论，不依赖固定阈值/摘要；派生文件不冒充原始来源。
- Workspace 未启用/不可用/满额、导出取消/重启、来源撤权和跨会话访问有明确结果。
- 对象访问者可看查询配置，管理能力控制编辑，不再要求三信号/字段独立权限。
- Q37 来源动态解析与执行冻结、Q38 不同来源同名实体不自动合并、Q39 UI/AI 标准下钻一致及跨信号目标限制；新模板版本不改写旧发布查询。
- 撤权、归档、版本变更、过期上下文、取消和 partial 均有明确结果。
- 在从未打开 Dashboard 页面的 Chat 中完成分析，确认没有页面状态依赖或页面变更。

### 5.3 临时 Kubernetes Namespace E2E

- 使用真实三信号数据完成 AI 两种 Panel 模式创建、服务端验证、宿主确认和发布。
- Chat 完成“未指定 → 授权列表 → 用户选择 → 真实文件导出 → 离线工具分析全部适用图 → 结论”。
- 覆盖 @、具体问题、默认参数、追问继承、明细来源、部分导出、预算/容量、撤权和 Dashboard 归档。
- 验证 Workspace 内实际字节及分析命令，不以模型文本声称文件存在或已完成分析作为验收。
- 覆盖模型重试、Worker 重启、Redis 清空、发布冲突、重复确认与恢复。
- 成功/失败按归属清理 Namespace、PVC、Topic、Bucket、Lease、测试 Action/绑定与临时诊断资源，保留脱敏证据并保护正常部署。

## 6. 退出标准

- AI 能用与人工相同的 builder/DSL 能力创建 Dashboard，经服务端验证和用户确认才发布。
- 用户可通过 @ 或授权列表明确选择一个或多个 Dashboard，AI 不猜测目标。
- Dashboard 没有导出功能；AI 数据查询工具在内部将已发布查询完整结果写入 Workspace，额外明细来自明确的已发布查询，再由 AI 使用文件工具分析。
- 泛问全覆盖、具体问题按需，证据不足不被总结为正常。
- 内部可追溯 Dashboard、Revision、Panel、来源查询哈希、资源/时间、export_id、文件 Hash 和完整性。
- 没有独立存储、权限、查询或确认路径；故障与授权变化下仍可恢复、可审计。
- 所有完成项均有实际测试证据，不把 Skill 接口存在或模型输出示例视为完成。

## 7. 主计划映射

对应 P2V-TOOL、AI-01～AI-07、E2E-02/03/05/06，以及共用的 EXEC、EXPORT、AUTH、ACTION、AUDIT 和 RELEASE 门禁。
