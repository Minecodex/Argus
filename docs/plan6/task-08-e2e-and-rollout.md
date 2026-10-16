# P6-T08：全链路 E2E 与部署

状态：completed。前件：T01–T07。

## 交付

- [x] 单元/契约/真实 PostgreSQL/Query 与适配器测试；必要重现故障形成稳定回归。
- [x] 企业门户 Playwright 真实模式，覆盖告警中心六 Tab、已有会话工作台的原生工具/确认，以及组织与权限的告警权限/审计；验证完整规则/告警/通知/处理/恢复/历史流程。
- [x] 在普通会话中验证模型自主工具查询、详情和调用，不预选告警业务工具；区分 Replay、真实模型、fixture 通知与真实渠道结果。
- [x] Worker 两副本、租约接管、陈旧 fence、重启/Redis 清空/Query 失败、通知超时/429/Unknown、状态恢复。
- [x] Docker 构建、嵌入模板/Schema、Helm 配置、Worker 配额/并发与必要网络策略。
- [x] 在临时 Kubernetes Namespace 部署完整链路，真实摄入指标改变触发与恢复；关联数据库事实和发送接收证据。
- [x] 测试前记录正式部署、PVC/Secret/资源所有权与副本；必要时暂停 Argus 正式服务降压，结束无条件恢复。
- [x] 按 Namespace/标签/UID 所有权清理测试资源，不操作 Judex 或其他项目；保留正式业务数据。
- [x] 发布手册、管理员设置/测试渠道指南、最终证据索引与剩余门禁。

## 验收

[验收矩阵](./acceptance-matrix.md) 全部 required 条目有实际证据。构建/测试通过与发布完成分别记录；只有用户授权的实际发布才执行正式 rollout。清理后临时 Namespace 为零，原服务副本/健康与关键数据核对通过。

涉及：D01–D08 的整体交付，不以本次 demo 验证代替。

## 本轮完成证据（2026-10-11）

源码指纹 `78eb7ef13d9b2c51d7286592160ce696a9ca6c7961e0c22d9a3fb7288805ca10`，完整运行 `plan6-20261011-w45`；正式构建、同组镜像升级与所有权清理已通过。

- [final-plan6-acceptance.json](../../artifacts/plan6-continuation-20261009-w/final-plan6-acceptance.json)
- [complete-required-acceptance.json](../../artifacts/plan6-continuation-20261009-w/complete-required-acceptance.json)
- [serial-gates-final.json](../../artifacts/plan6-continuation-20261009-w/serial-gates-final.json)
- [final-owned-cleanup-verification.json](../../artifacts/plan6-continuation-20261009-w/final-owned-cleanup-verification.json)
- [w45-process-result.json](../../artifacts/plan6-continuation-20261009-w/w45-process-result.json)
- [result.json](../../artifacts/plan6-e2e/plan6-20261011-w45/result.json)
- [accepted-images-w45.json](../../artifacts/plan6-continuation-20261009-w/accepted-images-w45.json)
- [formal-upgrade-results.json](../../artifacts/plan6-continuation-20261009-w/formal-upgrade-results.json)
- [formal-preservation-verification.json](../../artifacts/plan6-continuation-20261009-w/formal-preservation-verification.json)
- [formal-runtime-image-verification.json](../../artifacts/plan6-continuation-20261009-w/formal-runtime-image-verification.json)
- [formal-tls-health-verification.json](../../artifacts/plan6-continuation-20261009-w/formal-tls-health-verification.json)
- [report.json](../../artifacts/plan6-continuation-20261009-w/formal-portal-stable-v2/report.json)
- [fixture-postgres-backups.json](../../artifacts/plan6-continuation-20261009-w/fixture-postgres-backups.json)

完整业务与真实模型在同组镜像的隔离环境验收；正式环境保留既有数据和配置，并完成页面、权限入口、迁移、运行digest与恢复核对。外部机器人群和真实邮箱独立待配置；本轮真实HTTP/SMTP接收器已通过。

Docker Desktop重启由用户另行明确授权，重启后Argus/Judex UID、原副本和PVC保持。本地既有NetworkPolicy/外部Egress及共享Sandbox限制保留原含义，不将配置存在或其他计划的可选测试记为其执行验收通过。

## 2026-10-11 再复核补项关闭

P6-UI-R01采用租户可配置默认渠道解决，不写入固定Demo ID。新增24组新建/预览对照，完整228组视觉通过；新默认目标、取消保留、缓存刷新竞态、已有策略目标及确认展示均有回归。r02同一冻结源码完整all退出0，GLM10+4、真实浏览器9/9与正式revision9发布、UID/数据保留、72+3正式页面、恢复及所有权清理全部通过。

[本次最终验收](../../artifacts/plan6-review-20261011/final-plan6-acceptance.json) · [视觉对照](../../artifacts/plan6-review-20261011/visual/r01-complete-03/index.html) · [正式页面报告](../../artifacts/plan6-review-20261011/formal-portal-r02/report.json)
