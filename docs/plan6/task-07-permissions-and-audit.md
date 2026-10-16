# P6-T07：权限、隔离与审计

状态：completed。前件：T01（授权契约须共同设计），实施验证依赖 T02–T06。

## 交付

- [x] 固化告警权限并加入现有注册表、默认角色、生成矩阵与升级规则，不使用星号自动授予未来能力。
- [x] 显式 Host/Cluster 范围与后台 ServiceAccount，调用者不能借绑定评估主体扩大范围。
- [x] 范围过滤、对象读权限、跨资源聚合全覆盖和查询/写回/通知前再次授权校验。
- [x] 角色、账号、资源授权、资源删除/企业停用使查询游标、缓存、PendingAction 和评估任务失效。
- [x] 详情/计数/历史/规则样本/导出/通知文本都执行一致投影，不通过存在性和错误泄露对象。
- [x] 创建、修改、启停、删除、屏蔽/解除、处理、测试、补发与授权变更完整审计，关联 origin/ToolCall/Execution。
- [x] Secret 使用现有凭据系统；原值、私有 Token、SMTP 对话和 Webhook 参数敏感部分不进入日志/审计/模型。
- [x] 告警权限在组织与权限的现有角色矩阵配置，告警操作审计在该入口提供筛选视图；使用现有 role/audit API，不维护平行角色、授权或审计表。

## 验收

两个企业、管理员/编辑者/只读者、不同 Host/Cluster 范围、ServiceAccount 撤权、跨资源聚合部分授权、并发确认撤权和通知前撤权均验证。缓存或 Worker 重启不得恢复过期权限。真实数据库审计事实与浏览器/Tool 展示逐项对应。

涉及：D08，所有能力的安全前件。

## 本轮完成证据（2026-10-11）

源码指纹 `78eb7ef13d9b2c51d7286592160ce696a9ca6c7961e0c22d9a3fb7288805ca10`，完整运行 `plan6-20261011-w45`；正式构建、同组镜像升级与所有权清理已通过。

- [final-plan6-acceptance.json](../../artifacts/plan6-continuation-20261009-w/final-plan6-acceptance.json)
- [complete-required-acceptance.json](../../artifacts/plan6-continuation-20261009-w/complete-required-acceptance.json)
- [serial-gates-final.json](../../artifacts/plan6-continuation-20261009-w/serial-gates-final.json)
- [final-owned-cleanup-verification.json](../../artifacts/plan6-continuation-20261009-w/final-owned-cleanup-verification.json)
- [plan6-evaluator-authorization.json](../../artifacts/plan6-e2e/plan6-20261011-w45/plan6-evaluator-authorization.json)
- [plan6-dynamic-admission.json](../../artifacts/plan6-e2e/plan6-20261011-w45/plan6-dynamic-admission.json)
- [plan6-stale-previews.json](../../artifacts/plan6-e2e/plan6-20261011-w45/plan6-stale-previews.json)
- [go-final-description-guard-w43-coverage.json](../../artifacts/plan6-continuation-20261009-w/go-final-description-guard-w43-coverage.json)

完整业务与真实模型在同组镜像的隔离环境验收；正式环境保留既有数据和配置，并完成页面、权限入口、迁移、运行digest与恢复核对。外部机器人群和真实邮箱独立待配置；本轮真实HTTP/SMTP接收器已通过。

## 2026-10-11 r02当前验收绑定

评估账号授权/停用、动态纳入、3旧Preview拒绝、PG/HTTP权限审计与正式组织入口通过。 当前源码指纹`1f6cf72bbe381fa4146365b3e4b277ead5fdd972908553aa230b8e27ed00e0af`；r02单轮完整all退出0，同组六镜像正式revision9发布、数据/UID保留、恢复及所有权清理通过。remaining_gates为空，历史证据保留。

[本次最终验收](../../artifacts/plan6-review-20261011/final-plan6-acceptance.json) · [逐项证据](./progress.json)
