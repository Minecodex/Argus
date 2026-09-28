# 发布、权限与图型验收（2026-09-27）

本轮承接完成度复核，补齐不同账号发布竞争、旧预览、撤权恢复、分组迁移、十一类 Metrics 图型和资源快捷入口的正式部署验收。整个 PlanV2 仍未全部完成。

最终验收：`p2-boundaries-20260927-c` 的 **15/15 real Chromium 用例及文件/PVC 专项通过，Harness 退出码 0**。临时 Namespace 零残留；原有 10 个 PVC 的 UID 保持且均 Bound，40 个 Running Pod 均 Ready，Kubernetes `/readyz` 为 ok；外部 OpenSandbox 控制器及三份 CRD 的 UID/Spec/标签/Helm 所有者保持。

- [完整运行日志](../../artifacts/planv2-boundaries-run-c.log)
- [后端回归](../../artifacts/planv2-boundaries-unit.log)、[企业前端 146 项](../../artifacts/planv2-boundaries-frontend-tests.log)、[Mock 30 项](../../artifacts/planv2-boundaries-mock-tests.log)
- [文件交付与恢复](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/planv2-files.json)、[实际文件 Hash/记录数](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/planv2-file-proof.json)
- [实际模型未配置状态](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/planv2-real-model-status.json)
- [最终清理与原有资源核对](../../artifacts/planv2-boundaries-20260927/cleanup-c.json)、[修改源文件行数检查](../../artifacts/planv2-boundaries-20260927/source-lines.json)

本轮关闭项：

- [x] 普通编辑者无需读取全企业角色目录即可使用自己的有效权限，角色目录仍返回 403。
- [x] 不同编辑者独立草稿、发布基线冲突及差异整理、旧预览与重复确认。
- [x] 撤权使旧会话失效，MFA 重登后检查当次授权；恢复后保留草稿，旧预览仍被拒绝，重新预览才能发布。
- [x] 非空分组迁出后归档/恢复及历史保留。
- [x] 十一类 Metrics 的真实数据、单位与计算，两组语言/主题及无障碍检查。
- [x] Host/K8s 快捷绑定、预选、解绑与查询/授权独立。
- [x] 原有自监控、三信号工作台及真实文件/PVC 场景回归。
- [x] 临时资源清理、原有 PVC/服务健康和共享 OpenSandbox 保留核对。
- [ ] 实际模型及其他剩余组合验收；本次没有模型配置。

## 发布冲突修复

原有 Dashboard 领域能检测 Revision/草稿版本变化，但共用 PendingAction 层将领域冲突折叠为通用失效错误。先发布者更新 R1 后，后发布者的确认会被拒绝，却不能进入编辑页已有的差异整理界面。

现在只允许注册的 `DASHBOARD_VERSION_CONFLICT` 穿过动作重校验、异步 Worker 和 HTTP 错误映射；确认接口仅向动作创建者返回该领域错误，其他主体仍收到通用拒绝。原有对象权限、动作创建者、授权版本和确认门禁保留。未新增发布接口或绕过预览确认。

回归覆盖错误代码白名单、错误文本不能伪造公开代码、创建者隔离、异步错误传播，以及两条 HTTP 映射。数据库依赖用例在普通 Go 执行中可能跳过，真实页面结果另行记录。

## 普通编辑者的前端权限修复

第二轮真实账号暴露另一处共用问题：`useMyPermissions` 通过全企业角色/绑定列表重新计算自己的权限，而这些列表需要 `role.read`。Resource Admin 有 Dashboard 读写权限但没有 `role.read`，因此被前端误判为完全没有权限；此前使用超级管理员的页面验收未暴露此问题。

现在前端使用已有登录/会话 API 返回的有效权限，按企业、用户和会话隔离缓存，页面聚焦和组织权限变更后重新获取。角色名称列表只在有 `role.read` 时读取；Chat 权限摘要可直接展示当前权限数，不因无权读取角色目录而声称未分配角色。Mock 会话也复用统一的有效角色计算，排除未生效、过期或禁用的角色/绑定。后端接口仍逐次执行功能权限与对象授权校验。

