# PlanV2 实施状态

**当前复核覆盖下述历史状态：**2026-09-28 重新运行本地检查发现 api-client 审计测试 TS2493，前端类型门禁失败，P2V-RELEASE-01 重新打开，当前清单 36/37。正式功能和历史验收保留；最新分析提示及当前整版联合回归的边界见 [当前前后端复核](./current-code-review-20260928.md)。

本文件跟踪正式产品实现，Demo 不计作后端或端到端验收。2026-09-28 PlanV2 首期清单 37/37 关闭；模型场景采用 h/i/o/p 分轮证据，p 轮异常组 4/4、部署检查 20/20、退出码 0，临时资源已清理。非模型页面 32 个场景沿用已有验收。本地 evaluation 功能结果不等于生产强隔离验收，详见 [真实模型验收](./real-model-validation-20260928.md) 与 [非模型阶段报告](./completion-review-20260928.md)。以下较早日志保留当时事实。

2026-09-27 用户调整本轮验收范围：暂不执行依赖真实 AI 模型的创建质量、条件理解和结论质量验收；继续完成不依赖模型的产品、协议、界面、故障和部署工作。新增实现与进行中的验证见 [非模型收尾](./non-model-closure-20260927.md)。此调整不将未执行的模型验收标记为通过。

## 已冻结的实施决定

- Q34：APM 展示已接收样本的调用量、错误率和时延，明确标注样本口径，不推算全量。独立请求指标另外注明来源。
- 新取数采用最新发布版；每次执行冻结版本、时间、来源与有效条件。
- 首期主路径为标准 OTLP；2026-09-25 按用户要求追加 SkyWalking/Jaeger 原生 Trace 接收与 Argus 自监控验证，见 [新增范围及验收记录](./self-monitoring-native-traces.md)。Profiling 等其他后续范围不变。

## 进度

| 阶段 | 状态 | 退出条件 |
| --- | --- | --- |
| 遥测来源、查询正确性、APM | 三信号/三来源、跨批次/重复/迟到/缺父、Events/Links、授权展开/撤权、关联日志、停采/重装与冻结来源通过 | 非模型范围关闭；样本统计不推算全量 |
| Dashboard 领域、草稿与发布 | 真实草稿、发布、双编辑者/旧预览/幂等/撤权、分组/关联生命周期与审计通过 | 非模型范围关闭 |
| 正式 Workbench、图型与资源入口 | 十一类 Metrics、自由布局、变量分页/级联/回退、两层筛选、跨来源显式映射、日志/Trace 详情与 Host/K8s 入口通过 | 页面证据 f 轮 26 项 + g 轮定向 6 项 |
| Chat 创建、查询任务与 Workspace | Chat 选择/刷新/模式、builder/DSL 原生工具/宿主确认、真实 PVC 与六项故障通过；真实 GLM 变量关联与四轮条件场景通过 | 首期技术与代表性模型功能范围关闭；分轮证据和失败记录分别保留 |
| 临时 Kubernetes Namespace 验收 | h 轮 Runtime 退出码 0、同一 Server Pod 零重启；64 图与双 Query 副本测量；归属清理及原有资源核对通过 | Runtime 与页面证据分开记录，不声称单次全量重跑或生产吞吐量 |

非模型能力矩阵与当时暂缓范围见 [2026-09-28 验收结论](./completion-review-20260928.md)，后续真实模型结果见本页顶部链接。[早期缺口复核](./remaining-work-20260925.md) 和以下记录保留历史状态。

## 验证记录

### 2026-09-27：布局与变量交互

`p2-interaction-20260927-c` 通过 24/24 页面及三信号文件/PVC、文件审计核对。本轮修复候选失败被局部成功结果覆盖、局部 Catalog 使用不适用资源、弹窗重用过期选择及拖拽取消焦点/捕获问题。新增四个真实用例覆盖布局碰撞、最小尺寸、取消/恢复，250 个真实候选的分页与精确成员检查，级联、局部刷新、Metrics/Logs 显式映射及 Cluster→All→Host 切换。

Enterprise 148 个单元测试、共享 UI 定向 12 个测试、相关 Go 包、公开夹具契约、Enterprise/Platform/UI 类型及 ESLint/i18n/样式检查通过。A 轮 22/24 的脚本网络错误已修正；B 轮 Docker API 阻塞经用户授权正常重启后恢复，再完成 C 轮。Docker 层旧测试记录差异、Kubernetes 数据/服务核对、截图与验收边界见 [完整报告](./interaction-boundaries-20260927.md)。

### 2026-09-27：审计、生命周期与候选异常

`p2-audit-20260927-c` 通过 20/20 real 浏览器及三信号文件/审计关联。本轮补齐审计版本/执行/来源/文件事实、双语展示与服务端条件筛选；分页下推 PostgreSQL，每次最多查询页大小加一条，游标绑定所有条件与主体。新增分组恢复后的旧预览拒绝、关联两侧撤权/资源删除、候选异常保留及确认消失后回退场景。

上一轮 19/20 时双编辑者登录收到 502，同期 Server OOMKilled。保留 256 MiB 硬限额与密码算法，配置 `GOMEMLIMIT=192MiB`，复验前后同一 Pod Ready、零重启；此证据不代替大规模并发测量。相关 Go 回归、Enterprise 146 个单元测试、契约一致性、两门户类型、ESLint/i18n 及三种部署 Profile 渲染检查通过。逐轮记录、文件核对与清理结果见 [收尾报告](./audit-lifecycle-20260927.md)。

### 2026-09-27：发布边界、普通编辑者与真实 Metrics 图型

`p2-boundaries-20260927-c` 的 15/15 real 浏览器和文件/PVC 专项通过。新增不同编辑者发布竞争与差异整理、重复确认、旧预览、撤权/MFA 重登/恢复发布、非空分组迁移、Host/K8s 快捷入口，以及中文浅色/英文深色的十一类真实 Metrics 图型验证。

修复 Dashboard 版本冲突被共用动作层折叠成通用错误、普通编辑者因无法读取全企业角色而被前端误拒绝的问题。前端权限来自既有会话契约，保留角色目录权限和所有服务端门禁；Mock 会话复用统一的有效绑定规则。相关 Go 包回归、企业前端 146 个单元测试、Mock 30 个测试及类型/ESLint 检查通过。截图、失败轮次、清理与验收边界见 [本轮记录](./publication-gallery-boundaries-20260927.md)。

### 2026-09-26：工作台与真实 Workspace 文件

`p2-closure-20260926-e` 通过 10/10 real 浏览器及三信号文件专项。新增工作台用例覆盖空白创建、三信号真实查询、查询转换、拖动/键盘宽高、同账号旧标签页冲突、发布隔离、归档拒绝发布和恢复重新确认基线。文件经正式 Worker/对象存储/Workspace RPC 写入真实 Bound PVC，逐文件下载校验并由离线工具计算记录数和 Hash；排队取消、已完成任务在 Worker/Redis 重启后保留同 attempt/字节、跨会话拒绝及工作区删除通过。

修复共用文件下载在 HTTP 完整响应到达前尚未释放 Workspace 租约的问题：以最多 32 KiB 尾块保证先释放后完成，仍流式传输并保持 Range。HTTP/Workspace/Workspace IO 回归通过。真实模型入口已支持 PlanV2，当前未配置实际模型，记录 `executed=false`。全部产物、失败轮次和剩余范围见 [收尾记录](./closure-workbench-files-20260926.md)。

### 2026-09-26：Argus 自监控与原生 Trace

`p2-selfmonitor-20260926-k` 已通过原生 GO2Sky/Jaeger SDK → 受管 Collector → Kafka/ClickHouse → 已发布查询，以及三来源共 18 张 APM/Trace 图的取数和来源隔离。四组语言/主题各完成查看/下钻/变量依赖与编辑/恢复/样本/确认发布/历史列表，8/8 真实 Chromium 通过。首个取数快照中，Argus API→Query 拓扑具有 31 个接收目标入口样本；两套原生 SDK 各有 2 个请求样本和 1 个受控错误。统计只描述已接收样本，后续自监控查询产生新 Span，因此浏览器截图中的计数可能更高。

修复了草稿数据库序列化冲突被误报为编辑冲突/不存在、发布预览遗漏描述、自监控夹具错误隔离业务出口，以及共享 OpenSandbox 外部所有者的共存问题。APM 汇总表与验收布局也已改善。新增协议/平台支持边界、截图、失败记录及最终清理状态见 [完整验收记录](./self-monitoring-native-traces.md)。本项不关闭 Task 01/02 的其余退出条件。

