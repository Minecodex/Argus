# P6-T02：多规则评估与生命周期

状态：completed。前件：T01 已与 T07 共同固化的授权契约。

## 交付

- [x] 调度固定评估槽，持久领取/租约/fence，支持多 Worker、重启和错过评估的恢复。
- [x] 通过现有授权 Query RPC 执行 PromQL，冻结资源/来源、查询预算与规则版本。
- [x] 独立、AND、OR，按相同资源/来源/维度匹配；不使用不同主机的数值拼接条件。
- [x] 阈值、等级、持续时长、恢复阈值和恢复时长；数值单位、空向量、NaN/Inf、截断/超时语义。
- [x] NoData/Error 与失去授权的评估状态；失败不伪造恢复或正常。
- [x] 状态、现场值、逐规则结果、版本快照、触发/等级变化/恢复事件同事务持久化。
- [x] 规则预览/试算共享实际引擎与验证，但不产生正式通知。

## 验收

使用真实 Metrics 经 OTLP/ClickHouse/Query 进入规则，验证三种模式、同资源匹配、连续成功槽、恢复防抖、Counter reset/rate、无数据和后端失败。重复领取与旧 Worker 返回不重复写事件；重启和 Redis 清空后仍能恢复活动告警及持续时长。

涉及：A02–A09、D01/D02/D05。

## 本轮完成证据（2026-10-11）

源码指纹 `78eb7ef13d9b2c51d7286592160ce696a9ca6c7961e0c22d9a3fb7288805ca10`，完整运行 `plan6-20261011-w45`；正式构建、同组镜像升级与所有权清理已通过。

- [final-plan6-acceptance.json](../../artifacts/plan6-continuation-20261009-w/final-plan6-acceptance.json)
- [complete-required-acceptance.json](../../artifacts/plan6-continuation-20261009-w/complete-required-acceptance.json)
- [serial-gates-final.json](../../artifacts/plan6-continuation-20261009-w/serial-gates-final.json)
- [final-owned-cleanup-verification.json](../../artifacts/plan6-continuation-20261009-w/final-owned-cleanup-verification.json)
- [plan6-evaluation-matrix.json](../../artifacts/plan6-e2e/plan6-20261011-w45/plan6-evaluation-matrix.json)
- [plan6-lifecycle-edges.json](../../artifacts/plan6-e2e/plan6-20261011-w45/plan6-lifecycle-edges.json)
- [plan6-dynamic-admission.json](../../artifacts/plan6-e2e/plan6-20261011-w45/plan6-dynamic-admission.json)

完整业务与真实模型在同组镜像的隔离环境验收；正式环境保留既有数据和配置，并完成页面、权限入口、迁移、运行digest与恢复核对。外部机器人群和真实邮箱独立待配置；本轮真实HTTP/SMTP接收器已通过。

## 2026-10-11 r02当前验收绑定

8项真实评估及资源/来源完整性检查通过。 当前源码指纹`1f6cf72bbe381fa4146365b3e4b277ead5fdd972908553aa230b8e27ed00e0af`；r02单轮完整all退出0，同组六镜像正式revision9发布、数据/UID保留、恢复及所有权清理通过。remaining_gates为空，历史证据保留。

[本次最终验收](../../artifacts/plan6-review-20261011/final-plan6-acceptance.json) · [逐项证据](./progress.json)
