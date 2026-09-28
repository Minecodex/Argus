# APM 样本查询与发布契约

本页记录正式后端的统计口径与查询契约。首期 APM 页面、已发布下钻及 Q31 授权完整链路展开已完成对应真实验收，见 [2026-09-28 验收结论](./completion-review-20260928.md)；仅承诺已接收样本和首期能力矩阵，不推算全量或宣称兼容厂商所有功能。

## 查询入口

继续使用现有 Trace GraphQL 引擎和只读 ClickHouse 连接，新增以下查询；不引入 SkyWalking OAP 或第四种查询引擎。

| GraphQL Query | Builder operation / 结果类型 | 数据形状 |
| --- | --- | --- |
| `queryAPMServices` | `apm_services` | 每个资源、来源安装身份、服务的统计 |
| `queryAPMInstances` | `apm_instances` | 继续按真实 `service.instance.id` 区分实例 |
| `queryAPMEndpoints` | `apm_endpoints` | 按 Span 的 operation 名称区分接口 |
| `queryAPMRED` | `apm_red` | 按冻结时间起点和 `bucketSeconds` 分桶 |
| `queryAPMTopology` | `apm_topology` | 来源明确的服务节点及观测到的父子依赖 |

共同筛选包括 `serviceName`、`serviceInstanceName`、`operationName`、`sourceId`、`resourceId`；所有条件均与执行时的对象授权和冻结来源范围求交。`serviceInstanceName` 可匹配实例 ID 或名称，但实例统计只按真实 ID 建立实体，不按显示名称自动合并。空字符串是具体筛选值；缺省/null 才表示没有该条件。

`filters` 提供受控的 Span / 资源属性等值集合或排除集合。Builder 使用 `attributes.<key>` / `resource_attributes.<key>`，支持显式引用共享变量、局部参数、多选和 All。原生 GraphQL 可把 `[String!]` 变量绑定到 `values`，JSON Variables 中的 null 或省略 values 表示 All；当前语法子集不使用查询文本中的 null 字面量。属性筛选在去重后、聚合或拓扑父子匹配前生效，环境等共享条件不会混入其他环境的节点。

APM Builder 的可选 `limit` 是已发布查询的一部分；拓扑的 limit 限制边数。返回 `limited` 区分显式限制。未指定 limit 而超过系统预算时返回预算错误，不返回伪装完整的第一页。指标构建器的 `top_n` 保持其原有含义，不作为 APM limit 使用。

## 唯一统计口径

- 基于已接收、按 Span 身份去重的事实。相同来源的配置版本可参与同一统计，重装和不同来源/资源保持独立，不按服务同名合并。
- `observedSpanCount` 包含收到的各类 Span；`sampleCount` 只包含 OTLP SERVER / CONSUMER 入口 Span。INTERNAL / CLIENT / PRODUCER 不冒充请求样本。
- `basis=received_entry_spans`；错误数来自入口 Span 的 ERROR 状态，`errorRate` 为 0～1 的样本比率。`samplesPerSecond` 为收到的入口样本数除以实际查询区间秒数，不能标为全量请求率。
- 时延单位固定为毫秒；均值和分位数直接来自这些入口 Span 的 duration。服务/实例/接口/RED 使用 TDigest，结果明确返回 `percentileMethod=tdigest`，不基于均值伪造分位数。
- RED 分桶以本次绝对时间起点对齐，最后一个不完整桶使用实际秒数作为速率分母。无观测的桶不自动补零。
- 没有入口样本时，时延、错误率和样本速率返回 null；不返回“0 错误/0 时延”冒充验证正常。缺少服务、实例 ID、operation 或可靠来源时返回能力状态及 coverage。
- OTLP 摄入拒绝无效 Trace/Span/Link ID、缺失或倒置的时间区间，以及超出纳秒存储范围的时间值，避免错误去重和巨大伪时延。

独立的完整请求指标继续通过自己的 Metrics 查询获取和标注来源，不自动替换本页的样本数据。

## 服务拓扑

父子关系只来自相同 Trace ID 和 parent Span ID。先匹配同资源、同安装来源内的父 Span；跨采集实例时，必须是同一已知来源类型、当前授权范围内唯一匹配的父 Span。多个匹配不任意选择，跨厂商不隐式合并。

节点身份包含资源、来源和服务，不按服务名称合并。`serviceName` 用作拓扑中的服务锚点，可显示已授权的直接相邻节点；实例/operation 条件过滤被观察的子 Span。`sourceId` / `resourceId` 仍限定整个查询来源范围。

边的样本统计使用目的端入口 Span，口径为 `received_destination_entry_spans`，不是网络时延或全量调用率；分位数采用明确的 nearest-rank 方法。缺失父节点、歧义父节点、循环、未知来源和缺失服务字段计入 coverage；不会根据同名服务或未接收的数据补造连接。

拓扑只使用本次请求已经授权并选中的资源。Trace 详情的“打开完整链路”是单独发布的标准下钻，可从 A 扩大到另有授权的 B/C，并再次验证当前权限；它不修改 Dashboard 顶部选择，见 [下钻执行契约](./drilldown-runtime.md)。

## 发布与执行

五种图型通过同一 Draft / PendingAction 发布流程。服务端检查结果类型与图型一致，并要求原生 GraphQL 查询保留口径、分位数方法、coverage、来源身份和渲染所需字段。条件 Fragment/Directive 不可让必需字段在运行时消失。

候选、展示查询、APM 聚合和关系加载使用现有累计预算。能力缺失产生明确的 `APM_*` 警告，样本验证状态为 warning；语法、类型和预算仍是硬门禁。数据库连接中断等临时故障按 unavailable 处理，不误判为作者的查询语法错误。

已经通过真实 OTLP Span、PostgreSQL/ClickHouse 的统计/去重/来源/授权专项，以及 Collector → Kafka → ClickHouse → 正式 UI 的三来源 APM 自监控验收，见 [原生 Trace 报告](./self-monitoring-native-traces.md)。复杂 Span/关联日志/授权展开与来源重装的后续组合验证见 [非模型收尾](./non-model-closure-20260927.md)，不以早期局部测试替代这些组合门禁。
