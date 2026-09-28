# 审计、生命周期和候选异常收尾（2026-09-27）

本轮继续落实 [剩余清单](./remaining-work-20260925.md)。复用现有审计表/Hash 链、PendingAction、对象授权和 Query Runtime，没有新增发布或权限路径。最终修正版 `p2-audit-20260927-c` 已通过 20/20 真实页面及三信号文件/审计关联，服务端在页面/文件验收前后同一 Pod、Ready、零重启。Task 01/02 仍未全部完成。

## 实现变化

- 修复审计白名单丢弃 `draft_version` 的问题，保留草稿、编辑基线、发布版本、分组/关联和动作引用；发布事件关联实际 Dashboard 与不可变 Revision。
- 生命周期/关联修改在同一事务记录事件；发布预览失败或旧确认被拒绝不会伪造成功的领域事件。
- Runtime 和下钻记录实际版本、绝对时间、有效资源/来源、各 Panel/Target 状态、查询 Hash、扫描/返回量及 partial。变量只保留 ID、All/数量和选择 Hash，不记录查询正文、变量原值、结果数据或 Context Token。
- 查询任务在入队、封存、交付、取消/恢复和失败节点记录任务/尝试、版本及文件 Hash/来源。交付与分析状态分开，文件已交付不等于已经分析。
- 审计 API 增加操作者、资源类型/ID、结果、时间和文本条件，数据库按条件与 `(created_at,id)` 游标查询，每次最多读取页大小加一条；游标绑定完整条件与主体。修复 real 适配器忽略多数筛选/游标、丢失详情和 Hash 的问题。
- 企业审计页支持具体动作、中英文仪表盘事件/资源名称、前后页和不可变详情。取消只在第一页做本地筛选的行为；失败显示错误。当前页没有某种事件时，已登记的动作/资源类型仍可选择。
- 基本名称在领域事件中保存，归档、改名或删除后仍能解释历史。审计读取不会通过其他编辑者的草稿读取接口获取查询配置。

## 验证内容

1. 审计脱敏后仍保留版本/来源/文件校验信息，拒绝原始查询、变量原值和数据。
2. 服务端全部筛选条件参与匹配与游标签名；变更条件不能沿用上一页游标。
3. 两组语言/主题各通过公开 API 写入 51 条目标保存事件及 51 条干扰事件，验证跨页筛选、版本详情、发布 Revision 和实际执行审计。
4. 未发布草稿所在分组归档后不能发布，恢复后旧预览仍失效；草稿保留，新预览只创建一个版本。
5. 资源关联两侧分别撤权、恢复，不使旧预览复活；删除专门创建的未安装 Collector 的测试集群记录后，关联入口隐藏，仪表盘保留，已删除资源查询被拒绝。
6. 真实候选搜索无匹配与浏览器请求失败保留选值；切换到确认无数据的时间段后才回退 All，并保留资源约束与发布默认值。请求失败由浏览器局部注入，不声称已验证真实 Query 服务宕机。
7. 原有 15 个真实页面和三信号 Workspace/PVC 回归；新增检查文件审计与实际交付的任务、尝试、版本和 Hash 一致。

## 当前执行记录

