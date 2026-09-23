# Task 03：Tool 自带模板与通用 Template Host

## 目标

让每个需要富展示的 Argus 自有 Tool 保存模板，并在结果中返回 `template + presentation data`。前端只保留一个通用、安全、版本化的 Template Host。Agent、Tool Discovery 和 Compaction 不感知模板内容。

客户 MCP 仅以文字/结构化数据作为模型工具结果，不接入 Dashboard、HTML 或 Presentation Extension。本 Task 只覆盖自有 Tool 模板。翻页/换时间等重新查询必须由用户再发消息，产生新 Tool Call。参见 [已确认决策](./02-confirmed-decisions-and-open-questions.md)。

本 Task 不保留 Card Catalog、模板组件库、Slot Binding、Render Plan 或用户创建模板能力。

自有模板只展示业务详情；最终影响范围、确认/取消按钮以及审批执行状态由宿主通用 PendingAction 控件提供，用户只确认一次。该控件独立于模板 iframe，模板样式和加载状态不拥有最终确认入口。

## 当前实施状态

统一 Template Host/Runtime、双受众结果、不可变模板、六类查询和宿主单次确认已实现。正式 Preview 已扩展到 27 项，并补齐公开业务详情及来源 Run；新增业务模型入口正在集群验收。已有模板主题/语言/安全矩阵和最新文件/SSE 浏览器结果见 [验收报告](./acceptance-report.md) 与 [补齐记录](./fixes-2026-09-21.md)，不能用旧样本覆盖后续代码变化。

## 交付内容

### P5-P01：Tool Result 双受众 Envelope

- [ ] 固化 `model_result` 与 `presentation` 的版本化 JSON Schema。
- [ ] 两个投影共享 `tool_call_id/category/name/version/status`。
- [ ] 自有 Preview 的公开动作引用与权威影响范围由宿主消费，绑定同一 ToolCall/PendingAction；iframe 仅接收详情数据，不获取确认能力。
- [ ] Agent Event Sink 只把 `model_result` 转为原生 ToolResult Message。
- [ ] Conversation SSE 使用独立 `tool_presentation` 事件发送界面投影。
- [ ] PostgreSQL 保存 Presentation Metadata；Template Source 和大 Data 作为不可变 Artifact 保存一次。
- [ ] 历史消息按当次 Template Hash/Artifact 渲染，不读取 Tool 最新模板替换历史结果。
- [ ] Template Source 在 ModelCall Prompt、Compaction、Assistant Text 和普通 Tool Trace 中不可检出。

### P5-P02：Tool 内模板资产

- [ ] Native Tool Manifest 声明模板 Asset、Runtime Version、Template Version 和预期 Hash。
- [ ] 使用 `go:embed` 或等价构建机制把模板随 Tool 发布物编译。
- [ ] 启动时校验模板大小、编码、Runtime Version、CSP 能力和 Hash。
- [ ] Tool Handler 只产生业务结果；同包 Presentation Builder 将结果转换为 UI Data。
- [ ] 不需要富界面的 Tool 可以没有模板并降级为普通 Tool Trace。
- [ ] Template 未压缩源码上限 256 KiB；禁止运行时下载模板或依赖未锁定远程资源。
- [ ] 前端未来可以按 Hash 缓存模板，但服务端 Tool 仍是唯一来源。
- [ ] 不定义或适配客户 MCP Presentation Extension；客户 MCP 的结果不会进入 Template Host。
- [ ] 客户 MCP 即使返回 HTML/Dashboard 等展示元数据，也只消费支持的文字/结构化数据，不执行代码，不影响独立调用结果记录。

建议目录：

```text
internal/tools/<category>/<tool>/
├── manifest.go
├── handler.go
├── projector.go
├── presentation.go
├── templates/*.html
└── contract_test.go
```

### P5-P03：Template Runtime Protocol

- [ ] 定义 `argus-template/v1` Host/iframe 消息 Schema。
- [ ] Host 创建独立 Origin 或严格 sandbox iframe，并完成 nonce + `MessageChannel` 握手。
- [ ] 模板只获得详情 `presentation data`、Locale、Color Scheme、Design Tokens 和结果/资源公开 Ref；确认能力与宿主动作投影不注入 iframe。
- [ ] Bridge 消息包含 Version、Sequence、Request ID 和大小限制。
- [ ] 默认 CSP 禁止网络、宿主导航、弹窗、下载、表单外送、Cookie 和存储访问。
- [ ] 禁止 `eval`、动态代码下载和从 Tool Data 构造可执行脚本。
- [ ] iframe 销毁后拒绝迟到消息，错误 Origin、Nonce、Sequence 和 Schema 全部 fail closed。

### P5-P04：前端通用 Host

- [ ] 用一个 `ToolPresentationFrame` 替换业务 Card Frame。
- [ ] Frame 只认识 Runtime Protocol，不包含 metric/log/trace/host 等 Tool 分支。
- [ ] 注入 `@argus/design-tokens` 的颜色、字体、间距、圆角和状态语义，模板禁止硬编码全局主题。
- [ ] 处理 Loading、Runtime Error、Rejected、Collapsed/Expanded 和无模板文本降级状态。
- [ ] 使用 `ResizeObserver` 和受限 Bridge 自动更新高度，设置最大折叠高度。
- [ ] 提供键盘焦点、屏幕阅读器标题、错误说明和 Reduced Motion 基线。
- [ ] 前端正式桌面 Web 验收，不增加移动端特有逻辑。

