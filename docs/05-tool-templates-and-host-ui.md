# Tool 模板与宿主交互

本设计采用 PlanV5 基线。自有业务 Tool 随代码发布不可变模板；客户 Remote MCP 仅返回文字和结构化数据。模型不生成、选择、注册或执行平台模板。

## 数据与受众

一次 ToolCall 对应完整业务结果和有界模型投影。完整结果存于 `artifacts`，较大结果进入私有对象存储；模型只获得 `ToolResultProjection`。展示数据单独存于 `tool_presentations`，静态源码按 SHA-256 在 `template_assets` 中去重。

历史展示使用当次模板 Hash。读取展示时校验会话所有者、当前有效权限和资源授权范围；权限变化不会使旧结果自动获得新的访问权。模板源码不进入普通结果引用、ModelCall、压缩摘要或模型消息。

自有模板在对应 Tool 包中通过 `go:embed` 编译。启动校验源码大小、UTF-8、Runtime Version 和 Hash；源码上限 256 KiB。禁止远程依赖、动态代码下载及 `eval`。展示 Builder 只处理公开结果，移除提交令牌、内部计划和动作控制引用。

## Host 与 Runtime

2026-10-15 后续改造将工具展示收敛为业务模板、操作卡片、紧凑步骤和文件四类。Template Runtime 复用门户的列表、筛选、分页、字段网格和只读仪表盘布局，并使用相同 Tailwind/HeroUI 编译链；`result.page` 在封存集合内支持受限筛选排序。操作按服务端 ActionRef 合并，重复结果按 Query/Attempt 的结果键合并，不改变确认、权限与历史 Hash。完整边界及当前验收状态见[会话业务视图与管理门户统一](./plans/frontend-redesign/chat-business-views-20261015.md)。

2026-10-13 实施 `argus-template/v2`：工具绑定固定 Template，模板完成字段选择、布局、图表和本地交互。共享 Runtime 只提供预打包的 UI 原语和通用通道，门户不按工具 ID 渲染业务。旧 v1 资产按历史 Hash 保留，读取能力仅用于 v2。覆盖清单及执行证据见[实施与验收记录](./planv5/template-v2-implementation.md)。

共享 `@argus/ui` 提供 `ToolPresentationFrame` 和协议宿主；独立 `template-runtime` 应用负责加载模板。企业门户使用部署配置中的 `templateOrigin`，生产构建不得回退到本机或 Mock。

2026-10-15 会话工具结果默认收起为一行：工具名、执行状态、已有公开摘要、耗时和“查看”。共享 `ToolResultDisclosure` 在查看时才挂载展示查询与 Runtime，收起销毁 Runtime，刷新恢复收起；再次打开读取同一次封存结果。没有公开业务摘要时展示状态，不从 ID 或原始参数猜名称和统计。工具调用与稍后出现的展示事件按会话和 ToolCall ID 合并到原调用位置，未匹配的展示保留原位置；这只是显示投影，不改写 Ledger、正文或独立确认。

正文和输入区保持原阅读宽度。展开结果使用主内容区可用宽度（共享 token 上限 1600px），不越过侧栏或页面边界。2026-10-16 起会话使用 `context.layout=document`：iframe 按完整实测高度伸缩，取消 2000px 截断；代码、日志、列表和仪表盘内容随聊天页面纵向滚动，宽表保留横向滚动。共享 Runtime 的文档布局同样适用于历史 Hash，不改写封存源码。预览宿主仍可使用 `preview` 模式。收起按钮在顶部和底部提供，底部收起后焦点返回摘要按钮。参数/返回 JSON 通过顶部标签直接查看，宿主确认始终独立可见。详情见[置顶摘要与滚动验收](./plans/frontend-redesign/chat-summary-scroll-20261016.md)。

历史会话可同时包含大量卡片。共享宿主全局最多并行启动 4 个 Runtime，其他卡片保持原占位状态；启动名额在 ready、失败或销毁时释放，排队卡片卸载立即取消。15 秒加载时限从获得启动名额开始，避免浏览器并发请求耗尽导致无关卡片失败。该调度不含业务分支，不改变模板的数据能力、网络隔离、权限或结果绑定。

