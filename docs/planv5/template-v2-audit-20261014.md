# 工具模板计划完成度复核（2026-10-14）

## 最新结论

**2026-10-15 R8：原计划全部完成，A1～A16 已关闭，无未关闭项。** [实施状态](./template-v2-implementation.md)、[逐项验收](../../artifacts/template-v2-r8-plan-review.md)、[当前矩阵](../../artifacts/template-v2-test-matrix.json)、[发布核验](../../artifacts/template-v2-release-20261015-r8/release-verification.json)。以下保留历轮发现与修复前证据。

## R8 A16 关闭结果

- 共享普通指标采样保留端点、有限值极值及缺测转换边界；预算不足明确省略曲线，完整统计/封存值不改写。原 8 个实际模板复现均恢复正确。
- 统计卡片正确区分缺测最新值和无数据；独立指标与冻结仪表盘共用共享图表。正式 Matrix/Vector 数组可按绑定目标读取完整样本，跨目标游标拒绝。
- Go 全量、1385 前端测试及完整类型/i18n/Lint/样式/契约/构建/预算通过；完整浏览器同次 352 通过、0 失败、0 flaky、141 按范围跳过。493 项清单按正确测试环境执行，没有删除断言。
- r45：92 展示、55 业务视图、2 封存/2 SSE、重启后 92 一致、重复就绪 0；9002 指标样本及1800 APM 行完整分页；实际元数据 121 次、每卡最多2次、资源错误0，健康20/20。1200受控Span走真实遥测链路；真实模型未要求、未评测。
- 34 张全分组汇总页、6 张新增展开汇总页和重点原图均已人工查看，所有模板组及19图型、当前Hash/语言/主题/桌面矩阵与几何检查通过。
- 临时资源按所有权/UID清理，20原工作负载身份、副本与Ready恢复；新镜像发布revision 24。正式Chatbox、9运行容器实际imageID、健康20/20与16项保留检查通过，9PVC、93原Secret UID、37工作负载、3工作区/27文件保留；2273构建输入一致，632变动代码文件≤2000行。
- r41/r42受控公式的毫秒窗口、r43冻结零小数位断言、开发模块连接耗尽及源模块注入测试环境问题均保留失败记录并完成闭环。r44通过后补齐原始Matrix分页，再完整执行r45；未用旧通过项掩盖新分支。

证据：[门禁](../../artifacts/template-v2-r8-gates.json)、[原用例修复后](../../artifacts/template-v2-r8-sampling-proof/metric-sampling-render.json)、[r45](../../artifacts/tool-template-v2-k8s/r45/acceptance-summary.json)、[视觉](../../artifacts/tool-template-v2-ui/visual-r8/visual-review.json)、[展开](../../artifacts/tool-template-v2-ui/r8-sampling-expanded-final/visual-review.json)、[发布](../../artifacts/template-v2-release-20261015-r8/release-record.md)。

## A16 原始发现（修复前记录，现已由 R8 关闭）

`internal/presentation/view.go:30` 将桶内首点直接作为低/高点；该点为 NaN 时，后续有限值与 NaN 的比较均为假，真实峰值不能进入预览。桶内 NaN 又不会被选为低/高点，可能消失，曲线由此跨缺测连线。`result.page` 最终仍调用 `ViewData`，普通系列翻页不能恢复该曲线。

以正式 Prometheus `promql.Matrix` 序列化构造 600 点，经实际 `BindData`、`ViewData`、`PageData`、当前 Hash 模板与共享 `metricObservations`：

- 桶首 NaN 后的峰值：封存/精确统计最大值 10000，最终图表输入最大值 1。
- 桶内 NaN：封存中 1 个缺测标记，图表输入中 0 个。
- 独立指标与冻结仪表盘指标面板、中英文共 8 个受控复现，原始 600 点未丢失，模板源码 Hash 未改变。

证据：[构造与 Go 投影](../../artifacts/template-v2-review-20261015-r8/metric_sampling_probe.go)、[实际模板执行](../../artifacts/template-v2-review-20261015-r8/metric-sampling-render.mjs)、[输出](../../artifacts/template-v2-review-20261015-r8/metric-sampling-render.json)、[结构化复核](../../artifacts/template-v2-review-20261015-r8/review.json)。该次只读审查不是完整浏览器或临时 Kubernetes 重验，也没有修改产品代码或部署。

