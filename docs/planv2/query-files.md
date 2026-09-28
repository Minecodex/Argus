# 仪表盘查询任务与结果文件

## 当前边界

已接入已发布展示查询和下钻定义的持久任务、物化分片及 Workspace 导入。这是 Task 01 / Task 02 共用的取数基础，尚不代表 Chat 分析闭环完成。

- 沿用 PostgreSQL、ObjectStore、原生遥测引擎、Worker 和 Workspace；没有新增查询引擎、独立服务或存储系统。
- `dashboard_query` 队列由现有 default / agent Worker 池处理。启用遥测后接入；文件交付使用 Workspace IO，不要求模型参与搬运数据。
- 服务端后台交付允许在所属模型回合成功或失败结束后继续；它仍验证企业、用户、会话、来源和查询任务取消状态。普通模型文件工具仍要求活跃 Run。用户取消/超时不因后台交付身份获得豁免，已交付文件继续按当前来源权限授权。
- Dashboard 页面没有新增导出按钮、导出配置或 AI 分析区。
- Gateway 已注册 Dashboard 查询/进度/取消/恢复工具。结构化 Chat 选择和不可变 Run 上下文限定工具范围，条件经 `context.resolve` 冻结；模型不能传任意替代表达式或确认发布。
- 下钻文件任务与人工交互共用定义、参数、时间裁剪和完整链路授权校验。额外明细只能来自已发布的下钻引用，不能临时补写查询。

## 会话 API

| 方法与路径 | 作用 |
| --- | --- |
| `POST /conversations/{id}/dashboard-queries` | 冻结本次查询并入队；需要 CSRF 和稳定 Idempotency-Key |
| `GET /conversations/{id}/dashboard-queries/{job_id}` | 重新授权后读取进度、清单、路径和已登记的 Workspace 文件引用 |
| `POST /conversations/{id}/dashboard-queries/{job_id}/cancel` | 取消尚未终止的任务；重复取消不重复修改结果 |
| `POST /conversations/{id}/dashboard-queries/{job_id}/resume` | 以 expected_version 恢复失败任务；已物化结果继续交付 |

普通请求包含 `dashboard_id`、可选且须属于当前主体/会话的 `run_id`、以及既有 `ExecutionInput` 的时间/资源/Panel/变量/局部参数。下钻请求另外指定 `drilldown`，此时 `parameters` 必须为空，不得覆盖父结果的条件。拒绝替代表达式、来源 ID、对象存储地址、文件 Hash 和编译结果等字段。适用于企业用户拥有的会话，不接受 ServiceAccount 冒用用户会话。

HTTP 的明确请求是用户选择；模型工具另外验证所选 Dashboard 已存在于用户消息/Run 的权威上下文，不能仅凭模型传入的 ID 调用此领域方法。

Go/TypeScript 契约和 api-client real/mock 适配器已同步。mock 使用标记为 `MOCK_DATA` 的固定数据演示排队、取消及文件状态，文件下钻仅支持原查询不变、无行输入的 inspect；其他定义明确拒绝，不能退回整个仪表盘查询。mock 没有服务端 Run 身份，拒绝携带 run_id 的模拟请求，不能将其当作 Agent 或真实查询验收。

## 文件下钻

同一创建端点支持以下输入；`drilldown_id` 是父版本中的已发布引用，不是客户端查询定义：

```json
{
  "dashboard_id": "<dashboard UUID>",
  "parameters": {},
  "drilldown": {
    "parent_job_id": "<sealed parent job UUID>",
    "panel_id": "traces",
    "drilldown_id": "<published drilldown ID>",
    "values": {"trace_id": "<observed trace ID>", "source_id": "<observed source UUID>", "resource_id": "<observed resource UUID>"},
    "expand_authorized_resources": false
  }
}
```

- 父任务必须属于同一企业、主体、会话和 Dashboard，并已封存结果；可以继续已封存但尚未交付完的结果。读取服务端原始分片核对同一条记录的全部输入，不采用可修改的 Workspace 副本或客户端提供的 Hash。
- 普通文件下钻继承父结果的 Revision、有效变量/局部条件、资源、来源和时间；存在时间桶裁剪时仍与父范围求交。原查询的聚合、显式限制与字段选择保持不变。
- `authorized_trace` 必须显式开启展开；先在原范围核验锚点，再解析当前另有授权的资源与同来源类型，入队时冻结。队列等待期间的新授权不自动扩大范围，撤权使任务或文件访问失败。
- 行证据读取、完整链路锚点、元数据和详情取数共用一次累计预算。核对证据必须读取到 EOF 并验证总 Hash，不能因前半部分匹配就忽略损坏的后半部分。
- 最多连续 16 层。父任务/尝试/执行、引用、目标、选择值、深度和范围策略进入不可变计划及清单。后续 Span 日志、日志前后文等只能沿已发布依赖继续。
- 有 Run 的任务只能在同一 Run 内继续下钻。新的 Chat Run 必须先创建普通查询取得最新发布版，再下钻；解释旧结论仍可读取原文件。无 Run 的明确 HTTP 请求可沿旧文件导航，不能混入新版本定义。
- 持久文件上下文不使用浏览器 15 分钟签名 Token，始终重新检查权限与生命周期。已经入队的任务恢复不重新扩展来源，交付恢复也不重复取数。

