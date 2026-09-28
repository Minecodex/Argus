# 布局与筛选交互收尾（2026-09-27）

本轮继续落实剩余工作包中的复杂布局和变量交互。最终 `p2-interaction-20260927-c` 通过 24/24 真实页面以及三信号文件/PVC、文件审计关联，页面/文件检查前后同一 Server Pod、Ready、零重启。复用共享 Grid、ValueSelector、Dashboard Runtime 和既有 Catalog；不改变对象授权、查询语言或发布边界。来源停采/重装及跨厂商映射继续单列，不能用同一 OTLP 来源的跨信号映射代替其验收。

## 修复内容

- 查看页合并局部刷新结果时，原先只按 Panel/Target 重算 `partial`，会丢掉候选不可用或预算不足的事实。现在保留全局与各图局部候选的失败状态，并显示双语警告；无关图的局部失败在其他图刷新后仍保留，实际恢复后才清除。正常候选分页不视为查询失败。
- 局部候选使用当前执行已经解析的资源集合，并按本图适用资源类型收窄；空集合直接返回空候选，不向 Catalog 发送代表“全部授权资源”的空数组。
- 筛选弹窗打开期间，上游条件导致当前有效选值改变时，弹窗同步有效选值，避免提交旧选择；执行协调期间禁止应用，旧请求被取消。
- 共享网格开始拖动时获得键盘焦点，Escape 丢弃本次预览；丢失指针捕获时取消手势，忽略其他指针，未改变布局的指针操作不保存草稿。

## 新增真实验收场景

两组中文浅色/英文深色场景各执行一次，共新增四个用例，套件由 20 个扩为 24 个：

1. 四图连续碰撞下推，指针宽高缩放及最小尺寸，取消不保存，刷新恢复，键盘继续缩放。
2. 经真实 OTLP 接收 250 个 Metrics 序列（blue 230、green 20），验证 Runtime 的 200 个预览值之外仍可精确确认选值存在；Catalog 的 100 个候选页连续读取至第三页。
3. 成员变量只刷新引用图；父变量变化后成员及局部候选级联回退 All，保留无关图和发布默认值，局部操作不刷新其他图，手输文本无结果时保留。
4. 作者将 Metrics 的 pool 明确映射到 Logs severity，验证 blue→INFO、green→ERROR；从 Cluster 切换 Host 后只适用于 Cluster 的 Panel 显示不适用，局部候选不扩大到全部资源。
5. 候选失败警告由浏览器局部改写实际执行响应验证，明确不声称这是 Query 服务真实宕机；解除注入并刷新后警告消失。

## 验证状态

- 企业前端单元测试 148/148；筛选/布局/指针组件定向测试 12/12。
- Enterprise 类型、相关 ESLint、i18n、样式检查通过。
- 新数据与 Dashboard 配置夹具通过 Go 测试；公开 OpenAPI 预检发现 All 默认值序列化为 null 的夹具错误，已改为空数组并通过复验。
- `p2-interaction-20260927-a` 页面 22/24，通过两组新增布局、原有 20 项页面、三信号文件/PVC 及审计核对；文件实际为 3 条日志、1 个 Metrics 序列和 100 条 Trace（显式 limit）。同一 Server Pod 前后 Ready、零重启。驱动因下面两项脚本错误退出 1，整轮不计通过。
- A 轮两组候选用例已通过真实分页、级联和映射断言，但在浏览器局部故障注入步骤使用 `route.fetch` 时发生 TLS 连接错误：Node 请求未沿用 Chromium 的隔离入口域名映射。已改为浏览器执行实际查询，再以该结果注入候选失败状态；测试修正仍需重跑。此错误不归因于查询服务。
- A 轮临时 Namespace 已清理，独立复核 `/readyz=ok`、原有 10 个 PVC 的 UID/Bound 状态及 4 个外部共享对象不变，40 个运行中的 Pod 全部 Ready，见 [清理证明](../../artifacts/planv2-interaction-20260927/cleanup-a.json)。
- `p2-interaction-20260927-b` 在 `argusctl build` 启动临时 Registry 容器时遇到 Docker 容器管理 API 无响应，未进入镜像构建和应用测试，不计通过。已停止该轮进程，按标签/UID 核对后删除其入口 Namespace、ClusterRole/Binding、IngressClass，并以 UID/resourceVersion 前置条件删除所属测试 Lease。
- 用户明确授权后执行正常 Docker Desktop 重启，API 恢复；按归属核对并删除 B 轮临时 Registry。原有 10 个 Kubernetes PVC 的 UID/Bound 状态及 4 个共享对象不变，40 个运行中 Pod 全部 Ready，Agentx web-console 经临时端口转发返回 HTTP 200，转发进程已关闭。见 [B 轮清理/恢复核对](../../artifacts/planv2-interaction-20260927/cleanup-b.json)、[业务入口](../../artifacts/planv2-interaction-20260927/agentx-after-restart.json)。
- 另记录环境差异：API 阻塞期间从磁盘读取到的 15 组 `running=false` 的旧 `judex-test-pg-*` 容器及匿名卷记录，恢复后不再存在；期间也观察到其他 Judex 测试容器创建/销毁，具体清理来源未确认。手动删除命令只针对 B 轮 Argus Registry，不能据此宣称 Docker 层所有旧记录均保持不变。见 [磁盘元数据快照](../../artifacts/planv2-interaction-20260927/docker-before-restart.json)、[记录卷差异](../../artifacts/planv2-interaction-20260927/docker-volume-preservation.json)。
- `p2-interaction-20260927-c` 完整复验通过 24/24 页面，包含修正的故障注入、顶部 All 下局部资源收窄，以及完整桌面截图；三信号文件实际为 3 条日志（1208 字节）、1 个 Metrics 序列（733 字节）、100 条 Trace（34,534 字节，已发布显式 limit），文件 Hash 与审计核对通过。服务端前后 Ready、零重启。实际模型仍是 `not_configured / executed=false`。
- C 轮驱动退出码 0、临时 Namespace 无残留；独立清理复核再次确认 `/readyz=ok`，原有 10 个 PVC 的 UID 不变且 Bound，4 个共享对象不变，40 个运行中 Pod 全部 Ready。该结论针对 Kubernetes 保留范围，不抹去上文记录的 Docker 旧测试资源差异。

