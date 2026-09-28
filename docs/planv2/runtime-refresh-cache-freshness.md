# Runtime 收尾：刷新、缓存与数据时间

2026-09-25 实现记录。对应主设计 §4.2–4.4；不改变 PromQL、Argus KQL、Trace GraphQL、对象授权或 Chat 文件分析边界。

## 正式页面行为

- 页面隐藏时停止自动刷新计时，重新可见后按配置周期继续。在途请求不被自动刷新打断；用户改变时间、资源或过滤条件时取消旧请求，并隔离迟到响应。
- 共享变量按结构化引用和候选依赖做传递刷新，覆盖 Builder 普通过滤/错误分子、DSL 参数绑定、局部候选和已发布明细查询。无关图保留数据、候选及下钻上下文。
- 变量和局部条件修改沿用本次绝对时间范围。连续修改会合并尚未完成的受影响图，不丢弃全量查询期间发生的条件修改。
- 未被任何图引用的变量通过 `DashboardExecutionInput.candidates_only=true` 单独协调候选；空 `panel_ids` 原有“全部统计图”语义不变。文件查询任务拒绝 `candidates_only`。
- 服务端证明选值消失并回退 All 后，继续刷新尚未返回的依赖图。候选查询失败、分页不完整不触发回退。打开中的候选选择器在上下文变化时取消旧页并重新获取。
- 每张图和展开视图提供“数据时间与完整性”详情。使用共享 Dialog，避免长文案压缩图表高度；没有增加导出或 AI 分析入口。

## 结果缓存

正式 Telemetry Query 的 Coordinator 配置进程内缓存：最多 64 MiB、1024 条、30 秒 TTL，按最近使用情况淘汰。进程重启清空；不作为持久化任务或文件恢复的依据。

只有服务端根据已发布 Dashboard/Revision 生成的 64 位十六进制 namespace 可以启用结果复用；草稿样本、Catalog 和未携带发布身份的通用查询不使用该缓存。人工展示、已发布下钻和文件任务执行使用同一入口。

缓存键覆盖企业、主体类型/ID、授权版本、Dashboard/Revision/Panel/Target、完整查询/映射定义、来源绑定及实际来源集合（身份、安装代次、配置和能力版本）、变量/局部值/下钻参数、绝对时间、step、语言/查询模式、编译器版本、统一数据处理版本及本次预算。预算不同不复用；不跨主体复用。

命中前仍执行 Dashboard/Host/K8s 当前对象授权、候选协调、来源冻结和预算分配，返回前再次授权。Query RPC 租户限流、引擎并发闸门及审计仍执行。命中保留原扫描/样本/结果成本，由 Dashboard/Run 账本照常扣减；不会靠重复读取绕过累计预算。

仅保存经过统一脱敏及结果大小校验的成功、非 partial 结果；异常、超时、取消、超大结果不缓存。使用序列化副本隔离调用方修改。无数据结果也受同一 TTL 约束，因此迟到数据最多需要等缓存过期；页面明确标记命中及原查询完成时间。

## 新鲜度口径

| 字段 | 含义 |
| --- | --- |
| `query_completed_at` | 引擎执行及结果处理完成时间；缓存命中保留原值 |
| `cache_hit` | 本次是否复用了查询结果，审计也记录此状态 |
| `latest_sample_at` | 本次查询实际观察到的原始样本/事件时间最大值；缺少证据时不填 |
| `sample_time_basis` | `observed_query_samples` 或 `unknown` |
| `ingestion_status` | 当前为 `unknown`，没有从 Collector 运行状态推断数据完整到达 |

Metrics 在读取实际采样行时记录时间，排除 stale 标记，不使用 PromQL 求值点时间。普通日志/上下文记录实际日志时间；聚合结果没有保留可证明的原始时间时显示未知，绝不使用时间桶代替。Trace 记录 Span 事实时间；列表摘要和 APM 使用原始 `start_time` 的最大值，拓扑/瀑布使用实际读取的 Span。该字段描述查询观察范围，不证明全部来源、所有样本或整个时段已完整摄入。

## 验证与边界

- 刷新 Hook：慢查询、隐藏恢复、传递依赖、未引用变量、All 回退、局部候选保留、并发条件修改、旧响应、版本漂移、历史视图及卸载取消。
- 缓存：主体/企业/授权/来源/发布定义/预算隔离、TTL/容量淘汰、取消、partial/失败不缓存、脱敏、整数序列化、审计和原时间保留。RPC 编解码回归覆盖 namespace、命中状态、样本时间和每次限流。
- 临时 Namespace `argus-p2-runtime-20260925-a`：真实 PostgreSQL、ClickHouse、MinIO；Dashboard 全包重跑通过，三信号真实 OTLP Writer/Query、APM、来源、参数、发布和文件任务回归通过。另验证动态新增来源不命中旧缓存、旧授权请求被拒绝、APM 五类结果携带样本时间。
- 完整 `argus-dev contracts check` 已通过。修复 clean generation 中密码策略包晚于权限注册表生成导致的编译失败，并同步生成产物；仍有 545 条既有 OpenAPI lint 警告。
- 最终 Chromium 回归 12/12 通过，覆盖中英文/深浅色、草稿发布、十一类图型、布局恢复、来源入口、Trace/APM 详情及 Chat 选择；本组使用 mock API。刷新 Hook 9 项、共享组件定向 8 项、API Client 定向回归通过。Enterprise 类型、ESLint、i18n/样式、real 前端构建、包体积及相关后端包检查通过。
- Namespace 和本轮端口转发已清理；原有 9 个 PVC 的 UID/卷绑定保持一致，34 个原就绪 Pod 仍就绪，Kubernetes `/readyz` 为 ok。检查的 376 个变更源码文件均不超过 2000 行。
- 产物位于 `artifacts/planv2-runtime-20260925/`。首次存储回归中候选容器断言过严，修正后全包重跑通过；首次浏览器运行遇到同时重生成契约造成的模块暂时缺失，后续独立重跑。不能把这两次失败标记为通过。

最终证据入口：`dashboard-retest.jsonl`、`cache-apm-integration.jsonl`、`go-related.log`、`contracts-check-final.log`、`browser-accepted.log`、`build-real.log`、`cleanup.json`。最新中文浅色详情和英文深色图表截图分别位于 `browser-accepted/dashboards-display-options-0f887-and-expand-in-Chinese-light-chromium/data-freshness.png` 与 `browser-accepted/dashboards-display-options-bd7cd--and-expand-in-English-dark-chromium/published-display.png`。

本轮的存储测试和 mock 浏览器回归不能替代新版 Dashboard 的 real API/Kubernetes 页面闭环、真实模型创建与文件分析、Workspace PVC 故障组合及多图性能验收。Task 01/02 继续保持进行中。
