# P6-T04：通知渠道与可靠投递

状态：completed。前件：T01/T02/T03。

## 交付

- [x] 钉钉、企微、飞书、SMTP 四适配器，独立消息格式、鉴权/签名、业务返回码与测试接口。
- [x] 多渠道配置、Secret 引用、编辑时不回显原值、受控出网与目标验证。
- [x] 多接收目标路由、等级/资源标签匹配、确定优先级与继续匹配语义。
- [x] 模板展示规则、现场值、资源、时间、原因和详情链接；平台支持的 @ 功能。
- [x] 首次/持续/等级变化/恢复发送时机与分组设置。
- [x] Delivery/Attempt、冻结载荷、去重、Outbox、租约/fence、限流退避与重试期限。
- [x] 已受理、失败、Unknown 分别记录；测试/补发经明确预览确认并关联审计。
- [x] 发送前重查静默/恢复/撤权/渠道停用和版本，阻止迟到或失效通知。

## 验收

协议接收器和 SMTP 测试服务验证实际请求、签名、渲染、429/5xx、业务错误、超时、临时/永久失败及重启窗口。外部系统不具备幂等时记录 Unknown，不把发送开始当成功，不承诺 exactly-once。真实四渠道联通使用用户指定测试目标单独验收；本地 fixture 通过不代替真实渠道结果。

涉及：C01–C06、D03/D06。

## 本轮完成证据（2026-10-11）

源码指纹 `78eb7ef13d9b2c51d7286592160ce696a9ca6c7961e0c22d9a3fb7288805ca10`，完整运行 `plan6-20261011-w45`；正式构建、同组镜像升级与所有权清理已通过。

- [final-plan6-acceptance.json](../../artifacts/plan6-continuation-20261009-w/final-plan6-acceptance.json)
- [complete-required-acceptance.json](../../artifacts/plan6-continuation-20261009-w/complete-required-acceptance.json)
- [serial-gates-final.json](../../artifacts/plan6-continuation-20261009-w/serial-gates-final.json)
- [final-owned-cleanup-verification.json](../../artifacts/plan6-continuation-20261009-w/final-owned-cleanup-verification.json)
- [plan6-notification-faults.json](../../artifacts/plan6-e2e/plan6-20261011-w45/plan6-notification-faults.json)
- [plan6-protocol-first-reconciliation.json](../../artifacts/plan6-e2e/plan6-20261011-w45/plan6-protocol-first-reconciliation.json)
- [plan6-protocol-reconciliation.json](../../artifacts/plan6-e2e/plan6-20261011-w45/plan6-protocol-reconciliation.json)

完整业务与真实模型在同组镜像的隔离环境验收；正式环境保留既有数据和配置，并完成页面、权限入口、迁移、运行digest与恢复核对。外部机器人群和真实邮箱独立待配置；本轮真实HTTP/SMTP接收器已通过。

## 2026-10-11 r02当前验收绑定

4协议28批次对账及10故障场景通过；默认渠道仅初始化新建门户草稿。外部机器人群/真实邮箱独立待配置。 当前源码指纹`1f6cf72bbe381fa4146365b3e4b277ead5fdd972908553aa230b8e27ed00e0af`；r02单轮完整all退出0，同组六镜像正式revision9发布、数据/UID保留、恢复及所有权清理通过。remaining_gates为空，历史证据保留。

[本次最终验收](../../artifacts/plan6-review-20261011/final-plan6-acceptance.json) · [逐项证据](./progress.json)
