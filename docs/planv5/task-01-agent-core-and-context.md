# Task 01：Agent Core 与上下文重构

2026-09-23：R17 已完成工程、数据库及完整 P5 b 验收，见 [最新修复记录](./fixes-r17-2026-09-23.md)。Gateway 业务发现缺项已补齐，真实模型评测仍未运行，不能将全部任务清单整体标为完成。

2026-09-22：R12/R13 已修复并通过工程、数据库和完整 P5 d，见 [最新修复与验收](./fixes-r12-r13-2026-09-22.md)。真实中文模型评测仍未运行，下方任务清单不能整体视为完成。

## 目标

把现有 Agent Harness 收敛为单循环：模型输出原生 ToolCall，通用调度接口执行核心工具或客户 MCP 直接工具，并追加原生 ToolResult，在完整 Turn 边界压缩。移除自有业务 Tool 启发式选择和 Card 特殊分支。模型总工具数为 `3 + 可选的4 + N`，不限制客户 MCP 必须经过三个元工具。

本 Task 不实现业务 Search、Sandbox 命令或 MCP 传输；先固定 Provider-neutral 消息、跨 Run Context、核心/外接工具组合、能力快照与 Compaction 边界。已确认范围见 [决策记录](./02-confirmed-decisions-and-open-questions.md)。

## 当前实施状态

上述旧问题已实施切换：Loop 使用通用 ModelToolSet，Provider 保留原生调用/结果，实际投影进入请求，历史以 Conversation 跨 Run 组装，Card 特例已删除。运行环境身份固定、SSE 取消和持久租约补齐见 [复核后的记录](./fixes-2026-09-21.md)。2026-09-22 [最新复核](./review-2026-09-22.md) 新增 R06～R08；Preview 恢复、容量阈值与压缩失败终态现已修复，数据库与 P5 c/M4 a 回归通过，见 [最新补齐记录](./fixes-2026-09-22.md)。真实模型评测仍未运行，下方清单不能整体标为完成。

## 交付内容

### P5-A01：Provider-neutral 原生消息

- [ ] 将内部消息建模为 `system | user | assistant | tool`，保留 Provider-neutral ToolCall ID、名称、JSON 参数和 ToolResult。
- [ ] OpenAI Compatible `chat_completions` 与 `responses` Adapter 分别映射到供应商原生协议。
- [ ] 流式 Tool 参数只在完成、JSON 合法、Schema 通过且 Stop Reason 允许时执行。
- [ ] 不再把 ToolResult 包装成虚构用户消息，不把 ToolCall 序列化进普通 Assistant 文本。
- [ ] Provider Adapter 对未知内容块 fail closed，并保存安全诊断。

建议内部类型：

```go
type Message struct {
    Role       Role
    Content    []ContentBlock
    ToolCalls  []ToolCall
    ToolCallID string
}
```

内部类型不能直接复用任一 Provider SDK DTO，避免 ContextAssembler 与供应商协议耦合。

### P5-A02：跨 Run Conversation Context

- [ ] 增加按 `enterprise_id + conversation_id + event_id range` 读取 Conversation Event 的查询。
- [ ] ContextAssembler 从最后一个有效 Snapshot 的切点开始读取事件，而不是只读取当前 Run。
- [ ] 当前 Run 的用户输入、Assistant Delta、ToolCall 和 ToolResult 继续保存真实 Run ID，Context 只在读取时跨 Run 聚合。
- [ ] 已取消、失败和超时 Run 的可见消息仍按明确事件规则进入历史；内部错误正文和私有数据不进入模型。
- [ ] 用户确认、审批、Execution 等服务端事实只以公开投影进入 Context。
- [ ] 恢复上下文时关联持久 Workspace 身份；计算实例重建不虚构文件丢失，已明确删除的工作目录不从旧摘要恢复为存在。

### P5-A03：小内核 Agent Loop