已发布的 2266 构建输入无变化；实时 revision 23，9 个 R7 容器 Ready、实际镜像与发布一致，9 PVC 身份和卷保留且 Bound，临时 Namespace 零残留。当时要求修复、补测、重验和重新发布；上述要求现已由 R8 完成，未以 R7 旧结果替代新分支验收。

## R7 A12～A15 关闭结果

- A12：提供方生成独立曲线投影，200 点预算内按 source/resource/service/instance/operation 保留已展示序列的全窗端点、RED 极值及空值边界；精确 rows 不改写，独立工具和冻结仪表盘使用相同投影与已声明的封存分页。
- A13：五类 APM 根分别展示业务限量 true/false/未知，明确未返回的业务结果需要新查询；与执行 partial、曲线采样和卡片分页分开。false 不冒充业务完整。
- A14：共享宿主全局最多启动 4 个 Runtime，ready/失败/销毁释放名额，卸载取消排队；89 卡片受控压测和真实 r40 均通过，原产品加载超时未放宽。
- A15：历史 SSE 按 ToolCall 精确刷新、50ms 合并并去重，复用 15s 元数据缓存；QueryObserver 回放 89 卡片仅 178 读取，真实 r40 每卡最多三次、资源错误零。r37 的 2554 元数据请求与 1194 资源错误证据保留，修复后重跑完整门户和真实链路。
- 共享 Select 的文字挤压、复制的勾选图标和菜单过高已修复；全部语言/主题/桌面组合检查文字宽度、菜单高度及 P95 选择，保留原几何断言。
- 新增 19 专项业务夹具，现有 65 专项 + 4 基础样本 = 69 业务场景；Go、1351 前端测试及全部类型/i18n/Lint/样式/契约/真实构建/预算通过。完整门户单次 352 通过、0 失败、0 flaky、141 按范围跳过；正常 372、边界 840、失败读取 120、业务 828、19 图型 228 截图，另有 APM 展开 114。
- 临时 r40：1200 受控 Span 经过真实 OTLP/Collector/Kafka/ClickHouse/Query；89 ready 展示、2 封存/SSE，Worker 重启后 89 一致、重复就绪 0。15 类业务限量状态、两工具根各 600 精确行及冻结仪表盘 600 精确行可分页，窗口端点、实际 P95 峰值与空值检查通过，健康 20/20。
- r33 的共享配置参数缺失在预检停止；r34 的真实数据库测试暴露了同槽重复的夹具和微秒精度幂等问题。保留失败、按 UID 清理并恢复 20 原工作负载后，用修正的连续时槽和新增重复槽断言重跑 r40，未跳过门禁。后端这两文件修复期间前端/模板输入未变，Go 与真实链路重新执行。
- 最终临时 Namespace/集群资源/加载器/卷零残留，20 原工作负载 UID/副本/Ready 恢复；共享 Controller 沿用现有受信任身份，未改变其资源或所有权。
- 新 Backend/Web 固定摘要发布 revision 23：实际 9 容器 imageID、健康 20/20、正式 Chatbox 加载/刷新和验收企业删除通过。16 项保留核验通过，9 PVC、92 原 Secret UID、32 工作负载及 3 工作区/27 文件保留。2266 构建输入一致，625 变动代码文件 ≤2000 行。

证据：[R7 门禁](../../artifacts/template-v2-r7-gates.json)、[r40 摘要](../../artifacts/tool-template-v2-k8s/r40/acceptance-summary.json)、[视觉报告](../../artifacts/tool-template-v2-ui/visual-r7/visual-review.json)、[发布与保留](../../artifacts/template-v2-release-20261015-r7/release-verification.json)。确定性回放不作为真实模型评测；真实模型按本计划未要求、未执行。

## R7 验收发现与修复过程（已关闭）

### A14：大量 Runtime 同时启动