2026-10-16 起，卡片展开后通过顶部“视图 / 返回内容 / 查询参数”直接切换，替代卡片下方嵌套的调用详情。返回与参数取自同一次 ToolCall 的公开记录，沿用脱敏与大小上限；缺失记录明确提示。切换标签保留 iframe 与视图状态，收起整卡才销毁 Runtime；不发起新工具调用。详见[卡片结果切换](./plans/frontend-redesign/chat-result-tabs-20261016.md)。

会话 SSE 就绪事件按 `tool_call_id` 精确使该卡片的元数据失效，不能每次刷新全部展示。历史回放在 50 毫秒的固定窗口内合并事件列表刷新、合并同一卡片目标，并以最多 512 个近期事件 ID 去重；切换会话取消待处理刷新。展示元数据使用门户已有的 15 秒查询缓存，持久化事件主动刷新，实际数据读取仍逐次检查当前主体、权限和资源范围。

iframe 仅设置 `sandbox="allow-scripts"`，不开放同源、弹窗、表单、导航或下载能力。宿主通过绑定该 iframe 的 MessageChannel 传递上下文；Runtime 校验父窗口对象及父 Origin。协议 `argus-template/v1` 包含 nonce、严格递增 sequence、request_id 和大小上限。销毁后关闭通道，拒绝迟到消息。

CSP 拒绝网络、外部资源、嵌套框架和动态代码。模板仅接收公开详情、Locale、明暗主题和 design tokens。普通业务文本使用 `textContent`，不得把 Tool 数据拼成可执行 HTML。

业务 Bridge 只有三种操作：

| 操作 | 边界 |
| --- | --- |
| `resize` | 宿主限制显示高度；会话由外层摘要控制挂载，其他预览宿主可继续提供高度折叠/展开 |
| `open_resource` | 仅允许服务端提供的资源引用 |
| `open_result` | 仅允许服务端提供的结果引用 |

v1 不提供查询、翻页、刷新、换时间、确认或取消消息。v2 增加 `data_request/data_response/data_available`，请求 ID 相关联、序列递增、Nonce 隔离；拒绝重复请求 ID，限制并发和消息大小。依赖由固定 lockfile 预打包，在执行模板前完成加载，随后安装禁止网络的 CSP。

v2 只读接口 `POST /api/v1/conversations/{conversation_id}/tool-presentations/{tool_call_id}/read` 要求会话所有者、当前主体、权限、资源范围和 CSRF。每个展示保存声明操作、数据绑定、Query/Attempt；只允许资源上下文、指标上下文、告警详情、封存仪表盘、运行信息与封存结果分页，不接受任意 URL、业务工具调用或提交。游标绑定主体、授权版本、展示、数据版本、集合及分片。

冻结仪表盘的 `result.page` 同时接受提供方的对象封装和 PromQL 原始 Matrix/Vector 数组；在已绑定的 `panel_id/target_id` 内，将原始数组按 `items` 读取，`index/field=values` 遍历完整样本。跨目标复用游标会失败；这只扩展既有封存格式的读取，不扩大资源范围或重新执行查询。

组合告警以事件中的 `state.members` 展示各子规则当时的名称、表达式、实际值、单位、阈值及触发/恢复判定；组合模式和比较符从该事件的策略快照取得，不能读取当前配置替换历史事实。组合 `rule_id` 为空 UUID 时显示“组合规则”，列表与历史展开使用相同业务模板。成员单位分别保留，不把不同单位合并为一个值。

告警实例分别展示生命周期、实例评估健康和通知状态。成功评估在实例 `state.last_successful_at` 保存产生当前 Value/Members 的评估时间，失败保留该时间和数值；`last_evaluated_at` 仅表示最近尝试。模板在失败、无数据或权限不足时明确标注保留值，列表摘要和详情使用相同语义；缺失时间显示未知，不以最近尝试补齐。成功评估时间不等于遥测采样时间，不伪造采样时间。健康事件不显示虚构的指标值/阈值。Connector 列表软件版本读取 `software_version`，与记录版本、连接 epoch、收敛状态分开；缺失版本明确显示未知。