- [ ] 删除 `selectTurnTools` 和按业务名称/英文关键词维护的默认工具集合。
- [ ] 删除 `Loop.Cards`、Card Command 分支和 `card.render` 特殊处理。
- [ ] Agent Loop 接收通用 `ModelToolSet`，由 `CoreToolSet + ExternalMCPToolSet` 组合，不直接依赖业务 Registry、MCP Client 或具体传输。
- [ ] 自有业务工具仅通过三个元工具暴露；客户 MCP Schema 直接提供给模型，不通过 Search/Describe/Invoke。
- [ ] 模型别名确定性映射到来源、连接和上游 Tool，禁止外接同名工具覆盖核心工具或路由到自有隐藏 Commit。
- [ ] 每轮按顺序完成 `assemble → model → tool batch → append → continue`。
- [ ] 默认顺序执行；只有核心执行器明确证明无副作用且无依赖时才允许并行。
- [ ] 保留 Run Lease/Fence、取消、最大 Turn、额度预留、幂等和恢复机制。
- [ ] 模型没有 ToolCall 时正常完成 Run；模型调用不可执行的 Tool 名称时返回受控 Tool Error，不进入任意 Registry 回退。
- [ ] 自有确认后的提交由 Action Executor 确定性完成，执行后 Agent 可只读验证和总结；新的自有变更生成新 Preview。
- [ ] 客户 MCP 读写暂定直接执行，不套自有确认流程；自动只读验证阶段仍必须排除未知副作用或写能力。

### P5-A04：Capability Snapshot

- [ ] 为每个 Run 保存不可变的 `core_tool_schema_revision`、自有 `catalog_revision`、离线 Sandbox 能力、客户 MCP 连接/Tool Schema Hash 与实际模型别名映射。
- [ ] 始终注入三个元工具的 Schema。
- [ ] 只有离线 Sandbox Snapshot 为 `ready` 时注入 `read/bash/edit/write`；客户 MCP 按自己的连接能力直接注入，不依赖自有 Catalog。
- [ ] 本期 ExternalMCPToolSet 仅接入 Remote Streamable HTTP，不依赖离线 Sandbox；stdio 延后，不实现其能力快照或进程路径。按会话显式选择且当前仍获授权/启用的企业连接直接注入所选连接可用工具，不使用全企业全量目录。
- [ ] 会话保存所选连接 ID，新连接不自动加入旧会话；用户修改选择影响后续 Run，服务端撤权/停用在每次执行前立即拦截，不依赖旧 Run Schema 快照放行。
- [ ] 同一次流式 ModelCall 中不动态改变 Tool Schema。
- [ ] Run 恢复时重新校验安全状态；若 Sandbox 已失效，旧调用返回受控错误，下一次新 Run 使用新 Snapshot。
- [ ] Snapshot 只记录能力和版本，不保存 Endpoint、API Key、Secret 或 Profile 私密配置。

本 Task 可以先用假的 Sandbox Capability Provider 完成结构，真实 Probe 在 Task 04 接入。

### P5-A05：ContextAssembler 与 Compaction

- [ ] 修正模型请求，确保实际使用 ContextAssembler 输出的消息集合。
- [ ] 预算分别记录 System、核心 Tool Schema、客户 MCP Schema、Snapshot、Recent Tail 和当前输入 Token。
- [ ] Agent Run 执行前预检完整工具 Schema、必要系统上下文、当前输入/历史及输出预留；若工具集合/必要上下文本身已超模型容量，明确拒绝运行并提示减少连接或换模型，不先调模型/执行业务 Tool。
- [ ] 容量失败不静默丢弃所选工具，不自动切换模型，保留当前输入、上传文件与会话选择；调整连接/模型后重新计算。
- [ ] 上传文件正文不全量塞入 Prompt，Context 记录文件名、路径/受控引用和上传状态，模型通过离线工具读取；文件交付引用作为会话事实保存。
- [ ] 先执行确定性 Tool Result Projection，再判断是否压缩历史。
- [ ] 客户 MCP 只使用文字/结构化结果，不消费 HTML/Dashboard/Presentation Extension；保留原生工具消息顺序及来源。
- [ ] Compaction 只在完整 Turn、ToolCall/ToolResult Batch、PendingAction/Execution Event Group 边界切分。
- [ ] 使用“上一摘要 + 新增待压缩事件”增量生成下一摘要。
- [ ] 服务端从 PostgreSQL 生成活动 `action_ref`、Execution 状态、资源引用和稳定错误事实，不依赖模型摘要维持。
- [ ] Template、完整 UI Data、Secret、私有 Token、RemoteAccessTicket 和 Sandbox 凭据不进入 Compaction 输入。
- [ ] 当前显式激活 Skill 的 Version/Hash/Instruction 作为独立上下文块保留，不被摘要改写或从历史 Tool Result 恢复。
- [ ] Compaction 失败保留原始 Event，使用最后有效 Snapshot 重试；仍超限时返回稳定错误。

