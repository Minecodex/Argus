# P6-T01：领域、存储与接口契约

状态：completed。前件：产品/原型和架构契约评审。

## 交付

- [x] 固化策略/规则/版本、实例/episode/事件、处理记录、Silence/Inhibition、Channel/Route/Group/Delivery/Attempt。
- [x] 区分生命周期、评估健康、处理状态与通知受限；定义稳定指纹、配置变化、资源删除与来源变化语义。
- [x] PostgreSQL migrations、sqlc 查询、事务边界、索引、乐观版本、Outbox/Lease/Fence 与企业范围。
- [x] OpenAPI/JSON Schema、错误码、分页游标、时间范围、完整性与关联引用；生成 API Client。
- [x] 固化后台评估主体、当前授权校验、聚合范围和功能权限注册表，与 T07 共同确认。
- [x] 原子创建策略+N 规则，错误不产生部分策略；更新/启停/删除支持幂等和版本冲突。

## 验收

契约/迁移/仓储测试覆盖跨企业、复合资源范围、版本冲突、重复写、事务回滚和查询完整性。真实 PostgreSQL 验证索引和恢复；配置和规则快照可从数据库完整恢复。禁止仅有 HTTP 路由或内存仓储作为完成证据。

涉及：D01–D08 共用基础。

## 本轮完成证据（2026-10-11）

源码指纹 `78eb7ef13d9b2c51d7286592160ce696a9ca6c7961e0c22d9a3fb7288805ca10`，完整运行 `plan6-20261011-w45`；正式构建、同组镜像升级与所有权清理已通过。

- [final-plan6-acceptance.json](../../artifacts/plan6-continuation-20261009-w/final-plan6-acceptance.json)
- [complete-required-acceptance.json](../../artifacts/plan6-continuation-20261009-w/complete-required-acceptance.json)
- [serial-gates-final.json](../../artifacts/plan6-continuation-20261009-w/serial-gates-final.json)
- [final-owned-cleanup-verification.json](../../artifacts/plan6-continuation-20261009-w/final-owned-cleanup-verification.json)
- [go-final-description-guard-w43-result.json](../../artifacts/plan6-continuation-20261009-w/go-final-description-guard-w43-result.json)
- [go-final-description-guard-w43-coverage.json](../../artifacts/plan6-continuation-20261009-w/go-final-description-guard-w43-coverage.json)
- [contracts-rule-identity-current-result.json](../../artifacts/plan6-continuation-20261009-w/contracts-rule-identity-current-result.json)
- [formal-migration-verification.json](../../artifacts/plan6-continuation-20261009-w/formal-migration-verification.json)

完整业务与真实模型在同组镜像的隔离环境验收；正式环境保留既有数据和配置，并完成页面、权限入口、迁移、运行digest与恢复核对。外部机器人群和真实邮箱独立待配置；本轮真实HTTP/SMTP接收器已通过。

## 2026-10-11 r02当前验收绑定

契约与完整Go/独立PG通过，新增渠道默认属性及确认前不变/确认后保存/已有策略不变回归通过。 当前源码指纹`1f6cf72bbe381fa4146365b3e4b277ead5fdd972908553aa230b8e27ed00e0af`；r02单轮完整all退出0，同组六镜像正式revision9发布、数据/UID保留、恢复及所有权清理通过。remaining_gates为空，历史证据保留。

[本次最终验收](../../artifacts/plan6-review-20261011/final-plan6-acceptance.json) · [逐项证据](./progress.json)