Host 可以位于共享运行时包，但该包只能包含安全协议和通用壳，不得形成业务模板或组件 Catalog。

### P5-P05：最小展示 Bridge 与宿主动作控件

- [ ] iframe 第一版只允许 `resize/open_resource/open_result`，明确拒绝 `confirm_action/cancel_action`，也不允许模板通过其他消息改写宿主影响范围或触发提交。
- [ ] 宿主固定控件从服务端公开动作投影显示影响范围、确认/取消按钮及审批执行状态，点击时使用 `action_ref + request_id`。
- [ ] 宿主使用当前登录身份调用固定 PendingAction API，Template 不获得 API Client 或 Authorization Header。
- [ ] 服务端重新检查动作主体、状态、AuthorizationVersion、资源版本和幂等键。
- [ ] Template 不获得 Commit Tool、`argus__token`、冻结参数、Approval 内部状态或一次性结果正文。
- [ ] 不开放任意 `tools/call`、任意 HTTP Request 或动态 Query Binding。
- [ ] 不开放 `refresh_ref` 或服务端分页 Bridge；翻页/换时间必须由新消息触发新 Tool Call。当前已加载数据的本地折叠/展开不取新数据。

最终确认按钮只属于宿主固定控件，用户只确认一次，不增加第二次确认。模板加载失败时显示详情错误占位，宿主仍使用完整有效的服务端公开预览显示动作；公开预览缺失/失效时禁用确认，不由模板补造。客户 MCP 不使用这套确认。自有 Execution 完成后的只读验证/总结继续由后端事件驱动，不让模板触发任意模型或 Tool。

### P5-P06：首批模板迁移

至少迁移：

- [ ] `host.list/get`：主机摘要和状态表格。
- [ ] `k8s` 查询：工作负载/Pod 状态摘要。
- [ ] `metric` 查询：时间序列和异常区间。
- [ ] `log` 查询：时间、级别和代表日志行。
- [ ] `trace` 查询：调用链和慢 Span 摘要。
- [ ] `connector` 查询：连接状态和版本摘要。
- [ ] 一个变更 Preview Tool：模板展示业务详情，宿主展示权威公开影响范围、确认/取消与审批执行状态。

每个模板的数据结构由同 Tool 的 Presentation Builder 决定，不经过平台字段 Slot 映射。

### P5-P07：历史与缓存

- [ ] Presentation Artifact 内容寻址并记录 SHA-256、Media Type、Runtime Version 和 Tool Version。
- [ ] 同一模板可以跨结果去重存储，但 Tool Result 始终保存明确 Hash/Version 引用。
- [ ] SSE 第一版可以总是内联模板；前端缓存命中优化不得改变结果语义。
- [ ] Artifact 缺失或校验失败时显示安全错误占位，不使用 Tool 当前模板重放旧结果。
- [ ] 企业停用或权限撤销后，历史 Presentation 按现有会话授权重新校验；模板本身不能恢复已撤销数据。

## 安全测试

- [ ] 模板尝试读取 `window.parent.document`、Cookie、localStorage 和 IndexedDB 均失败。
- [ ] 模板尝试 `fetch/WebSocket/EventSource/sendBeacon` 均被 CSP/Host 阻止。
- [ ] 错误 Origin、Nonce、乱序、重复、大消息和销毁后消息被拒绝。
- [ ] Template Data 中的 HTML/脚本字符串不能突破渲染边界。
- [ ] 模板不能自行调用任何 Tool、Commit 或 PendingAction API，也不能通过 Bridge 触发确认/取消。
- [ ] 模板加载时主动发确认消息或伪造风险/影响范围，不触发提交且不改写宿主公开预览。
- [ ] Browser DOM、Network、Console、SSE Fixture 和模型请求中搜索不到私有 Token/参数。
- [ ] Template Hash 不一致、Runtime Version 不支持和超大小时 fail closed。

## 浏览器与 E2E

- [ ] Light/Dark、中文/英文、空数据、错误、部分数据、大数据八类场景。
- [ ] 六类查询模板和一个 Preview 模板真实渲染。
- [ ] 自动高度、折叠/展开、刷新页面和历史回放。
- [ ] 宿主统一确认入口覆盖确认一次、取消、双击幂等、过期、审批等待与执行状态更新。
- [ ] 模板加载失败不影响已取得有效公开预览的宿主动作展示；公开预览缺失/失效时确认禁用，不增加第二次确认。
- [ ] 权限撤销后旧页面刷新不继续展示已撤销数据。
- [ ] 重新查询依赖新消息，旧结果保留其查询时间/参数来源，不把新数据静默覆盖到旧 ToolCall。
- [ ] 客户 MCP 结果只展示普通 Trace；不会因为含有模板字段而创建 iframe。
- [ ] 不存在交互卡片设置页、创建命令或模板选择入口。

## 完成标准

1. 所有正式模板资产属于具体自有 Tool 包，客户 MCP 不参与模板运行时。
2. Agent 请求和上下文中没有 Template Source。
3. 前端只有一个无业务语义的 Template Host。
4. Query 与 Preview Tool 都能通过同一 Envelope 渲染。
5. PendingAction 安全链保持不变，模板只展示详情，宿主统一提供影响范围、确认/取消及审批执行状态，用户只确认一次。
6. Card Slot、Binding、Render Plan 和模板 Catalog 不再参与任何正式运行路径。
