# PlanV2 当前前后端复核（2026-09-28）

后续本地安装更新：审计测试的 mock 已声明 `typeof fetch`，TS2493 已修正；完整 `pnpm typecheck` 和该审计测试通过。以下失败记录保留复核当时事实。最新分析提示及当前整版联合回归仍未补跑，本地重新部署与安装自检不替代这些验收。

本次响应“整个前端和后端是否全部完成”，复核当前工作区代码并重跑本地门禁。结论：首期主要功能已有正式实现和分轮验收证据，本次抽查未发现新的整块功能空缺；但当前前端类型门禁失败，不能把历史 37/37 等同于当前版本全部通过。重新打开 P2V-RELEASE-01，清单为 36/37。

## 正式实现核对

| 范围 | 当前实现与证据 |
| --- | --- |
| 前端入口与编辑 | `router.tsx` 注册目录、查看、编辑页面；个人草稿、样本执行、发布预览、版本冲突处理接入真实 API；历史和归档另有既有 real E2E |
| 图型与布局 | `@argus/ui` 的 ObservationPanel、metric-options、DashboardGrid 和 dashboard-layout 提供图型渲染、十二列拖拽、缩放、碰撞和键盘操作；既有十一类 Metrics gallery 验收 |
| Logs / Trace / APM | 正式共享渲染器与详情、下钻组件存在；查询使用当前 PromQL、Argus KQL 和只读 Trace GraphQL 范围，不代表完整复刻所有厂商产品能力 |
| 资源与变量 | Host/Cluster 正式页面使用 ResourceDashboardLinks；顶部资源、变量依赖和局部筛选接入运行时；候选与来源另有 real E2E |
| 后端领域与发布 | publication.go 核对草稿版本、起始 Revision、对象生命周期版本、Folder 版本及当前权限；通过 PendingAction 私有 Commit 生效 |
| 后端执行与文件 | 来源解析、共享预算、查询任务、不可变结果文件与 Workspace 交付已有实现；Worker 检查租约、取消和来源授权 |
| Chat | 正式选择与创建交互、工具契约、文件分析已有实现；真实模型证据为 h/i/o/p 分轮的 13 个代表场景 |

这是一轮代码路径抽查和本地门禁复核，不是逐行无缺陷证明，也未重新部署 Kubernetes 或运行浏览器、真实模型全套。

## 本轮实际检查

| 检查 | 结果 |
| --- | --- |
| `pnpm --filter @argus/ui --filter @argus/api-client --filter @argus/enterprise test` | 三个包通过，Enterprise 153 项通过 |
| `pnpm typecheck` | **失败**：`web/packages/api-client/src/adapters/real/audit.test.ts:59`，TS2493；零参数 mock 被推断为无参数元组，但断言读取第一个调用参数 |
| 独立 Enterprise / Platform 类型检查 | 通过；不能抵消共享 api-client 的失败 |
| UI / observability / template-runtime 类型检查 | 通过 |
| `pnpm check:i18n`、`pnpm check:styles` | 通过 |
| dashboard / dashboardcontext / telemetry/queryengine / toolgateway / workspace / agent / httpapi 的 `go test` | 通过（部分缓存）；本轮未配置 PostgreSQL、ClickHouse 集成端点，不能称为重新通过真实存储集成测试 |
| `go run ./cmd/argus-dev contracts lint` | 退出 0，契约测试通过；OpenAPI 推荐规则仍有 warnings，不是零告警 |

## 尚需收口

1. 修正审计 API 测试 mock 的参数类型，重新通过仓库前端类型门禁。此处是测试类型错误，尚无证据表明仪表盘运行功能因此失败。
2. 复验最新三信号分析提示。o 轮实际曾达到步骤上限；后续将额外证明文件要求简化为 Chat 摘要，但新版提示尚无独立成功实测。当前三信号分析通过记录来自 i 轮，不能改写 o 轮失败。
3. 若要声明“当前整版前后端已完整验收”，需要冻结同一源码/镜像后，运行正式页面、真实数据链路及全部 13 个模型场景的整体验收。目前页面证据为 f/g 分轮 32 场景，模型证据为 h/i/o/p 分轮通过；最后 p 轮只执行异常组 4 项，`browser_executed=false`。这些历史证据有效，但不足以证明当前整版一次性全绿。

Profiling、告警、实时协作、移动端和 K8s 下级对象绑定继续属于首期外范围，不计入以上剩余项。SkyWalking/Jaeger 原生 Trace 接入已追加实现与验收，不再列为未做。生产强隔离和生产规模性能也不能从本地 evaluation 结果推定。

历史结果参见 [非模型报告](./completion-review-20260928.md) 和 [真实模型报告](./real-model-validation-20260928.md)。本轮仅更新复核记录和发布门禁状态，未修改业务实现或测试代码。
