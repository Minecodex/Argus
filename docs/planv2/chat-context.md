# Chat 仪表盘引用与原生工具

本页记录已经接入的实现；完整验收范围及未完成项以 [实施状态](./implementation-status.md) 为准。

## 自动发现、条件和 Run

2026-10-13 的[自动分析规则](./automatic-analysis-20261013.md)替代手动选择前件。用户描述目标，AI 用 list 检索授权候选并用 get 读取相关定义；get 经服务端授权，在会话锁内为本 Run 追加条件基线，最多采用 20 个仪表盘。目录不会自动选中全部结果。@ 作为自然语言中的稳定对象提示传递，不要求点击或管理范围。

消息接受时的 dashboard_run_contexts 保持不可变；运行时发现从 dashboard_run_parameters 组合，参数解析和每次查询分别生成不可变 Context/FrozenPlan。旧 dashboard_context 字段仍支持 none/analyze/create 和版本校验，作为调用方提供的初始提示，不再限制 AI 对其他授权仪表盘的发现；普通后续消息不会继承旧手动分析提示，条件在 get 时按仪表盘独立继承。GET /conversations/{conversation_id}/dashboard-context 读取的是接受消息时保存的提示；页面不维护常驻范围编辑器。

context.resolve/v2 保存用户消息解释或 ai_selection 来源的条件，独立记录 quote 或 reason。用户明确限制只能由其本轮纠正修改；AI 决策不能冒充人工输入。新查询冻结最新发布版、有效资源、绝对时间、变量和局部筛选，结果模板顶部直接展示这些服务端事实。用户纠正后重新取数并分析新文件，旧结果保留原参数。

/创建仪表盘 与 /create-dashboard 继续启用创建 Skill。普通会话加载 reference Revision 4，分析 Skill Revision 5 自动检索、解析、取数和分析；创建的宿主发布确认保持不变。每次读取、调用和恢复重新校验企业、主体、对象状态及授权。

条件兼容性、候选回退、累计预算见[条件契约](./chat-conditions-budget.md)。

## 原生工具

所有业务工具仍由 tool.search → tool.describe → tool.invoke 发现和执行，分类为 dashboard。模型不能提供 Run/会话身份、编译查询、对象存储路径或提交 Token。

| 名称 | 行为 |
| --- | --- |
| list | 授权已发布目录；有界分页，由 AI 按问题挑选相关定义 |
| get | 授权仪表盘的最新发布定义、继承条件和时钟；记录本 Run 对象 |
| catalog.resources | 分析/创建模式分页查看授权 Host/Cluster 名称与注册来源能力；not_sampled 不代表已有数据 |
| context.resolve | 合并本轮条件，校验版本、用户引用或 AI 选择来源，返回不可变 context_ref |
| context.candidates | 读取已发布过滤器及依赖的候选，预览不保存条件 |
| budget.get | 读取当前 Run 共用的持久剩余额度 |
| catalog | 按真实数据发现指标、字段及候选，复用人工 Catalog |
| convert | 与 UI 相同的构建器/语句无损转换 |
| draft.create / draft.get / draft.save | 个人草稿创建、读取和按版本保存；已有 Dashboard 必须先由 get 授权读取并纳入本 Run |
| draft.validate | 创建模式复用人工编辑器的硬校验与样本检查，返回问题路径及原因；不发布，不授予分析模式执行未发布查询的能力 |
| draft.drilldowns | 按来源和图型生成平台标准下钻，保存到同一草稿 |
| publish.preview | 硬校验、独立样本状态、不可变 PendingAction；绑定当前 Run，由宿主确认 |
| query | 使用 context_ref 执行已发布展示查询或下钻，建立持久文件任务 |
| query.get / query.cancel / query.resume | 进度、取消、失败恢复及已封存文件交付 |

draft.create 按不可变工具调用身份和初始输入复用个人草稿，响应丢失后重试不重复创建。draft.save 和生成下钻检查草稿版本。已有仪表盘打开个人草稿时以已发布基线初始化，后续修改通过 draft.save 保存。

创建 Skill Revision 2 要求先读取完整契约，并在保存后处理返回的结构/编译 `validation.issues`，再调用 `draft.validate` 运行共享编辑器校验。配置错误返回 `valid=false`；未执行样本明确标为 `not_executed`，发布仍受原有硬门禁阻止。该反馈同样适用于人工样本 API；无数据、临时不可用与配置错误分开。公开 Schema 明确版本、时间类型、编辑模式、语言及完整步长策略，模型不用猜测字符串含义或通过反复转换推测参数。

