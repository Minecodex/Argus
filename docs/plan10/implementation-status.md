# Plan10 正式实现与验收

日期：2026-10-28。实现目标为企业门户 `/observe`。以下记录代码实现与独立验收；用户随后授权的本地发布已完成，见[本地 Kubernetes 发布](./local-deployment-20261028.md)。

## 已实现

- 单一观测查询入口，查询工作台与指标/字段目录；首次创建空白指标构造器，必须先选资源。
- 三信号混合临时面板，独立资源与来源、共享冻结时间、最多 3 个执行请求；新增、复制、排序、删除、取消与执行全部。离开或刷新清空，不写查询组/历史/草稿。
- 面板与目录均通过资源范围按钮打开共享选择弹窗，目录范围独立保存。空集合不回退全部；复制、目录筛选和资源快捷入口不修改其他面板。
- 三类 Builder 与服务端无损转换；共享 CodeMirror 6、PromQL 0.314.0、Argus KQL 及后端固定只读 GraphQL SDL。支持上下文补全、取消、范围隔离和错误提示。
- 无效 JSON 保留输入并阻止执行；目录类型元数据异步补齐不取消合法查询。资源/主体变化不沿用旧候选。
- 类型化 `scope` 与旧 `resource_ids` 互斥，保留旧显式范围语义；新增资源查询目录接口，保持原仪表盘接口权限。应用读取主体没有 Dashboard 权限也能发现、转换和查询自己的数据。
- 指标按来源版本、名称、类型及单位分页，不把同名异义项合并。推荐语句来自内置版本化模板；未观测和未知元数据如实显示。
- 主机与 K8s 顶层三信号 Tab 已移除，使用观测查询快捷入口；关联仪表盘、采集配置、终端和 Pod 实时日志保留。

## 2026-10-28 排版调整

- 未选资源时，在面板资源范围按钮旁显示“请先选择资源”，不在编辑区插入大块空状态。
- 指标构造器与仪表盘编辑页共用 `MetricQueryFields`：指标、跨资源计算、拆分曲线同排，类型/单位提示、条件标签和高级查询沿用相同组件。日志与链路使用原共享编辑器的表单布局，删除观测页的三列覆盖规则。
- 顶部联动十字线、共享时间与执行全部使用共享 Card 和表面颜色 token 包裹；时间下拉按内容宽度排列，与执行按钮保持同排。深色使用对应表面配色。
- 根据后续反馈，目录资源范围改为同款多选弹窗；目录的时间选择移入名称、资源类型、资源范围、来源与已观测状态所在的筛选栏。切换目录不保留顶部单独的时间工具栏。
- 目录分页游标绑定当前资源、信号、名称搜索和绝对时间窗口；这些条件变化时回到第一页，避免旧游标导致查询失败。目录刷新在明确零资源时不发起查询，不回退全部有权资源。
- 9 项观测/条件编辑浏览器回归和 1 项仪表盘桌面布局回归通过，包含中英文、深浅色与 1280/1440px 对齐检查。类型检查、i18n、样式检查和 real Web 镜像构建通过。证据在 `artifacts/plan10-layout-20261028/`，空白面板截图为 `browser-final/observe-observation-workspace-zh-light-chromium/empty-workspace.png`。
- 后续目录调整的最终 9 项观测回归通过，新增时间/名称变更时分页重置、目录资源多选/取消/明确空集合检查；四组中英文与深浅色目录截图位于 `artifacts/plan10-catalog-layout-20261028/browser-final/`。这轮只改变前端，真实采集引擎仍沿用前次验证和发布的后端内容。

## 2026-10-28 行内指标构造器

