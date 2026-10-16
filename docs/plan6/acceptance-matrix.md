# 验收矩阵

2026-10-11当前required矩阵完成：最终单轮r02完整all退出0，真实GLM10+4、真实浏览器9/9、视觉228/72基础、同组六镜像正式revision9升级、117资源/17表数据保留、恢复与所有权清理通过。P6-UI-R01已关闭，新增24组新建/预览状态证明默认目标与取消保留一致；T01–T08完成。w45与原204组保留为历史证据。

当前绑定：[最终验收](../../artifacts/plan6-review-20261011/final-plan6-acceptance.json)、[r02完整结果](../../artifacts/plan6-e2e/plan6-20261011-r02/result.json)、[228组视觉](../../artifacts/plan6-review-20261011/visual/r01-complete-03/index.html)。各业务结果绑定同一冻结源码；原型验证另见 [prototype-verification.md](./prototype-verification.md)。

| ID | 场景 | 必要证据 |
| --- | --- | --- |
| P6-A01 / D01 | 真实指标超限，当前列表/详情与数据库实例一致；筛选与合并成员完整 | OTLP/Query + PostgreSQL + real 浏览器 |
| P6-A02 / D02 | 等级变化、恢复、配置变更和屏蔽都有历史；导出符合当前授权 | 数据库快照、时间线、导出文件 |
| P6-A03 / D03 | 四适配器实际请求与通知记录相符；失败、重试、Unknown 和补发可追溯 | 接收器/SMTP、投递事实、浏览器；真实渠道另列 |
| P6-A04 / D04 | 普通会话中模型经 Tool Hub 查询/详情/调用自主创建多规则；宿主确认前无配置变化，确认一次原子提交；取消不生效 | 三个元工具、业务 ToolCall、PendingAction、Execution、策略/规则 |
| P6-A05 / D05 | 普通会话中模型自主查询；规则存在/不存在、健康无告警、活动告警、NoData/Error 和屏蔽分别正确 | Tool Hub 发现/详情/调用、真实模型/查询范围/状态与数据库 |
| P6-A06 / D06 | 会话工具修改阈值、启停、屏蔽/解除均由会话宿主预览确认，过期/冲突/撤权拒绝 | 三个元工具、Execution/授权/版本与实际配置 |
| P6-A07 / D07 | 确认、认领、评论改变处理记录，自动恢复由评估产生 | 浏览器/工具与审计；告警状态独立 |
| P6-A08 / D08 | 组织与权限内配置告警能力并查看范围内告警审计；跨企业/跨范围不泄露，后台账号撤权生效 | 现有 IAM/audit API、两企业、多主体、权限撤销与审计 |
| P6-A09 | 独立/AND/OR、同资源匹配、持续/恢复防抖、数据缺失/超时/NaN | 真实 Query、受控时间槽和状态转换 |
| P6-A10 | 静默/抑制/合并保留历史；解除时不发送旧恢复事件的触发通知 | 事件、降噪决策、Delivery |
| P6-A11 | 重启/两 Worker/Redis 清空/旧 fence 不丢状态、不重复业务提交 | Pod/数据库/任务/投递证据 |
| P6-A12 | 中英 × 明暗 × 三个桌面视口，键盘、空/错/无权限/冲突 | 浏览器结果与截图 |
| P6-A13 | 完整临时 K8s E2E、测试资源清理、正式服务恢复、业务数据保留 | Namespace/UID/PVC/副本/健康前后核对 |
| P6-A14 | 告警中心只有六个业务 Tab；AI 工具属于会话工作台；权限与审计属于组织与权限 | 三入口导航、Tab 归属与真实模式浏览器验证 |

本轮 GLM-5.3 真实模型、四类真实 HTTP/SMTP 协议接收器是必需门禁。外部机器人测试群与真实邮箱按已确认交付方式保留为独立待配置项，不计入本轮协议门禁。原型或 Replay 不替代真实模型结果。

## 最终证据绑定

所有条目状态为passed，当前完整运行目录为 `artifacts/plan6-e2e/plan6-20261011-r02`，发布/视觉/清理目录为 `artifacts/plan6-review-20261011`。汇总：[final-plan6-acceptance.json](../../artifacts/plan6-review-20261011/final-plan6-acceptance.json)。旧w45和此前发布证据保持归档。

| ID | 状态 | 核对证据 |
| --- | --- | --- |
| P6-A01 | passed | plan6-runtime.json、plan6-database-facts.json及九项真实浏览器 |
| P6-A02 | passed | plan6-lifecycle-edges.json、数据库事件/规则快照及真实历史/导出浏览器 |
| P6-A03 | passed | plan6-notification-faults.json、protocol-first/protocol-reconciliation.json及真实详情/补发浏览器 |
| P6-A04 | passed | plan6-real-model.json、business/create/modify/cancel与tools证据、宿主确认浏览器 |
| P6-A05 | passed | plan6-real-model.json四查询状态、read-results和model-state证据 |
| P6-A06 | passed | plan6-real-model.json、stale-previews.json及撤权/过期/冲突浏览器 |
| P6-A07 | passed | plan6-runtime/database-facts及真实认领/评论/恢复/历史浏览器 |
| P6-A08 | passed | evaluator-authorization/dynamic-admission/stale-previews、PG授权审计及正式组织页面 |
| P6-A09 | passed | plan6-evaluation-matrix.json八项与lifecycle-edges七项 |
| P6-A10 | passed | lifecycle-edges、notification-faults与数据库降噪事件 |
| P6-A11 | passed | runtime-faults、notification-faults及PG并发/SQLSTATE回归 |
| P6-A12 | passed | visual/r01-complete-03 228组含新建/预览24组，九项真实浏览器与formal-portal-r02 72+3 |
| P6-A13 | passed | r02-process-result exit0、formal-preservation/runtime/tls/migration、final-owned-cleanup-verification |
| P6-A14 | passed | 九项真实浏览器、228视觉与正式72+3导航/页面 |

完整业务与模型在临时同组镜像环境验证；正式空配置与既有数据保留并验证页面入口、迁移、服务、TLS和digest。外部机器人测试群/真实邮箱另待配置；本地既有NetworkPolicy/Egress/共享Sandbox/外部LB限制与可选测试范围保持独立，不宣称其他计划的验收完成。