创建 Skill Revision 3 补齐人工编辑器的资源类型默认规则：用户未限定时，统计图同时适用 `host` 与 `kubernetes_cluster`。只有用户明确限定或来源能力不支持时才缩小；不能把当前样本只来自某一类型，误作仪表盘永久排除另一类型的依据。无数据仍是独立样本状态，不能通过擅自缩小适用类型回避真实数据。

创建 Skill Revision 4 与 catalog.resources 输出 v2 提供明确的时间依据：`server_time`、企业 `timezone` 及 `suggested_catalog_range.from/to`。新建默认最近一小时的目录检索可直接复用该窗口，其他相对时间按服务端时钟推算；不能猜测当前年份或无目标扫描历史月份。目录不完整限制缺失/全集推断，不否定已经返回的指标及类型存在证据。真实 PostgreSQL 回归验证即使资源目录为空也返回当前时钟，建议窗口与默认一小时一致。

将草稿写入与 publish.preview 拆开，是为了保留 Q2/Q9 的“积累修改、保存不反复确认、最后统一发布”，并复用人工草稿服务。服务端私有提交处理器仍为 telemetry.dashboard.publish.commit；不向模型暴露执行入口，没有第二套 AI 发布逻辑。

分析模式只开放仪表盘读取/查询任务、文件交付和离线 Workspace 能力；创建模式另外开放配置工具。通用三信号查询、资源变更和客户 MCP 在这些模式下不作为替代查询通道。普通会话继续保留 PlanV5 的客户 MCP 直连。Gateway 缓存及分页上下文包含 Run ID，避免不同模式复用发现结果；权限和选择在读取缓存前重验。

query 自动绑定所属 Run。新根查询读取最新发布版；同一 Run 的子下钻沿父任务冻结版本和参数。新 Run 不能通过恢复未物化的旧任务绕过最新版本规则；已物化的旧文件可以继续交付，但必须保留原清单的版本、时间和完整性。文件交付完成不代表模型已经分析，结论仍需实际 read/grep/bash 证据。

创建模式的 `get` 只读取已选仪表盘定义，不要求分析模式的条件记录。确认后的公共执行事实同时保留输入目标和 `result_resource_*` 输出身份；对象生命周期版本、发布版本号和会话条件版本分别解释，不能把不同计数器直接比较。该补充不扩大仪表盘选择或查询权限。

## 验证范围

查询工具输出 v2 另附当前 `run_budget` 与 `run_budget_observed_at`，与不可变查询清单分开。分析 Skill Revision 4 要求发生失败或限制时在文件检查后重新读取 `budget.get`，再描述各维余额。失败取数会消耗预留预算，因此“本次查询服务不可用”与“之后预算已耗尽”可以同时成立；不能仅凭取数前余额或剩余调用次数宣称预算充足。

后台文件交付遇到前台 read/bash 持有 Workspace 租约时，在读取文件字节之前最多等待两分钟，每次重新校验来源授权；取消仍立即生效。只等待 `WORKSPACE_BUSY`，不在这层重放文件写入或重试容量不足、撤权和失去租约。查询执行状态与模型分析覆盖分开：清单的 Panel `success` 不表示文件已交付或 AI 已分析。

本阶段验证包括真实 PostgreSQL 的消息事务、不可变 Run、版本冲突、撤权、工具范围和草稿/预览，以及 mock 浏览器双语主题下的旧选择流程、追问和创建入口；当前自动流程另见 2026-10-13 专项记录。后台文件任务已有此前的真实 PostgreSQL/ClickHouse/MinIO 分段验证。

正式 Worker、Collector/Kafka、Workspace PVC 的确定性文件链路后续已经通过；本轮又以独立 PostgreSQL/ClickHouse/MinIO 验证条件状态、工具选择、隐藏 Commit、文件下钻与恢复。正式 Chat 界面和部署中的工具组合验收记录见 [非模型收尾](./non-model-closure-20260927.md)。此前暂缓的是非模型收尾阶段的安排；后续真实 GLM 的首期代表场景已取得分轮通过证据，当前状态与准确范围见 [真实模型验收](./real-model-validation-20260928.md)。
