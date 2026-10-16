# 原型验证

日期：2026-10-08。UI 修订：2。状态：入口归属已修订，交互原型可评审。

本次按用户确认将告警中心收敛为六个业务 Tab，D04/D05/D06 的会话工具属于已有会话工作台，D08 的权限和告警审计属于组织与权限。正式功能任务全部保持 todo。

## 本次验证

- 浏览器：Codex In-app Browser，在桌面视口直接操作控件并对可见状态断言。
- 21 项交互断言通过：六 Tab 与旧入口移除；独立会话/组织入口；示例问题只填入自然语言输入；发送后生成 Tool Hub 查询、详情、调用记录；隐藏 Commit 不进入模型工具记录；取消不写入、宿主确认一次提交、跨入口共享策略状态；组织内权限与审计、只读拒绝；规则存在性与评估健康；旧查询结果清除；多指标编辑与键盘切换；返回保留原告警 Tab。
- 72 项告警中心布局检查通过：6 Tab × 中英文 × light/dark × 1280×800 / 1440×900 / 1920×1080，确认选中状态、Tab 归属及整体无横向溢出。
- 24 项独立入口检查通过：会话工作台和组织与权限 × 四种语言主题 × 三个桌面视口；页面标题与入口归属正确，告警中心 Tab 不出现在这两个页面。
- 12 项工作台确认复核通过：四种语言主题 × 三个桌面视口；会话消息中用户确认可见，模型不直接调用 Commit。
- JavaScript 语法检查、文档链接、任务依赖、设计 Token 与单文件长度检查通过；浏览器未记录 JavaScript console error。

当前证据：[prototype-checks.json](./prototype-checks.json)。初版证据保留为 [prototype-checks-first-layout.json](./prototype-checks-first-layout.json)，仅代表入口修订前的原型，不作为当前布局结论。

原型回复与 Tool Hub 记录为确定性交互模拟，未连接真实模型或正式 Tool Gateway；模型侧只展示 `tool.search`、`tool.describe`、`tool.invoke` 的调用链路。配置、权限、审计与投递只更新本地演示状态，不执行正式后台评估或 HTTP/SMTP 通知。生产后端、实际模型、实际工具与 K8s 门禁另按任务验收。

## 截图

| 界面 | 截图 |
| --- | --- |
| 中文浅色：告警中心六 Tab | [overview-zh-light.jpg](./screenshots/overview-zh-light.jpg) |
| 中文深色：告警中心六 Tab | [overview-zh-dark.jpg](./screenshots/overview-zh-dark.jpg) |
| 英文浅色：告警中心六 Tab | [overview-en-light.jpg](./screenshots/overview-en-light.jpg) |
| 英文深色：告警中心六 Tab | [overview-en-dark.jpg](./screenshots/overview-en-dark.jpg) |
| 会话工作台：工具结果与宿主确认 | [ai-preview-zh-light.jpg](./screenshots/ai-preview-zh-light.jpg) |
| 组织与权限：告警能力与资源范围 | [organization-access-zh-light.jpg](./screenshots/organization-access-zh-light.jpg) |
| 多指标策略编辑器 | [policy-editor-zh-light.jpg](./screenshots/policy-editor-zh-light.jpg) |

完整交互请打开 [demo.html](./demo.html)，从侧栏切换告警中心、会话工作台和组织与权限。
