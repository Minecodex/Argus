# ADR: 单进程 Telemetry Query Engine

## 状态

已接受，适用于 M10 Query 重构。

## 决策

遥测查询进程只保留一个进程边界，但在进程内运行三个独立语义引擎：Prometheus PromQL Engine、Argus KQL Engine 和 SkyWalking GraphQL Engine。三种语言不再强行转换为共享语义 AST。

PromQL 通过 Prometheus `promql.Engine` 和 `storage.Queryable` 执行；ClickHouse 只实现租户感知的 `Queryable/Querier/SeriesSet/Iterator` 适配层。Thanos 仅提供执行边界和查询统计参考，不引入 StoreAPI、Sidecar、Store Gateway、Replica Dedup、Query Frontend、Downsampling 或分布式 Fanout。

KQL 只允许固定字段、布尔条件、范围比较和受控 Pipeline。SkyWalking GraphQL 使用固定只读 SDL `internal/telemetry/queryengine/skywalking/schema/trace.graphql`，由 `graph-gophers/graphql-go` 在进程初始化时构建 Schema；请求只解析 Query Document。它允许有界展开的命名/内联 Fragment，拒绝 Mutation、Subscription、introspection、循环或未定义 Fragment，并在展开后限制深度、字段数、分页和关系扩展。

PlanV2 补充：KQL 阶段按 parse → where/unwrap → stats → sort → limit 验证；不能无损支持的顺序直接拒绝，不通过移动 limit/过滤来近似执行。字符串内的管道符和转义由词法分析识别。stats 支持 count/sum/avg/min/max/p95、单字段分组及 bin(timestamp, interval) 时间分桶。结果区分 log_entries、table、timeseries；没有观测值的时间桶暂不补零。显式 limit 属于查询定义，系统预算则读取一个额外结果检测截断并报告 partial。p95 是实际所选日志数值字段的样本分位数，不推断全量请求时延。

所有引擎共享 Scope、Budget、超时、并发、审计和结果投影，但不能绕过统一权限流程。ClickHouse 表名只能由可信 Enterprise UUID 通过 `TenantTableRouter` 生成，查询文本不得影响表名、字段名或 SQL 标识符。

PlanV2 将读取权限收敛为 Dashboard/Host/Cluster 对象能力和数据授权；Signal 仅路由引擎，旧三信号及敏感字段权限已移除。普通日志/Trace 细节保留，凭证屏蔽统一适用于所有主体；内部查询 RPC v4 及目录游标绑定主体身份和投影版本。见 [对象查询授权](../planv2/object-query-authorization.md)。

PlanV2 的 Scope 增加冻结的 source_keys（来源 UUID:配置版本），并纳入范围 Hash；来源筛选在聚合之前应用于三个 Engine 和 Trace 成员子查询。Catalog 是同一 Query 进程上的只读发现接口，不是第四种查询引擎。来源由控制面登记、Collector 按 Receiver 标记、Ingest 按认证身份校验，不从 SDK 自报名称推断厂商身份。

Trace 摘要与父子关系以 Span 事实为依据，在只读查询进程内组装，不再由单个 Kafka 批次生成最终摘要。先按资源、来源安装身份、Trace ID、Span ID 去重，再聚合摘要并连接父子边；同一来源的配置版本在已冻结范围内可共同参与，重装来源保持分离。列表返回 sourceId / resourceId；Trace ID 存在歧义时，详情必须显式指定这两个身份，不任意挑选或合并。

详情保留 Span 事件、Links、资源属性、scope、状态信息，父子边显示缺失父节点与循环。`observed_consistent` 只表示已接收样本的父子结构一致，不证明全量采集；`missing_spans` / `invalid_graph` 明确区分结构缺失和冲突。关系展开达到预算时返回错误，不静默截断后假装完整。Q31 由 Dashboard 已发布下钻执行单独的锚点验证和当前对象授权扩展；`queryTraceGraph` 仅在传入范围内连接唯一、同来源类型的父子关系，不自行获得范围外权限。详见 [下钻执行契约](../planv2/drilldown-runtime.md)。

