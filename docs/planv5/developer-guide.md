# PlanV5 开发手册

## 原生工具

业务能力由 `internal/toolgateway` 和对应业务包注册到自有 Registry。模型只获得 `tool.search`、`tool.describe`、`tool.invoke`；Provider 边界转换为合法函数名。分类固定为 `host/k8s/metric/trace/log/connector/workflow`。

Manifest 必须提供稳定 ID、输入输出版本、严格 Input Schema、风险和权限。每个模型可见的自有工具还须在所属业务包提供 `mcp.Discovery`，包含中英文业务标题、用途、关键词、结果语义、前件和意图/参数获取示例；禁止由 Tool ID 临时拼接代替。查询执行时重新检查当前主体和资源范围，不能信任参数中的企业、用户或调用方身份。Search 默认 10 项、最大 20 项；缓存包含企业、用户、权限版本、权限集合、CatalogRevision 和查询参数。

变更返回 PendingAction 的公开预览，私有计划由 Action Executor 使用。隐藏 Commit 不接受新业务参数，也不进入模型目录。平台下发的实施版本通常是 backend 镜像 Digest，它参与 CatalogRevision；部署后旧 Run 不会静默调用新版 Handler。

新增展示在 Tool 同包添加 `templates/*.html`，创建 `TemplateAsset`，并提供公开结果到展示数据的 Builder。参考 `internal/toolgateway/presentation.go` 和 `internal/telemetry/presentation.go`。模板不能持有确认能力、凭据或 API Client。

## 客户 MCP

管理入口 `/settings/mcp` 使用 `/api/v1/enterprise/mcp-connections`。创建/编辑提交 `name/endpoint/auth_type/member_ids`，凭据仅通过 `credential_value` 写入。编辑、启停和成员更新使用 `expected_version`。只有企业管理员可管理；平台管理员不代管业务凭据。

首期固定 Remote Streamable HTTP 与协议 `2025-11-25`，支持无认证、Bearer 和 Basic。initialize、分页 tools/list、JSON/SSE、Session、取消和有界恢复由 `internal/integration/remotemcp` 负责。Schema 快照与连接修订不可变；每次发送前重新校验启停、成员授权、配置与凭据修订。

新工具集合在预检时重新发现所选连接的当前目录；不能依靠旧缓存隐藏新增工具。恢复旧 Run 时必须与固定快照比较，变化则明确失败。派发状态在业务 HTTP 请求发送前通过任务 fence 与数据库条件更新确认，initialize/tools-list 不会提前将业务调用标为 dispatched。

`ConversationUpdate.selected_mcp_connection_ids` 保存显式选择。所选工具直接提供给模型，不进入自有 Registry、不要求自有分类，不接入模板。外接调用状态为 `prepared/dispatched/succeeded/failed/result_unknown/cancelled`；已发送而无终态的调用不自动重发。

## 文件与 Workspace

每会话独立 PVC，固定挂载 `/workspace`。平台设置默认 2 GiB、企业预留池 20 GiB，单次上传/交付默认 100 MiB。Enterprise 或模型不能扩大平台配置。RawFile ext4 thick 文件系统提供实际容量上限。

文件 API 不要求 Run 或 ToolCall：

1. `POST /conversations/{id}/workspace/uploads` 提交文件名和字节数。
2. `PUT /conversations/{id}/workspace/uploads/{upload_id}/content` 以 `application/octet-stream` 流式提交内容。
3. 用返回的文件 ID 写入 `MessageCreate.file_ids`。文件路径和上传记录由服务端提供。
4. 生成文件后通过自有 `workflow.publish_file` 发布；从会话交付接口下载。

上传采用同卷临时文件、大小校验、SHA-256、fsync 和原子重命名。工作目录保留最新状态，不提供逐次修改回滚。`workspace-io` 是同一 worker 二进制的固定文件 RPC 角色，使用 mTLS，不加载模型、数据库或客户业务凭据。

受控导入先将 `result_ref` 登记到 Workspace 的持久来源集合，再写入 PVC；写入或文件元数据提交中断也保留来源。任意代码可复制或改名，因此来源授权约束整个目录。挂载、热复用、输出返回和文件流读取均检查当前授权；活动访问每秒复查，撤权取消 RPC 并进入原有写入者撤销流程。失去来源授权只拒绝访问，不自动删除目录，显式删除仍可执行。不可变交付保存发布时的来源集合，下载重新鉴权。

所有文件变更和代码执行由 PostgreSQL lease/fence 串行仲裁。健康实例在空闲窗口内复用：先取消并排空旧文件 RPC、清理旧用户进程，再绑定递增 fence。实例失效、配置变化或撤销失败时，物理回收旧 Pod 后才允许重新挂载；不得使用 Redis TTL 或 RWO 当作互斥证明。关闭页面、归档、结束 Run、闲置和计算实例回收不清理文件。