### 2026-09-24：后端基础

已实现：

- `internal/dashboard`：版本化配置、受控构建器编译、结构/语法验证、变量依赖和候选确定性回退、个人草稿、已发布版本、分组和生命周期。
- PostgreSQL migration 00003 与 SQLC：Dashboard 对象授权约束、个人草稿唯一性、不可变 Revision、同事务首次发布/创建者授权/草稿消费，以及分组和资源关联存储。
- 新建草稿不进入已发布列表；重开编辑恢复现有个人草稿；保存检查版本，发布检查草稿/Revision/对象/分组版本；重新确认基线保留个人内容。
- 复用真实 PendingAction 确认与私有执行器，服务端和 Worker 均注册 Dashboard 分派。过期基线、归档恢复和撤权使旧预览失效。
- 正式 OpenAPI、生成的 Go/TypeScript DTO 和 HTTP 路由：Dashboard 列表/详情/版本、个人草稿创建/读取/保存/放弃/重新确认基线、发布预览、分组和归档/恢复预览。
- 发布 Diff 使用公共 Action 数组契约，逐项呈现同数量下的查询/默认值/布局变化；完整前后配置冻结在 preview 的 JSON 文本字段中，避免原生 GraphQL 参数键被公共预览的键名规则拒绝。读取发布配置不返回其他编辑者的样本数据。
- KQL 按顺序解析阶段，拒绝无法保持语义的重排；处理引号内管道符、空字符串、转义与数值字符串；提供 count/sum/avg/min/max/p95、时间分桶、分组和对应结果类型。系统行数截断与显式 limit 分开标记。
- Trace 实例/标签筛选子查询使用与外层相同的资源范围，不再通过未授权 Span 决定是否命中；空资源范围拒绝。

验证结果：

| 验证 | 结果 |
| --- | --- |
| `go test ./internal/dashboard ./internal/authorization ./internal/telemetry/... ./internal/transport/httpapi ./internal/app/server ./internal/app/worker ./tests/contract` | 通过；普通执行中依赖环境变量的数据库测试跳过，另按下述真实环境执行 |
| `go vet ./internal/dashboard ./internal/telemetry/queryengine/kql ./internal/telemetry/queryengine/skywalking` | 通过 |
| `ARGUS_DASHBOARD_TEST_DATABASE_URL` + `go test ./internal/dashboard -count=1 -v` | 真实 PostgreSQL 17 通过；包含真实 PendingAction 确认、重复预览、双编辑者冲突、旧保存版本、撤权保留、分组幂等、非空分组和归档恢复 |
| `ARGUS_TEST_DATABASE_URL` + `go test ./tests/postgres -run '^TestMigrations$' -count=1 -v` | 通过；包含 migration 00003 的 down/up；初次集成测试也已从空库执行全部 up |
| `ARGUS_CLICKHOUSE_TEST_ADDRESS` + `go test ./internal/telemetry -run 'Test(KQLClickHouse|SkyWalkingGraphQLClickHouse)' -count=1 -v` | ClickHouse 26.3.17.110 真实写入和查询通过；后续新增通配符/logfmt 用例单独复跑通过 |
| `pnpm --filter @argus/api-client typecheck`、`pnpm check:i18n` | 通过 |
| OpenAPI lint、`git diff --check`、修改文件 2000 行限制 | 通过；OpenAPI 存在已有的通用规则警告，未作为完整生成一致性检查通过的证据 |

生成工具注意：完整 `argus-dev contracts generate` 被 Windows 下 pnpm/Node 子进程退出异常中断。已恢复生成目录并用同一工具的直接 Node 调用重新生成受影响的 dashboardapi、telemetryapi、主 bundle、表单约束和契约索引；未宣称完整 `contracts check` 已通过。

测试资源：本轮独立 Docker PostgreSQL/ClickHouse 容器及其匿名卷已按完整容器 ID 和 `argus.test=planv2` 标签验证后清理，确认返回 404；临时诊断程序已删除。Windows Docker 命名管道查询卡住时通过虚拟机内 Docker API 完成清理。Kubernetes `/readyz` 返回 ok。

### 2026-09-24：来源链路与基础运行时

- PostgreSQL migration 00004 登记来源、配置版本和安装代次。Collector ID 重装时可以复用，source_generation 必须更新；来源 ID 由安装代次与 Receiver 实例生成，配置变更保留来源 ID、递增配置版本。
- Collector 在不同 Receiver 进入批处理之前写入来源引用。Gateway 保留下游引用并覆盖身份声明；Ingest 使用认证 Collector、当前安装代次及登记配置验证来源，SDK 名称不作为可信厂商身份。缺少来源的旧输入明确标记 unknown。
- ClickHouse Schema v4 给三信号及 Trace 派生表增加来源维度，补日志资源属性；升级保留旧记录，迁移/验证失败不自动删租户表。Metrics 使用可信资源/来源标签区分同名流，Trace 排序键隔离不同来源。
- 三个 Engine 与 gRPC Scope 均接入 source_keys，执行范围冻结到来源 ID 和配置版本。目录发现复用现有 Query 进程的只读连接，提供真实指标目录、候选值、截断状态和独立的当前选值存在性验证。
- Dashboard Runtime 已接到正式服务端：固定已发布 Revision、绝对时间、授权资源与来源集合，限制共享扫描/结果/样本预算，重新检查资源权限，保留逐 Target 状态和执行指纹。可对基础无参数统计图执行草稿样本、发布预览、确认发布及发布版查询。
- 新增 `/dashboards/{id}/execute`、`/dashboard-drafts/{id}/sample`、`/dashboards/catalog/query` 的 OpenAPI、生成 DTO 与 HTTP 实现；普通执行不接受查询文本覆盖。
- GraphQL 原生参数按真实 Schema 校验；PromQL/KQL 未声明的完整参数引用不再作为普通字符串通过校验，PromQL 正则替换的命名捕获保持原生语义。
- 修复过期/未生效角色仍授予对象范围的问题；修复 PromQL 序列上限静默截断。Writer 在暂时失败时重试当前记录，Schema 暂未就绪不进入永久 DLQ；缺少时间戳时使用稳定的 Kafka 记录时间，避免重试产生重复日志。

本次验证使用独立 Kubernetes Namespace `argus-p2-runtime-20260924` 中的 PostgreSQL 17 和 ClickHouse 26.3.17.110，不使用产品数据：

| 验证 | 结果 |
| --- | --- |
| 三信号同名/同 Trace ID 的来源隔离、目录发现、Trace 筛选子查询 | 真实 ClickHouse 通过 |
| v3 租户表升级 v4 并保留旧日志 | 通过 |
| PromQL 与上游引擎的结果对照 | 通过；对照数据包含相同的可信资源/来源标签 |
| 有数据统计图保存 → 样本/发布预览 → 真实 PendingAction 确认 → 发布 → 查询 | PostgreSQL + ClickHouse 集成通过 |
| 无权资源、撤权、过期角色、来源重装与历史保留 | 通过 |
| 当前选值未命中搜索但仍实际存在 | 通过，selected_exists 保留正确结果 |
| Collector 配置、Gateway 子模块、API/契约、相关服务编译、API Client 类型/i18n、go vet | 通过 |

这些证据尚不包含真实 Collector 进程到 Kafka/Writer 的完整传输、正式页面或真实模型分析，不作为全链路 E2E 已完成的证明。

清理：本轮临时 Namespace 及两个 Pod 已按 `argus.io/test-owner=codex-planv2` 核验后删除，专用端口转发进程已停止；未创建 PVC。清理后 Kubernetes `/readyz` 正常。

### 2026-09-24：变量、局部过滤与候选分页

- 参数绑定按语言结构实现：PromQL 标签匹配、KQL 条件树、GraphQL 声明类型；全局变量只影响明确引用者，跨来源必须显式映射。详见 [执行契约](./runtime-parameters.md)。
- 查询候选按依赖刷新，确认任意选值消失才整组回退 All；部分页、搜索和暂时失败不造成误重置。局部文本/数值条件无数据时保持，局部刷新只执行指定统计图。
- 执行结果返回有效选值、候选验证状态、重置标记和来源快照；发布默认值保持不变。候选/类型目录/统计图共用累计预算和请求取消。
- Catalog 支持排序键分页和上下文绑定游标；当前选值检查不受搜索/分页影响。KQL 补充 JSON 数值等值匹配，并显式保留带引号的字符串类型，避免 ClickHouse 自动字符串化改变语义。
- OpenAPI / Go / TypeScript、Catalog RPC 与 HTTP 转换同步新增参数、候选状态、映射和分页字段。

