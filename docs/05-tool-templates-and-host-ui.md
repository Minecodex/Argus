# Tool 模板与宿主交互

本设计采用 PlanV5 基线。自有业务 Tool 随代码发布不可变模板；客户 Remote MCP 仅返回文字和结构化数据。模型不生成、选择、注册或执行平台模板。

## 数据与受众

一次 ToolCall 对应完整业务结果和有界模型投影。完整结果存于 `artifacts`，较大结果进入私有对象存储；模型只获得 `ToolResultProjection`。展示数据单独存于 `tool_presentations`，静态源码按 SHA-256 在 `template_assets` 中去重。

历史展示使用当次模板 Hash。读取展示时校验会话所有者、当前有效权限和资源授权范围；权限变化不会使旧结果自动获得新的访问权。模板源码不进入普通结果引用、ModelCall、压缩摘要或模型消息。

自有模板在对应 Tool 包中通过 `go:embed` 编译。启动校验源码大小、UTF-8、Runtime Version 和 Hash；源码上限 256 KiB。禁止远程依赖、动态代码下载及 `eval`。展示 Builder 只处理公开结果，移除提交令牌、内部计划和动作控制引用。

## Host 与 Runtime

共享 `@argus/ui` 提供 `ToolPresentationFrame` 和协议宿主；独立 `template-runtime` 应用负责加载模板。企业门户使用部署配置中的 `templateOrigin`，生产构建不得回退到本机或 Mock。

iframe 仅设置 `sandbox="allow-scripts"`，不开放同源、弹窗、表单、导航或下载能力。宿主通过绑定该 iframe 的 MessageChannel 传递上下文；Runtime 校验父窗口对象及父 Origin。协议 `argus-template/v1` 包含 nonce、严格递增 sequence、request_id 和大小上限。销毁后关闭通道，拒绝迟到消息。

CSP 拒绝网络、外部资源、嵌套框架和动态代码。模板仅接收公开详情、Locale、明暗主题和 design tokens。普通业务文本使用 `textContent`，不得把 Tool 数据拼成可执行 HTML。

业务 Bridge 只有三种操作：

| 操作 | 边界 |
| --- | --- |
| `resize` | 宿主限制显示高度，并提供折叠/展开 |
| `open_resource` | 仅允许服务端提供的资源引用 |
| `open_result` | 仅允许服务端提供的结果引用 |

不提供查询、翻页、刷新、换时间、确认或取消消息。Chat 中需要重新取数时，用户发送新消息，产生新 ToolCall。Dashboard 和独立资源页面保留各自业务域，不能借模板 Bridge 绕过授权 API。

## 单次确认

PendingAction 控件独立于 iframe，从权威公开预览显示影响范围、风险、确认/取消以及审批和执行状态。模板仅提供详情，不能改写宿主影响范围或决定确认入口。

用户确认后，服务端重新校验主体、AuthorizationVersion、动作状态、资源版本、审批条件和幂等键，由 Action Executor 执行冻结计划。确认不再调用模型，也不增加第二次确认。执行后的自动验证是 Run 上持久化的只读阶段，Worker 恢复不会重新取得变更权限。

模板加载失败只展示详情错误占位。有效公开预览仍由宿主显示；预览不可用或过期时禁用确认，不能从模板重构授权。

## 文件

附件按钮上传真实字节到当前会话 Workspace，展示进度并保存服务器文件引用。上传与代码执行串行；失败不破坏已有目标。容量预检失败保留用户输入和附件。

`workflow.publish_file` 核验 `/workspace` 文件并发布不可变交付对象。下载使用企业/会话授权、Hash、私有缓存策略和 Range，不接受模型提供的任意外部 URL。工作文件后续修改不影响历史交付。

## 验收

验收覆盖桌面 Web、中英文及明暗主题，包括三类越权 Bridge 消息、重放/nonce 校验、模板错误降级、宿主单次确认、审批执行状态，以及真实上传到离线分析再到不可变下载。实际执行结果见 [PlanV5 实施记录](./planv5/implementation-status.md)，不能以设计描述代替验收完成。