## 最终证据

| 范围 | 证据 |
| --- | --- |
| 24 个真实页面 | [Playwright 状态](../../artifacts/planv2-e2e/p2-interaction-20260927-c/playwright-planv2/.last-run.json)、[运行日志](../../artifacts/planv2-interaction-run-c.log) |
| 实际字节和文件恢复 | [文件任务/PVC](../../artifacts/planv2-e2e/p2-interaction-20260927-c/planv2-files.json)、[工具计算的记录数/Hash](../../artifacts/planv2-e2e/p2-interaction-20260927-c/planv2-file-proof.json)、[文件审计](../../artifacts/planv2-e2e/p2-interaction-20260927-c/planv2-audit-files.json) |
| 服务健康 | [检查前](../../artifacts/planv2-e2e/p2-interaction-20260927-c/planv2-server-health-before.json)、[检查后](../../artifacts/planv2-e2e/p2-interaction-20260927-c/planv2-server-health-after.json) |
| 最终清理与退出 | [资源保留复核](../../artifacts/planv2-interaction-20260927/cleanup-c.json)、[驱动退出码](../../artifacts/planv2-interaction-20260927/run-c.exit) |
| 定向回归 | [企业前端 148 项](../../artifacts/planv2-interaction-enterprise-tests.log)、[筛选/布局 10 项](../../artifacts/planv2-interaction-ui-tests.log)、[指针 2 项](../../artifacts/planv2-interaction-grid-tests.log)、[后端包](../../artifacts/planv2-interaction-backend-tests.log)、[公开夹具契约](../../artifacts/planv2-interaction-fixture-tests.log) |
| 静态检查 | [Enterprise 类型](../../artifacts/planv2-interaction-final-types.log)、[共享 UI/Platform 类型](../../artifacts/planv2-interaction-shared-types.log)、[ESLint/i18n/样式](../../artifacts/planv2-interaction-lint.log) |

以下截图是实际发布配置与真实数据：父变量切到 green 后，引用图的值为 210，无关图仍为 26775，日志通过显式映射查询 ERROR。半宽日志表中正文仍受元数据列挤压，已计入下一项展示收尾。

![真实候选与跨信号映射](../../artifacts/planv2-e2e/p2-interaction-20260927-c/playwright-planv2/planv2-cascade-real-real-c-2d5c3-d-explicit-mapping-zh-light-chromium/cascade-local-mapping.png)

下图是碰撞下推后缩到声明最小尺寸、取消拖拽、刷新恢复并继续键盘缩放的个人草稿；没有把布局测试的未执行图卡当作数据查询证据。

![碰撞与最小尺寸后的恢复布局](../../artifacts/planv2-e2e/p2-interaction-20260927-c/playwright-planv2/planv2-layout-real-real-gr-def11-lation-and-recovery-en-dark-chromium/grid-boundaries.png)

## 仍需独立完成

跨厂商显式映射、来源停采/重装与执行来源冻结的正式页面组合，Logs/Trace/APM 深度联动，实际模型创建/分析，文件取数或交付中途故障，多副本/规模/完整无障碍矩阵。Task 01/02 继续进行中。