验证：独立 Namespace `argus-p2-params-20260924` 中 PostgreSQL 17 / ClickHouse 26.3.17.110 已通过参数化草稿 → PendingAction 确认 → 发布 → 真实查询；验证级联回退、发布默认值保持、局部刷新、来源隔离、目录分页与精确检查。语言绑定/候选单元测试及 API 转换检查通过。正式浏览器、Collector/Kafka、Chat E2E 仍未执行。

### 2026-09-24：Trace 事实去重与跨批次组装

- Writer 停止把单次 Kafka 批次写成最终 Trace 摘要/边。查询引擎在授权资源和冻结来源范围内，按 Span 身份去重后计算摘要；同一安装来源的配置版本不重复统计，重装和不同来源保持可区分。
- 子 Span 先到、父 Span 迟到、重复上报、后续错误 Span 和配置更新均从现有事实重新组装。详情保留 Events / Links / 资源属性，明确缺失父节点、循环、根节点状态与样本结构一致性。
- 列表和详情新增来源/资源身份；同 ID 存在多个匹配来源时，详情要求明确选择。统计和关联子查询都不能由其他资源或来源的数据影响。
- 展开超出累计关系/行数/扫描预算返回预算错误；同次详情复用已加载 Span，避免属性、边与完整性字段重复取数。

验证：真实 ClickHouse 已通过跨批次、重复、迟到、配置版本去重、Events / Links 保留、缺失父节点修复、明确来源选择、来源碰撞和关系预算回归。此阶段仅覆盖普通授权范围内的 Trace，未宣称 APM 或 Q31 完整链路展开完成。

本轮收尾验证：带真实数据库环境的 `go test ./internal/dashboard ./internal/telemetry -count=1` 通过；Query Engine、HTTP、Server、Worker、契约测试及相关 `go vet` 通过；API Client 类型检查、i18n、OpenAPI lint 和变更文件长度检查通过。OpenAPI lint 仍有 17 条通用 4xx 响应规则警告，未宣称完整生成一致性检查或产品 E2E 已通过。

清理：已核验本轮 Namespace 的 `argus.io/test-owner=codex-planv2` 标签和无 PVC 状态，删除 `argus-p2-params-20260924` 并确认删除完成；两条专用端口转发按 PID 和启动时间核验后停止。未暂停或修改其他命名空间，清理后 Kubernetes `/readyz` 返回 ok。

### 2026-09-25：APM 样本查询、拓扑与发布

- 现有只读 Trace GraphQL 引擎新增 `queryAPMServices`、`queryAPMInstances`、`queryAPMEndpoints`、`queryAPMRED`、`queryAPMTopology`，对应五类专用图型和受控 Builder。原生 DSL 同样检查结果形状、口径和必需字段；不将 APM 聚合误识别为 Trace 列表。
- 请求样本只统计去重后的 SERVER / CONSUMER Span；返回样本量、错误率、样本速率、均值及 P50/P95/P99。内部 Span 不冒充请求，RED 最后一个不完整桶按实际秒数计算；无入口样本返回 null 和能力状态。
- 服务/实例/接口保留资源和来源身份。拓扑依据已接收的父子 Span 建立边，同来源类型跨采集实例的匹配必须唯一；缺失、歧义、循环和未知来源明确计数，不按服务名称合并节点。
- 属性集合过滤支持环境等共享变量、多选和 All；属性条件在聚合/拓扑匹配前执行。拓扑不会因属性筛选而补取其他环境或未授权资源的节点。
- 五类 APM 统计图已接入个人草稿、PendingAction 确认发布和运行时。能力缺失保留配置有效、样本状态 warning；数据库连接中断等可识别的暂时故障不再误标为查询语法错误。
- OTLP 摄入补 Trace/Span/Link ID、时间区间、Kind/Status 校验；拒绝会破坏去重或伪造时延的无效 Span。APM 细则见 [查询与发布契约](./apm-query-contract.md)。

验证使用独立 Namespace `argus-p2-apm-20260924` 中的 PostgreSQL 17 / ClickHouse 26.3.17.110：真实 OTLP Span 经 Writer 写入后，统计、重复、来源隔离、时间桶、缺失字段、属性过滤、授权拓扑和预算回归通过；五类图型携带环境变量的草稿 → 确认发布 → 执行通过。该证据不包含真实 Collector/Kafka 传输、正式浏览器或 Chat。

收尾检查：真实数据库环境下 Dashboard / Telemetry 测试通过；Query Engine、HTTP、Server、Worker、契约测试与相关 go vet 通过；API Client 类型检查、i18n、OpenAPI lint、diff 和 2000 行限制检查通过。OpenAPI lint 仍有 17 条已有通用 4xx 响应规则警告；完整生成一致性检查和正式 E2E 未标记完成。

清理：按 Namespace 所有者标签核验后删除 `argus-p2-apm-20260924`，确认已不存在；未创建 PVC。专用端口转发按 PID 与启动时间核验后停止，其他命名空间未修改或暂停，Kubernetes `/readyz` 正常。

### 2026-09-25：标准下钻、授权完整链路与日志前后文

- 新增标准下钻生成与执行 HTTP API、Go / TypeScript 契约。生成按草稿版本保存，后续预览/确认发布复用同一 PendingAction；重复生成不覆盖作者修改。
- APM/Trace/Logs/同查询展开分别保存 origin/detail 引用、信号、来源和类型化行输入。Builder 与可安全派生的单根 GraphQL 原查询保留业务条件；RED 点位携带实际时间桶并裁剪到原窗口。
- 查询返回主体绑定的 15 分钟签名上下文；详情核对原查询中的实际选择，连续步骤最多 16 层。上下文不是授权凭据，所有访问重新检查对象权限和生命周期；原执行不混入后续发布，新执行采用最新 Revision。
- `queryTraceGraph` 在给定范围内按来源身份去重，从锚点片段连接明确父子关系，报告缺失、歧义及未连接片段。Q31 显式完整链路先验证原范围内锚点，再冻结当前另有授权资源，不改变顶部范围。
- Span 关联日志可继续打开受控 KQL 前后文；前后文保持时间、资源、来源及流身份，按 timestamp/event_id 排序。没有锚点时返回空结果，不放宽查询。
- 展示、核对、锚点与详情共用查询执行器和累计预算。需要选中记录的详情在发布预览中明确标注尚未样本执行；不会把配置有效等同于全部详情数据已验证。

验证：Namespace `argus-p2-drill-20260925` 中 PostgreSQL 17 / ClickHouse 26.3.17.110 通过生成/草稿/确认发布、A 范围详情、显式 A/B 展开、未授权 C 排除、Span 日志及前后文、撤权、上下文主体/签名/期限、伪造选择、R1/R2 隔离和时间裁剪。Dashboard / Telemetry 真实存储测试、相关 Query/HTTP/Server/Worker/契约测试、go vet、API Client 类型和 i18n 检查通过；文件长度与 diff 检查通过。OpenAPI lint 有 19 条通用 4xx 响应规则警告。

正式页面、Chat 与 Collector/Kafka 全链路仍未验收，未标记 PlanV2 整体完成。详细边界见 [下钻执行契约](./drilldown-runtime.md)。

清理：核验所有者标签和无 PVC 状态后，已删除 `argus-p2-drill-20260925` 并确认不存在；专用端口转发按 PID / 启动时间核验后停止。其他命名空间未修改或暂停，清理后 Kubernetes `/readyz` 正常。

### 2026-09-25：正式工作台与共享前端组件