进入 Workspace 时在会话锁内复核来源：模型操作绑定仍有效的 Run，上传/读取绑定原文件记录的 Workspace ID。显式删除与迟到请求竞争时，旧操作不能在删除完成后自动分配替代工作区。用户正常文件请求仍不要求 Run/ToolCall。

## 上下文扩展与恢复

动作状态变化使用 `conversation.ScheduleActionRun` 与领域写入共同提交；确认/拒绝/失效/到期/完成都必须能够唤醒 Run。正在运行的 Agent 可能已读取旧状态，所以通知仅与 pending 任务合并，不能因存在 running 任务而丢弃。到期处理只针对尚未派发的动作。

Run 的公开查询与变更按当前企业、Run actor、Conversation owner 和未删除状态统一鉴权，变更事务中再次核验；不得只依靠 `conversation.read/use` 路由权限。幂等结果也不能绕过后续会话删除。

预检和实际 Provider 请求共用 `ContextFacts`，每次读取当前 Workspace ID/状态，不能从旧摘要推断目录仍可用。附件按原文件 ID/Workspace ID 重新投影；失效引用去除路径，同名新文件不会重新绑定旧引用。历史事件保留原值。压缩使用 v2 Prompt 和同一份目录事实，快照记录其所用状态；模型必须读取实际文件确认最新内容。

Run 的动作阶段由绑定的 PendingAction/Execution 恢复。在完整工具批次持久化后立即发布幂等的宿主确认引用并暂停，不能将“模型再次总结”作为进入等待状态的条件；未完成批次先补齐原生工具配对。已确认操作只由 Action Executor 提交，完成后恢复 `verification_only`，包括在非 `execution_verify` 原因下恢复的任务。取消多个 Preview 中的一个不能清除其他动作。

容量门禁与 Agent/Compactor 共用 `modelprovider.ContextBudget`。`usable_tokens` 是减去输出预留和安全余量的物理预算，运行硬阈值为其 85%；固定输入必须严格小于该阈值，容量错误的 `input_limit_tokens` 给出可接受的最大估算值。工具和当前输入自身超限时直接拒绝创建 Run；历史压缩不能掩盖固定输入超限。

Run 终态统一通过 `conversation.FinishRunRecord` 在持有 Run 行锁的事务中写入，并追加同一事务的 `run_state_changed`。调用者保留 Task Lease/Fence；不能只更新 `runs` 后期待 SSE 自动补发事件。完整恢复与并发终态测试需要隔离的 PostgreSQL，跨包数据库测试使用 `go test -p 1` 避免测试夹具竞争全局任务队列。

`SkillContextSource` 仅提供 `argus.skill_context/v1` 的 ID、Revision、Text 和内容 Hash。上下文固定在 Run 快照中，并经过预算检查，不注册工具、不扩大权限。本期不提供 Skill 编辑器或 Marketplace。

ConversationEvent 是跨 Run 的事实来源；ContextSnapshot 归属于 Conversation，以版本条件更新避免覆盖。压缩先使用模型投影，再按完整调用组做增量摘要。Provider 发送的实际 JSON Hash、工具快照和能力快照保存在 ModelCall。模型迭代上限 24 次按 Run 持久计数。

推理和压缩共用 `TokenUsage`：输入、输出分别记录 `provider/estimated/missing/invalid`，字段缺失与 Provider 明确返回零不同。所有 Chat 流式请求都请求 usage；重复实报按累计值覆盖，不重复相加。估算只用于保守记录与额度结算，结算记录保留其来源。公开用量的 `input_tokens/output_tokens` 仅合计实报，估算单独返回；金额包含估算时页面显示用量不完整。压缩没有实报输出时不填写伪造的实际摘要 Token，事件中的字节估算使用 `estimated_tokens_after`。

模型输出事件记录生成时的 `authorization_scope`；摘要的 TypedCheckpoint 保存同一授权范围和执行检查点。权限或资源授权变化后，旧摘要不进入下一次上下文，旧模型输出、调用参数和结果被替换为不可用说明，同时保留原生调用 ID 的配对。增量压缩使用同样的过滤规则，并将授权范围纳入 SourceHash，防止撤权数据经旧摘要再次进入模型。

Workspace admission 独立校验挂载来源和镜像：仅接受所属 PVC、临时卷以及固定文件角色专用 TLS 卷，移除默认 ServiceAccount 挂载，拒绝额外凭据环境引用、外部文件系统和文件角色自定义入口。不能只依赖上游模板中声明的 SecurityContext。