r35 的 89 份封存展示和 Worker 重启读取一致，但真实 Chatbox 同时启动 89 个 Runtime 时，Chromium 记录 1408 次资源耗尽错误；第 17 个 iframe 加载失败，UI 验收超时。共享宿主加入最多 4 个并行启动名额，ready/失败/销毁释放，卸载取消排队；15 秒产品加载时限从实际启动开始。受控压测又捕获排队 iframe 空白页提前消耗握手监听器的问题，修正监听器注册时机并补回归，89 卡片加载与逐卡内容检查通过。

r36 在上述压测失败后中断，按 UID 清理自有 Namespace、RBAC、Ingress、Lease，并依照既有本地规则处理从未分配地址的自有删除中 LoadBalancer；20 个原工作负载身份、副本和 Ready 恢复。没有清理外部共享 Controller 或无所有权的 CRD。

首轮 R7 全门户命令漏传编译后 Runtime origin，349 通过、144 跳过中有 3 个必须执行用例。该报告保留，补齐参数后完整重跑；最终 352 通过、141 按范围跳过。

### A15：历史 SSE 放大元数据请求

r37 的 Linux 容器产物与失败浏览器加载的 chunk Hash 一致，包含启动队列；轨迹中的 89 个展示元数据 GET 共 2554 次，最高每卡 30 次，Chromium 记录 1194 次资源耗尽错误。就绪事件原先使整个会话的展示查询失效，历史回放放大了请求。

已按 ToolCall 精确失效、50ms 合并列表刷新和近期事件去重，展示元数据沿用门户 15s 缓存。五项补测包括真实 QueryObserver 的 89 卡片回放，断言每卡两次、共 178 次；r38 的类型导入路径错误在构建阶段失败，修正后通过全部类型和构建门禁。r39 真实浏览器请求已降为 122 次，每卡最多两次、资源错误零，但仍因下述夹具不满足预期失败，未计为通过。

### 拓扑限量夹具与最终关闭

拓扑 limit 约束调用边，r39 的独立 RED Span 没有提供第二条真实调用边，实际 limited=false 正确。r40 为同一 1200 条 RED 观测补齐实际父子 Span 关系，每条 Trace 保持两节点树、服务间有两个方向的观测调用边；增加构造回归和 15 类限量状态前置校验。此次仅修改 Go 夹具及验收代码，完整前端和模板输入未变。Go 全量和 m4e2e 夹具测试重新通过。

r40 完整真实工具链、89 卡片、15 类限量、冻结仪表盘、SSE、刷新、Worker 恢复及 20/20 健康通过；实际元数据请求 120 次，每卡最多两次，资源错误零。清理与恢复通过后，R7 新镜像发布 revision 23，正式 Chatbox 与全部 16 项保留核验通过。历史失败与中断证据继续保留。

## R6 再次复核：APM 完整性原始发现（已由 R7 关闭）

| 编号 | 问题 | 依据与实际模板复现 |
| --- | --- | --- |
| A12 | RED 大结果预览截去最新时段，不满足覆盖全窗口的有界预览约定。 | `internal/presentation/view.go:82` 对普通数组取前 200 项，仅 `values` 样本对执行全窗口采样；`internal/telemetry/templates/trace.html:30` 直接把该 `view.rows` 传给 RED 图。600 个逐秒点最终只有 200 个进入图表，末点由 10:09:59Z 变为 10:03:19Z。通用预览警告存在，封存数据仍完整；问题是曲线没有覆盖查询窗口及最新点。 |
| A13 | 服务/实例/入口/RED 的业务限量事实未显示。 | `internal/telemetry/queryengine/skywalking/apm_query.go:77` 在实际组数超过显式 limit 时设置 `limited=true`；`internal/telemetry/templates/trace.html:29` 不读取该字段。冻结投影含 `limited=true`、`status=available` 的服务摘要，无受限提示或字段；展示分页不能取回此次未查询的业务组。 |

两个输入均通过正式 Schema 校验，执行实际投影及当前 Hash 模板，中英文共 4 个受控检查。不是实际查询、完整浏览器视觉、临时 Kubernetes 或真实模型测试。[Go 构造程序](../../artifacts/template-v2-r6-review-apm.go)、[输入与投影](../../artifacts/template-v2-r6-review-apm-inputs.json)、[实际模板执行程序](../../artifacts/template-v2-r6-review-apm.mjs)、[输出](../../artifacts/template-v2-r6-review-apm-probes.json)。