- Enterprise 新增仪表盘目录、个人草稿、编辑与发布版查看路由；支持 Folder 新建/归档/恢复、仪表盘归档/恢复、只读版本历史、草稿刷新恢复、发布预览和现有 PendingAction 确认。查看页面没有新增数据导出或 AI 分析区域。
- API Client 新增 Dashboard 共用类型与 mock/real 适配器，HTTP 路径、动词和必需确认请求头与权威 OpenAPI 对照测试通过。模拟结果明确标为测试数据，模拟校验仅验证结构，不作为查询编译或授权证据。
- 十二列网格支持指针移动/缩放、键盘移动与 Shift 缩放、碰撞下推、最小尺寸，布局进入个人草稿。自动保存串行执行；保存期间的新编辑不会被旧响应覆盖；页面内部跳转先保存，失败时保留当前编辑。
- 变量使用构建界面；图内支持多查询、局部查询/文本/数字筛选、参数绑定与显式跨来源值映射；候选可以搜索和继续加载。候选请求失败或分页缺少当前选值不会由界面回退 All，实际回退仍以执行响应为准。局部响应只更新对应图，全局条件回退触发整盘重新执行。
- 配置与样本分开显示，编辑后不会继续展示旧样本为已验证。预览显示冻结的前后配置和独立样本状态；冲突确认使用已经展示的 Draft/对象/Revision 版本，确认时不自动读取并接受更新的版本。
- `@argus/ui` 新增查询编辑器、网格、候选选择器、十一种指标展示、日志结果表/搜索/已返回记录分页、Trace 瀑布与 Span 属性/事件/Links、APM 服务/实例/接口表、RED 三种统计切换和拓扑。RED 按来源/资源/服务等完整身份分系列，APM 明示已接收样本口径。经典直方图只在同一标签身份的累积桶上求差；缺失样本不替换为历史正常值。
- 发布版查看支持时间/资源/变量与局部筛选，固定时间范围和相对时间均可选择；标准下钻沿已发布定义执行并支持回到上一步，显式完整链路保持独立范围。历史配置页面不混入当前版本样本。
- 修复共用 Select 在动态更换选项时由原生表单代理回传空值导致必填运算丢失的问题；中文/浅色和英文/深色浏览器覆盖均通过。
- 两门户共用精确依赖分包策略；图表按需注册、日期控件按需加载，避免共享样式把可选引擎带入首屏。正式构建不含 mock 种子；首屏与单块体积门禁通过。

验证证据：

| 检查 | 已验证范围 |
| --- | --- |
| API Client 与 Enterprise TypeScript | 通过（Enterprise 引用共享 UI 类型） |
| 41 个相关单元/契约回归 | API mock 发布隔离及旧预览拒绝、real adapter/OpenAPI 对照、串行草稿保存、候选失败/分页、图表数据形状和身份分组、共享表单组件、PendingAction 双语呈现 |
| `e2e/dashboards.spec.ts` 3 个 Chromium 场景 | 创建/保存/刷新/键盘布局/预览确认/未发布隔离；十一类 Metrics 及指针移动；英文深色 Trace 详情、RED 与拓扑。使用 mock API，**不是**真实遥测 E2E |
| `pnpm build:real`、`pnpm check:bundle` | 两门户和 Template Runtime 正式构建、mock 排除检查与包体积门禁通过 |
| `node scripts/check-web-runtime.mjs` | 浏览器加载生产登录页及全部生产 JS 块；使用固定未登录 API 响应，仅证明构建后运行时，无真实业务后端 |
| i18n、样式、受影响前端 ESLint、2000 行限制 | 通过 |

浏览器使用专用 4273/4274/4276 端口，避开 4173 上其他项目；生产冒烟使用进程内临时 loopback 端口，结束后关闭。此阶段未创建 Kubernetes 资源，也未暂停其他项目服务。

### 2026-09-25：资源快捷关联与仪表盘对象授权

- 按主设计实现 Host / Kubernetes Cluster 关联读取、关联/解除预览和 Dashboard 关联摘要 API。正式页面复用共用关联组件和 PendingAction 确认卡；快捷打开只预选系统资源，进入后可以切换到其他有权资源。仪表盘配置、Revision 与数据权限不会因关联而改变。
- 关联操作检查资源管理权限、目标资源授权、Dashboard 读取及对象授权，不要求 Dashboard 编辑权限。预览冻结资源版本、Dashboard 生命周期版本及原绑定 ID/版本；确认与提交都重新校验并锁定对象。双重关联、旧解除操作、资源改动和撤权均不能静默覆盖。
- 快捷入口按当前主体的双侧授权动态过滤；删除或归档隐藏入口，保留关联记录；恢复后可重新显示。某位用户被撤权不会修改其他用户仍可见的关联。草稿提出的关联建议也复用相同资源权限/生命周期校验。
- Dashboard 接入现有用户、部门、角色和 ServiceAccount 数据授权目录与批量修改。复用授权版本失效、角色继承和审计，不添加三信号或字段权限。旧主体版本不能读取新授权上下文，仅 Dashboard 授权不能获得 Host/Cluster 数据访问。
- 共用授权双栏控件新增仪表盘分类，继承条目只读；保存其他分类时不会把未修改的继承授权写成直接授权。查询目录失败时禁止提交。mock 新建对象登记创建者显式授权，Dashboard 模拟执行也按资源授权过滤。
- 授权目录原来只在领域生成入口声明，未进入总 OpenAPI 的请求校验。现已抽出共用 paths/components 并加入总契约；HTTP 回归覆盖合法 Dashboard 授权、非法信号权限分类、绑定版本必填及 Host/Cluster 请求。

验证：独立 Namespace `argus-p2-bind-20260925-c43d` 的 PostgreSQL 17 通过真实草稿发布、关联预览/确认、同幂等键重试、并发关联、删除重建后的旧操作拒绝、资源版本变化、撤权、归档/恢复、删除资源、跨企业，以及 Dashboard 授权/角色继承/授权版本失效。Dashboard、Authorization、HTTP、Server、Worker、契约测试和相关 go vet 通过；依赖 ClickHouse 的用例本阶段未配置，不能据此宣称三信号全链路复验。

前端：API Client 全量 89 个测试、共享 UI 25 个测试、组织/授权/确认呈现 8 个测试通过；6 个 Chromium 场景通过，新增 Host/Cluster 关联、从 A 切换 B、解除后独立访问和授权抽屉授予/撤销。浏览器使用 mock API；真实 PostgreSQL 和浏览器证据尚未合并为正式后端产品 E2E。正式模式构建、包体积门禁、生产登录及全部 JS 块加载、i18n、样式与受影响 ESLint 检查通过。

OpenAPI 领域 lint 通过，有 37 条通用 4xx 响应警告（Dashboard 24、EnterpriseAuthz 13）；总契约 lint 有既有通用规则与路径歧义警告。本轮使用定向生成，未宣称全量生成一致性检查通过。

清理：核验 Namespace 所有者标签、仅有本轮 PostgreSQL Pod 且无 PVC 后，删除 `argus-p2-bind-20260925-c43d`，确认不存在；专用端口转发按 PID/启动时间核验后结束，25432 无监听。未暂停其他服务，清理后 Kubernetes `/readyz` 返回 ok。

### 2026-09-25：查询转换与下钻详情编辑

- 新增受管理权限保护的整图模式转换契约；展示和详情查询原子转换，不能完整表达时保留原定义。复用原生 PromQL AST、KQL 条件/阶段模型和 GraphQL AST，转换后重新编译并核对结果类型。
- 支持常用指标、日志及 Trace/APM Builder；保留动态变量、局部条件、行输入、显式映射、All、多选、limit 和步长。同一输入在 GraphQL 标量/列表等不同类型中使用时生成别名，避免变量失去关联或名称碰撞。转换不冒充样本验证。
- CompilerVersion 升为 v2，整图展示/详情统一编辑模式；标准生成器在 DSL 图中生成 DSL 详情，规范化新定义的空集合。新增日志显式 limit 构建能力，未引入新查询引擎。
- 正式页面提供详情查询编辑、来源/信号、范围策略、行输入映射、时间裁剪与依赖整理。共享查询控件、JSON 编辑字段及双语文案；无效 JSON 阻止提交，嵌套表单不提前提交外层统计图。只读查看包含发布的展示和详情定义。
- 具体可转换范围与保守拒绝规则见[查询编辑契约](./query-authoring.md)。mock 仅演示基本无参数指标转换，完整语言能力以真实服务为准。

验证：独立 Namespace `argus-p2-convert-20260925-d92e` 的 PostgreSQL 17 / ClickHouse 26.3.17.110 通过三信号及 APM 的 Builder → DSL → Builder 发布与固定范围结果对照、转换后标准 Trace 下钻、原有变量和授权详情回归。数据为直接存储输入，不能替代真实 Collector/Kafka 摄入验证。

API Client 89 个回归、共享 UI/JSON 字段 26 个回归、草稿与详情依赖 2 个回归通过；7 个 Chromium 仪表盘场景通过，包括新增模式拒绝/往返、嵌套详情编辑与刷新恢复。Dashboard/HTTP/Server/Worker/契约测试、相关 go vet、类型、i18n、样式和受影响 ESLint 检查通过。浏览器仍是 mock，真实查询是独立存储集成。

正式构建、包体积和生产登录/全部 JS 块加载检查通过；Dashboard OpenAPI lint 有 25 条通用 4xx 响应警告。使用定向生成，未宣称全量生成一致性检查通过。