Connector 列表和详情在工具展示投影中补充 `related_resources`（id、type、name、status）。同一主体的权限、分类型资源授权和展示 Scope 使用同一访问快照，关联主机/集群通过一次批量查询解析；必须同时拥有相应 `host.read`/`kubernetes.read` 与该类型的资源授权。无权或已删除对象统一显示“未授权或已删除”，保留原返回的 ID，不泄露名称或生成导航能力。仅已解析且授权的引用进入宿主导航白名单。所有名称和状态表示查询时事实；历史 Hash 与冻结数据仍独立保存。

查询完成后以 Presentation 为键幂等保存封存数据与会话 `tool_presentation(status=data_ready)` 事件；后台发布器补偿持久化竞态和 Worker 重启，刷新读取相同事实。占位卡片中央显示菊花与“正在渲染中”，失败和超时提供重新加载入口。大序列采用覆盖全时间窗口的有界预览，保留精确统计；完整数据仍由封存分页/分片及会话文件读取。Chat 中改时间或资源仍由新消息产生新 ToolCall 和卡片。

普通 PromQL 指标的展示预览在每条序列的点数预算内优先保留首尾、有限值全局极值，以及有效值和非有限值（NaN、正负无穷、null）转换的两侧原始样本，再补充全窗口局部极值和实际观测点。共享 `ViewData` 同时用于独立指标、冻结仪表盘及序列分页；字节压力下重复缩减仍遵守上述约束。若必需的缺测边界无法容纳，`values` 返回空预览并通过 `argus_sample_preview.state=omitted_gap_budget` 明确说明原因，不把曲线省略解释为无数据，也不跨缺测区间连线。精确 `argus_statistics` 和完整封存样本不变，统计图型继续读取精确统计，按 `field=values` 的分页仍提供完整样本。共享图表统一展示采样/省略说明，模板和门户的业务边界保持不变。

业务数据展示以正式类型为准：目录展示按本次 `kind` 区分 metrics/fields/values/resources，保留单位、字段类型、已选值存在性、注册能力与业务页完整性；展示分页只遍历封存结果，不隐式继续业务查询。Collector 使用 desired_revision/effective_revision/last_seen_at，记录版本与发行版本分开，修订相等仅表示配置修订一致。告警规则使用 expression；历史阈值和规则来自触发快照，当前策略另行展示。夹具按真实嵌套字段构造，并同时验证可见业务内容与布局。

健康类告警的 kind=health 表示评估健康事件，列表、历史和详情均将指标值/阈值标为不适用。KQL 的 log_entries 使用日志字段，table/timeseries 使用实际返回的分组及聚合列，不将聚合结果套入日志正文列。

Trace/APM 由查询提供方按正式 GraphQL 文档解析根类型、别名、片段及多根结果，展示投影保存为 result 集合；根响应 key 保留，字段别名还原为 schema 名。每个结果保留 rows/spans/nodes/edges 及实际统计依据、覆盖状态。只读分页复用既有 collection=result、index、field 选择器；游标绑定结果根和数组字段，新增 nodes/edges 明确白名单。完整原始 ToolResult/分析文件不改写，展示冻结结果避免重复存储原 GraphQL 响应。业务布局仍在工具模板，宿主不解析 GraphQL 或分发业务类型。共享图表在容器尺寸有效后初始化，折叠或隐藏 iframe 不触发零尺寸图形计算。

APM RED 的曲线与精确表格使用分开的投影：查询提供方在展示副本生成 `chart_preview`，按 source/resource/service/instance/operation 分组，在总点数预算内保留每个已展示序列的首尾、三类 RED 数值的极值及空值边界，补充其余实际采样点；不平均 P95、不合并不同资源，也不补造数值。无法在预算内保留空值间隔的序列或身份/时间不明的记录明确计数并标为未绘制。普通数组截断不再影响曲线的时间覆盖。独立调用链模板和冻结仪表盘共用该提供方投影及预打包图表；完整封存 `rows` 不变，仍可分页读取。仪表盘 APM 目标沿用既有 panel_id/target_id + collection=result/index/field 选择器，按冻结响应根 key 排序绑定索引，不新增读取能力或网络通道。

