# Namespace 日志来源与查询范围

状态：实施中。此记录补充 T03/T06/T08/T14/T15 的实现边界，不代表完整平台验收完成。

## 选择授权与存储身份

`kubernetes_namespace` 是独立的查询选择范围，固定 Namespace ID、UID、父 Cluster 和版本。它不把 Namespace ID 当 Host ID，也不向用户授予父 Cluster 或应用的访问权。

数据保持原始观测归属：业务日志存 Application/Service 身份，基础设施日志存 Cluster 身份。`observationscope.Resolver.Sources` 在控制面先解析当前授权，再匹配已登记来源的 Namespace UID、Collector 所属 Cluster、来源类型和信号。查询引擎只收到这些来源对应的实际资源身份及精确 `source_id:revision` 集合；内部查询协议拒绝直接把 Namespace 虚拟范围当成存储资源。

普通 Namespace/Cluster 读取不会开放其他应用的日志。业务日志还需要应用授权，或显式的 `kubernetes.operational_logs.read`；后者仅扩大已授权 Cluster/Namespace 内的日志来源，不开放应用详情、Trace 或其他 Namespace。没有受信 Namespace 来源的 SDK 数据不能仅凭 SDK 提供的 Namespace 名称纳入运维范围。

应用筛选与 Namespace 范围取交集。业务来源按原应用筛选；基础设施来源按当前关联筛选，不能因为用户有运维权限就把其他应用的日志加入应用面板。仅有日志来源时，Namespace 选择不会退回整集群指标或 Trace。

## 冻结、候选值与撤权

来源解析同时返回选择范围成员和实际数据资源。创建目录使用前者，查询使用后者；因此不会把内部可查询的应用 ID 当成新授予的应用资源入口。

显式空资源或空来源集合保持为空。查询、计数、聚合和候选值必须在取数前应用精确来源限制。执行保存选择条件、资源/关联版本、来源版本和范围签名；自然到期的角色即使尚未递增主体版本，也必须在返回前重新核验来源。被撤回的来源不能通过旧查询、候选值或缓存继续交付。

Namespace 是选择范围，不是新增的普通仪表盘快捷关联对象；客户端区分查询资源类型与 Host/Cluster/Application/MonitoringTarget 关联类型。Namespace 指标的 UID 归属仍需单独完成，当前没有该范围来源时返回无数据，不能扩大成全 Cluster。

## 当前证据

- [真实 PostgreSQL 来源授权](../../artifacts/plan8-namespace-log-source-authority-verified.log)：普通读取、应用授权、显式运维权限、应用筛选、空集合、角色自然过期、旧 Namespace 退役。
- [真实 PostgreSQL 查询分发](../../artifacts/plan8-namespace-dashboard-query-dispatch-final.log)：Namespace 授权转换为精确应用来源，候选值沿用范围，查询期间角色到期不返回结果；查询后端是受控记录器，不当作 ClickHouse 或平台 E2E。
- [真实 ClickHouse](../../artifacts/plan8-namespace-log-clickhouse-verified.log)：规范化日志经 Writer 写入，跨 Namespace 来源在原始记录、聚合和候选查询前被限制；已识别凭据内容脱敏。来源登记使用受控测试实现，Kafka 网络和部署链路未在此验证。
- [内部协议](../../artifacts/plan8-namespace-query-physical-protocol.log)：Namespace 选择必须先解析为真实存储身份，不能直接进入查询 RPC。
- [原生解析隐私](../../artifacts/plan8-native-filelog-parse-privacy.log)：格式错误的 JSON 保留上报，诊断输出不打印原文。锁定 quiet 解析策略和 info 日志等级；这不替代 Ingest 的统一脱敏。

正式仪表盘的应用/多类型资源选择已采用分页弹框，查询条件、候选值与缓存共同保存 all_authorized/explicit；详情见[T14](./task-14-dashboard-templates.md)。待完成的主要路径包括 Namespace 日志到行/上下文/Workspace 的下钻、APM/Trace 关联、Namespace 指标来源，以及部署后跨入口与真实模型验收。