- 观测查询和仪表盘主查询编辑页共用 `QueryTargetEditor` 的指标布局：采集来源、指标、计算方式、分组依次排列；指标选择使用紧凑宽度，窗口和 Top K 只在相应运算下出现。
- 条件直接在页面新增/删除，移除条件弹窗。标签名通过当前指标的字段目录分页搜索，值通过当前标签和其他条件读取；权限、资源、来源与时间继续使用共享 Catalog 上下文。正则条件保持原语义，未解析变量不扩大候选范围。
- 不再为指标显示层叠的“高级查询与过滤”区。查询方式（时间序列/当前值）与自动/固定步长直接显示；固定模式才显示秒数。变量映射仅在存在变量、局部参数或既有绑定时保留。设计依据见[查询交互说明](../planv2/query-authoring.md)。
- 共享 ComboBox 保留正在输入的搜索词，异步空列表显示加载状态；切换上下文清除旧候选。mock 转换仅补充固定的字面量标签夹具，真实模式继续使用 Go 编译器。
- 18 项浏览器回归、14 项相关单元测试、4 组真实 SDK/Collector/Ingest/Kafka/Writer/ClickHouse/浏览器测试通过。真实浏览器已验证标签名和值选择后带条件执行；不是只验证原型或 mock。临时 Namespace `argus-plan10-inline-f578bc7b` 已按 owner 核对后删除，未使用业务数据库；证据位于 `artifacts/plan10-inline-builder-20261028/`。

## 2026-10-28 图表浮层、绝对时间与卡片折叠

- 所有共享统计图与旧遥测曲线的详情改为 HTML 浮层，挂载到各自 document.body，使用独立最高层级 token。详情可越出图表和卡片裁剪区域，名称与值仍走 ECharts 默认文本转义；没有跨 iframe 访问父页面。
- 观测查询复用仪表盘 `TimeRangePicker`，默认显示当前一小时的绝对起止时间，可切换相对范围或编辑任意有效起止时间。执行全部先冻结一组窗口，三信号使用相同边界，自动步长按实际跨度计算；目录与补全也读取当前窗口。
- 点击查询卡片标题栏空白或标题文字可收起/展开，也提供带 aria-expanded 的键盘按钮。资源/类型/排序/复制/删除控件及它们的弹窗不触发折叠。内容保留挂载，结果与配置不会丢失；隐藏图表清除详情浮层并暂停联动，展开后恢复尺寸和联动。折叠面板校验失败时自动展开定位字段。
- 12 项浏览器检查与 16 项相关单元测试通过，覆盖深浅色遮罩/弹窗、详情文本转义、历史绝对窗口、三信号同窗、取消时间修改、折叠保留结果和无效字段展开。类型、样式、i18n 与定向 ESLint 检查通过。
- 本轮使用独立 Web 发布副本；另一个任务未完成的模型治理 UI 仅从副本中排除，原工作区 15 个相关文件的内容摘要保持一致，本轮 14 个功能文件与发布副本逐字节一致。证据位于 `artifacts/plan10-chart-time-collapse-20261028/`，包含 `staging.json`、`feature-hashes.json`、`browser-final.log` 和 `tooltip-verified.log`。

## 2026-10-28 查询按钮与目录筛选分组

- 查询面板移除语言徽标和面板内重复的临时查询提示；执行/取消按钮移到构造器/DSL 切换栏右侧并保持同排。
- 指标的图表/表格切换在存在查询结果后才显示；查询条件变化清除结果时同步隐藏，执行状态仍保留。
- 目录通过共享 FilterBar 的 `inlineActions` 槽位，将资源范围、来源和已观测开关紧接名称/资源类型筛选；绝对时间与刷新组成右侧操作区。原有 FilterBar 调用不改变布局。
- 10 项浏览器回归通过，覆盖中英文、深浅色、1280/1440px 查询栏对齐、1920px 目录左右分组与查询前后切换状态。类型、i18n、样式和定向 ESLint 检查通过。证据位于 `artifacts/plan10-query-toolbar-20261028/`。
- 发布基于上次已验证的独立 Web 副本，仅叠加本轮 4 个产品文件的改动，保留工作区其他未完成内容。

## 接口与部署注意

新增 `GET /api/v1/enterprise/telemetry/query/resources`、`POST /api/v1/enterprise/telemetry/query/catalog`；原指标 instant/range、日志和 GraphQL 入口扩展结构化 scope。资源范围不拼入语句，不增加三信号权限或数据表。

内部 Catalog 协议为 `argus.telemetry_catalog/v5`，API 与 Query 服务应同时发布；后续本地发布已配套更新。GraphQL 编辑器构建时直接读取后端固定 SDL，Web Dockerfile 包含相同文件。原生 GraphQL 的参数不等同于已退役的资源标签模型；契约门禁仅对 Lezer 的精确语法符号别名导入作区分，业务旧字段仍禁止。

