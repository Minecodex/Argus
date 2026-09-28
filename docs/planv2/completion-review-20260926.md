# PlanV2 完成度复核（2026-09-26）

结论：**尚未全部完成，Task 01、Task 02 均不能关闭。** 本次对照主设计、两个 Task 的退出标准、正式调用接线、测试代码和已保存的验收产物；没有重新部署或重跑全量测试。

后续实施更新：模型 Harness 接入已补；新增工作台与真实 Workspace/PVC 验收在 `p2-closure-20260926-e` 通过 10/10 real 浏览器及文件/恢复检查。实际模型尚未配置运行。最新结果见 [本轮收尾记录](./closure-workbench-files-20260926.md) 和 [更新后的剩余清单](./remaining-work-20260925.md)。下文保留最初复核时的证据和缺口。

## 已取得的证据及其范围

- `p2-selfmonitor-20260926-k` 的真实部署、原生 SDK 接收、查询、浏览器及归属清理已通过；日志仍保留 `8 passed` 和 `E2E phase planv2 passed`。
- 8 个浏览器用例是**2 条流程 × 2 种语言 × 2 种主题**：展示/来源/变量/下钻，以及个人草稿修改/恢复/样本/发布/历史列表。不能解读成 8 类独立业务流程或全量 PlanV2 验收。
- 18 张图是**6 类 APM/Trace 图 × 3 种来源**。前置 M10 确实查询了真实三信号，但专项 Dashboard 没有因此覆盖十一类 Metrics 图型和 Logs 工作台。
- 草稿最初由验收程序通过 real API 创建，浏览器修改的是已有发布仪表盘；不能据此认定从空白页面创建、完整构建器/DSL 编辑及所有布局交互已真实验收。
- 共享 UI 61 项和完整契约检查已有通过产物。与上面一样，这些通过数不代表所有业务退出条件通过。

对应证据：[验收报告](./self-monitoring-native-traces.md)、[real 浏览器用例](../../web/apps/enterprise/e2e/planv2-real.spec.ts)、[自监控夹具](../../internal/app/argusdev/e2e_planv2.go)、[运行日志](../../artifacts/planv2-selfmonitor-run-k.log)。

## 尚未关闭的项目

| 项目 | 现有实现/证据 | 关闭条件 |
| --- | --- | --- |
| 人工编辑与发布边界 | 个人草稿、Revision、PendingAction 和数据库冲突测试存在；基本 real UI 发布已通过 | 从空白页面创建；多人/多窗口、旧预览、重复确认、撤权、非空分组及归档恢复接到正式页面和审计 |
| Metrics / Logs / 自由布局 | 十一类 Metrics 图型、显示配置、KQL、日志探索、网格和编辑器已有代码及分段/mock 验证 | 真实数据下的全部图型/单位/计算、builder/DSL 转换、拖拽缩放和键盘操作；日志上下文及关联 Trace 的页面闭环 |
| 来源、过滤与资源入口 | 来源身份、候选、All 回退、参数依赖与 Host/Cluster 关联已有实现；此次仅验证 Environment 引用者刷新及来源隔离 | 候选分页/失败/消失、局部过滤、跨来源映射、停采/重装、资源快捷入口及双方撤权组合 |
| APM 与链路授权 | 实际服务/实例/接口、RED、拓扑和 Trace 列表；Jaeger 已发布瀑布下钻通过 | 复杂属性/Events/Links、迟到/重复/缺父、关联日志及从 A 展开授权 B/C、展开后撤权的完整 UI 组合 |
| AI 创建发布 | Dashboard 工具注册、严格配置、草稿、预览和隐藏 Commit 已接入正式 Server/Worker | 实际模型生成 builder/DSL、发现真实 Catalog、校验、宿主确认和发布；证明不能绕过确认/版本检查 |
| Chat 文件分析 | 条件继承、后台查询、分片、Hash、Workspace 导入和权限回查已接线 | 从 @/授权列表，经真实查询任务和 PVC，到模型实际 read/grep/bash 并给出有覆盖依据的结论 |
| 跨服务故障与恢复 | 真实数据库/对象存储分段测试、注入的交付失败和进程内 Workspace RPC 测试存在 | 正式 Worker/Workspace/PVC 下验证取消、重启、Redis 清空、损坏、满额、撤权、跨会话、多仪表盘预算和恢复 |
| 性能、体验与最终交付 | 自动刷新、缓存、预算和引擎并发限流已实现；自监控页面四种组合的 axe 检查通过 | 多图/慢查询/多副本规模与取消延迟，必要时补有界并发调度；其余页面键盘/读屏；逐项验收矩阵和主清单关闭 |

原生 SkyWalking gRPC / Jaeger Thrift HTTP 自检已在约定范围通过。Jaeger gRPC 客户端、其他语言 SDK、复杂原生 reference、Windows/arm64 实际运行尚无本轮端到端证据，不能扩写成全厂商兼容已完成；这些扩展也不能全部自动算入首期范围。

## 本次核查进一步确认的工程缺口

1. **实际模型验收不只是缺配置。** [Harness](../../internal/app/argusdev/e2e.go) 当前将 `--real-model-config` 限制为 `p5` suite；PlanV2 场景仅执行自监控和 `planv2-real.spec.ts`。还需复用/扩展现有模型配置与运行机制，新增 PlanV2 创建、取数、文件分析场景；模型端点/模型 ID/密钥配置是另一项运行前提，不能把全部未完成归因于用户未提供配置。
2. **分段文件测试不能代替真实 PVC 闭环。** [查询任务测试](../../internal/dashboard/query_jobs_integration_test.go) 的交付适配器将文件保存在内存 map 并写入真实元数据；[Workspace 导入测试](../../internal/workspace/import_query_test.go) 使用临时目录和 `bufconn` gRPC。它们验证了重要协议和恢复规则，但未覆盖完整部署路径。
3. **18 张图成功不等于性能已过关。** [Runtime](../../internal/dashboard/runtime.go) 仍逐 Panel/Target 执行；现有引擎限流和累计预算不能证明多图刷新延迟达标。是否需要有界并发，应以约定规模和测量结果决定。

因此当前应描述为“核心实现已形成，基础仪表盘/APM 真实闭环通过，组合验收与 AI/Workspace 全链路仍未完成”，不能描述为“全部功能已完成，只差模型密钥”。

## 范围与后续顺序

1. 补人工、Metrics/Logs、过滤、绑定和授权的 real/Kubernetes 组合用例。
2. 补查询任务 → 正式 Workspace/PVC → 离线工具的确定性链路与故障场景。
3. 接入 PlanV2 实际模型验收流程，再验证创建、条件理解、覆盖和分析结论。
4. 完成性能、其余可访问性和逐项清单，再判定 Task 01/02 完成。

Profiling、告警/通知、实时协作、移动端和 K8s 下级对象绑定仍是后续范围。Dashboard 导出按钮或页面 AI 分析区不应新增。长期生产升级也不是本轮临时 Namespace 验收的同义项。