每个 APM 根分别展示 `limited` 的 true/false/未提供状态；false 仅表示没有被业务 limit 截断，不代表数据完整。业务限量、执行 partial、曲线采样和封存分页分别表达：业务 `limited=true` 提示未返回的结果需新查询，缺失字段显示未知；展示页翻完不等于业务查询没有更多结果。查询时间、资源纠正仍产生新的 ToolCall/卡片，历史 Hash 与数据保持不变。

## 单次确认

PendingAction 控件独立于 iframe，从权威公开预览显示影响范围、风险、确认/取消以及审批和执行状态。模板仅提供详情，不能改写宿主影响范围或决定确认入口。

用户确认后，服务端重新校验主体、AuthorizationVersion、动作状态、资源版本、审批条件和幂等键，由 Action Executor 执行冻结计划。确认不再调用模型，也不增加第二次确认。执行后的自动验证是 Run 上持久化的只读阶段，Worker 恢复不会重新取得变更权限。

模板加载失败只展示详情错误占位。有效公开预览仍由宿主显示；预览不可用或过期时禁用确认，不能从模板重构授权。

## 文件

附件按钮上传真实字节到当前会话 Workspace，展示进度并保存服务器文件引用。上传与代码执行串行；失败不破坏已有目标。容量预检失败保留用户输入和附件。

`workflow.publish_file` 核验 `/workspace` 文件并发布不可变交付对象。下载使用企业/会话授权、Hash、私有缓存策略和 Range，不接受模型提供的任意外部 URL。工作文件后续修改不影响历史交付。

`workspace_file_added` 与 `artifact_published` 在历史和流式会话中均作为独立文件活动展示，不因事件缺少 `run_id` 而生成用户消息。相邻同一次仪表盘查询文件可折叠分组；历史刷新与流式交付按文件 ID 去重。没有正文或其他可展示内容的消息不显示标题、头像或空气泡；用户文字消息保留名称与时间，不显示用户头像。

会话输入区由外层复合控件统一承载边框和焦点光晕，内部共享 `Textarea` 使用 `appearance="embedded"`，不重复绘制焦点框。普通表单的 Textarea 保留默认焦点样式。相关修复与验收范围见[会话输入与文件展示记录](./plans/frontend-redesign/chat-composer-and-files-20261012.md)。

## 验收

会话正文由共享 Markdown 控件渲染；工具步骤、卡片与文件按实际事件序列展示。用户输入是视觉回复分界：连续 AI 正文、工具、卡片、确认及自动文件归入同一回复区，只出现一次身份标题；Run 或工具回调不重复生成 AI 头像与大段间隔。历史与流式共用这一投影，不修改原始事件。工具公开参数/返回投影、资源名称导航以及授权文件大弹窗预览的实现与历史版本边界见[会话展示修复记录](./plan6/chat-ui-followup-20261012.md)。业务展示模板继续由 Tool 所有，不通过门户改写历史模板 Hash。

2026-10-13 工具步骤和结果卡片名称按当前语言显示，原始 ID 保留为展开详情中的次要信息；名称由公开 Tool Discovery 生成，未知/客户 MCP 名称保留原文，不改变路由或授权。业务模板已按固定 v2 入口实现；工具覆盖、布局和验证结果见[实施与验收记录](./planv5/template-v2-implementation.md)，早期问题分析保留于[名称与业务模板分析](./planv5/chat-tool-labels-and-business-templates-20261013.md)。临时真实工具链和正式发布以验收记录的实际状态为准。

验收覆盖桌面 Web、中英文及明暗主题，包括三类越权 Bridge 消息、重放/nonce 校验、模板错误降级、宿主单次确认、审批执行状态，以及真实上传到离线分析再到不可变下载。实际执行结果见 [PlanV5 实施记录](./planv5/implementation-status.md)，不能以设计描述代替验收完成。