注册表/覆盖/当前资产一致，2,252 个构建输入没有漂移；相关 Go 包（缓存）及重新执行的 839 项 Runtime 测试通过。R6 1263 项前端测试、352 浏览器通过和 r32 的 73 展示仍是既有覆盖范围的证据，不包含以上两个新分支。实时 revision 22、关键服务 Ready、9 PVC Bound、临时 Namespace 零残留。修复内容、补测对应 Chatbox 与临时真实工具链并重新发布后，再恢复完成状态。

## R6 A9～A11 关闭结果

- A9：健康事件的值/阈值明确标为不适用，no_data/error 的列表、历史和详情分别测试；不再将类型零值作为阈值。
- A10：日志 table/timeseries 根据真实返回列展示分组、计数/数值和时间，保留零值、空值与实际分页；使用正式 KQL 编译结果及真实查询验收。
- A11：正式 GraphQL 提供方投影支持 10 个根、别名/片段/多根和重复选择；Trace、服务/实例/入口、RED、拓扑分别渲染。页游标绑定结果索引/字段，跨根及越界拒绝，Host 不增加业务渲染分支。
- 另修复零尺寸容器图表初始化、RED 单点标记及会话历史首分页限制。长表格截图以自然滚动分段覆盖，各段仍执行严格边界断言；旧轮失败证据保留。
- Go 全量、1263 项前端测试及类型/i18n/Lint/样式/契约/构建/预算通过。最终门户单次 352 通过、0 失败、0 flaky、141 按独立环境或移动端范围跳过；当前 31 Hash × 12 正常矩阵及 50 × 12 业务场景齐全，19 图型、840 边界及 120 读取失败截图齐全。
- r32 真实工具链 73 份 ready 展示、2 次封存/SSE；Worker 重启后 73 份一致，重复就绪 0；20/20 健康。临时资源零残留，原 20 个工作负载 UID/副本/Ready 恢复。r30 的历史分页、r31 的长表格捕获失败已修复后重跑，不计为通过。
- Backend/Web 重建固定摘要发布 tool-templates-20261015-r6、Helm revision 22；9 个实际容器 imageID、20/20 健康、正式 Chatbox 加载/刷新和自有验收企业清理通过；16 项业务/配置保留检查全部通过，9 PVC、91 个原 Secret UID、35 工作负载及 3 个工作区/27 个文件保留。

证据：[R6 门禁](../../artifacts/template-v2-r6-gates.json)、[r32 摘要](../../artifacts/tool-template-v2-k8s/r32/acceptance-summary.json)、[截图复核](../../artifacts/tool-template-v2-ui/visual-r6/visual-review.json)、[发布保留核验](../../artifacts/template-v2-release-20261015-r6/release-verification.json)。构造告警事实与实际遥测查询分别记录；按原计划使用确定性回放，真实模型评测未要求、未执行。

## R5 再次复核：A9～A11 原始发现（现已关闭）

| 编号 | 问题 | 依据与实际模板复现 |
| --- | --- | --- |
| A9 | 健康类告警列表显示无业务含义的零阈值。 | worker.go:345 的 healthEpisode 使用 Kind=health、无 Value、Threshold 类型零值；templates/result.html:36 的事件/历史值列统一格式化 Value/Threshold。当前列表为 `— / 0`，同时提示“无指标观测值”；详情已排除健康事件值/阈值，列表不一致。 |
| A10 | KQL 聚合主表漏掉分组和聚合值。 | queryengine/kql/query_plan.go:88、:103 支持 table/timeseries 并输出 group_value/count；templates/log.html:7 始终使用日志明细列。输入 group_value=orders-api/count=427 时，table 主列四个 `—`，timeseries 只有时间；原始详情虽保留字段，主表未展示业务结果。 |
| A11 | 部分有效 Trace/APM 返回被错误判为空。 | projection.go:55 和 templates/trace.html:8 仅处理部分 Trace 根；正式 skywalking/schema/trace.graphql:53～:57 含 queryAPMServices/Instances/Endpoints/RED/Topology，另有 queryBasicTracesByName。六种有结果的当前模板均显示空态，拓扑图为零。模板读取 queryServiceTopology，而该根不在正式 schema 中。 |

