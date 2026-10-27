# 类型化 APM 工具查询与安全反馈

日期：2026-10-21。状态：实现与真实 SDK 组件回归通过，r46 真实模型未通过，预算修正后待重跑，整体未发布。

## 实际问题

r45 已通过隔离安装、主机/文件/Journald、完整 K8s、数据库独立启停和迁移、Redis/Nginx、SDK 三协议，以及三个真实模型场景。应用与共享数据库分析在原十分钟上限内失败。其日志和指标查询成功，但模型反复套用上游 SkyWalking 的 `condition/queryDuration/segmentId` 词汇，十次 Trace 查询失败，只得到通用失败提示。Argus 的 `queryBasicTraces` 支持平铺参数，并不是相同的上游输入合同；工具又没有提供足够的实际词汇和 APM 构建入口。

失败证据保留在 `artifacts/plan8-calico-completion-20261021-r45` 和 `artifacts/plan8-r45-trace-tool-errors.json`。独立 HTTP APM 基线查询成功，不能把本次失败归为资源不足或没有接入数据。整轮恢复记录 passed、原 server 1/1、本轮 Namespace 清零。

## 实现

仍使用同一个 `trace/query` 原生工具，输入版本升至 v2，Describe 修订与共享双语分类说明同步更新。新增 `application_view`，支持 services、instances、endpoints、red、topology、traces；直接调用门户使用的版本化 `ApplicationAPMQuery` 构建器，不要求模型猜 GraphQL。

- `resource_ids/from/to` 仍必填，并执行同一个 ScopeResolver、Query Engine、预算、来源及 Workspace 交付流程。
- APM 默认列表上限 100；`limit` 保持统一的总执行行数预算，默认 5000，列表在预算内为统计信息预留一行，不增加预算。服务筛选若提供为 1～200 个 ID，不将空选择扩大成全部。
- RED 时间桶默认 60 秒，可选 60～3600；其他视图拒绝无关的桶参数。
- 专家 `document/operation_name/variables` 模式保留，与类型化模式互斥；Describe 给出实际根和可执行示例，明确上游不适用的参数。
- 文档在执行前按同一真实 Schema 校验。已知输入错误返回现有 `TOOL_INPUT_INVALID/schema_validation` 结构和固定字段路径、受支持视图，不回传原查询、拒绝值、SQL 或未知后端错误。

初次实际组件执行发现 Go 构造的服务变量使用 `[]string`，与 GraphQL 原生 JSON 变量树不一致。现与门户一致使用 `[]any`，时间桶使用 JSON 数值；没有绕过 Schema 或扩大授权。

验收驱动另修正 Trace 输入字段应为 `document`，此前错误地按 `query` 判断。现在识别合法文档及类型化 APM 视图，同时保留显式应用范围、两应用 APM/日志/共享指标、真实 Workspace 及统计结论核验。

## 验证

- `artifacts/plan8-apm-tool-input-tests.log`：六视图与真实 Schema 对齐、互斥模式、空范围/空服务筛选拒绝、安全反馈不含 canary。
- `artifacts/plan8-typed-apm-six-views-native-product.log`：真实 SDK 三信号经过原生 Collector、mTLS、Kafka、Writer、ClickHouse 后，六个类型化视图实际执行；服务样本/错误计数与 HTTP/文档查询一致，跨根拒绝、撤权、Trace 日志关联及 Key 轮换回归通过。使用受控注册，不冒充完整正式接入。
- `artifacts/plan8-typed-apm-regression-r2.log`：Telemetry、Tool Gateway、Agent、Argus Dev 四包回归通过。
- `artifacts/plan8-typed-apm-contracts.log`：隔离生成检查通过。

原生组件通过不能代替 r45 失败的真实模型场景。下一轮仍按原 32 次调用、600 秒上限执行，其他未运行的模型业务、支持组合、全量矩阵及发布门禁继续保留。


## r46 预算冲突与验收隔离修正

r46 再次通过正式主机、日志、K8s、多数据库迁移、Redis/Nginx、SDK，以及 K8s 恢复（9 次调用）、独立暂停（8 次）、Key 交付（6 次）的真实模型。应用分析仍在原 600 秒内未完成；Worker 的五次实际失败均为 `QUERY_BUDGET_EXCEEDED`。本轮未通过，后四场未运行，原环境恢复 passed、server 1/1、自有 Namespace 清零。

实际原生 SDK 验证了服务筛选可省略，并复现显式 `limit=100` 时 APM 列表请求也为 100、统计信息另占一行导致拒绝。修正为在既有总行数预算内构建至多 100 行的页面，例如预算 100 时页面 99；不修改引擎硬限制、扫描预算或真实模型 600 秒/32 次调用门禁。服务 ID 上限同步真实引擎的 200。

- 修正前失败：`artifacts/plan8-typed-apm-explicit-budget-before.log`。
- 修正后实际 SDK 六视图（有/无服务筛选、显式预算 100）、授权、撤权和关联回归通过：`artifacts/plan8-typed-apm-explicit-budget-after.log`（38.658 秒）。
- Telemetry/Agent/Tool Gateway/Argus Dev 回归通过：`artifacts/plan8-apm-budget-model-cleanup-regression.log`。

已知查询预算错误只返回固定的缩小查询/检查预算建议，未知后端文字仍不回传模型。独立模型阶段失败后记录失败并继续其余场景，整体仍返回失败；只取消该驱动刚创建的准确 Run，取消未确认时停止后续执行。超时证据保留 Run 和场景 ID。此编排调整还需要下一轮正式执行，不视为尚未运行场景通过。
