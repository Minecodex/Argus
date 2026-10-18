# Plan7：Workspace 分析、ReAct 实施与持久化交互

2026-10-18 新增[业务结果目录与元数据](./19-workspace-results-and-metadata.md)：新收集结果使用 AI 命名的 `/workspace/results/<业务相对路径>`，执行／步骤编号集中到 `results/meta.json`。源码、独立数据库／文件 RPC、真实 GLM 检索及浏览器回归已完成；本次变更尚未部署，不与下述 revision 40 的既有验收混用。

> 2026-10-18 本轮已完成：[Markdown 计划、命令授权与普通会话执行](./18-markdown-plan-command-consent.md)。本地 revision 40、schema 19，健康 20/20；两种模式的实际修复和停止共 4 个最终场景通过，38 次干净模型调用用量完整。计划正文、临时审批和工具执行记录已分离，历史数据保留。

> 2026-10-18 交互改造与两项收尾问题已完成：[修复与业务验收](./16-interaction-closure.md)，其 revision 36 证据包括 4 类真实场景和 45 次完整模型调用。此前 revision 37 追加[计划与问答布局修复](./17-chat-reading-column.md)：17 项交互回归和 12 组真实历史页面复核通过。该历史版本后端为 chat-interactions-20261018-r02、schema 18；失败轮次及旧版本证据保留各自适用范围。

创建日期：2026-10-15。更新日期：2026-10-18。原 Plan7 范围及 P7-R01～R04 的独立 GLM-5.3 历史回归为57/57、104 项具名检查、19/19 浏览器。前一阶段在 `kakj` 实际账号使用 APIMart `glm-5.3-flash`＋Docker Linux 通过取数／修复、连续确认、断线恢复、私有并发上下文案例：**47/47 次模型调用和用量结算完整、浏览器2/2**；这些结果与当前交互改造分别存证，详见[历史实际账号实测记录](./14-apimart-linux-chatbox.md)。

2026-10-16 用户确认本轮验收环境：本地 Docker 中运行含 SSH/systemd 的 Linux 目标，独立 Kubernetes namespace，本机 Windows 11 专业版（独立 Connector/执行进程与临时目录）。本轮 Windows 实机结果对应 Windows 11；Windows Server 兼容性另记待验证，不以本机结果替代 Server 兼容证据。真实模型复用 GLM-5.3 及环境变量 GLM_API_KEY。共享命令进程默认空闲回收为 30 分钟，可通过部署配置调整。

2026-10-17 本地存储前件补充：Docker Desktop 缺少高序号 loop 设备节点导致第九个 Workspace 卷挂载失败。验收器核对固定 CSI 身份后补齐标准 loop0～loop63 节点，并增加 9 卷并发挂载／换 Pod 读取验证；已有挂载与数据保留。该操作仅限本轮明确的本地节点，详见[实施记录](./08-implementation-notes.md)。

平台业务变更保留现有 Preview／Confirm；connect、计划执行、共享上下文、文件交换和相关安全门禁已完成本轮验收。本次修复版本已通过独立完整回归并发布到本地既有部署；数据和配置保留。

交互改造前一版发布为 plan7-http1-20261018-r03，Helm platform revision 33，健康20/20；源码冻结为 `7daac58ef2731e8e084bdaf6031ff92476e6e796ab8e9068eea47dfa981b2018`，2652 份清单构建前后及验收后一致，2169 项 Go 检查通过。历史 57 项跨平台门禁与该版工程／Linux 工作流分别存证，不声称重新完成 Windows／Kubernetes 的 Flash 矩阵。已定位的预算、截断状态、HTTP/2 与 ALPN 路径已修复；此前间歇连接失败仍留存，不承诺第三方永不失败。

## 原始目标（历史执行语义，当前以第 18 篇为准）

以现有 OTLP → ClickHouse 数据链路为主要证据来源，让 AI 先通过平台工具取得有界数据，在会话 Workspace 中反复分析；确实缺少现场证据或需要修复时，通过新的 connect 工具申请远程执行。

Chatbox 提供「排查」「计划」两个模式。排查中的每次远程命令都需要宿主确认；计划模式先生成「描述＋命令片段」组成的计划，用户一次确认后，由服务端执行其中已经冻结的请求。模型不能在执行中补充未批准的命令。

首期支持 Linux、Kubernetes 和 Windows PowerShell。同一计划只串行执行；批准后在后台继续，页面关闭／切换模式不自动取消。提交按钮转为执行状态并提供明确的停止操作，只有用户点击停止／取消才构成主动中止；既有失败、超时和撤权规则仍然生效。

Q15 最新确认支持一批指令：同一资源上的整份计划和连续 connect 调用共享同一 shell／PowerShell 会话，目录、变量等状态可以延续；批次授权仍逐次或按固定计划取得。平台需要提供可靠隧道和 Workspace↔远端文件传输抽象，让 AI 主要编写目标环境中的命令。具体提案见 [共享会话与文件传输](./07-shared-sessions-and-file-transfer.md)。