复现执行实际 BindData 与当前 Hash 模板，使用正式 alerting.Instance、KQL 编译器支持的结果形态及真实 Trace/APM 根；中英文共 20 个受控检查。证据：[输入与投影](../../artifacts/template-v2-r5-review-contracts.json)、[Go 构造程序](../../artifacts/template-v2-r5-review-contracts.go)、[模板执行结果](../../artifacts/template-v2-r5-review-probes.json)、[执行脚本](../../artifacts/template-v2-r5-review-probes.mjs)。本次不是实际查询、真实模型、完整 Chatbox 视觉或临时 Kubernetes 重验。

覆盖声明与资产无漂移，相关 Go 包及 752 项 Runtime 测试仍通过，说明现有绿灯遗漏了这些分支。R5 原 1170 前端测试、352 浏览器/0 失败、r29 的 54 展示和发布 16 核验结果继续保留；不能作为 A9～A11 的通过证据。关闭后仍需补用例、相应 Chatbox/临时真实工具链验收及新发布，再恢复整体完成状态。

## R5 A7、A8 关闭结果

- A7：Connector 主列读取正式 software_version，已知版本正确呈现、缺失显示未知。绑定正常预期、已知/未知/空态专项、正式数据库工具返回和真实 Chatbox 主列均验证；区分记录版本及连接 epoch。
- A8：生命周期、实例评估健康和通知状态分开；成功评估写入 state.last_successful_at，失败保留该时间、数值及成员；最近评估尝试单列，不冒充采样时间。覆盖异常、无数据、撤权、恢复、未知时间和历史展开，临时 PostgreSQL 评估器检查与真实工具链均通过。
- 31 Hash × 12 正常组合齐全，业务专项 336 张；另有 54 张健康/版本可见区域截图。人工复核 29 张汇总页及重点原图，未自动更新基线。
- r27 的回放上下文不足、r28 的平滑滚动截图坐标失败均保留记录，分别调整测试夹具容量和截图测量；生产限制与几何断言不变。r29 完整通过，三轮临时资源零残留、原 20 个工作负载恢复。
- 新镜像固定摘要发布至 revision 21；9 个运行容器 actual imageID 验证、健康 20/20、正式加载/刷新和验收企业清理通过；9 PVC、90 个原 Secret、37 个工作负载及业务原 ID/工作区文件保留。

证据：[R5 门禁](../../artifacts/template-v2-r5-gates.json)、[r29 真实链路](../../artifacts/tool-template-v2-k8s/r29/acceptance-summary.json)、[发布核验](../../artifacts/template-v2-release-20261014-r5/release-verification.json)、[截图复核](../../artifacts/tool-template-v2-ui/visual-r5/visual-review.json)。

首次审查未修改产品代码或部署，当时正式环境为 R2 revision 17。后续已按用户授权修复、补测并发布 R3 revision 18。

## R4 再次复核：A7、A8 原始发现（现已关闭）

2026-10-14 21:14，重新从注册表生成资产并追踪正式返回、模板和断言。没有修改产品代码或部署。

| 编号 | 问题 | 依据与复现 |
| --- | --- | --- |
| A7 | Connector 列表丢失已知软件版本。 | `internal/toolgateway/resources.go:605` 的共用 DTO 为 list/get 返回 `software_version`；`templates/resource.html:23` 的版本主列仅读 resource_version/version/convergence_status/summary.ready。给当前不可变模板传入合法软件版本 `v9.8.7` 后，主列为 `—`，展开却能显示该版本。`internal/tooltemplatefixture/resources.go:27` 只在详情夹具设置软件版本，`variants.go:116` 仅断言自身/关联名称。 |
| A8 | 告警事件未显示评估失败及已有数值的有效性。 | `internal/alerting/worker.go:243` 失败路径保留 state 的数值及 row.lifecycle，`:261` 更新实例 health 和最近评估时间；`read.go:24` 将 health 正式返回。`templates/result.html:18` 的事件摘要没有读取 item.health，只显示生命周期/通知状态、数值与最近评估。分别传入 normal/error/no_data/authorization_denied，当前详情 DOM 完全相同，均显示旧值 91%/85% 和新评估时间，三个异常健康状态都未出现。现有评估健康表针对策略配置/列表，不能替代实例自身状态。 |

