# 应用性能、日志/Trace 入口与精确时间窗口

状态：领域身份、查询入口、共享门户和分层回归已补充；真实 SDK 到部署后 APM、去重策略和完整关联验收仍未完成。

## 应用入口

应用详情加入服务、运行实例、接口、请求/错误/耗时、拓扑、调用链和应用日志视图，复用 `@argus/ui` 图表、表格、瀑布图、弹框、选择器和主题。时间及服务筛选在这些视图间保留，服务使用搜索分页弹框。APM 下钻保留服务身份、来源、操作和窗口；调用链详情继续固定当前应用和已选择的来源，不扩大资源范围。

日志按受信服务 ID 筛选。存在 Trace ID 时进入该应用的 Trace 检索；缺少 ID 时明确是当前服务/时间窗口的近似检索，不能当作精确关联。没有 SDK 样本时不生成性能数值；mock 中新应用的日志和 Trace 也保持为空，不套用主机演示数据。

GraphQL 查询由 Go 引擎中的注册表生成到客户端，七种文档以实际只读 Schema 验证，避免门户另写一套字段。这里复用的是现有 Trace 查询语言；SkyWalking 原生接入协议继续暂缓。

## 稳定身份与入口权限

APM 聚合、RED 曲线和拓扑保留 `argus.service.id`、`service.namespace`，同应用中同名而不同命名空间的服务不能混成一项。服务筛选使用稳定 ID，不依赖显示名；来源仍分别展示，跨来源去重和完整拓扑关联尚需继续实现及验证。

修复直接遥测 API 和原生工具仍限定 Host/K8s 功能权限的遗漏。四类资源的读取能力采用同一清单，应用读者无需额外获得主机权限；当前数据授权、来源和撤权仍由同一 ScopeResolver 检查。ServiceAccount 继续受工具白名单约束，关联不授予资源权限。

- [真实 Writer/ClickHouse 的服务身份、日志过滤及拓扑分离](../../artifacts/plan8-application-service-log-topology-identity.log)。数据由受控写入构造，不冒充真实 SDK 接入。
- [真实 PostgreSQL 的应用独立读取、工具发现和撤权](../../artifacts/plan8-application-query-entry-access-postgres-verified.log)
- [七种门户查询与真实 Schema](../../artifacts/plan8-application-observability-query-schema.log)
- [七个视图、选择器、中英明暗及可访问性](../../artifacts/plan8-application-trace-log-portal.log)，属于 mock 门户。测试等待确认弹框真正关闭，再通过可见控件获得焦点并验证方向键切换；初次直接聚焦隐藏 input 导致的失败保留。

## DateTime64 边界

真实 ClickHouse 26.3.17.110 测试发现，带小数秒的 Trace 范围采用普通 Go `time.Time` 参数时，投影条件为真而 WHERE 返回空。锁定驱动 v2.40.1 默认绑定走秒精度 DateTime；实际改为明确的 DateTime64(9) 边界后查询恢复。相关上游背景见 [ClickHouse 的类型比较讨论](https://github.com/ClickHouse/ClickHouse/issues/50353)，不将历史问题编号当作本次具体根因已被上游确认。

Trace、APM 时间桶、KQL、候选值目录和三信号概览使用 `fromUnixTimestamp64Nano`；PromQL 将毫秒边界明确转换为 DateTime64(9)，避免整数与时间直接比较及秒精度丢失。测试保留纳秒边界，不把时间截整秒来掩盖问题。

- [真实三信号的半开纳秒/毫秒窗口和概览](../../artifacts/plan8-three-signal-subsecond-query.log)
- [实际 ClickHouse 全包回归](../../artifacts/plan8-apm-time-clickhouse-regression-verified.log)
- 初始诊断 `plan8-apm-service-identity-nanos-boundaries.log` 保留。日志上下文测试的直接 INSERT 同样会被默认绑定截整秒，现改成精确时间插入，保留原有行数、流边界及扫描预算要求。

尚需真实模型、跨应用关联、协议重复上报处理、完整服务拓扑、日志上下文/Workspace、SDK 地址与 TLS 暴露以及正式部署后回归；不能将本节当作 T07/T08/T15 全部通过。