清理：验证测试 Namespace 标签、仅有 PostgreSQL/ClickHouse 两个测试 Pod 且无 PVC 后，删除 `argus-p2-convert-20260925-d92e` 并确认不存在；两条专用端口转发按 PID/启动时间核验，25432/29000 已无监听。未暂停其他服务，Kubernetes `/readyz` 返回 ok。

### 2026-09-25：字段探索与受控请求错误率

- Catalog 新增实际观测字段目录，复用来源/资源授权、Catalog gRPC、扫描/结果预算和取消。Metrics 标签、日志原生/结构化字段及 Trace 属性按资源、来源、时间和目录条件发现；类型标注使用存储类型，正文解析阶段字段仍由显式 KQL 定义。字段搜索与分页不冒充完整集合，空范围不生成虚构字段。
- `@argus/ui` 增加共用字段探索组件；正式展示/详情编辑器可以搜索、翻页、读取字段值并添加支持的条件。按仪表盘默认时间冻结探索范围，来源或指标变化后重建上下文；读取失败不自动清空选值，卸载取消请求。mock 提供标记明确的分页测试目录。
- Metrics Builder 新增同一 Counter 的 `error_rate`。公共变量、筛选、窗口和分组同时约束分子/分母，错误分类是固定标签条件。结果为 0–1 比例，显示单位提供中英文标签；零请求、无样本或已观察到的错误子序列不足两个点时不伪造 0%。与 APM Span 样本统计分开。
- 原生 PromQL 编译和无损往返保留比例保护规则。不同 Counter、匹配规则、分组或保护逻辑不能被强行转回 Builder；另补静态 `$name` 字面值保护，避免转换为语句时被公共变量意外捕获。完整约束见[查询编辑契约](./query-authoring.md)。
- OpenAPI 目录定义拆入共用 `dashboard-catalog.yaml`，避免单文件膨胀；拆分前后生成 Go/TypeScript 及总 bundle 校验值一致。

验证：独立 Namespace `argus-p2-catalog-20260925-a8f4` 的 PostgreSQL 17 / ClickHouse 26.3.17.110 通过字段跨来源/资源/时间隔离、条件筛选、分页、空结果、存储类型、预算和撤权回归。真实 Counter 样本得到 20% 错误率、正常请求 0%、零请求和采样不足无数据；包含错误率的三信号/APM 仪表盘完成 Builder → DSL → Builder 发布后固定范围结果对照。

Dashboard、HTTP、Server/Worker 编译或测试、Catalog、契约测试及相关 go vet 通过。API Client 89 个测试、共享 UI 46 个测试、类型、i18n、样式和受影响 ESLint 通过；UI 全套在并发负载下出现一次已有 DateTimePicker 懒加载等待超时，限制为两个 worker 后全套通过，未放宽断言。8 个 Chromium 仪表盘场景通过，新增字段分页/选值与错误率草稿恢复；本轮截图已核对。正式构建、包体积及生产登录/全部 JS 块加载检查通过。

浏览器使用 mock，查询验证通过直接存储输入；本轮不计作 Collector/Kafka 摄入或真实产品/Chat E2E。Dashboard OpenAPI lint 仍有 25 条通用 4xx 响应警告，未宣称全量生成一致性门禁完成。

清理：核验 Namespace 所有者标签、仅有两个测试 Pod 且无 PVC 后删除并确认不存在；专用端口转发按 PID/启动时间核验后结束，25432/29000 无监听。未暂停正常部署服务，Kubernetes `/readyz` 返回 ok。

### 2026-09-25：后台展示查询与 Workspace 文件交付

- 增加会话级查询任务创建、状态、取消和带版本恢复 API，生成 Go/TypeScript 契约及 real/mock 适配器。任务冻结最新发布版、绝对时间、来源、资源、有效参数和剩余预算；输入不能提供替代查询文本或存储地址，不新增 Dashboard 导出界面。
- migration 00005 保存任务、独立取数尝试、不可变文件清单及交付记录。现有 default / agent Worker 池消费 `dashboard_query` 队列；沿用 PostgreSQL、ObjectStore 和原生查询引擎。任务租约/owner/fence 约束提交，取消和撤权终止在途工作。
- 结果按 Target 保存原生 JSON 和 1 MiB 对象分片，另有独立清单。未封存取数中断后使用新尝试；已封存后的交付恢复复用旧文件，不重复取数或拼接不同尝试。清单记录 Definition、请求/有效条件、来源、实际步长、Hash、状态及真实执行授权版本；交付完成不等于已分析。
- Workspace 在写入前登记不可变尝试来源，逐片验证后流式原子导入；来源检查覆盖整个目录及派生文件。长度/Hash 不一致拒绝提交；重复导入补齐崩溃后缺少的登记，不覆盖被用户修改的副本。保留现有配额和来源撤权规则，不自动删旧文件。
- 删除会话会取消查询任务；对象前缀清理等待活跃查询租约结束。失败重试、用户恢复和后台执行统一锁顺序。补齐 Trace 未显式限制时的预算截断状态，明确区分用户指定 pageSize 与系统限额。
- 协议、限制和状态定义见[查询任务与结果文件](./query-files.md)。目前只覆盖已发布展示查询，尚未注册模型 Dashboard 工具；已发布下钻的文件任务、明确选择与 Run 上下文、Chat 创建/分析仍待接入。

验证：临时 Namespace `argus-p2-query-20260925-f6c1` 中使用真实 PostgreSQL 17、ClickHouse 26.3.17.110 与 MinIO。通过分片损坏拒绝、取数中断的新尝试、交付/空间故障恢复、手动恢复幂等、旧 Worker 拒绝、查询及 Run 取消、跨企业/会话、撤权、入队固定版本/新请求取最新、真实 Processor 消费和删除清理边界。结果直接写入遥测存储；交付故障使用注入适配器，不能算作真实 Collector 或完整 Workspace 部署 E2E。

另用真实 PostgreSQL 和本进程文件 gRPC/文件系统验证 Workspace 原子导入、重复交付、写入后未登记的恢复、用户修改保护、Hash/大小拒绝和目录来源撤权；未运行 Kubernetes PVC 容量/节点故障场景。MinIO 使用节点已缓存镜像的固定 digest `sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e`；仓库开发 MinIO 镜像拉取失败后未改动镜像仓库、代理或其他服务。

Dashboard、Workspace、Workspace IO、Conversation、Runtime、HTTP、Worker、Server 编译或相关测试、遥测回归、契约及相关 go vet 通过。API Client 90 个测试、类型、受影响 ESLint、i18n/样式、正式构建、包体积和生产 JS 块冒烟通过；8 个已有仪表盘 Chromium 场景复验通过（mock）。Dashboard OpenAPI lint 保留 25 条通用 4xx 响应警告；定向生成，不宣称全量生成一致性门禁完成。

清理：验证 Namespace 所有者标签、仅有 PostgreSQL/ClickHouse/MinIO 三个测试 Pod 且无 PVC 后，删除 `argus-p2-query-20260925-f6c1` 并确认不存在；三条端口转发按 PID/启动时间核验后结束，25432/29000/29001 无监听。没有暂停正常部署服务，Kubernetes `/readyz` 返回 ok。

### 2026-09-25：已发布下钻的持久文件链路

- 会话查询任务新增已发布下钻输入，只接受父任务、Panel/下钻引用、声明的行值及完整链路开关。不能覆盖父条件、跳过依赖、提交替代语句或引用别的会话/仪表盘。
- 人工交互与文件任务共用下钻准备器，统一参数、时间裁剪、来源策略和锚点检查。文件行证据从服务端封存分片读取，验证到 EOF 及总 Hash；不以可修改的 Workspace 副本作为证据，也不为核验行值重跑展示查询。
- 继承父版本、有效参数、时间和来源；明确打开完整链路时，入队固定当前另有授权的资源。新增授权不扩大已排队范围，撤权后旧任务和来源访问均拒绝。行证据读取、锚点和详情共用预算。
- 清单新增父任务/尝试/执行、目标、选择和深度，支持 Trace 详情、完整链路、Span 日志、日志前后文等连续下钻，最多 16 层。恢复只执行已冻结的详情，交付恢复不重复取数。
- 同一 Run 内沿父结果继续；新 Chat Run 必须重新获取最新发布版，省略 Run 不能绕过绑定。持久文件上下文独立于浏览器 15 分钟 Token。新的普通查询仍读取最新发布版。
- Go/TypeScript、HTTP 严格字段校验及客户端适配器已同步。mock 只演示无行输入、查询完全相同的 inspect，其他定义明确拒绝；模型工具尚未注册。