以上使用当前 Hash 的实际模板进行受控 JSDOM 检查，输入沿用生成夹具的正式返回结构并设置真实字段；不是新一轮真实工具链、浏览器视觉或实际告警评估。后端合法失败状态另由已有 `evaluation_integration_test.go:117`、`:127` 的保留生命周期和成员检查确认。

证据：[输入](../../artifacts/template-v2-r4-review-semantic-inputs.json)、[当前模板复现](../../artifacts/template-v2-r4-review-semantic-probes.json)、[复现脚本](../../artifacts/template-v2-r4-review-semantic-probes.mjs)。

本次同步核对：106 个绑定/97 个模板/31 Hash 与已生成资产及覆盖清单一致；已发布 2234 个构建输入无差异、无新输入；相关 Go 包测试（缓存）及 Template Runtime 708 项通过。本地 revision 20 的 Server/Worker/Web 均 Ready、镜像固定摘要未变、模板临时 Namespace 为零。R4 全量浏览器原报告 352 通过/0 失败/141 跳过、r26 的 42 份展示及发布 16 项核验保留；本次没有重做这些验收。

关闭条件见实施计划。修复后应添加真实字段/失败状态的内容断言、中英文/明暗/桌面 Chatbox、临时 Kubernetes 验收及新发布，再恢复整体完成状态。

## R4 A1～A6 关闭结果（既有验收）

- A5：正式 `state.members` 的各规则实际值、单位、历史阈值及触发/恢复状态进入详情、列表展开和历史；every/any 来自事件快照，不以当前配置替代，全零组合 RuleID 不再作为触发规则展示。
- A6：Connector 的关联资源通过当前主体权限、分类型资源授权及一次批量名称查询补齐；不可用对象不提供名称和导航能力，未关联状态单列；列表、详情及主机/集群导航均有断言。
- Go 全量、1124 项前端测试、类型/i18n/Lint/样式/API/SQLC/构建/预算通过；完整浏览器单次 352 通过、0 失败、141 按范围跳过；372 张当前 Hash 正常矩阵、204 张业务专项及全部图型/边界证据齐全。
- 临时 r26 的 42 份展示、2 次封存/SSE、重启一致性、真实 Chatbox 和 20 项健康检查通过；临时资源清理，原 20 个工作负载恢复。r25 的历史首行错误测试假设已修正，失败记录保留。
- 本地最终 revision 20：9 个运行容器实际 imageID 匹配新摘要，健康 20/20、正式加载/历史刷新通过；16 项数据与配置保留检查通过。Web 旧内容别名被严格核验拒绝后，加入 OCI 版本标签重建，不放宽检查。

证据：[最终矩阵](../../artifacts/template-v2-test-matrix.json)、[当前截图](../../artifacts/tool-template-v2-ui/visual-r4/index.html)、[r26 摘要](../../artifacts/tool-template-v2-k8s/r26/acceptance-summary.json)、[发布核验](../../artifacts/template-v2-release-20261014-r4/release-verification.json)。真实模型评测未要求、未执行；构造历史与 Connector 元数据不作为实际告警评估或新接入证据。

## R3 再次复核：A5、A6 原始发现（现已关闭）

2026-10-14 18:47 重新追踪正式结果、展示投影、当前 Hash 模板及内容断言，未修改产品代码或部署。原四项修复仍有效，以下两项属于原计划要求的业务语义遗漏。

