# Collector 空采集与摄入重试边界

状态：内核及真实数据库验证进行中，未替代 T05/T06/T16 的完整验收。

所有固定采集关闭时，Collector 身份、健康和独立采集实例应保留。此前存储和 Bundle 已允许空 Profile，但 API 和领域校验仍要求至少一项，造成断层。现在请求显式接受 `profile_ids: []`，不隐式启用 Host 或 OTLP 来源；预览和提交都要求分发包实际包含 `nop` 能力，旧包不能伪装为控制连接。

来源准入另外区分三种结果：

| 控制事实 | 结果 |
| --- | --- |
| 已登记、当前生效的来源/采集版本 | 继续认证及入队 |
| 已登记、属于当前期望配置，正在 prepared/applying | gRPC Unavailable / HTTP 503，保留重试语义 |
| 来源不存在、信号不符、停采、已失败/替换或归属失效 | 拒绝；不扩大范围 |

基础设施来源需要匹配实际生效配置，关闭之后旧配置不能继续写入。独立采集仍按其自身生效版本和分配代次判断，避免把其他实例的配置变化错误当成自己的迁移。查询历史与新数据准入分开，历史来源没有被清空或改写。

数据库/控制连接错误也返回临时不可用；没有把它们当成永久的身份拒绝。PKI、下游身份、来源和应用发现的读取保留错误类别。HTTP 503 带 Retry-After，诊断不附原始 SQL、SDK Key 或日志正文。正在应用的配置不提前接收数据，只让客户端重试。Ingest 仅通过窄视图读取配置状态，未增加读取 rendered_config 的权限。

- [空 Profile 内核](../../artifacts/plan8-control-only-kernel-unit.log)：显式空集合不引入默认来源，未知配置契约与退役分发状态拒绝。
- [真实 PostgreSQL 摄入准入](../../artifacts/plan8-source-admission-retry-postgres.log)：应用/采集生效窗口、失败与回滚、停采、信号不符、旧版本围栏及真实驱动关闭连接的错误分类。
- [40 项门户回归](../../artifacts/plan8-comprehensive-portal-e2e.log)通过。全量前端单测已通过，初次 i18n 检查发现工作负载类型缺少词条，补齐后[国际化检查](../../artifacts/plan8-final-i18n-check.log)通过。样式检查发现先前授权页面引用未定义 token，已修正并[复验](../../artifacts/plan8-final-style-check.log)。

旧 PlanV2 自监控依赖免 Key 原生协议，不能再作为 Plan8 的真实接入验收；入口会明确提示改用应用 Endpoint/Key。历史定义/查询形状保留读取验证，SkyWalking 不再参与新接入。正式 Plan8 Harness 仍需完成认证应用接入、Kafka/Writer、门户、Agent 和真实模型的统一业务闭环，不能用这次回归替代。