验证：临时 Namespace `argus-p2-detail-20260925-b7e2` 中的真实 PostgreSQL / ClickHouse / MinIO 完成 Trace 列表 → 详情 → 授权完整链路 → Span 日志 → 日志前后文文件交付；验证伪造/跨记录拼接、未封存父任务、原分片损坏、参数覆盖、行证据预算、冻结授权集合、撤权、发布新版本后的旧链路与新查询、新 Run 限制，以及交付重试不重新取数。Workspace 交付故障沿用注入适配器；数据直接进入存储，仍不算 Collector/Kafka 或正式 Workspace PVC E2E。

Dashboard、HTTP、Workspace、Conversation、Worker、Server 编译或相关测试、契约和相关 go vet 通过。API Client 90 个测试、类型、受影响 ESLint、i18n/样式、正式构建、包体积和生产 JS 块加载通过；8 个既有仪表盘 Chromium 场景复验通过（mock）。OpenAPI lint 保留 25 条既有通用 4xx 响应警告。

清理：核验所有者标签、仅有 PostgreSQL/ClickHouse/MinIO 三个测试 Pod 且无 PVC 后，删除 `argus-p2-detail-20260925-b7e2` 并确认不存在；三条端口转发按 PID/启动时间核验后结束，25432/29000/29001 无监听。正常部署服务未暂停，Kubernetes `/readyz` 返回 ok。

### 2026-09-25：Chat 结构化选择、创建入口和 Dashboard 原生工具

- 正式 Chat 的 @ 列表加入授权仪表盘，选择保存在独立字段中；输入区和历史消息显示标签。支持多选、追问沿用、刷新恢复、移除不可用对象和显式退出。创建入口按当前语言填写 /创建仪表盘 或 /create-dashboard。
- migration 00006 保存会话选择及不可变 Run 选择。消息提交与选择版本检查同事务，preflight 不写状态；发送、恢复、工具调用重验授权。旧窗口冲突不能静默覆盖，旧 Run 不受后续选择影响。
- 抽出共用对象授权 package，避免 Conversation 与 Dashboard 服务的循环依赖。SkillContext 的 ID/版本/Hash 纳入 Run 快照；模型事实从服务端 Run 选择重建，压缩摘要不能改选对象。
- 新增 dashboard Gateway 分类，注册授权目录、选中定义、真实 Catalog/资源来源目录、无损转换、个人草稿、标准下钻、发布预览和查询任务的 14 个原生工具。Schema 复用 OpenAPI；模型不能提交 Run 身份、替代查询或私有对象路径。
- 创建草稿按不可变工具调用身份幂等复用；保存/生成下钻检查版本。发布预览关联当前 Run，仍由 PendingAction 宿主一次确认，私有 publish.commit 不对模型开放。更新对象须已被用户明确选择。
- 分析/创建模式限制查询通道，Gateway 发现缓存和分页范围包含 Run；普通会话保留客户 MCP 直连。新根查询采用最新发布版；新 Run 不得恢复旧的未物化取数来绕过此规则，旧封存文件可继续交付。
- mock 只演示选择与入口；仪表盘模式不再复用通用的“整体正常”假回复，也不声称已经取数或分析文件。实现契约和边界见 [Chat 上下文与原生工具](./chat-context.md)。

验证：临时 Namespace argus-p2-chat-20260925-c4d1 的真实 PostgreSQL 17 从空库升级至 migration 00006。覆盖消息与 Run 固定选择、跨主体拒绝、旧窗口版本冲突、恢复保持原选择、撤权后拒绝、显式清空、命令激活、普通 MCP 保留/分析隔离、未选对象不能查询、个人草稿重复调用复用、关联当前 Run 的预览及模型无法提交发布。目录验证未授权资源不泄漏。

相关 Dashboard、Conversation、Gateway、DashboardContext、Agent、Runtime、HTTP、Server、Worker 和契约检查通过；go vet 通过。首次多 package 共用测试库并发运行出现 Serializable/全局任务队列争用，改用干净测试库并串行执行后全部通过；不将首次失败当作通过。

API Client 91 个测试、Enterprise 136 个测试通过；新增 2 个 Chat Chromium 场景覆盖中文浅色和英文深色，连同 2 个既有 MCP/Workspace 场景最终合跑 4/4 通过。英文初次出现 Chromium ERR_NO_BUFFER_SPACE，独立重跑与最终合跑均通过。类型、受影响 ESLint、i18n/样式、正式 real 前端构建、包体积、git diff --check 及修改/新增源码不超过 2000 行检查通过。Conversation OpenAPI lint 通过，保留 13 条通用 4xx 响应规则警告；定向生成，不宣称完整 contracts check 已通过。

当前验证没有使用真实模型、Collector/Kafka 或 Workspace PVC；三信号文件服务沿用前阶段分段证据。本轮仍未实现显式时间/资源/变量条件的持久继承及新 Revision 兼容性，也未完成跨工具目录与多次查询累计预算和实际模型 E2E，Task 02 继续保持未完成。

清理：核验 Namespace 的 codex-planv2 所有者标签、仅有 PostgreSQL 测试 Pod 且无 PVC 后，删除 argus-p2-chat-20260925-c4d1 并确认不存在；按 PID/启动时间核验结束端口转发，25432 无监听。正常部署服务未暂停，Kubernetes /readyz 返回 ok。

### 2026-09-25：显式条件、发布兼容性与 Run 累计预算

- migration 00007 保存会话条件、每个 Run 的独立条件和不可变分析上下文。仅继承明确覆盖，仪表盘默认值与临时 Panel 选择不写为长期条件；相对时间保留长度，每次解析冻结绝对端点。
- 新增 context.resolve、context.candidates、budget.get；原生 query/v2 只接收 context_ref 和临时 Panel 范围或已发布下钻。服务端校验最新发布版、条件版本、主体、会话及 Run，过期上下文不能绕过最新版本。
- 条件变更逐项关联本轮用户原文；服务端核对引用与结构/权限，记录为模型解释，不宣称证明自然语言语义。get 补服务器时间和企业时区。语义不兼容需重新明确，不能自动 reset_all。
- 参数指纹覆盖来源、候选定义、依赖、值映射及规范化查询使用方式。查询将同一参数移到别的字段、变量删除或依赖含义变化会拒绝静默继承；标题、默认值和支持的等价格式变化保持兼容。
- 查询入队时原子记录候选已证明消失后的 All；仅更新显式值并保留 candidate_reset 来源，不覆盖更新条件。候选预览仅执行已发布定义及依赖，不执行统计图、不修改条件，截断/部分列表不会冒充完整候选。
- 分析模式可读取授权资源目录以解析名称；重名仍须澄清。修复大资源目录被 1000 资源执行上限挡住的问题，目录先分页再读取元数据，未放宽执行上限。
- migration 00008 建立跨目录、变量、指标元数据、Panel、下钻和行证据的持久额度。先原子预留，再按可信使用量结算；未知、过期或崩溃预留不返还，分页/重试/新 Runtime 不重置。并发结算不能重复返还，超限后迟到结算也不能重新开放取数。
- 沿用 256 MiB 扫描、8 MiB 结果/行证据、50,000 行、5,000,000 Metrics 样本限额，并限制每 Run 256 次目录/取数/行证据调用。HTTP 人工查看继续受原单次执行限额约束。完整契约见 [条件继承与累计预算](./chat-conditions-budget.md)。

验证：临时 Namespace argus-p2-conditions-20260925-d8a4 中运行真实 PostgreSQL 17、ClickHouse 26.3.17.110 和 MinIO。PostgreSQL 从空库迁移至 00008；新增确定性/集成测试覆盖条件独立继承、选择性重设、候选回退、未知/跨 Run 上下文、最新版本检查、重复调用、未发布候选拒绝、1001 资源目录、并发预算、重复/未知/过期结算、超限和取消。预算边界使用可控后端计量；已有展示、Trace/APM 下钻、文件分片及交付恢复的真实三存储测试复跑通过。

相关 Dashboard、Context、Gateway、Conversation、Agent、HTTP、Server、Worker、契约和 go vet 通过。OpenAPI 描述中的行内逗号曾被解析成多余键，已修正、重新生成并复验 HTTP/契约；最终 lint 通过，保留 25 条既有通用 4xx 警告，不宣称完整 contracts check 已通过。API Client 91 项、相关 Chat/i18n 28 项测试通过，4 个 Chromium Chat/MCP/Workspace 场景通过（mock）；类型、i18n/样式、正式 real 前端构建、包体积和修改/新增源码不超过 2000 行检查通过。

