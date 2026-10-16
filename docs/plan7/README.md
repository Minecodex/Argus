# Plan7：Workspace 分析与按次确认的远程执行

创建日期：2026-10-15。状态：产品基线已明确至 Q24，当前无待回答产品问题；开发契约在实施中固化，生产任务全部未开始。

用户已确认平台业务变更保留现有 Preview／Confirm，并要求先记录当前结论、形成开发计划，再继续提问。本文档不代表 connect、计划模式或相关安全门禁已经实现、测试或发布。

## 目标

以现有 OTLP → ClickHouse 数据链路为主要证据来源，让 AI 先通过平台工具取得有界数据，在会话 Workspace 中反复分析；确实缺少现场证据或需要修复时，通过新的 connect 工具申请远程执行。

Chatbox 提供「排查」「计划」两个模式。排查中的每次远程命令都需要宿主确认；计划模式先生成「描述＋命令片段」组成的计划，用户一次确认后，由服务端执行其中已经冻结的请求。模型不能在执行中补充未批准的命令。

首期支持 Linux、Kubernetes 和 Windows PowerShell。同一计划只串行执行；批准后在后台继续，页面关闭／切换模式不自动取消。提交按钮转为执行状态并提供明确的停止操作，只有用户点击停止／取消才构成主动中止；既有失败、超时和撤权规则仍然生效。

Q15 最新确认支持一批指令：同一资源上的整份计划和连续 connect 调用共享同一 shell／PowerShell 会话，目录、变量等状态可以延续；批次授权仍逐次或按固定计划取得。平台需要提供可靠隧道和 Workspace↔远端文件传输抽象，让 AI 主要编写目标环境中的命令。具体提案见 [共享会话与文件传输](./07-shared-sessions-and-file-transfer.md)。

Q20 已确认只同步当前 Workspace 的专用 input，平台封存后在批准的执行中自动上传，远端 output 在结束后统一取回；不要求 AI 逐文件声明。Q21 已确认原命令进程失效时先暂停远程派发，由 AI 分析所需上下文、提出恢复与续执行方案；新远程命令仍需确认，不重跑已经成功的修改来恢复变量。

Q22 允许无活动执行时闲置回收，保护已保存结果和未收集的唯一输出。Q23／Q24 明确不同聊天的独立会话可同时操作同一资源，不做资源级业务排队；并发命令事实只注入 AI 上下文，不在聊天记录或人类提示 UI 中展示。单份计划及同一 shell 内仍只串行，现有权限边界不变。

## 已确认的能力边界

| 场景 | 模型入口 | 执行规则 |
| --- | --- | --- |
| 会话 Workspace | read / write / edit / bash；grep 通过 bash 使用 | 隔离、离线环境内免逐次人工确认；仍有会话归属、配额和运行环境约束 |
| 平台业务 | tool.search(category) → tool.describe(category,name) → tool.invoke(category,name,arguments) | 继续当前功能权限和显式资源授权；平台业务变更继续 Preview／Confirm |
| 远程资源执行 | 新增模型入口 connect | 每次完整执行请求确认；已包含在获批计划中的请求不重复确认 |

客户 Remote MCP 继续 PlanV5 已有规则，独立于这三类自有场景；本轮不将客户 MCP 强制纳入 connect 或自有确认流程。因此不能对整个会话承诺「所有外部写操作都经过 connect」。

不再建设以下初案能力：所有 Linux 命令的一对一业务工具目录、同工具／同参数授权等级、会话内 24 小时免确认、按命令前缀记忆授权，以及通过解析任意 Python／shell 自动证明安全。

主机沿用资源接入账号的实际权限，Kubernetes 沿用资源接入凭据／身份；不新增一套运维账号角色管理产品。现有接入路径的身份解析缺口列为开发前件，不能默认为 root 或改用 Connector 服务账号。

## 文档入口

| 文件 | 内容 |
| --- | --- |
| [01-decisions-and-qa.md](./01-decisions-and-qa.md) | 当前决定、Q1～Q24 的讨论记录与被替代方案、设计树 |
| [02-architecture-and-contracts.md](./02-architecture-and-contracts.md) | 执行场景、connect、计划、授权、恢复、证据和资源适配边界 |
| [03-development-plan.md](./03-development-plan.md) | P7-T01～T09 的依赖、开发范围和完成条件 |
| [04-acceptance-matrix.md](./04-acceptance-matrix.md) | 权限、故障恢复、真实模型、真实资源、浏览器与隔离环境验收 |
| [05-product-research.md](./05-product-research.md) | 官方产品资料对照与本计划采用／不采用的做法 |
| [06-open-questions.md](./06-open-questions.md) | Q15～Q24 已明确答案；当前无待回答产品问题 |
| [07-shared-sessions-and-file-transfer.md](./07-shared-sessions-and-file-transfer.md) | 共享会话、目录交换、恢复、生命周期与并发感知契约 |
| [implementation-status.md](./implementation-status.md) | 当前源码事实、实现缺口与验收状态 |
| [progress.json](./progress.json) | 可机读的任务状态与待决问题 |

## 交付顺序

1. 固化领域、授权与持久化契约，回答会改变执行语义的问题。
2. 建立单次 connect 的预览、宿主确认、远程执行、结果导入与恢复闭环。
3. 建立固定计划的组合预览、一次批准、逐步执行、修改意见和执行记录。
4. 调整 Agent 在操作完成后的能力判断，使 Workspace 分析可以继续，新增远程执行仍须再次确认。
5. 完成真实模型、隔离资源与真实门户的端到端验收；实施与部署结果分别记录。

## 完成边界

- 模型输出、用户看到的内容、服务端批准的版本和实际执行的命令必须可逐项关联。
- 不以一次连接授权后续任意命令；连接复用只是后端优化。
- 不以原生平台权限替代具体变更确认；也不以人工确认补齐缺失的资源或执行权限。
- 不把只读推断、命令解析、AI 风险摘要、Mock 卡片或受控 Replay 当作安全／真实模型验收通过。
- 企业门户正式范围为桌面 Web，复用 @argus/ui、design tokens、中英文与明暗主题。
- E2E 使用临时且可核对所有权的 namespace；暂停既有服务时先记录副本与健康，完成后恢复并清理本次资源，保留既有业务数据。

相关基线：[系统不变量](../00-decisions-and-invariants.md)、[平台动作](../04-agent-mcp-and-action-workflow.md)、[Connector](../03-connectors-and-resources.md)、[Agent Harness](../16-agent-harness-and-context-management.md)、[PlanV5](../planv5/README.md)。
