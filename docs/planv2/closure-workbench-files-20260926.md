# PlanV2 工作台与文件链路收尾（2026-09-26）

本轮承接 [完成度复核](./completion-review-20260926.md)，补齐真实页面及 Workspace/PVC 验收路径，并修复共用文件下载的租约释放时序。`p2-closure-20260926-e` 验收和清理通过，Harness 退出码 0：10/10 real 浏览器、三信号真实文件交付、离线脚本及恢复检查通过。整个 PlanV2 仍未全部完成。

## 当前通过范围与证据

| 验收 | 结果 |
| --- | --- |
| real 页面 | 既有八个自监控用例，加中文浅色/英文深色的两个工作台用例，10/10 通过 |
| 人工工作台 | 空白创建三信号图、PromQL 双向无损转换、拖动位置、键盘调整宽高、刷新恢复；同账号双标签页冲突、普通查看不泄漏草稿；归档拒绝发布、恢复要求重新确认基线；每次发布后实际三图查询成功 |
| 文件/PVC | 已发布查询 → Worker/对象存储 → Workspace RPC/真实 Bound PVC；三份数据文件及清单均交付，并核对下载字节和 Hash |
| 离线工具 | 实际 read 读取短日志样本；bash/Python 读取全部四个文件、验证 Hash、计算记录数；通过 workflow.publish_file 发布核对文件，再下载校验 |
| 恢复/隔离 | 排队任务取消后零执行尝试；Worker 停启与测试 Redis 清空后，已完成任务保留相同 attempt 和原始文件 Hash；其他会话请求返回 403；工作区显式删除完成 |
| 回归 | 相关 argusdev、HTTP、Workspace、Workspace IO 测试通过；前端类型/ESLint 检查通过。依赖真实存储的普通 Go 测试跳过部分，以本次真实部署证据补充相应路径 |
| 实际模型 | 配置和判定逻辑已接入；本次未提供配置，明确记录 `not_configured / executed=false`，不作为模型效果通过 |

离线核对得到 1 条日志、1 个 Metrics 序列和 100 条 Trace 记录。Trace 数量来自已发布查询的显式 `pageSize:100`，没有去掉 limit 或声称覆盖全部原始链路。

- [完整运行日志](../../artifacts/planv2-closure-run-e.log)
- [中文工作台恢复发布截图](../../artifacts/planv2-e2e/p2-closure-20260926-e/playwright-planv2/planv2-workbench-real-real-ea97d-es-archived-drafts-zh-light-chromium/three-signals-restored.png)、[英文深色截图](../../artifacts/planv2-e2e/p2-closure-20260926-e/playwright-planv2/planv2-workbench-real-real-e1736-res-archived-drafts-en-dark-chromium/three-signals-restored.png)
- [文件任务/PVC/恢复结果](../../artifacts/planv2-e2e/p2-closure-20260926-e/planv2-files.json)、[从实际文件计算的记录数与 Hash](../../artifacts/planv2-e2e/p2-closure-20260926-e/planv2-file-proof.json)
- [实际模型未执行状态](../../artifacts/planv2-e2e/p2-closure-20260926-e/planv2-real-model-status.json)
- [清理与原有 PVC 核对](../../artifacts/planv2-closure-20260926/cleanup-summary.json)、[外部 OpenSandbox 保留核对](../../artifacts/planv2-closure-20260926/preservation-result.json)

清理后无本轮测试 Namespace 残留；原有 10 个 PVC 的 UID 均保持且全部 Bound，40 个 Running Pod 均 Ready，Kubernetes `/readyz` 为 ok。外部 OpenSandbox 控制器及三份 CRD 的 UID、Spec、标签和 Helm 所有者与本轮开始前一致。

本轮具体关闭项：

- [x] 空白三信号创建、转换、基本布局和恢复发布的真实页面流程。
- [x] 同账号过期标签页保存冲突、普通查看与草稿隔离。
- [x] 归档期间拒绝发布，恢复后重新确认基线。
- [x] 正式 Worker → 真实 PVC → 离线工具的三信号文件字节/Hash/记录数核对。
- [x] 连续下载租约释放修复、排队取消、已完成文件的 Worker/Redis 恢复与跨会话拒绝。
- [x] PlanV2 实际模型 Harness 接入及配置/判定回归。
- [x] 按归属清理、原有 PVC 和外部共享依赖保留核对。
- [ ] 实际模型执行与完整 AI 产品验收；本次未提供配置。

文件获取由公开查询任务 API 启动，离线工具由 Replay 驱动；这证明真实文件链路，不替代 Chat 中 @/自然语言条件解释及实际模型自主选工具的验收。不同编辑者的发布竞争、所有信号/图型、取数中途崩溃与文件损坏/满额等场景仍单列后续。

## 新增验证范围