Q20 已确认只同步当前 Workspace 的专用 input，平台封存后在批准的执行中自动上传，远端 output 在结束后统一取回；不要求 AI 逐文件声明。Q21 已确认原命令进程失效时先暂停远程派发，由 AI 分析所需上下文、提出恢复与续执行方案；新远程命令仍需确认，不重跑已经成功的修改来恢复变量。

Q22 允许无活动执行时闲置回收，保护已保存结果和未收集的唯一输出。Q23／Q24 明确不同聊天的独立会话可同时操作同一资源，不做资源级业务排队；并发命令事实只注入 AI 上下文，不在聊天记录或人类提示 UI 中展示。单份计划及同一 shell 内仍只串行，现有权限边界不变。

## 原能力边界（历史验收范围）

| 场景 | 模型入口 | 执行规则 |
| --- | --- | --- |
| 会话 Workspace | read / write / edit / bash；grep 通过 bash 使用 | 隔离、离线环境内免逐次人工确认；仍有会话归属、配额和运行环境约束 |
| 平台业务 | tool.search(category) → tool.describe(category,name) → tool.invoke(category,name,arguments) | 继续当前功能权限和显式资源授权；平台业务变更继续 Preview／Confirm |
| 远程资源执行 | 新增模型入口 connect | 每次完整执行请求确认；已包含在获批计划中的请求不重复确认 |

客户 Remote MCP 继续 PlanV5 已有规则，独立于这三类自有场景；本轮不将客户 MCP 强制纳入 connect 或自有确认流程。因此不能对整个会话承诺「所有外部写操作都经过 connect」。

不再建设以下初案能力：所有 Linux 命令的一对一业务工具目录、同工具／同参数授权等级、会话内 24 小时免确认、按命令前缀记忆授权，以及通过解析任意 Python／shell 自动证明安全。

主机沿用资源接入账号的实际权限，Kubernetes 沿用资源接入凭据／身份；不新增一套运维账号角色管理产品。现有接入路径的身份解析缺口列为开发前件，不能默认为 root 或改用 Connector 服务账号。

## 文档入口

用户指定 APIMart `glm-5.3-flash`＋Docker Linux 的接续实测见 [14-apimart-linux-chatbox.md](./14-apimart-linux-chatbox.md)，结果与原完整矩阵分别记录。

| 文件 | 内容 |
| --- | --- |
| [01-decisions-and-qa.md](./01-decisions-and-qa.md) | 当前决定、Q1～Q24 的讨论记录与被替代方案、设计树 |
| [02-architecture-and-contracts.md](./02-architecture-and-contracts.md) | 执行场景、connect、计划、授权、恢复、证据和资源适配边界 |
| [03-development-plan.md](./03-development-plan.md) | P7-T01～T09 的依赖、开发范围和完成条件 |
| [04-acceptance-matrix.md](./04-acceptance-matrix.md) | 权限、故障恢复、真实模型、真实资源、浏览器与隔离环境验收 |
| [05-product-research.md](./05-product-research.md) | 官方产品资料对照与本计划采用／不采用的做法 |
| [06-open-questions.md](./06-open-questions.md) | Q15～Q24 已明确答案；当前无待回答产品问题 |
| [07-shared-sessions-and-file-transfer.md](./07-shared-sessions-and-file-transfer.md) | 共享会话、目录交换、恢复、生命周期与并发感知契约 |
| [08-implementation-notes.md](./08-implementation-notes.md) | 实施过程、失败迭代与修复记录 |
| [09-acceptance-report.md](./09-acceptance-report.md) | 历史 F33 验收、环境边界与配置 |
| [10-completion-review.md](./10-completion-review.md) | 完成情况复查及两项已关闭问题 |
| [11-follow-up-acceptance.md](./11-follow-up-acceptance.md) | 最新 F40 完整验收与复查问题闭环 |
| [12-local-deployment-and-apimart.md](./12-local-deployment-and-apimart.md) | 历次接续与失败过程 |
| [13-local-release-result.md](./13-local-release-result.md) | 2026-10-17 历史发布、独立验收与早期 APIMart 失败 |
| [14-apimart-linux-chatbox.md](./14-apimart-linux-chatbox.md) | 实际 kakj 账号、APIMart、Docker Linux 的接续、修复和界面历史 |
| [15-chatbox-interactions.md](./15-chatbox-interactions.md) | 工作模式、权限、持久化询问及上一版指导性计划的历史验收 |
| [16-interaction-closure.md](./16-interaction-closure.md) | 报告发布、实施 Run 停止、最终真实模型复测与本地部署 |
| [18-markdown-plan-command-consent.md](./18-markdown-plan-command-consent.md) | Markdown 文档授权、一次性步骤和普通工具时间线 |
| [17-chat-reading-column.md](./17-chat-reading-column.md) | 计划、问答摘要与聊天内容列对齐修复及实际页面复核 |
| [19-workspace-results-and-metadata.md](./19-workspace-results-and-metadata.md) | 业务结果目录、单一元数据索引、碰撞与收集恢复 |
| [implementation-status.md](./implementation-status.md) | 已完成任务、验收结果与范围边界 |
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
