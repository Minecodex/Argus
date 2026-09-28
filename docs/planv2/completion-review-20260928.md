# PlanV2 非模型范围验收结论（2026-09-28）

本页保留非模型收尾阶段的结论。用户随后恢复 GLM 实际模型验收，四项基础场景已通过；当前范围、剩余门禁和清理证据见 [真实模型验收](./real-model-validation-20260928.md)。

按用户最新决定，完成不依赖实际 AI 模型的实现与验证。Task 01 人工仪表盘闭环、Task 02 的 Chat 选择/契约/工具/确认/后台查询与 Workspace 文件能力已取得对应证据。实际模型的生成质量、条件理解、排查策略和结论质量按用户要求暂不执行，不能记为通过。

## 验收矩阵

| 能力 | 实际验证与证据 |
| --- | --- |
| 目录、个人草稿、发布、冲突、历史、归档与资源入口 | 正式页面覆盖双编辑者、旧预览、重复确认、撤权恢复、分组迁移、Host/K8s 导航与对象/数据授权；见此前 [交互收尾](./interaction-boundaries-20260927.md) 和本轮页面结果 |
| 图型、布局与筛选 | 十一类 Metrics、十二列拖拽/缩放/键盘、真实候选分页/依赖/All 回退、局部刷新、跨信号及跨厂商显式映射；同名 SkyWalking/Jaeger 服务保持来源隔离 |
| 来源与 Trace/Logs/APM | 真实 Collector 卸载后历史可查、重装新身份、旧执行冻结来源；真实跨批次/重复/迟到 Span、父节点缺失、Events/Links、授权链路及撤权、Span 日志与前后文；展示已接收样本口径 |
| 查询与预算 | KQL 前后文单次流关联及稳定排序；真实 PostgreSQL/ClickHouse/MinIO 28 项领域/工具（含子项）+4 项遥测回归，无跳过；覆盖兼容性、最新版本、权限、预算持久化与并发、分片及恢复 |
| Chat 与创建协议 | 中英文选择/@、刷新继承、退出/创建模式、键盘与无障碍检查；确定性 Gateway 使用真实 Catalog 创建 builder/DSL 草稿，宿主确认后发布，模型隐藏 Commit 保持拒绝 |
| 文件与后台交付 | 真实三信号查询→Worker→对象存储→Workspace RPC/PVC→read/bash/Python 核对字节/Hash/记录数与来源审计；模型回合结束后，两个已选仪表盘的后台任务完成，默认条件独立解析，共享 Run 预算 |
| 故障与取消 | 6/6：取数中 Worker 崩溃产生新 attempt；部分交付后崩溃保留已封存 attempt；约 1.088 秒取消；对象分片损坏及真实 PVC 满额修复后同 attempt 继续；归档后任务与已交付文件拒绝访问，恢复和清理完成 |
| 容量与恢复 | 64 张小型真实 instant 查询，两个 Query 副本：冷查询 940 ms、缓存 494/469 ms（64/64 命中）、替换副本后 907 ms；仅代表本次数据和查询规模，不是生产吞吐量承诺 |
| 服务资源约束 | Runtime 验收前后同一 Server Pod、零重启，保留 256 MiB 硬上限和 192 MiB Go 软预算 |

## 证据组合与范围

- 页面：`p2-depth-20260927-f` 的 26 项通过，加 `p2-depth-20260927-g` 定向复验的 6/6 通过，合计覆盖 32 个场景。没有声称最后一轮一次重跑全部 32 项。
- Runtime：`p2-depth-20260927-h/result.json` 为 `passed`，明确 `scope=runtime_protocol_faults`、`browser_executed=false`。`planv2-inflight-faults.json` 6/6 通过；`planv2-tool-protocol.json` 验证 builder/DSL 宿主确认及两个后台查询。实际模型 `executed=false`。
- 类型、样式、i18n、相关 Go 包、带 `m4e2e` 的采集/Replay 验证及完整 `contracts check` 通过。前端共享 UI 67、企业门户 153、API Client 97，共 317 项通过。
- 无障碍证据为桌面键盘操作、具名区域/控件及 Axe serious/critical 检查，没有执行指定屏幕阅读器的人工认证。移动端仍在首期之外。
- 部署使用本地 `evaluation` profile。[h 轮部署核验](../../artifacts/planv2-e2e/p2-depth-20260927-h/verify/verify.json) 明确记录 NetworkPolicy 强制隔离未验证、外部 Egress Gateway 缺席及共享普通容器 Sandbox；验收通过只证明上述功能和故障场景，不证明生产环境网络隔离或沙箱隔离已经验收。
- 最终 Harness 退出码 **0**；临时命名空间残留 0、`/readyz=ok`，原有 10 个 PVC 的 UID/Bound 未变，4 个外部 OpenSandbox 对象未变，40 个运行 Pod 全部 Ready。证据：`artifacts/planv2-depth-20260927/run-h.exit`、`cleanup-and-preservation.json`；没有暂停需要恢复的正式工作负载。

本轮修复和失败轮次保留在 [详细记录](./non-model-closure-20260927.md)。运行产物位于 `artifacts/planv2-e2e/p2-depth-20260927-{f,g,h}`，通用回归及清理核对位于 `artifacts/planv2-depth-20260927`。截图：[日志前后文](../../artifacts/planv2-e2e/p2-depth-20260927-g/playwright-planv2/planv2-depth-real-real-tra-e8e2b-and-log-context-zh-CN-light-chromium/log-context-fields.png)、[来源历史](../../artifacts/planv2-e2e/p2-depth-20260927-g/playwright-planv2/planv2-depth-real-real-tra-62c83--and-log-context-en-US-dark-chromium/installation-history.png)。

## 按用户决定暂不执行

实际模型根据自然语言生成配置、理解跨轮条件、泛问/具体问题的查询策略、关联建议及分析结论质量。确定性 Replay 只能证明协议和系统约束，不能替代以上模型验收。Profiling、告警、实时协作、移动端和 K8s 下级对象绑定继续属于后续范围；长期正式部署升级和远端发布未包含在本轮临时环境验收中。

## 本次完成度复核

重新核对主计划、两个 Task、当前相关代码与 f/g/h 原始结果：P2V 共 37 项，31 项已有非模型证据并关闭，6 项实际模型验收按用户要求保留未勾选。本次未发现新增的非模型功能缺口；同步修正设计阶段“全部待实现”等过期状态，并补充部署环境的明确限制。本次为代码和已有证据复核，没有再次运行全套 Kubernetes E2E。
