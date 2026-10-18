# Chatbox 工作模式、权限与持久化交互

> 后续调整见 [第 18 篇](./18-markdown-plan-command-consent.md)：Markdown 中明确绑定的命令可随计划确认获得一次性授权，执行保持普通 Harness 工具调用。本篇的指导性计划规则与证据保留其历史适用范围。

日期：2026-10-18。状态：工作模式、权限、计划与询问交互及收尾验收已完成，最终发布见 [修复与最终验收](./16-interaction-closure.md)。本页保留 r01 实施和初次复核证据。

本轮用户确认将工作模式设为 execute / plan，权限独立设为 request_approval / full_access。新聊天默认执行和请求审批，选择保存于当前聊天。权限变化只影响随后创建的操作；Workspace 和普通查询保持原规则。

本轮替代原 Plan7 的新计划执行语义：计划作为 ReAct 实施指导，确认不是整批命令授权。确认后创建绑定不可变计划版本的执行 Run，AI 可根据结果选择与调整命令；每次操作仍按创建时的权限策略处理。完全访问的普通操作由服务端自动确认，继续通过账号授权、MFA、企业审批、派发复核与审计。用户提问和计划实施始终需要人工答复。完全访问与计划同时开启时，connect 也可自动确认。

计划通过 workflow 分类 plan.propose 产生；未确认修订仅展示最新版本，已确认计划独立保留。输入框承载计划确认、反馈和用户提问。request_user_input 支持 1～3 个问题、单选、自由文字、显式跳过和可靠暂停恢复。后台保存不可变请求和答复，Worker 等待期间释放租约，后续工具不可越过暂停点。

添加菜单分为添加与技能，沿用上传文件、引用资源和创建仪表盘行为。计划从添加菜单开启；权限按钮紧邻 +。共享组件、设计 token、桌面端、中英文和明暗主题沿用门户规范。

计划卡片和已回答问题摘要复用普通执行卡片的聊天内容列，与正文、文件及助手标题等宽居中；内容保持完整展开，长文本换行、长命令在代码块内滚动。相关修复与页面验收见 [布局统一](./17-chat-reading-column.md)。

历史验收记录仍对应旧语义和各自源码版本，本轮另行保存确定性、浏览器、真实模型和部署证据。不得用历史通过数替代本轮验收。


## 实施与接口

- Conversation 保存 mode=execute|plan 与 access_mode=request_approval|full_access；Run 保存启动快照及 approved_plan_id。迁移 17/18 已在隔离数据库和本地部署执行。
- conversation_interactions 保存版本、内容 Hash、会话、工具调用、授权版本、答复和实施 Run。GET /conversations/{id}/interactions 返回当前交互及活动 Run；独立读取和 respond 接口支持精确版本的 answer、skip、revise、confirm。接口、生成客户端和 real/mock 适配器同步。Connector/远程执行 protobuf 未改变语义，不新增跨通道询问或批准能力。
- plan.propose 位于 workflow 分类，继续遵守 search → describe → invoke。request_user_input 为独立顶层控制工具；控制元数据来自受信 Provider，不从工具 JSON 推断。用户提问保持工具调用与最终结果配对，不能由完全访问自动回答。
- 计划确认原子结束规划 Run、保存确认消息、将聊天切换为 execute，并派发实施 Run。其内容在每轮事实和压缩后重新注入。计划意见产生新版本；已确认版本保持不可变，历史固定批次计划只读。
- 普通操作冻结 confirmation_mode 和策略版本；user_confirmations.source 区分 user 与 access_mode_policy。自动确认复用现有执行、MFA、企业审批和派发复核；明确停止在实际派发前阻止任务执行。
- 回答问题后，同批尚未派发的旧决策保留取消事实，再让 ReAct 读取真实答案后重新决定；不因旧参数已经排队而越过用户意图。Worker 在计划已落库而 ToolResult 尚未落库时可恢复暂停点，不调用模型或派发后续命令。

## 本轮验收进度

源码冻结：43d1193302c37863c153dddbfae780fb978b6dc9b0a50a31f1aba685b2232c51，2693 份源文件。所有手写 Go/TypeScript/TSX 文件符合 2000 行限制。

工程门禁：Go 2230 项通过、98 项按独立环境开关跳过；前端 1319 项通过，所有门户类型检查、ESLint、样式、i18n、契约 lint/check 与 real 构建通过；浏览器 17 项通过，覆盖三种桌面视口、中英文及明暗主题。

Windows 11 实际 PowerShell 与 runtimewire 回归通过。Kubernetes 重新提供实际打包 helper 后，Pod 状态、文件交换和实例替换回归通过，临时 namespace 已清理。本机 NetworkPolicy 未观测到有效拒绝，受控 kubectl 环境按设计返回不可用；相应 fail-closed 验证通过，不将其写为本机可安全执行任意 kubectl。独立 Egress Gateway 缺失与原部署一致。

