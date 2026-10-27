# 范围证明下的聚合与准入诊断

2026-10-22，r53 局部验收完成，原部署与存储身份恢复40.51秒通过，server1/1、本轮Namespace清零。PostgreSQL17.11正式通过真实创建、查询、默认面板、目标授权/撤权和独立启停。原活跃授权窗口的问题已通过正式路径。

HTTP探测目标已实际启动且有真实指标；默认视图验证因聚合输出没有目标/实例标签失败。默认HTTP成功数量使用sum、TLS期限摘要使用min，PromQL的合法聚合会移除标签，不能仅凭缺标签认定已跨范围，也不能放宽原始序列验证。

现补两层验收：先核对实际执行的scope_signature、唯一显式目标资源，以及每个面板的source ID/revision与本轮实际数据库登记的collector/collection/target来源集合一致；随后对数据仍严格核对身份标签。仅在固定内置模板对应panel/target的PromQL AST确有AggregateExpr、且前述范围证明全部通过时，允许无身份标签的聚合输出。原始指标无标签、外来标签、无签名、外来资源/来源仍拒绝。没有改模板或制造零值来迎合测试。

失败面板完整执行结果会脱敏保存在本轮artifact，避免只留泛化错误。固定表达式与范围证明、错误来源、错误目标、原始丢标签和合法聚合的回归已通过；五类探测的正式全链路还须复验，不据此先报通过。

安全OTLP诊断补齐transport identity、forwarded identity、source、Namespace UID/object、application identity六类固定阶段。只输出固定安全码，保留原始错误的可重试/永久拒绝语义，不打印私密输入、原始SQL、属性、对象名称和遥测请求体。下一轮K8s定向故障将按实际原因处理，不先放宽UID归属或删除队列断言。

证据：`artifacts/plan8-calico-focused-20261022-r53/`、`plan8-r54-default-aggregate-proof.log`、`plan8-r54-admission-diagnostics.log`。Oracle长密码组合的用户范围决定仍pending，其他范围继续推进。完整73组/76项/支持矩阵和发布仍未完成。