| 编号 | 问题与影响 | 代码依据与复现 |
| --- | --- | --- |
| A5 | 组合告警丢失各子规则的当时观测，无法完整判断为什么触发。组合触发规则显示全零 UUID。 | `internal/alerting/evaluation.go:385` 生成正式 `Members` 并将组合 `RuleID` 置空，`lifecycle.go:11` 保存到 `InstanceState.Members`；`templates/result.html:7` 只读取主规则摘要，不读取 `members`。当前不可变模板接收 CPU 97.25%、内存 88.75% 两个成员后，只显示主值，内存值与成员名称均未呈现，触发规则为 `00000000-0000-0000-0000-000000000000`。 |
| A6 | Connector 关联主机/集群没有名称与结构化关系展示，仍需读原始 UUID。已存在且有权访问的对象也不会解析。 | `internal/toolgateway/resources.go:601` 返回 `host_id`/`kubernetes_cluster_id`；`presentation.go:75`、`:87` 仅生成 Connector 自身引用，`:54` 的批量名称解析因此没有关联资源；`templates/resource.html:7` 不读取这两个关联字段。当前模板受控输入包含已知主机 ID，完整字段显示该 UUID，关联主机名未出现。 |

现有内容断言解释了为何绿灯没有覆盖这些问题：`internal/tooltemplatefixture/variants.go:116` 对 Connector 仅要求自身名称，`:130` 对告警详情只要求主摘要；正式类型告警夹具配置了两条策略规则，但未构造 `InstanceState.Members` 的组合观测。

复现为当前实际源码的受控 JSDOM 内容检查，使用正式字段结构，**不是新增真实工具链或浏览器验收**。证据：[输入](../../artifacts/template-v2-review-semantic-inputs.json)、[复现结果](../../artifacts/template-v2-review-semantic-probes.json)、[可重复执行脚本](../../artifacts/template-v2-review-semantic-probes.mjs)。A6 的服务端解析遗漏另由上述实际调用链确认；只有 ID 的受控输入本身不证明对象是否授权。

本次同步核实：

- 从当前正式注册表重新生成 106 个绑定、97 个模板绑定、30 个入口、31 份源码，与文档声明及已验收生成资产逐项一致；372 张正常截图及几何文件无遗漏、无旧 Hash。
- `go test ./internal/presentation/... ./internal/tooltemplatefixture/... ./internal/toolgateway/... ./internal/transport/httpapi/...` 通过（缓存结果）；Template Runtime 666 项重新执行通过。未重新执行全量 Go、门户浏览器或临时 Kubernetes。
- 最终 R3 浏览器原始报告确为 352 通过、0 失败、0 flaky、141 跳过。跳过原因是独立真实环境前置条件或排除的移动端；模板的临时真实链路和正式加载另有独立验收记录。
- 当前 Helm release `argus-platform` 位于 `argus-system`，revision 18、deployed；Server/Worker/Web 各 1/1 Ready，镜像仍为 R3 固定摘要；9 个 Argus PVC Bound；模板临时 Namespace 为零。当前产品源码未发现晚于 R3 构建时间的改动。

该轮要求的新 Hash 内容/边界断言、中英文/明暗/桌面 Chatbox、临时 Kubernetes、发布门禁与重建发布，均已由上述 R4 执行；原始 R3 记录继续保留。

## 首次复核问题（现已关闭，保留原始发现）

| 编号 | 问题与影响 | 代码依据 | 关闭条件 |
| --- | --- | --- | --- |
| A1 | `telemetry.dashboard.catalog` 返回有效 `fields` 或 `values` 时，展示被归一化成空列表，卡片错误显示“当前范围没有数据”。 | `internal/presentation/projection.go:67` 只选择 metrics/resources/candidates/signals；真实 `DataCatalogResult` 包含 fields/values，且 metrics 可为空数组。 | 指标、字段、取值三种目录结果分别正确展示；保留单位/字段类型/选中值存在性及业务分页状态；使用真实结果类型测试。 |
| A2 | Collector 列表/详情的最近采集时间缺失，详情期望/观测版本显示 ` / `。原始字段折叠区能看到真实值，摘要却没有使用。 | `internal/telemetry/templates/overview.html:6` 读取 desired_version/observed_version/last_heartbeat_at；实际 `db.CollectorInstance` 为 desired_revision/effective_revision/last_seen_at。 | 修订号、采集时间与收敛状态按实际字段展示，区分数据版本与发行版本，补齐列表和详情断言。 |
| A3 | 告警策略及历史快照的规则主表“指标”列为 `—`，没有直接展示实际规则表达式；展开原始字段才能找到 expression。 | `internal/alerting/templates/result.html:6` 读取 rule.metric；真实 `alerting.Rule` 使用 expression。夹具反而构造 metric，掩盖错位。 | 使用真实 Rule/PolicySpec 展示表达式与触发依据；覆盖多规则和策略变更前后快照，并对具体内容作断言。 |
| A4 | 边界夹具并未总是改变模板真正读取的业务字段，截图数量不能证明该场景已覆盖。 | `internal/tooltemplatefixture/data.go:134` 通用地修改顶层 items/name；例如告警详情 empty 仍保留 instance，策略详情 missing-name 仍保留 configuration.name，Collector long-name 仍使用原 collector.name。浏览器边界测试主要检查几何形状。 | 按业务类型构造边界数据，并断言空态/缺名/长名确实出现在对应组件。正常用例需覆盖每个绑定的真实结果结构及不同查询模式。 |

