# 自动仪表盘分析与查询参数展示

2026-10-13 用户确认：用户描述分析目标，AI 自行检索相关仪表盘和决定查询条件。取消会话右侧上下文面板、输入区分析范围栏及强制手动选择；每次查询的实际参数放在对应仪表盘结果上方。用户用对话纠正后重新取数、分析，历史结果保留原参数。

## 对象发现与执行边界

`list → get → context.resolve → query → query.get → Workspace 文件分析`。@ 只是可选的对象提示，不是执行前件；AI 根据问题及已发布定义选择对象，资源身份有歧义才澄清。目录不自动选中全部候选，也不授予数据权限。

get 经当前企业、用户、对象授权和发布状态校验后，在会话锁内追加本 Run 的仪表盘条件基线。旧手动分析提示仅用于对应消息，普通后续消息自动重新发现对象，避免已撤权的旧提示阻止纠正。消息接受时的 dashboard_run_contexts 保持不可变；dashboard_run_parameters 记录已授权发现，查询的 dashboard_analysis_contexts 和 FrozenPlan 独立冻结。恢复从这些数据库事实组合当前分析范围，不从模型摘要推断。每个 Run 最多采用 20 个仪表盘，发现、候选和查询继续共用累计预算。

普通会话可以发现其他工具；一旦本 Run 读取仪表盘，动态 Tool Policy 限定其分析使用已发布仪表盘查询、文件交付和离线工具。创建仍使用已有显式入口、个人草稿和宿主发布确认。

## 参数来源和纠正

context.resolve/v2 的 evidence 支持两种来源：

- message_interpretation：引用本轮用户消息的精确片段，用于明确条件及纠正。
- ai_selection：记录 AI 自主选择的 reason，不能伪装为人工输入，也不能覆盖已有用户明确条件。

每张仪表盘分别继承条件。未修改时使用 changes={}、evidence=[]；默认值仍由当前发布定义解析。用户指出时间、资源或筛选错误时，创建新的不可变 Context 和查询任务，分析新文件；不修改旧标签、旧清单或旧文件。模型选择仍受类型、候选、资源授权、契约兼容性与预算校验。

## 结果展示

Dashboard Query 使用领域内的固定 Tool Presentation，模板为 internal/dashboard/templates/query.html。参数区位于图表上方，展示实际资源名称、绝对起止时间及企业时区；更多参数包含资源 ID、发布版本、有效变量和 Panel 局部筛选。失败、无数据和未完成取数同样保留参数与状态。

参数来源是服务端 FrozenPlan 或封存 Manifest.Execution，不能由模型输出标题或摘要替代。历史展示使用各自的冻结记录。图表预览只读取经分片及完整文件 Hash 校验的数据；读取和渲染均有上限，受限预览明确提示完整数据仍在会话文件中。预览不进入模型结果，不代表 AI 已读取或分析文件。

## 验证记录

- Go 领域、Tool Gateway、Agent 和会话回归通过；独立 Kubernetes PostgreSQL 验证自动采用、用户纠正、新查询身份、历史时间保持、跨主体及不存在对象拒绝。
- Enterprise 44 文件、197 项单元测试通过；Template Runtime 4 文件、11 项通过。
- 桌面浏览器中文浅色、英文深色 2/2 通过：无侧栏/范围按钮、直接发送、查询参数位于图表上方、两个结果保留独立时间、刷新及完整宽度。该组使用 mock UI 和明确的服务端展示夹具，不代表真实模型验收。
- 真实 GLM-5.3 automatic 组 c 轮 2/2 通过：自动半小时分析 13 次模型调用，一小时纠正 8 次；Provider 用量完整，合计 21 次、411,311 Tokens。两轮接受消息的 Dashboard IDs 均为空，实际图、参数、文件分析和结论经独立 Oracle 核验，每轮两份 Presentation 的时间与封存查询一致。证据见 [模型报告](../../artifacts/chat-analysis-20261013/model-c/planv2-real-model.json)、[自动检索](../../artifacts/chat-analysis-20261013/model-c/p2-auto-discovery.json)、[纠正](../../artifacts/chat-analysis-20261013/model-c/p2-auto-correction.json)。浏览器使用独立 mock UI 检查，不将模型 API 场景写成同轮真实浏览器全量验收。

b 轮在真实模型启动前的文件分析前置任务出现 CONTEXT_COMPACTION_FAILED；保持上下文门限不变，将普通会话参考指引缩短，并增加 2048 字节门禁。其失败快照保留，c 轮重跑已通过真实采集/查询、持久文件、离线工具及两个真实模型场景。原失败不改写为通过。

c 轮临时部署健康检查 20/20 通过。原 E2E 进程在收尾返回退出码 1：Strimzi 在恢复脚本之前已将 Kafka Entity Operator 恢复到原副本数，受保护的 0→原值 Patch 因此拒绝。保留该退出事实；恢复逻辑现只接受相同 Namespace/Workload UID、Release 和精确原副本数，其他并发修改仍拒绝。新增单元检查及真实集群恢复复验通过，20 项工作负载 UID/副本一致且全部 Ready，临时 Namespace 0；未把该独立复验写成整套 E2E 重跑。见 [恢复记录](../../artifacts/chat-analysis-20261013/restored-workloads.json)及[清理结果](../../artifacts/chat-analysis-20261013/cleanup-verification.json)。

实现和独立验证完成后，按用户后续要求将六个 Argus 镜像重新构建为 `chat-analysis-20261013-r1` 并发布到本地 Kubernetes；正式健康检查 20/20 通过，运行配置、业务记录与持久卷核对见[发布记录](./local-publish-20261013.md)。这次发布不扩大上述模型与浏览器验收范围。历史 PlanV2 选择流程及 2026-09-28 模型记录保留其当时事实，不能作为本次自动流程的通过证据。