Workspace 命令由离线镜像内的 `argus-workspace-supervisor` 提供 mTLS RPC（8448），固定文件角色仍只提供文件 RPC（8447）。Supervisor 使用 UID 10001 和 SETUID/SETGID/KILL 三个管理能力，用户命令使用每 Workspace 独立 UID（>=100000）、清空全部 capabilities 并设置 no_new_privs。管理证书在 Supervisor 所有的 0700 父目录下，命令环境不继承管理配置；每次命令终止后按 UID/pidfd 清理脱离进程组的后台进程。取消、失联或清理失败按结果未知处理并禁止自动重发。OpenSandbox 保留生命周期管理及固定上游镜像版本，Workspace 不再通过 Execd 的任意命令接口派发用户代码。

## 修改与生成

MCP 返回数据在 Remote 传输解析入口检查本次已知认证值，命中则拒绝整个信封。不能只对字段名脱敏，也不能改写 Schema 的默认值/枚举。已发送的调用失去可用结果时维持未知结果语义，不自动重发；错误诊断不携带被拦截正文。目录和结果共用 JSON/SSE 检查路径。

`ContextSource` 与组装消息绑定；ModelCall 的摘要 ID/Hash 和事件范围来自该来源，禁止事后读取当前活动摘要推断。缓存输入为输入总量的子集，单独保存实报/缺失标识；缺失不等于零，缓存统计不能重复累计到输入 Token 或自动推导折扣金额。

先改 OpenAPI/JSON Schema/protobuf 和数据库权威基线，再生成：

```powershell
go run ./cmd/argus-dev contracts generate
go run ./cmd/argus-dev repo sqlc
go tool buf generate api/proto --template api/proto/buf.gen.yaml
```

SQLC 命令会运行声明级拆分器，使生成的 Go 文件保持在 2000 行内。如果直接运行 `go tool sqlc generate`，随后运行 `go run ./scripts/sqlc-split`。

验证继续使用 Go、Vitest、Playwright、Helm 和 `argus-dev e2e run --suite p5`。P5 串行覆盖 agent-lite、agent-sandbox，以及两者的无 MCP/Remote MCP 情况；M7/M10 使用 P5 原生展示和恢复回归替代旧展示依赖。门禁状态及临时资源见 [实施记录](./implementation-status.md)。

异常接管边界：只有正常释放租约的 Pod 可以热复用。领取使用观察到的 fence 做条件更新，过期 owner 的实例必须物理撤销后重建。旧 owner 收尾前再次核验并延长当前租约；已经失去租约时不再执行 Kubernetes 回收，避免晚到清理误删新 owner 的实例。

### 多次用量更新的一致性

流结束后统一校验聚合值：缓存输入必须有 Provider 实报输入总量且不超过它，已知计数不能为负。重复更新按字段替换；允许先报缓存、后报输入或后续修正，不能只验证同一帧。若最终矛盾，推理和压缩返回 `MODEL_USAGE_INVALID`，不执行该响应中的工具，不自动重试；压缩不发布新摘要。硬限制压缩结束等待中的 Run，手动/软限制压缩只使压缩任务永久失败。

合法解析的非负观察值保留作诊断，但输入、输出和缓存来源同时标为 `invalid`，调用只能是失败/取消状态。数据库禁止将缓存大于输入的值写成 Provider 实报；公开统计和评测不累计 invalid 计数，完整性为 false。额度保留调用前预留估算并标记 invalid，不使用矛盾计数重新算金额；不推断供应商的实际账单。

### 自有发现元数据

资源和 Preview 元数据与 `toolgateway` 工具同包，遥测元数据在 `telemetry` 包，文件导入/发布元数据在 `workspace` 包。Gateway 启动时按版本化 JSON Schema 校验，缺少元数据、空关键词、空示例或超限均拒绝启动。隐藏 Commit 不需要面向模型的描述，也不能进入目录。

关键词大小写归一、去重和排序后计算内容 Hash；业务说明、前件、示例、结果语义的变化同样改变 Manifest Version 和 CatalogRevision。代码实现版本仍参与目录身份。旧 Run 与旧游标不能静默使用新版目录。元数据校验会复制切片，调用方修改注册输入不能改变已经建立的目录。

Search 在名称、标题、描述和关键词中确定性评分；空白分隔的所有查询词都须命中，结果按得分和规范名称稳定排序。英文按词边界匹配，`install` 不匹配 `uninstall`；中文按已维护短语匹配，常用无空格短语应在工具关键词中明确维护。它不是任意中文自然语言分词器，不在 Agent 内恢复业务关键词选择器。Search 摘要最多 256 个 Unicode 字符；Describe 以 `argus.tool_manifest/v1` 返回完整业务说明、严格输入 Schema、权限与风险，以及元数据 Hash。示例解释业务意图和参数获取方式，不提供可能被误用的虚构生产资源 ID。

新增工具须补充目录覆盖、中英文词与同义词、权限/缓存、Manifest 变更和 Schema 预算回归。P5 的 `p5-business-discovery.json` 通过真实 Agent/持久化结果验证七类工具的九组检索与描述，不能替代真实模型评测。