## 验证层次与证据

原型的 33 项检查见 [原型验证](./verification.md)，不计入正式实现验收。正式检查使用当前源码，日志与 mock 浏览器证据位于仓库 `artifacts/plan10/`，real 浏览器证据位于 `.cache/observe-real-results/`。

| 层次 | 验证内容 | 状态 |
| --- | --- | --- |
| 类型与静态门禁 | Enterprise 类型检查、i18n、设计变量/样式、文件行数 | 通过；类型检查包含在最终真实构建中 |
| 组件与适配器 | 并发与取消、语言草稿、上下文过滤、AbortSignal、目录适配与既有元数据回归 | 21 项通过（Enterprise 9、共享 UI 7、API client 5） |
| mock 浏览器 | 工作台、旧入口迁移、资源范围隔离、目录筛选、三语言补全、无效 JSON、双语配色、共享 ComboBox | 16 项通过 |
| 入口回归与布局 | 原有三信号浏览器用例迁移；1440px 目录筛选栏同排布局 | 2 项迁移回归、1 项定向布局复核通过 |
| 真实 PG/ClickHouse | 显式/伪造范围、读取权限、转换、撤权、来源/单位变体与游标隔离 | 已通过 |
| 实际采集与 real 浏览器 | SDK → 原生 Collector → 认证 Ingest → SASL Kafka → Writer → ClickHouse → HTTP → 页面；无 Dashboard 权限的应用读取主体 | 最终源码通过，4 组中英文/深浅色浏览器测试通过 |
| 契约与构建 | OpenAPI、Go 契约、真实三门户构建、bundle 预算 | 通过；OpenAPI 仍有 683 条 warning 级提示 |
| 清理 | 删除本轮临时 Namespace，结束端口转发，确认正常服务与非本轮 Docker 目标保留 | 已完成，原有 18 个 Deployment/StatefulSet 全部就绪 |

主要日志：`frontend-unit.log`、`ui-unit-final.log`、`api-client-unit-final.log`、`mock-browser-final.log`、`legacy-browser-final.log`、`layout-check.log`、`backend-integration.log`、`native-pipeline-final.log`、`contracts-final.log`、`build-real-final.log`、`bundle-final.log`、`static-final.log`；清理核验为 `cleanup.json`。本轮没有执行整个历史 M7 部署用例集，三信号真实页面链路由新增测试覆盖。

真实测试使用独立 Namespace 承载 PostgreSQL 18.6 与 ClickHouse 26.3；Kafka、Redis、Collector 使用测试工具创建并回收的独立 Docker 资源，HTTP/查询与浏览器执行当前源码。数据由实际 Go SDK 产生并穿过真实采集、认证、队列与写入链路，不直接插入结果冒充采集。

临时 Namespace `argus-plan10-aa74e54e` 已核对本轮 owner 标签后删除，无持久卷；25432/29000 端口转发已结束，本轮原生链路容器无残留。原有 `argus-system` 和 Plan9 Docker 服务保留，未修改正式部署或业务数据。

证书签发、Collector 注册与测试会话由夹具准备；本轮不声称完成真实主机安装、正式镜像部署、Query gRPC 网络部署或真实模型验收。目录/类型未知不推断，原生查询仍由服务端引擎校验。更复杂、不能映射为目录条件的补全上下文明确不可用，不丢条件扩大候选。

## 可重复命令

```powershell
pnpm --filter @argus/enterprise typecheck
node scripts/check-i18n-keys.mjs
node scripts/check-web-styles.mjs
pnpm --filter @argus/enterprise exec playwright test e2e/observe.spec.ts e2e/combobox-pagination.spec.ts --workers=2
go test ./internal/observationscope ./internal/dashboard ./internal/telemetry ./internal/transport/httpapi
go run ./cmd/argus-dev contracts lint
pnpm build:real
node scripts/check-bundle-budget.mjs
```

真实采集测试为 `TestNativeObservationQueryPipelineAndPortal`。必须显式设置专用测试数据库、ClickHouse、Collector 二进制以及 `ARGUS_MONITORING_NATIVE_PIPELINE=1`、`ARGUS_OBSERVE_PIPELINE=1`、`ARGUS_OBSERVE_BROWSER=1`；不得使用正式数据库运行该测试。