PlanV2 在同一固定 GraphQL SDL 中增加服务、实例、接口、RED 与拓扑查询。统计基于去重的 SERVER / CONSUMER 入口 Span；显式返回已接收样本口径、单位、分位数方法及缺失字段状态，不能推算全量。拓扑只连接当前授权和来源范围内的明确父子身份，歧义连接不猜测。详细契约见 [APM 查询与发布](../planv2/apm-query-contract.md)。

## 租户存储

查询和 Writer 使用以下物理表名后缀：

```text
metric_series_<enterprise_uuid_hex>
metric_samples_<enterprise_uuid_hex>
logs_<enterprise_uuid_hex>
traces_<enterprise_uuid_hex>
trace_summary_<enterprise_uuid_hex>
trace_span_edges_<enterprise_uuid_hex>
```

`TenantSchemaManager` 负责创建、验证和删除租户表。企业创建/启用事务提交后，`argus-server` 通过内部 mTLS RPC 同步请求 Query 创建六表并写入 readiness；企业 disabled 时同步标记 deleting 并删除六表。Query 启动和周期对账只负责异常恢复与自愈。Schema 验证必须核对引擎、列类型、排序键和 TTL，不能只检查表名存在。

Schema v4 增加 source_id、source_revision、source_type 和物化 source_key，Trace 的排序键包含来源维度；旧记录保留为未知来源。升级或验证失败保留数据并记录错误。Metrics 增加可信 argus_resource_id、argus_source_id、argus_source_revision 标签，非空 instrumentation scope 同样进入流身份；显式聚合仍遵循原生 PromQL。Writer 在临时故障期间重试当前 Kafka 记录，不能提交后续 offset 跳过它。

`trace_summary` 与 `trace_span_edges` 暂保留在 Schema v4 的表生命周期中，Writer 已停止写入，Trace Engine 已停止读取；它们不再是查询事实来源。新实现无需对摄入批次进行跨批更新，也不增加存储进程或权限边界。

Query Pod 内的 ClickHouse 连接分为两类：查询 Engine 使用 `argus_telemetry_query` 只读账号；`TenantSchemaManager` 使用 `argus_telemetry_migration` 账号执行受信 DDL。Schema Manager 连接不得传入 PromQL、KQL、GraphQL 执行路径。

## 结果与失败策略

PromQL HTTP/MCP 返回 Prometheus `status/data.resultType/data.result/warnings`，并在 `argus_meta` 携带审计所需执行统计；KQL 返回独立 `argus.kql_result/v1` 日志结果；GraphQL 返回标准 `data/errors` 和 `extensions.argus`。三者不共享外部结果 Envelope。预算、超时、复杂度或权限失败时返回明确错误，不返回伪造的成功 partial 结果。

## 已接入与验证边界

PromQL Storage Adapter 已支持 float、stale marker、经典 Histogram、Summary quantile/count/sum 和 OTLP Exponential Histogram 到 `FloatHistogram` 的转换。HTTP、gRPC、MCP 三个入口显式区分 Instant/Range；HTTP/MCP 会先校验请求资源是否完全落在服务端 Data Scope 内。

PromQL 查询预算包含独立的 `MaxSamples` 与 `MaxSeries`，后者默认 100,000、硬上限 1,000,000，并贯穿所有协议入口和 ClickHouse Storage Adapter。旧统一 Query JSON Schema、统一 HTTP Envelope 和对应生成客户端不再属于契约。

锁定版 ClickHouse 的真实样本读取、Schema 漂移检查和 Prometheus 参考 Engine 差分测试已纳入门禁；Kubernetes E2E 继续作为每次发布的部署验收。

## 未覆盖

Recording/Alert Rule 管理、Thanos 特殊能力、完整 KQL、完整 SkyWalking OAP / Query Protocol 兼容、任意 SQL 和旧查询协议不属于本 ADR 范围；PlanV2 的受控 APM 查询不代表完整厂商后端兼容。
