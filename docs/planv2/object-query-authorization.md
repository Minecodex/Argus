# 遥测对象权限与统一数据投影

本页落实 Q20/Q21 的三信号读取边界；权限不再按 Metrics、Logs、Traces 或字段拆分。

## 入口和范围

| 入口 | 功能能力 | 数据范围 |
| --- | --- | --- |
| Host 页面、资源查询 HTTP/原生工具 | host.read | 当前用户或服务账户明确获得的 Host 授权 |
| Kubernetes 资源查询 | kubernetes.read | 当前主体明确获得的 Cluster 授权 |
| Dashboard 页面、已发布查询和文件分析 | telemetry.dashboard.read | 可访问的 Dashboard，再按 Host/Cluster 数据授权裁剪 |
| 创建、编辑、发布 | 对应对象管理能力 | 保持原草稿基线、生命周期和当前授权校验 |

资源查询复用同一个 ResourceQueryActor，检查企业/主体/部门状态、授权版本、读取能力和显式授权并集。持有 host.read 不能读取仅有数据授权但缺少 kubernetes.read 的集群，反之亦然。请求包含未授权资源时整体拒绝，不把剩余子集表现为完整结果；空集合不变为全部。

HTTP 查询支持用户会话以及服务账户 API Key；服务账户仍须通过现有 allowed_tool_ids 白名单。普通 Chat 的三个元工具使用相同校验。查询返回前再次核对权限和对象范围，执行中撤权不返回已取到的数据。

Dashboard 关联仍只是快捷入口，不授予资源访问权。Dashboard 查询不额外要求各资源页面的功能权限，但必须有明确资源数据授权。Collector 管理与用量等独立运维能力保持其原有权限边界。

## 注册表与迁移

- 移除 telemetry.query.metrics、telemetry.query.logs、telemetry.query.traces、telemetry.sensitive_fields.read；不增加另一组等价的信号权限。
- 注册表版本更新为 11。migration 00009 删除旧权限及角色关联，递增受影响角色版本和成员/服务账户授权版本，使旧会话与来源指纹失效。
- 不将旧信号权限自动转换为更宽的 Host/Cluster 读取能力。已有对象读取能力和数据授权不变；只有旧权限的自定义角色须明确配置相应对象读取能力。
- 前端权限矩阵和 mock 内置角色从后端注册表生成，契约测试检查一致性。界面的“全部已知权限”提交当前完整清单，不发送服务端不支持的星号权限，也不自动授予未来新能力。
- Native Manifest 增加 any_required_permissions，表达至少具备 host.read 或 kubernetes.read；发现、描述和调用均检查，不能因模型知道工具名跳过。

## 查询协议、游标及 Workspace

内部 ExecuteQuery RPC 升级为 argus.telemetry_query/v4。TelemetryQueryScope 保留资源和来源版本，删除 allowed_signals 与 sensitive_fields 并保留旧字段号；新增 signal 作为引擎路由信息，以及 subject_id / subject_type。Server、Worker 和 Telemetry Query 须一起升级，旧 Scope 不降级接收。

范围哈希包含企业、主体类型/ID、资源、授权版本、信号路由、来源版本和统一数据投影版本。目录游标也绑定主体与数据投影版本；同授权版本、同资源范围的不同用户仍不能互换游标。Dashboard ID 不能充当 Host/Cluster ID。

普通 Tool Result/Workspace 来源指纹补齐 Dashboard 授权和数据投影版本。Dashboard 编译/文件协议更新为 v3，旧编译器生成的查询文件不能绕过新的数据投影规则；旧文件保留，但需要重新取数生成当前版本来源，不自动删除 Workspace 内容。

## 统一凭证屏蔽

普通日志正文、Trace attributes/resourceAttributes/events/links 保留，所有主体使用同一投影。识别出的凭证键、认证头、令牌赋值和私钥文本遮蔽为 [REDACTED]，没有管理员或角色开关。发生屏蔽时记录 CREDENTIAL_VALUES_REDACTED。

目录不返回凭证字段的候选值或携带原值的续页键，并标记候选不完整；不能将这种受保护状态解释为选值已消失并扩大查询范围。普通字段发现保持可用。

这是统一数据处理规则，不是另一套字段 RBAC，也不代表能够识别任意未标记或编码后的秘密。展示和文件分析须以实际投影及完整性状态为准。

## 验证边界

真实 PostgreSQL 用于 HTTP/工具一致性、主机与集群能力区分、混合/空范围、服务账户白名单、执行中撤权、会话失效及迁移不扩大授权；真实 OTLP 对象写入 ClickHouse 后验证普通日志/Trace 细节保留及凭证屏蔽。目录游标与 RPC 哈希验证主体隔离，角色矩阵有双语浏览器检查。

这些分段证据不能代替真实 Collector/Kafka、模型、Workspace PVC 和正式镜像的全链路验收。完整状态见 [实施记录](./implementation-status.md)。