复现使用正式 Go 类型，经当前 `presentation.BindData` 投影，加载生成夹具中与当前源码 Hash 一致的真实模板。执行的是受控 JSDOM 内容检查，**不是新一轮浏览器视觉或真实业务链路验收**。

证据：[复现结果](../../artifacts/template-v2-audit-20261014/contract-probe.json)、[正式类型样本](../../artifacts/template-v2-audit-20261014/contract-cases.json)、[Go 样本生成程序](../../artifacts/template-v2-audit-20261014/contract-probe.go)、[DOM 复核程序](../../artifacts/template-v2-audit-20261014/contract-probe.mjs)。

## 首次复核时的交付与证据边界（历史记录）

- 重新从正式注册表生成：125 个绑定、113 个模板绑定、31 个入口、32 份源码；当前源码、已生成夹具、文档声明的 Hash 一致。`go test ./internal/tooltemplatefixture ./internal/presentation -count=1` 本次通过。这证明声明与资产覆盖，不证明每个业务分支语义正确。[覆盖核对](../../artifacts/template-v2-audit-20261014/coverage-check.json)
- 已有最终 R2 日志：Go 全量、994 项前端测试、类型/i18n/Lint/样式/构建/预算和 40 项模板浏览器测试通过。该结果保留，但 A1～A4 说明现有断言不足。
- 既有门户全量首次执行为 274 通过、23 失败、140 跳过；后续针对失败用例修复并复跑，适用用例累计 297 通过。并非最终 R2 上重新执行全部门户的单次全绿结果。140 个独立环境或非本版本移动端用例没有计为通过。
- 临时 Kubernetes r18 的 28 份展示、两次封存、SSE、Worker 恢复和清理恢复有通过记录；这是选定工具链路，不能推导全部 125 个绑定、所有参数模式及所有非空告警内容都经过真实链路验证。真实模型评测按原计划不要求，本轮未执行。
- 本次实时检查：`argus-platform` revision 17 为 deployed；`argus-system` 20 个运行中 Pod 全部 Ready；相关 Backend/Web 使用 R2 固定摘要；`argus-system` 与 `argus-sandbox` 合计 9 个 PVC Bound；`tool-templates-*` 临时 Namespace 为 0。发布时业务数据、原 Secret/PVC 保留以已有 R2 发布核验记录为准，本次未重做全量数据库比对。
- 现有截图及视觉报告保留，已发现的内容缺陷尚未修复。不能继续使用“全部渲染及业务问题关闭”的状态。

## 当时收尾任务（现已完成）

1. 修复 A1～A3，并全局核对同类模板字段与真实工具输出的一致性。
2. 修复 A4；增加真实类型、查询模式和可见业务内容的断言，避免只有卡片标题、非空和几何检查。
3. 对修订后的真实告警、Collector、目录结果重新进行 Chatbox 中英文/明暗主题/桌面尺寸及临时 Kubernetes 验收；执行原发布门禁，记录通过、失败和未执行项。
4. 重新构建、发布并复核数据保留。全部关闭后才能把整体计划恢复为完成。
