# 状态重放幂等与新增模型流程核验

状态：幂等缺陷已由真实 PostgreSQL 复现并修复，相关包回归通过；r35 主机日志恢复动作失效的最终触发原因仍未确证，后续正式运行继续核对。

r35 在文件日志恢复动作 `monitoring.collection.set_enabled` 后台执行阶段返回 ACTION_INVALIDATED，尚未运行新增模型场景。此次基础链路整体失败，记录位于 `artifacts/plan8-deployed-model-flows-r35/`；原环境恢复 passed、server 1/1、自有 Namespace 清零。不能以之前 r33 同流程通过代替本轮结果。

检查中确认 `ApplyCollectorRevisionOutcome` 对完全相同的生效结果再次投影时仍增加 Collector 版本。真实隔离 PostgreSQL 新测试在修复前失败，明确观测到未变化意图的版本和更新时间被改变，证据 `artifacts/plan8-collector-outcome-idempotency-before.log`。

SQL 现仅在状态或生效 revision 真正变化时递增版本与更新时间，重复当前结果仍被识别为当前结果，以便后续安全对账正常进行。企业/Collector/期望 revision 约束保持，旧 revision 和已生效后的迟到失败仍拒绝，不降低并发控制或授权检查。sqlc 按标准流程生成，未修改历史数据或引入新迁移。

`plan8-collector-outcome-idempotency-after.log` 的 Collector 状态、Connector、Telemetry、Direct Executor 回归通过；资源、监控、Action 和驱动补充回归也通过。第一次补充回归发现错误包装改变了“快照不可用”的既有内部语义，已恢复原行为，未修改测试预期，结果为 `plan8-idempotent-outcome-action-regression-r2.log`。

内部失效诊断现只保留有限的引用类别，例如 collector/runtime/collection，不记录配置、连接地址、凭据引用或摘要。公开错误合同不变。这个幂等问题已修复，但不能仅据此断言它就是 r35 的失效根因；下一轮仍须依据实际版本和执行证据判断。

隔离集群依赖的首次下载前件已补齐：9 个固定镜像在两个自有 Kind 节点共 18 项内容检查全部 complete、已解包，见 `artifacts/plan8-calico-dependency-ready-check.json`。Redis 初次认证 EOF 与失败缓存导出/多架构导入保留各自失败记录，只有后续同版本实际内容检查计为前件完成。它不等于 NetworkPolicy 或业务阶段通过。