定向 Hook 回归验证无需角色目录读取、撤权刷新和换账号隔离；Mock 回归验证过期/未来绑定不进入会话权限。没有给普通编辑者额外授予 `role.read`，也没有放宽服务端接口。

## 新增正式验收

- 使用公开用户创建、临时密码换密、MFA 和对象授权接口准备第二位编辑者；两人各有独立草稿。
- 仪表盘可读而资源未授权时拒绝执行；单独授权资源后可查询。两个编辑者从相同发布版开始，后发布者确认旧预览必须看到差异，整理后再发布。
- 草稿保存使旧预览失效；重复确认不创建额外版本；撤权期间不能读取草稿/执行/发布，恢复授权后草稿保留但旧预览仍失效。
- 非空分组归档被拒绝；通过正式发布迁出仪表盘后可归档和恢复，历史保留。
- 真实 OTLP Gauge 与经典 Histogram 经受管 Collector、Kafka、ClickHouse 和已发布 PromQL，覆盖十一类图型。核对 Gauge=7、比例=0.7 对应 70.0%、累积桶 2/5/6 对应区间计数 2/3/1；中文浅色与英文深色分别截图并检查 serious/critical 无障碍问题。
- Host/K8s 页面配置关联、确认后跳转并预选资源、解除关联；主机范围不能读到仅属于集群的标记指标，解除关联不撤销已有数据权限。

测试继续运行既有三来源自监控、三信号人工创建、个人草稿和真实文件/PVC 专项。实际模型仍需要显式配置；没有配置时不将 Replay 或真实文件链路称为实际模型效果通过。

## 执行状态

首轮 `p2-boundaries-20260927-a` 页面 12/15 通过：非空分组迁移/归档恢复、Host/K8s 快捷入口及原有十个页面用例通过。三个失败为测试自身问题，均已修正并等待新环境重跑：

- 双编辑者用例在登录后的页面导航完成前执行浏览器 fetch，执行上下文被销毁；补充进入仪表盘及标题可见的就绪条件。
- 两个 Metrics 用例在热力图处错误要求单个 Canvas；ECharts 使用三个绘图层，改为逐层检查可见。此前 Gauge、比例及真实直方图的数值/渲染断言通过，直方图截图已人工核对为 2/3/1。

首轮文件专项独立执行并通过，三信号数据、四个 Workspace 文件及校验脚本输出均取得证据。首轮清理无 Namespace 残留，原有 10 个 PVC 保持 UID 且均 Bound，40 个 Running Pod 均 Ready，外部 OpenSandbox 控制器及三份 CRD 的 UID/Spec/标签/Helm 所有者保持。

第二轮 `p2-boundaries-20260927-b` 页面 14/15 通过：两组十一类 Metrics 的实际数值/单位/渲染及 serious/critical 无障碍检查均通过，分组/资源入口和既有页面继续通过。双编辑者用例停在普通编辑者的前端权限误判处，随后按上述方案修复并重新部署。

第二轮文件专项及清理/原有资源保留核对通过。权限修复后，企业前端 146 个单元测试、Mock 30 个测试及前端类型/ESLint 检查通过；第三轮 `p2-boundaries-20260927-c` 的全部页面及文件/PVC 场景通过。双编辑者用例按已有 `AuthorizationVersion` 规则，在授予/撤销/恢复对象权限后重新进行正常 MFA 登录，并检查撤权使旧会话失效。

## 可查看的页面证据

- [不同编辑者的发布冲突与差异](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/playwright-planv2/planv2-collaboration-real--83f90-grants-protect-old-previews-chromium/two-editor-conflict.png)
- [撤权恢复后的正式发布](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/playwright-planv2/planv2-collaboration-real--83f90-grants-protect-old-previews-chromium/two-editors-restored-access.png)