- 后端相关包、审计字段/条件/投影回归通过：`artifacts/planv2-audit-backend-tests.log`。
- 企业前端 146 个单元测试通过；契约重新生成和 `contracts check` 通过。生成过程中并行类型读取发生过文件暂缺，已在生成结束后顺序重跑并通过 Enterprise/Platform 类型、ESLint 和 i18n 检查：`artifacts/planv2-audit-types.log`。
- 本机 Node 24.10.0 的 Redocly 退出阶段触发 libuv 断言；改用已有捆绑 Node 24.19.0 运行同一生成器并成功，没有修改全局 Node 配置。
- `p2-audit-20260927-a` 在安装前中断，没有运行页面测试。遗留入口 Namespace、ClusterRole/Binding、IngressClass、注册表容器及所属测试 Lease 已核对归属后清理，原有资源核对通过。
- 自动审批复核拒绝删除 `C:\Users\23574\AppData\Local\Temp\argus-dev-e2e-2585107958` 中的 `argusctl.exe` 和 `ingress.yaml`，返回原因仅为 `blocked by policy`；这两个文件保留，未用其他手段删除。
- `p2-audit-20260927-b` 19/20 页面通过，新增审计、分组、双方授权/资源删除、候选异常均通过；文件/PVC 和文件审计关联也通过。唯一失败为既有双编辑者用例的登录请求收到 502，服务端容器在同一时段 OOMKilled（退出 137，重启一次），不是密码错误。该轮已清理并核对原有资源。
- 当时审计表为 556 行，详情总大小约 214 kB，单行最大 2896 字节，不能断言全表读取是 OOM 唯一原因。服务端原硬上限 256 MiB，Argon2 每次使用 64 MiB；未配置 Go 软内存预算。后续将审计筛选/分页下推数据库，并为现有 256 MiB 服务端配置 `GOMEMLIMIT=192MiB`，保留原密码算法参数。三种部署 Profile 的渲染回归通过。
- `p2-audit-20260927-c` 修正后的正式部署通过 20/20 浏览器及文件/PVC、文件审计关联。前后健康证据确认同一 Server Pod、硬上限仍为 256 MiB、软预算为 192 MiB、Ready 且零重启；没有把此负载下的结果推定为高并发性能已完成。
- 三个数据文件实际为 1 条日志（404 字节）、1 个 Metrics 序列（708 字节）和 100 条 Trace（34,524 字节，已发布查询显式上限）；工具实际读取和计算 Hash。入队/封存/交付审计的版本与任务匹配，后两者的 attempt、三个文件 Hash 与来源引用一致。`complete=true` 与 `analysis_status=not_analyzed` 分开记录。
- 实际模型状态仍为 `not_configured / executed=false`。确定性文件读取和 Hash 核对不能替代模型分析验收。
- 最终驱动退出码为 0；本轮临时 Namespace 全部删除。独立清理复核确认 `/readyz=ok`、原有 10 个 PVC 的 UID 不变且 Bound、4 个外部 OpenSandbox 对象不变、40 个运行中 Pod 全部 Ready。上一轮 A 的两个被拒绝删除的本机临时文件仍保留，不将其写为已清理。

## 最终证据索引

| 范围 | 证据 |
| --- | --- |
| 20 个真实页面 | [Playwright 结果](../../artifacts/planv2-e2e/p2-audit-20260927-c/playwright-planv2/.last-run.json)、[运行日志](../../artifacts/planv2-audit-run-c.log) |
| 三信号文件、恢复和 PVC | [任务及文件证明](../../artifacts/planv2-e2e/p2-audit-20260927-c/planv2-files.json)、[实际 read/bash 计算](../../artifacts/planv2-e2e/p2-audit-20260927-c/planv2-file-proof.json) |
| 文件审计关联 | [入队/封存/交付事件](../../artifacts/planv2-e2e/p2-audit-20260927-c/planv2-audit-files.json) |
| 服务端内存和重启 | [验收前](../../artifacts/planv2-e2e/p2-audit-20260927-c/planv2-server-health-before.json)、[验收后](../../artifacts/planv2-e2e/p2-audit-20260927-c/planv2-server-health-after.json) |
| 临时资源清理与原有环境 | [独立清理复核](../../artifacts/planv2-audit-20260927/cleanup-c.json)、[最终退出码](../../artifacts/planv2-audit-20260927/run-c.exit) |
| 实际模型边界 | [未配置状态](../../artifacts/planv2-e2e/p2-audit-20260927-c/planv2-real-model-status.json) |
| 回归检查 | [Go](../../artifacts/planv2-audit-backend-tests.log)、[分页最终回归](../../artifacts/planv2-audit-final-paging-tests.log)、[前端类型/样式/i18n](../../artifacts/planv2-audit-types.log)、[生成契约一致性](../../artifacts/planv2-audit-contracts.log) |

正式审计抽屉中，中文浅色与英文深色均可查看草稿版本、编辑基线和配置 Hash；长 Hash/JSON 使用共享 CodeBlock 横向滚动与复制。

![中文浅色审计事实](../../artifacts/planv2-e2e/p2-audit-20260927-c/playwright-planv2/planv2-audit-real-real-aud-c52ea-nd-immutable-facts-zh-light-chromium/draft-audit-facts.png)

![英文深色审计事实](../../artifacts/planv2-e2e/p2-audit-20260927-c/playwright-planv2/planv2-audit-real-real-aud-dbc2a-and-immutable-facts-en-dark-chromium/draft-audit-facts.png)

## 仍未覆盖

复杂布局碰撞/最小尺寸、真正的候选多页/级联/局部条件、跨来源映射及停采/重装、复杂 Trace 授权联动、文件在途故障、实际模型全流程、规模和完整无障碍矩阵继续单列。此次审计和候选工作不能代替这些验收。
