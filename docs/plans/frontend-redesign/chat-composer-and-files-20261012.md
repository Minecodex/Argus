# 会话输入与文件展示修复

日期：2026-10-12。范围：企业门户桌面会话、共享按钮/文本域与文件事件展示；保留工作区已有的消息时间线和文件预览改动。

## 原因与修复

1. 上传与 `@` 按钮同时使用 Button 与 Tooltip Trigger。供应商 Tooltip 的 `display: block` 覆盖了按钮的 flex 布局，导致图标落在背景左侧。共享 `@argus/ui` Button 明确设置 flex 居中，图标按钮移除图标外边距；正文型按钮仍保留自己的内容布局。
2. 会话外层已有圆角边框，内层 Textarea 又应用共享表单焦点环和供应商字段阴影，因此顶部两个角出现第二层框。共享 Textarea 增加 `appearance="embedded"`，由会话外层使用既有 `--control-focus-*` token 绘制完整焦点状态；普通表单默认样式不变。
3. 原消息投影把没有 Run ID 的 `workspace_file_added` 当成空正文用户消息，旧渲染器还无条件输出正文气泡。仪表盘查询通过服务端导入 Workspace 的事件不带 Run ID，因而每生成一个查询文件就出现用户名称、头像与空条。当前消息时间线将其归为文件活动；本轮让历史与流式共用文件投影、分组与去重，并在渲染入口排除无可见内容的消息。文件交付和工具详情仍保留；用户文字消息移除用户头像。
4. 浏览器回归同时发现 `@` 接在已有文字后不满足引用语法。按钮现在在必要时补空格，打开选择器后仍将焦点交回输入框。mock 在线预览读取完整 Data URL 时错误地直接调用 `atob`，已改为仅解码逗号后的 Base64；预览与下载保留相同字节。

后端文件写入、授权、会话事件与产物协议没有变化；文件展示遵循 PlanV5 既有边界。相关权威源为 `internal/workspace/import_query.go`、`internal/workspace/files.go` 和 `internal/transport/httpapi/conversation_handler.go`。

## 验证

- 企业门户最终 Vitest：42 个文件、188 项通过，包括无 Run ID 的生成文件、空消息、流式交付、部分持久化分组去重和无头像渲染。
- 桌面浏览器：中英 × 明暗检查按钮背景内的图标中心、外层焦点框、内部无边框/光晕/阴影、引用选择器、发送与刷新；另验证真实上传字节、在线预览和下载。结果及截图保存在 `artifacts/chat-ui-20261012/`。
- 最终浏览器回归 12/12 通过：新会话修复场景 5 项、仪表盘会话选择 2 项、1440px 公共弹窗矩阵 4 项、Workspace 上传下载与删除 1 项。最终截图位于 `artifacts/chat-ui-20261012/verified/`；中途因源码更新导致的页面重载、旧下载选择器和 mock 预览解码问题均已复验。
- 企业门户 TypeScript、共享样式 token 检查与改动范围 ESLint 通过。

复验命令：

```powershell
pnpm --filter @argus/enterprise test
pnpm --filter @argus/enterprise typecheck
pnpm --filter @argus/api-client typecheck
pnpm check:styles
$env:ARGUS_E2E_EXTERNAL='1'
$env:ARGUS_E2E_ENTERPRISE_ORIGIN='http://127.0.0.1:4193'
$env:ARGUS_E2E_ARTIFACTS='../../../artifacts/chat-ui-20261012/verified'
pnpm --filter @argus/enterprise e2e chat-composer-ui.spec.ts p5-mock.spec.ts dashboard-chat.spec.ts dialogs-presets.spec.ts --grep 'chat composer|upload and file|uploaded bytes|Chat dashboard|1440' --workers=2
```

浏览器测试由 mock Enterprise 开发服务与 Template Runtime 提供；公共弹窗回归另使用 mock Platform。复验时需先启动对应服务。

验证使用正式 React 组件与 mock API，不代表真实模型、对象存储或 Kubernetes 全流程验收。本轮没有更新正式部署。