本轮没有实际模型自然语言与文件分析、Collector/Kafka 或 Workspace PVC 的正式镜像 E2E；没有用 fake 计量后端替代真实三信号数据验收，也没有把确定性消息引用检查当作模型理解正确的证明。Task 01/02 继续保持未全部完成。

清理：核验 codex-planv2 所有者标签、仅有 PostgreSQL/ClickHouse/MinIO 三个测试 Pod 且无 PVC 后，删除 argus-p2-conditions-20260925-d8a4 并确认不存在。三条端口转发按 PID/启动时间核验后结束，25432/29000/29001 无监听；正常服务未暂停，Kubernetes /readyz 返回 ok。

### 2026-09-25：遥测对象权限统一与数据投影

- 移除 telemetry.query.metrics/logs/traces 与 telemetry.sensitive_fields.read，注册表升为 11。资源查询使用共享 ResourceQueryActor，HTTP 和原生工具按 host.read/kubernetes.read 及明确对象授权读取所有信号；Dashboard 保持自身访问与资源数据范围校验。空范围、混合越权、停用对象及执行中撤权拒绝返回数据。
- Native Manifest 增加 any_required_permissions，发现/描述/调用一致检查对象能力；服务账户仍受 allowed_tool_ids 限制。权限迁移仅移除旧能力并递增受影响版本，不自动增加对象访问。
- 前端权限矩阵及 mock 内置角色由后端注册表生成，并加契约一致性检查。全部权限勾选保存明确清单；移除不存在的旧遥测权限选项，同步模型配额的规范权限名。
- 内部查询 Scope 删除信号权限数组和字段开关，保留旧 protobuf 字段号；RPC v4 携带主体类型/ID 和技术路由 signal。范围哈希、目录游标和查询审计绑定主体；相同资源/授权版本的不同用户仍隔离。
- 统一凭证投影保留普通日志正文和 Trace 属性/事件/Links，遮蔽识别出的凭证并给出警告。凭证目录不返回原值或原值续页键；普通字段目录继续可用。投影没有管理员绕过开关，不宣称可识别任意未标记/编码秘密。
- Workspace/Tool Result 指纹加入 Dashboard 授权及投影版本；Dashboard 编译器为 v3，旧查询文件要求重新取数，不自动删除旧 Workspace。迁移和协调升级要求见 [对象查询授权](./object-query-authorization.md)。

验证：临时 Namespace argus-p2-access-20260925-e3c7 中运行 PostgreSQL 17、ClickHouse 26.3.17.110、MinIO。从空库升至 00009，并在事务内重建旧权限验证撤销不扩大授权、角色/成员/服务账户版本变化。HTTP、原生工具、服务账户白名单、混合/空范围、停用集群、执行中撤权、会话版本及跨主体 Scope/游标检查通过。真实 OTLP 对象写入 ClickHouse 后验证日志与 Trace 普通细节保留、凭证遮蔽；复跑三信号查询、仪表盘、下钻和 MinIO 文件任务回归通过。

相关 Authorization、Telemetry/Engine、Dashboard、Gateway、Presentation、Conversation、Workspace、HTTP、Server/Worker、开发工具和契约检查通过，go vet 与 protobuf lint 通过。删除旧未使用投影函数后发现一条历史单测仍调用它，已移除过时断言并以统一投影测试覆盖，Telemetry 全包复验通过。API Client 93 项、Enterprise 136 项测试，角色目录/权限矩阵及 Chat 共 6 个 Chromium 场景通过（mock）；类型、i18n/样式、受影响 ESLint、正式 real 构建与包体积检查通过。

本轮没有执行新版内部 RPC 的正式集群滚动升级、真实 Collector/Kafka 到模型/Workspace PVC 的全链路验收。已更新 M7 验收脚本为统一屏蔽和对象撤权语义，但没有把脚本更新标记为该完整部署验收已运行。

清理：核验 Namespace 和三个测试 Pod 的 codex-planv2 标签、确认无 PVC 后，删除 argus-p2-access-20260925-e3c7 并确认不存在。三条端口转发按 PID/启动时间核验结束，25432/29000/29001 无监听；正常服务未暂停，Kubernetes /readyz 返回 ok。

### 2026-09-25：正式图型显示配置与结果口径

- Panel.display 增加样本显示计算、自动/固定范围、时序折线/面积/柱形、堆叠和平滑；既有 thresholds 开始实际参与渲染。OpenAPI、Go/TypeScript 契约、mock、服务端验证与正式编辑器同步，人工与 AI 创建复用相同边界。
- last 保留末尾缺失，其他计算只统计有限样本；按每条序列独立处理，不合并同名来源。mean 不按时间加权，sum 不等同计数器增量；编辑和查看都明确口径。百分比比例的范围/阈值使用原始 0–1。
- 固定范围、阈值顺序/数量/颜色、图型适用性受服务端硬校验；阈值只展示、不触发告警。切换 Metrics 图型保留查询模式；不再偷偷把区间统计变成瞬时值。
- 编辑、预览、发布、普通查看和全屏使用相同配置。显示变化进入完整发布 Diff 和新 Revision，不改变查询哈希及 AI 文件取数语义。Canvas 的阈值颜色解析设计 token，兼容双主题；补齐 ECharts 坐标轴、图例、色阶、Tooltip 与 Gauge 标签的显式主题配色，避免默认灰色在深色背景上对比度不足。
- 补齐 Histogram 的边界唯一性、合法性与同一时刻检查；空 Heatmap 不产生无限色阶，时间索引使用 Map。小图时间标签隐藏重叠；饼图明确提示被省略的负值，并按相同计算选择正确下钻序列。
- 正式能力矩阵、单位和计算规则见 [显示配置](./chart-display.md)。本轮没有增加 Grafana 全量变换、多轴、字段覆盖等额外承诺，也没有增加仪表盘导出功能。

验证：临时 Namespace argus-p2-display-20260925-f6b2 运行 PostgreSQL 17 和 ClickHouse 26.3.17.110。Dashboard 全包测试通过；新增显示配置硬校验、发布差异及真实发布/查询回归，验证显示配置和阈值经过 Revision 与 builder/DSL 转换仍保留，显示修改前后实际查询哈希和返回数据相同。此轮没有配置 MinIO，文件服务集成保持前阶段证据，不计作重新验收。

共享 UI 55 项、API Client 95 项、Enterprise 136 项测试通过（共享 UI 先全跑 52 项，再定向跑新增与相关共 9 项）。仪表盘 Chromium 10/10 通过，覆盖新增中英文/深浅色显示配置、刷新恢复、发布与全屏，以及十一类图型、布局、授权和资源入口。浏览器使用 mock API，未将其标为真实部署 E2E；截图检查后修正时间标签重叠与主题对比度并复跑全部场景。

相关 HTTP、Server/Worker、契约测试和 go vet 通过；类型、ESLint、i18n/样式、real 前端构建和包体积检查通过。OpenAPI 定向生成和 lint 通过，保留 25 条既有通用 4xx 警告，不宣称完整 contracts check 已完成。最初使用了错误的 Go package 路径，修正为 internal/transport/httpapi 与 internal/app 后重跑通过。

清理：核验 Namespace 及 PostgreSQL/ClickHouse 两个 Pod 的 codex-planv2 所有者标签且无 PVC 后，删除 argus-p2-display-20260925-f6b2 并确认不存在。端口转发按 PID/启动时间核验后结束，25432/29000 无监听；正常服务未暂停，Kubernetes /readyz 返回 ok。

### 2026-09-25：真实 Collector 配置、组件目录与来源转发