| 图型 | 中文浅色 | 英文深色 |
| --- | --- | --- |
| 趋势图 | [查看](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/playwright-planv2/planv2-gallery-real-eleven-de3bd-stogram-buckets-zh-CN-light-chromium/timeseries.png) | [查看](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/playwright-planv2/planv2-gallery-real-eleven-1a80f-istogram-buckets-en-US-dark-chromium/timeseries.png) |
| 数值 | [查看](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/playwright-planv2/planv2-gallery-real-eleven-de3bd-stogram-buckets-zh-CN-light-chromium/stat.png) | [查看](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/playwright-planv2/planv2-gallery-real-eleven-1a80f-istogram-buckets-en-US-dark-chromium/stat.png) |
| 仪表盘 | [查看](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/playwright-planv2/planv2-gallery-real-eleven-de3bd-stogram-buckets-zh-CN-light-chromium/gauge.png) | [查看](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/playwright-planv2/planv2-gallery-real-eleven-1a80f-istogram-buckets-en-US-dark-chromium/gauge.png) |
| 条形仪表 | [查看](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/playwright-planv2/planv2-gallery-real-eleven-de3bd-stogram-buckets-zh-CN-light-chromium/bar_gauge.png) | [查看](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/playwright-planv2/planv2-gallery-real-eleven-1a80f-istogram-buckets-en-US-dark-chromium/bar_gauge.png) |
| 柱状图 | [查看](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/playwright-planv2/planv2-gallery-real-eleven-de3bd-stogram-buckets-zh-CN-light-chromium/bar.png) | [查看](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/playwright-planv2/planv2-gallery-real-eleven-1a80f-istogram-buckets-en-US-dark-chromium/bar.png) |
| 饼图 | [查看](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/playwright-planv2/planv2-gallery-real-eleven-de3bd-stogram-buckets-zh-CN-light-chromium/pie.png) | [查看](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/playwright-planv2/planv2-gallery-real-eleven-1a80f-istogram-buckets-en-US-dark-chromium/pie.png) |
| 直方图 | [查看](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/playwright-planv2/planv2-gallery-real-eleven-de3bd-stogram-buckets-zh-CN-light-chromium/histogram.png) | [查看](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/playwright-planv2/planv2-gallery-real-eleven-1a80f-istogram-buckets-en-US-dark-chromium/histogram.png) |
| 热力图 | [查看](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/playwright-planv2/planv2-gallery-real-eleven-de3bd-stogram-buckets-zh-CN-light-chromium/heatmap.png) | [查看](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/playwright-planv2/planv2-gallery-real-eleven-1a80f-istogram-buckets-en-US-dark-chromium/heatmap.png) |
| 状态时间线 | [查看](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/playwright-planv2/planv2-gallery-real-eleven-de3bd-stogram-buckets-zh-CN-light-chromium/state_timeline.png) | [查看](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/playwright-planv2/planv2-gallery-real-eleven-1a80f-istogram-buckets-en-US-dark-chromium/state_timeline.png) |
| 散点图 | [查看](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/playwright-planv2/planv2-gallery-real-eleven-de3bd-stogram-buckets-zh-CN-light-chromium/scatter.png) | [查看](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/playwright-planv2/planv2-gallery-real-eleven-1a80f-istogram-buckets-en-US-dark-chromium/scatter.png) |
| 表格 | [查看](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/playwright-planv2/planv2-gallery-real-eleven-de3bd-stogram-buckets-zh-CN-light-chromium/table.png) | [查看](../../artifacts/planv2-e2e/p2-boundaries-20260927-c/playwright-planv2/planv2-gallery-real-eleven-1a80f-istogram-buckets-en-US-dark-chromium/table.png) |

页面使用内部滚动区域，因此逐图截图作为图型证据；目录视口截图不代表一次展示所有十一张图。

## 仍需后续完成

复杂布局/候选回退/跨来源映射、复杂 Trace 与跨资源授权展开、实际模型及 Chat 条件组合、取数/交付中断与损坏/满额、规模性能和完整可访问性矩阵仍未关闭。最新剩余项以 [剩余清单](./remaining-work-20260925.md) 为准。
