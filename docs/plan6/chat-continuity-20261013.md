# 会话列表与连续回复（2026-10-13）

按用户两项截图反馈继续修复，保留全部未提交工作。

- 共享 `ConversationListItem` 移除图标，保留标题、日期、选中状态、长标题省略与键盘/点击操作。
- 用户输入是回复分界。连续 AI 正文、工具步骤、卡片、确认及自动文件归入一个回复区，身份标题和头像只显示一次；不同 Run 不产生新的视觉回复。内部事件顺序保持原 Ledger 事实。
- 流式与历史刷新使用同一分组；乐观用户输入先于后续流式输出，新输入开始新的回复区。用户发送附件也是输入，自动生成文件不拆分回复。
- 回复区内部使用统一 token 间距；不把中间的正文重新画成带身份标题的新回复。

本轮代码与局部验收完成，尚未发布本轮代码。证据位于 [artifacts/chat-continuity-20261013](../../artifacts/chat-continuity-20261013)：

- `chat-tests.log`：会话组件 47 项测试通过；共享会话行单项验证通过。
- `mock-browser.log`：中文浅色、英文深色两项流程，验证第二次用户输入和刷新后的回复分界。
- `real-history-browser.json`：同两种界面组合下读取原真实 GLM 告警会话，验证多个正文段/实际工具/卡片共用一个身份标题且事件顺序保留，左侧图标为零。静态资源为当前 real build，API/会话事实来自正式环境，未生成新的模型 Run，也未更改正式镜像。
- `typecheck.log`、`lint.log`、`styles.log`、`real-build.log`、`bundle.log`：相关门禁通过。
- `screenshots/local/alert-timeline-zh-light.png`：连续回复与无图标列表的实际截图。

额外检查旧指标报告时，旧 `dashboard-queries/99cc8276-8815-4cb9-947c-d21736e50b22` 与文件读取返回 403，UI 显示 `FILEFORBIDDEN`。失败事实保留在 `prior-metric-file-authorization-failure.json`，未更改授权或将这条旧文件路径计入本轮通过范围；本轮正向真实浏览器范围为可读取的告警会话。该旧查询/文件拒绝需单独核对来源和授权版本。

正式上线沿用 [本地发布门禁](./local-release.md)，与其余未提交修复统一冻结和验收后发布。