### P5-A06：持久化与可观测性

- [ ] ModelCall 保存实际 Projection Hash、Event Range、Snapshot ID、Core Tool Schema Revision 和 Capability Snapshot Hash。
- [ ] ToolCall/ToolResult 保存 Provider Tool Call ID 与 Argus Invocation ID 的映射。
- [ ] 外接调用记录连接身份、原始 Tool 名称、Schema Hash 和执行结果，不冒充自有 PendingAction/Execution。
- [ ] Conversation SSE 继续投影流式文本，但完成状态只在事务提交后发布。
- [ ] 记录输入/输出/缓存 Token、Compaction Token、工具数量和投影大小。
- [ ] 日志和 Trace 不记录完整 Prompt、文件正文、Template 源码或敏感 Tool Result。

## 代码边界

主要修改范围：

```text
internal/agent/
internal/conversation/
internal/integration/modelprovider/
internal/storage/postgres/queries/runtime.sql
internal/storage/postgres/queries/conversation.sql
internal/runtime/
```

`internal/agent` 不得重新引入对 `internal/card`、具体 Host/Kubernetes/Telemetry Tool 或 OpenSandbox Client 的依赖。

## 测试

- [ ] 无 Tool、单 Tool、连续多 Tool、多轮 Tool 的原生消息顺序测试。
- [ ] Chat Completions 与 Responses Adapter 生成等价内部语义。
- [ ] Tool 参数截断、非法 JSON、内容过滤、取消和未知 Tool 均不执行。
- [ ] 两条用户消息创建两个 Run 后，第二个 Run 可以读取第一 Run 的 Assistant 和 Tool 历史。
- [ ] ContextProjection 确实成为 Provider Request，而不是只用于 Token 统计。
- [ ] Compaction 不拆分 Tool Batch、PendingAction 或 Execution 事件组。
- [ ] Redis 清空和 Worker 重启后可以从 PostgreSQL Snapshot/Event 恢复。
- [ ] Template 标记、私有 Token 和 Secret Fixture 在模型请求与摘要中不可检出。
- [ ] 同一 Event Range、Snapshot 和 Capability Snapshot 生成稳定 Projection Hash。
- [ ] 模型工具集合确为 `3 + 可选的4 + N`；外接调用不经过三个元工具，同名/近似名无法覆盖自有工具。
- [ ] 会话连接选择跨刷新/Run 保存，未授权连接不能注入；管理员新增连接不影响旧会话，撤权/停用后的旧工具调用被拒绝。
- [ ] Schema 超容量在运行前给出明确诊断，无模型/业务 Tool 执行、无静默裁工具或模型回退；减少连接或显式换模型后可重新运行。
- [ ] 已上传业务文件在多轮 Context 可定位，模型生成文件的交付引用在刷新和跨 Run 后仍可使用，不泄漏存储或容器内部路径。
- [ ] 自有提交无需模型推理，执行完成后可验证/总结；验证阶段不调用客户 MCP 写工具。

## 完成标准

1. Agent 模型可见工具不再由业务关键词选择器生成。
2. Provider 收到的是 ContextAssembler 最终投影和原生 Tool 消息。
3. Conversation Context 可以跨 Run 连续恢复。
4. Card 创建和 `card.render` 不再位于 Agent Loop。
5. 核心三/七工具与外接 N 个直接工具的组合和身份路由已固定，Task 02/04 可独立实现对应提供方。
6. 单元、数据库集成和双 Provider Adapter 测试通过。