- 从当前源码重新构建 Linux amd64 Collector。发行目录改用统一组件注册表，补齐 resource、health_check、k8s_cluster、argus_gateway_identity，并修正 k8s-cluster Profile 组件 ID；CatalogRevision 为 2。API Client mock 版本/组件由同一注册表生成，避免继续展示过期 0.132.0 元数据。
- 修复 collector-self 只启用主机指标的问题，按 30 秒周期抓取回环地址上的实际 Collector 内部指标；Host、K8s Agent/Gateway 都使用独立 prometheus/collector_self 来源。Profile 与 mock 仅声明真实支持的 metrics，Host basic 不再声称包含自身采集；已安装 Collector 须应用新配置。
- 增加三平台 OCB 清单与注册表的契约检查，以及真实二进制的组件输出比对。在独立 Namespace argus-p2-collector-20260925-g4c8 中，四类完整 Host/Gateway/K8s 配置通过实际 Collector validate；仅挂载 kubelet 公开证书，不读取私钥。
- 运行真实 OTLP 与 Prometheus Receiver、来源标记 Processor、Batch 和 OTLP Exporter，三信号及 Collector 自身指标到达接收端。验证同名指标来自两个 Receiver 时来源身份不同、自身指标另有来源、配置版本保留、SDK 伪造的来源 ID/版本被覆盖。测试端点/TLS/注册/队列使用夹具，不把该测试视为 Ingest/Kafka 或全链路验收。
- 正常 Go、CollectorManager、开发工具和契约回归通过，go vet 通过。真实 wire 测试初次因夹具缺少 gzip 解码器失败；补齐后全部重跑通过，正式 Ingest 原有 gzip 注册不需修改。复现步骤和准确边界见 [Collector 验证](./collector-validation.md)。

阻塞：Docker 服务端版本调用在 10 秒限时内没有响应，已结束只读客户端检查。Kubernetes 正常，未重启 Docker/WSL、未暂停其他项目服务。不能据此完成正式镜像构建与 Collector/Kafka/Chat 全链路验收；本地 Collector 重建也不代表远端签名产物或已安装 Collector 已升级。

清理：确认 argus-p2-collector-20260925-g4c8 所有者标签、仅有自有 collector-validation Pod 且无 PVC 后，删除 Namespace 并确认不存在；Kubernetes /readyz 返回 ok。没有对共享宿主文件执行写入或清理，所有测试 Collector 子进程随测试结束退出。

### 2026-09-25：Docker Windows 通道恢复

已解除前一阶段的 Docker 构建阻塞。Linux Engine/Unix 代理原本正常，Windows 命名管道与控制接口无响应；常规 restart 也被阻塞。重新启动 Windows 后端时发现失效 AF_UNIX 套接字，且 Codex MSIX 缓存重定向使第一次 Secrets Engine 隔离未作用于真实目录。采用同一用户的非 MSIX 进程核验并改名保留临时套接字目录后，Docker Engine 29.7.2、BuildKit 和 doctor e2e 恢复。

恢复时保留原 4 个 Docker 容器、36 个卷和 9 个原绑定 PVC。为恢复现有 MinIO，将不可拉取的 latest 引用固定为节点中已核验的原镜像摘要并使用 IfNotPresent，保持数据与镜像内容。原有 34 个运行 Pod 全部 Ready，Runtime Gateway 就绪，现有业务登录页返回 200。正式 Collector Dockerfile 已实际构建和运行通过；详细操作、资源核验及这个依赖补丁的范围见 [恢复记录](./docker-recovery-20260925.md)。

恢复后的 a 轮暴露 Web Dockerfile 漏复制共享构建脚本的问题，已改为复制完整 scripts 工具目录；同时修复隔离 Ingress 在目标 Namespace 尚未创建时退出的问题，改为按本次 release ID 监听并保留独立 IngressClass。b 轮继续发现 Helm 只执行 ClickHouse 第一份 SQL、停在 v3；现已按序打包/执行全部迁移，并增加集合、内容、运行时版本与 Helm 渲染检查。

b 轮在自有环境补跑完整迁移后通过；最终 c 轮（p2-recovery-20260925-c）从新 Namespace 和空数据库重新安装，全程无需手工修补，Ingest/Writer/Query 首次启动均就绪且重启为 0。M2/M3/M4/P5-native/M7/M10 全部通过，真实 Chromium 23/23 通过；Collector 安装/配置/修复/升级/卸载、三信号经 Kafka/Writer/ClickHouse 查询、堡垒机、权限与故障恢复，以及 HTTPS/证书、存储、OpenSandbox 健康验证通过。Agent 部分使用确定性 Replay，不作为实际模型能力证据。

两轮进程均以 0 退出并完成清理；临时 Namespace、PVC、RBAC/Lease 与自有镜像引用已清理。清理后原 9 个 PVC 仍绑定原卷、原 34 个运行 Pod 全部 Ready、36 个 Docker 卷保留。相关 Go/契约测试、go vet、diff 与源码行数检查通过。原 a 轮失败与 b 轮修补经过保留，详细证据见恢复记录；不能把基础部署通过等同于新版 Dashboard/Chat 的全部专项验收完成。

## 历史未关闭门禁（现状已由 2026-09-28 验收矩阵更新）

当前不是可用的完整仪表盘产品，不可把上述基础标记为 Task 01/02 完成。

1. `PublicationVerifier` 与 Runtime 已接入变量、局部筛选、候选依赖、有效参数绑定、五类 APM 查询和标准下钻。基础正式页面与模拟浏览器已接入；真实 API 的浏览器发布、冲突、撤权和归档恢复组合验收仍待完成。
2. 来源链路已有单元、真实存储和新构建 Collector 的配置/实际转发回归；m10-query 已覆盖三信号经 Ingest/Kafka/Writer/ClickHouse/Query 的完整传输，以及临时环境的签名包安装/升级。Dashboard 专用来源过滤、历史代次与 UI 的组合验收仍待补齐，尚未对长期正式部署发布升级。
3. Trace 已按事实跨批次去重和组装，支持迟到更新、缺失/冲突结构状态；APM 查询、标准下钻、关联日志和 Q31 显式授权扩展已有后端实现。跨源歧义和未连接片段明确展示状态；完整链路页面及真实采集来源验证仍待完成。
4. Catalog 已有指标/字段/候选分页、独立选值检查、变量/局部条件绑定、标准下钻、模式转换和错误率构建器。展示和下钻查询文件任务已支持物化、交付、取消和恢复；缓存及真实 Workspace 部署验收仍待完成。共享预算覆盖候选、元数据、展示、行证据及下钻查询。Dashboard 与旧资源查询入口已统一到对象权限；三信号/字段权限已从注册表、HTTP、工具、前端、游标和 Workspace 来源路径移除。缓存优化和正式部署验证仍待完成。
5. Workbench、十一类图型及 Demo 中的适用显示配置、可选阈值、拖拽缩放、两层过滤、Trace/APM 共享展示、Host/K8s 快捷关联、Dashboard 授权、查询转换/详情编辑、原始字段探索及 mock/real 适配器已实现。仍待这些功能的真实采集数据/浏览器组合验收；具体展示能力以显示矩阵为准，Demo 不计入完成量。
6. 后台展示/下钻查询、不可变分片、Workspace 来源登记/恢复已有实现和分段验证。Chat 结构化选择、创建入口、个人草稿/发布预览和 Dashboard Gateway 工具已接入。显式时间/资源/变量条件的持久继承、发布兼容性、已发布候选和跨工具累计预算已有实现与确定性回归。自然语言解释、实际模型文件分析及完整 Workspace 部署 E2E 仍未完成，不能因接口和后台任务可用就视为 AI 分析完成。
7. 已在临时 Kubernetes Namespace 通过正式镜像、真实 Collector/Kafka 三信号和 M2/M3/M4/P5-native/M7/M10 基础部署回归；仍需新版 Dashboard 发布编辑、APM 详情、Dashboard 查询文件到 Chat/Workspace 的专用组合 E2E，实际模型验收仍待配置。


## 2026-09-25 Runtime 刷新、缓存、新鲜度及契约收尾

详见 [行为、隔离范围和验证口径](./runtime-refresh-cache-freshness.md)。正式页面拆出执行调度 Hook，支持可见性暂停、自动刷新忙碌跳过、传递依赖刷新、候选单独协调及并发条件合并。Query Coordinator 接入 64 MiB/1024 条/30 秒缓存，保留限流、授权、预算和审计；RPC 传递原查询完成和原始样本时间。共享 UI 提供数据时间详情。

临时 Kubernetes 中的 Dashboard/三信号/APM/文件任务存储回归与专项缓存/来源/撤权验证通过。修复契约 clean generation 的密码策略 bootstrap 顺序后，全量 contracts check 通过；未把分段存储或 mock 浏览器替代正式 Dashboard/Chat/Workspace 专项。

最终 mock Chromium 12/12、刷新 Hook 9 项和共享组件定向 8 项通过，类型/ESLint/i18n/样式、相关后端测试、real 前端构建及包体积通过。测试 Namespace 已删除，原有 9 个 PVC 和 34 个就绪 Pod 核验保持。剩余真实页面/模型/PVC 组合验收及多图性能仍列于复核清单。