- 从正式目录的空白创建开始，配置 Metrics/Logs/Traces 三信号图；验证 PromQL builder→DSL→builder 无损转换、样本执行、拖动位置、键盘调整宽高和刷新保留。
- 同一编辑者的两个标签页从相同草稿版本出发；旧页面保存必须冲突，展示两份内容并允许载入服务器草稿；未发布修改不改变普通查看。
- 保留未发布草稿后归档 Dashboard，归档期间拒绝发布；恢复后必须重新确认基线，草稿内容保留，再经预览/确认发布。
- 真实已发布三信号查询经正式 Worker、对象存储及 Workspace RPC 写入真实 PVC；通过普通下载接口核对每个文件字节数与 SHA256。
- 使用现有 Replay 驱动普通 read/bash/workflow.publish_file 工具，在离线 Workspace 中实际读取文件、计算记录数和 SHA256，再下载核对生成的证明文件。此项验证工具执行与文件交付，不作为实际模型理解/诊断能力证明。
- 停止本测试 Namespace 的 Worker 后建立并取消排队任务；恢复 Worker、清空测试 Redis 后，已完成任务保留同一执行尝试及相同文件字节；其他会话不能读取原任务。

复用 Server/Worker、QueryJobs、Workspace lease/来源校验、PlanV5 离线工具及宿主确认，没有新增业务导出按钮、另一套查询服务或文件权限路径。

## 实际模型入口

`argus-dev e2e run --suite planv2 --real-model-config <配置路径>` 现可复用已有严格模型配置：HTTPS 端点、模型 ID、协议、上下文/额度及 API Key 环境变量引用。入口默认不自动发现凭证，不提供配置时记录 `not_configured / executed=false`。

可选真实模型场景包括：

1. 用自然语言要求模型发现目录、创建 builder 趋势图和发布预览；检查实际草稿/查询内容，宿主确认前不能已发布。
2. 同样检查 DSL 编辑来源；确认后核对正式发布版本，不能只凭模型回复判定成功。
3. 明确选择三信号仪表盘后，实际建立查询任务、交付文件、调用离线工具并发布核对结果；从服务端原始文件校验记录数与 Hash。

模型入口的配置、编译和验收判定回归已通过，尚未取得本轮实际模型配置并运行；也不代表泛问、追问、多仪表盘、所有授权和诊断质量场景已关闭。

## 验收执行记录

- `p2-closure-20260926-a`：真实文件任务完成交付，但 Metrics 数据为空，严格非空断言失败；已清理。M7 旧探针直送转发 Gateway，没有注册 Receiver 来源，不能用来证明按 OTLP 来源筛选的 Dashboard。现改为通过受管本机 OTLP Receiver 发送新三信号数据，保留来源硬约束；Sidecar 保留 OTLP 三条及原生 Trace 两条独立管道。另发现 Windows 的 `filepath.Base` 会将 Playwright 正则转义反斜杠视为路径分隔符，导致页面测试被跳过；已改为斜杠路径处理并增加回归。该轮不作为新页面通过证据。
- `p2-closure-20260926-b`：注册 Receiver 的三信号注入与原有八个浏览器用例通过；新增两个工作台用例因指标控件的精确标签匹配失败。控件的无障碍名称为“指标/Metric”，标签文本包含必填标记，已改为与现有工作台测试一致的 combobox 角色定位。该轮已清理，未进入文件专项。
- `p2-closure-20260926-c`：三信号文件任务 complete、三图 success、四个文件已交付，见该轮 `planv2-files-observed.json`。两个新页面用例因把编译结果 `metric{}` 与 `metric` 作纯文本比较而失败，现接受这两种等价形式并继续验证转回构建器。独立文件检查捕获连续下载的真实 `WORKSPACE_BUSY`：HTTP 已发送 Content-Length 全部字节，但 Workspace 租约仍在 defer Close 中释放；客户端结束请求会使旧访问无法停放复用，进入保留租约的 Pod 回收。该轮已清理。
- 修复共用文件响应：最多保留最后 32 KiB，先完成底层 reader/租约释放，再发送尾块；大文件前缀仍流式传输，Range 行为保持，零字节也先关闭后完成响应。关闭失败不能返回完整文件，未提交的小响应不保留成功文件的 Content-Length。相关 HTTP、Workspace、Workspace IO 回归通过。
- `p2-closure-20260926-d`：四个文件连续下载及 Hash 核验、离线 read 成功，未再出现 WORKSPACE_BUSY；两个新页面流程已完成恢复后 R3 发布和三图查询，在最后描述文本的精确匹配上失败（实际段落包含“已发布 · R3 · 描述”），已改为正确的包含匹配，并加强普通查看不泄漏草稿的检查。文件脚本前的 Replay Run 因读取完整 9247 字节清单后触发上下文压缩失败而停止；现只向上下文读取短日志样本，完整清单与三信号仍由脚本在 PVC 内读取、校验 Hash 和计数，不放宽服务端上下文限制。该轮已清理。
- `p2-closure-20260926-e`：最终页面 10/10、完整离线脚本与本轮恢复/隔离场景通过，具体证据及边界见本文件顶部。
- 原有 `p2-selfmonitor-20260926-k` 的 8/8 是上轮两条流程的四种组合；本轮新增的工作台流程不能借用其通过结果。

仍待后续覆盖：不同编辑者发布竞争、非空分组迁移/撤权组合、十一类 Metrics 图型、Logs 关联详情、复杂跨资源 Trace、文件取数中断/损坏/满额恢复、性能与完整可访问性矩阵。
