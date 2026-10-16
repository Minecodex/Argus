# P6-T06：企业门户告警中心

状态：completed。前件：T01 契约，T02–T05 实际能力。

## 交付

- [x] 一个告警中心菜单，六 Tab：当前告警、告警历史、通知记录、告警策略、屏蔽与抑制、通知配置；详情路由、权限可见性、计数、筛选和返回状态一致。
- [x] 当前列表与合并展开、详情趋势/现场值、时间线、确认/认领/评论与快速屏蔽。
- [x] 历史筛选、规则版本快照、导出；通知详情、失败记录与受控补发。
- [x] 多指标规则编辑、三种组合模式、试算/预览、取消/冲突/过期与单次宿主确认。
- [x] 屏蔽和抑制管理，四类渠道/路由/模板/分组参数与测试。
- [x] D04/D05/D06 集成已有会话工作台，由模型通过 Tool Hub 的查询/详情/调用自行使用；复用消息内 Tool Trace、ToolPresentationFrame 与宿主 PendingActionCard。
- [x] D08 放入组织与权限：现有角色矩阵增加告警能力，现有资源授权与审计服务提供范围与告警审计视图。
- [x] `src/i18n/alerts.ts` 模块注册；中英文 × 深浅色，页面级 CSS 与 argus-* 类名，所有设计尺寸/颜色引用 token。
- [x] 正式控件只用 HeroUI v3/@argus/ui；禁止将原型 native 控件代码迁为第二套业务组件库。
- [x] real/mock Adapter 同契约，显式 API mode、失败保留输入、加载/空/无权限/错误和未保存状态。

## 验收

三个桌面视口与四种语言主题组合，长名字/表达式、键盘 Tab/方向键、读屏、对话框焦点与 Escape，真实 API 的创建→触发→通知→确认→静默→恢复→历史闭环。权限撤销立即使页面/工具/导出不可继续读取。

涉及：D01–D08，分布在告警中心、会话工作台和组织与权限三个已有产品入口。

## 本轮完成证据（2026-10-11）

源码指纹 `78eb7ef13d9b2c51d7286592160ce696a9ca6c7961e0c22d9a3fb7288805ca10`，完整运行 `plan6-20261011-w45`；正式构建、同组镜像升级与所有权清理已通过。

- [final-plan6-acceptance.json](../../artifacts/plan6-continuation-20261009-w/final-plan6-acceptance.json)
- [complete-required-acceptance.json](../../artifacts/plan6-continuation-20261009-w/complete-required-acceptance.json)
- [serial-gates-final.json](../../artifacts/plan6-continuation-20261009-w/serial-gates-final.json)
- [final-owned-cleanup-verification.json](../../artifacts/plan6-continuation-20261009-w/final-owned-cleanup-verification.json)
- [frontend-rule-identity-current-results.json](../../artifacts/plan6-continuation-20261009-w/frontend-rule-identity-current-results.json)
- [summary.json](../../artifacts/plan6-continuation-20261009-w/visual/w-complete-48/summary.json)
- [visual-current-source-binding-w45.json](../../artifacts/plan6-continuation-20261009-w/visual-current-source-binding-w45.json)
- [report.json](../../artifacts/plan6-continuation-20261009-w/formal-portal-stable-v2/report.json)
- [kubernetes-run-w45.log](../../artifacts/plan6-continuation-20261009-w/kubernetes-run-w45.log)

完整业务与真实模型在同组镜像的隔离环境验收；正式环境保留既有数据和配置，并完成页面、权限入口、迁移、运行digest与恢复核对。外部机器人群和真实邮箱独立待配置；本轮真实HTTP/SMTP接收器已通过。

## 2026-10-11 再复核补项关闭

P6-UI-R01采用租户可配置默认渠道解决，不写入固定Demo ID。新增24组新建/预览对照，完整228组视觉通过；新默认目标、取消保留、缓存刷新竞态、已有策略目标及确认展示均有回归。r02同一冻结源码完整all退出0，GLM10+4、真实浏览器9/9与正式revision9发布、UID/数据保留、72+3正式页面、恢复及所有权清理全部通过。

[本次最终验收](../../artifacts/plan6-review-20261011/final-plan6-acceptance.json) · [视觉对照](../../artifacts/plan6-review-20261011/visual/r01-complete-03/index.html) · [正式页面报告](../../artifacts/plan6-review-20261011/formal-portal-r02/report.json)