本地 Helm platform revision 34，原部署与 PVC 身份、副本和卷绑定共 60 项保留核验通过。数据库备份位于本机私有证据目录，凭据不写入普通验收文档。真实模型两种权限流程均已完成，最终结果如下。


## 真实模型最终结果

使用 kakj 实际账号、APIMart glm-5.3-flash 和正式 Connector 通道，2/2 浏览器流程通过。4 个实际 Run 均成功，36/36 次提供方推理调用用量完整且已结算；本轮没有触发模型压缩，因此不把该结果写为新增真实压缩验收。

- [请求审批会话](https://argus.dev/?c=01a14cb6-a29e-7de1-bab8-8e56e11cebe3)：真实询问、计划意见与第二版确认后，实施阶段 2 次普通操作继续由用户确认；含取证共 3 次实际命令确认，source=user。
- [完全访问会话](https://argus.dev/?c=01a14cbb-4cd9-7d28-a43f-578de79a970c)：询问和第二版计划仍由用户处理，2 次实际普通操作 source=access_mode_policy；实施阶段没有人工点击普通命令确认。

两个独立目标目录均从实际 HTTP 503 / status=broken 恢复为 HTTP 200 / status=healthy，保留配置副本。计划确认前均仍为 503；刷新后只显示最新计划，确认记录与问答历史保留。

模型发生了可纠正的 2 次询问参数错误和 2 次 connect 参数错误，并通过校验反馈自行修正。请求审批案例另外尝试发布 Workspace Markdown 报告 4 次均返回 TOOL_EXECUTION_FAILED，未生成交付记录；报告文件写入和修复结果验证成功。文件发布链路问题的根因及其与本轮变更的关系尚未确认，不宣称文件发布／下载验收通过。完整原始调用和工具错误保留。

主要证据：

- [工程门禁](../../build/chat-interactions-20261018-r01/engineering.json)、[源码冻结](../../build/chat-interactions-20261018-r01/source-after-acceptance.json)。
- [真实调用与审批审计](../../build/chat-interactions-20261018-r01/acceptance-audit.json)、[模型调用](../../build/chat-interactions-20261018-r01/accepted-model-calls.json)、[人工／自动确认来源](../../build/chat-interactions-20261018-r01/approval-origins.json)。
- [实际浏览器日志](../../build/chat-interactions-20261018-r01/actual-model.log)、[完整 UI 矩阵](../../build/chat-interactions-20261018-r01/browser-final.log)。
- [部署](../../build/chat-interactions-20261018-r01/deployment.json)、[资源保留](../../build/chat-interactions-20261018-r01/preservation.json)、[部署后健康](../../build/chat-interactions-20261018-r01/health-after/verify.json)。

Windows Server 兼容性与本机未提供的网络隔离能力继续按原边界标记，不以 Windows 11 或预期拒绝测试代替这些能力的成功证据。

## 完成度复核

以下是 r01 修复前的复核记录。CB-R01／CB-R02 已在 r02 修复并通过真实链路复测，当前结论见 [最终验收](./16-interaction-closure.md)。

2026-10-18 对照最新用户计划重新核对代码、冻结证据和当前部署。2693 份源码与验收清单逐文件 Hash 一致；argus-platform revision 34 为 deployed，server、worker、web 均为 1/1。两个实际测试会话仍保留：各有已回答询问、被替换的第一版计划、已确认的第二版计划和成功结束的实施 Run。

| 范围 | 复核结论 |
| --- | --- |
| 独立工作／权限模式、添加菜单、计划修订及确认、用户询问 | 已实现，工程检查、17 项界面测试及 2 条真实模型流程有通过证据 |
| 持久暂停、确认原子性、权限边界、停止派发 | 已有确定性后端测试；不能用这些测试替代浏览器停止操作验收 |
| 报告发布 | 未关闭：4 次 workflow.publish_file 失败且没有交付记录；需定位根因、补回归，并用真实流程验证发布和下载 |
| 新计划实施 Run 的停止按钮 | 待补本轮浏览器回归，覆盖确认后停止、刷新恢复、停止后不继续派发；已有 plan7-real.spec.ts 停止用例对应历史冻结计划卡片 |
| 本机受控 kubectl、Windows Server | 环境能力仍待验证；本机隔离不可用时正确拒绝，Windows 实测仅为 Windows 11 |

该次复核结论为“功能已实现，验收有未关闭项”。后续收尾保留这些原始发现、工程／真实模型记录与旧版本历史证据，不将当时未通过的项目改写为已通过。