## 冻结与取数

入队固定最新 active Revision、绝对时间、对象资源、安装来源及配置代次、候选验证结果、有效变量/局部条件、原始请求条件和剩余预算。相同幂等键重试复用任务；不同请求内容复用同一键返回冲突。

任务从不可变 Revision 恢复定义，复用与人工页面相同的准备/执行链路。编译版本不一致则拒绝继续取数。新发布不会改变已经入队的查询；新的查询请求取得新的 active Revision。

每个取数尝试有独立 ID。保留原有聚合、显式 limit/pageSize、字段选择和 Metrics 步长；清单记录实际步长、查询 Hash、引擎统计与每个 Target 状态。预算截断或失败标为部分结果，不冒充完整数据。已观察样本的定义与 APM 接收样本口径保持不变，不估算全量。

物化指一次执行尝试的查询结果，不宣称 ClickHouse 支持跨图事务快照。不同查询的实际读取时刻可能不同；相同绝对时间窗口也不能消除迟到数据。

## 物化与交付

- 每个有结果的 Target 保存原生结果 JSON；不会把聚合还原成原始点，也不会丢弃嵌套字段。独立 JSON 清单记录 Definition、实际范围、主体/执行时授权版本、请求条件、有效条件、来源、查询/结果类型、数量、完整性及文件校验值。
- 对象存储按企业/会话/任务/尝试/文件分组，以 1 MiB 不可变内容分片存放；数据库只保存任务、尝试和文件清单，不把结果塞入 Tool Result。
- 取数尝试完整封存后才能交付。清单和已封存尝试的关联不能被后续 UPDATE 改写，文件元数据不可修改。
- Workspace 的 source_ref 对应一个不可变取数尝试；各文件另外保存独立 ID、Hash 和 Panel/Target。来源在任何字节进入 Workspace 前登记，避免复制、派生文件或崩溃遗漏元数据绕过撤权。
- 分片逐个读取与校验，流式重组成文件；Workspace 在原子提交前再验证总长度和 Hash。重试核对既有文件内容并补齐未提交的元数据，不覆盖用户已经修改的副本。
- Workspace 文件是可修改副本，不能自证原始性。原始事实由服务端封存清单和 Hash 决定；`complete` 表示交付完成，`analysis_status=not_analyzed` 明确说明尚未分析。

复用现有限额：目录、变量、元数据和展示查询共用扫描、行数、样本数和结果大小预算；分片不扩大查询限额。Workspace 继续执行文件/目录容量与租约限制，空间不足不自动删除旧文件。

## 恢复、取消与权限

状态顺序为 queued → fetching → materialized → delivering → complete / partial；另有 cancelled / failed。

- 取数或物化未封存即中断：旧尝试标为 abandoned，恢复重新取数；旧文件不能混入新尝试，不能取得新的来源授权。
- 已封存后交付失败：重用同一尝试和校验值继续交付，不重新查询。自动重试耗尽后可以带版本恢复；恢复和 Worker 使用一致的锁顺序。
- Worker 的状态提交和文件登记均检查任务租约、owner 和 fence；失去租约的进程不能提交旧结果。取消、会话 Run 取消和撤权检查会终止在途操作。
- 状态读取、恢复、交付及 Workspace 后续访问均检查企业、会话归属、Dashboard 生命周期、当前 Dashboard/Host/Cluster 授权；不新增信号或字段权限。
- 删除会话会取消其查询任务；对象清理等待仍有效的查询 Worker 租约结束。原有持久目录与对象前缀清理规则继续生效。

## 验证与剩余工作

临时 Kubernetes 中的真实 PostgreSQL / ClickHouse / MinIO 已验证分片校验、失败恢复、旧 Worker 拒绝、取消、版本固定/新请求取最新、权限及会话隔离、Worker 队列消费和删除清理边界。交付故障使用注入适配器；Workspace 原子文件 RPC、长度/Hash、重复导入、崩溃补登记、用户改动保护和来源撤权另由真实 PostgreSQL + 本进程 gRPC/文件系统测试验证。

下钻文件链路另已验证 Trace 列表 → 详情 → 授权链路 → Span 日志 → 日志前后文，以及伪造/拼接选择、原文件损坏、参数覆盖、行证据预算、排队后的授权变化、版本继承与新 Run 限制。

后续 `p2-interaction-20260927-c` 已通过正式镜像、真实 Collector/Kafka、Worker、对象存储和 Workspace PVC 的三信号交付，逐文件核对字节、Hash 和记录数，见 [布局与筛选交互收尾](./interaction-boundaries-20260927.md)。[Chat 结构化选择、个人草稿/发布预览及 Gateway 原生工具](./chat-context.md)已接入；[显式条件继承与跨工具累计预算](./chat-conditions-budget.md)已有实现。2026-09-27 用户明确暂不执行依赖实际模型的验收，继续完成确定性协议、界面及故障验证；不得将 Replay 当作真实模型能力证据。最新状态见 [非模型收尾](./non-model-closure-20260927.md)。
