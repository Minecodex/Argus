# Task 04：离线 Sandbox、持久 Workspace 与客户 MCP 直接模型工具

2026-09-23：R14/R15 已完成工程、数据库及完整 P5 b 验收，见 [最新修复记录](./fixes-r14-r15-2026-09-23.md)。真实模型评测仍未运行，不能将全部任务清单整体标为完成。

2026-09-22：R12/R13 已修复并通过工程、数据库和完整 P5 d，见 [最新修复与验收](./fixes-r12-r13-2026-09-22.md)。来源约束整个持久目录和派生文件；真实中文模型评测仍未运行，不能将任务清单整体标为完成。

## 目标

把 OpenSandbox 扩展为可回收的计算运行时，并为每个会话建立独立持久 Workspace。`read/bash/edit/write` 在无网络 Sandbox 中自由执行，计算实例回收不删除工作目录。

首期客户 MCP 只支持 Remote Streamable HTTP，各个 Schema 直接注册为模型工具，不经过自有 `tool.search/describe/invoke`，不要求自有分类、Preview/Commit 或模板。服务端 Adapter 调用客户远程服务，读写暂可直接执行，仅使用文字/结构化结果。stdio 和旧双端点 HTTP+SSE 延后。参见 [已确认决策](./02-confirmed-decisions-and-open-questions.md)。

企业管理员管理企业连接/凭据并授权成员，用户在会话中显式选择已授权连接并保存选择；新连接不自动加入旧会话，撤权/停用立即阻止执行。所选工具超模型容量时运行前明确提示减少连接/换模型，不静默裁工具或自动换模型。离线环境使用预装语言/分析库，支持业务数据上传到会话 Workspace 与模型文件产物下载。

## 当前实施状态

当前已实现类型化 PVC 挂载、独立 Workspace 状态机、固定文件 RPC、离线 Supervisor、四基础工具、Remote MCP 和附件交付。Supervisor 与文件角色使用 mTLS、租约/fence 和独立用户 UID；健康且正常释放的实例可复用，异常过期 owner 的 Pod 必须物理撤销后重建。最新证据见 [补齐记录](./fixes-2026-09-21.md)。

下方复选框保留完整任务范围，不能因局部门禁通过就整体勾选。P5 g 已通过执行中崩溃、Sandbox API 中断、Profile 变更、空闲回收、Worker 强制退出与旧 owner 接管验收；2026-09-22 [最新复核](./review-2026-09-22.md) 的 R07 容量边界问题已由 [最新修复](./fixes-2026-09-22.md) 关闭，最新 P5 c 再次通过上述 Workspace 和 MCP 门禁。

目录使用 RawFile LocalPV 的独立定长文件系统，交付内容使用私有对象存储；单项 Tool Artifact 上限不再承担整个目录配额。固定版本的真实挂载、重挂载、直接写满和不可变交付均已有 Kubernetes 证据。真实中文模型使用这些能力的评测仍需实际配置。

## 交付内容

### P5-S01：OpenSandbox 可选配置

- [ ] Helm/Config Schema 允许显式关闭或不配置 OpenSandbox。
- [ ] Agent Worker Readiness 不依赖 OpenSandbox；Sandbox 专用能力单独报告状态和原因。
- [ ] 未配置时不创建 Sandbox Backend/Profile 默认对象，也不持续产生错误重试任务。
- [ ] 已配置但不健康时只暂停 Sandbox 类调用，不影响 Native Tool、模型调用和远程 MCP。
- [ ] 分别报告离线 Workspace Runtime、文件存储/传输与 Remote MCP 连接能力，不能因 Sandbox 可用而授予离线代码网络权限；本期不增加 stdio Runtime。
- [ ] Platform 管理页显示 `not_configured/unhealthy/ready`，企业工作台不获得 Backend/Profile 选择权。
- [ ] 用户/模型不能通过会话请求临时配置 OpenSandbox Backend Endpoint、镜像或网络；企业管理员维护客户 MCP 连接使用独立管理 API，不等于获得 Sandbox 基座管理权。

### P5-S02：持久 Workspace 与计算实例分离

- [ ] 增加固定 `agent_workspace` Task Kind/Profile 契约。
- [ ] 离线 Profile 固定批准镜像、CPU、内存、临时磁盘、PID、无网络策略和计算实例 TTL；镜像预装常用语言/分析库，不增加用户上传依赖包的安装流程。
- [ ] 新增 Workspace 记录，保存 `enterprise_id + conversation_id + workspace_id`、持久存储引用、容量、状态和环境版本，不以一次 ToolCall 或上游 Session 作为目录身份。
- [ ] 同一会话的 Run 复用完整工作目录最新状态；实例可重新附着原持久存储，并发写入通过 Lease/Fence 协调。
- [ ] 关闭页面、结束 Run、长期未使用、归档和实例 TTL 到期只允许回收计算，不删除持久目录。
- [ ] 仅明确删除 Workspace 或永久删除会话才清理目录；删除必须先撤销/隔离旧实例写入权，再回收计算和存储，不能让迟到调用复活被删除数据。
- [ ] 第一版不承诺文件逐次修改回滚；覆盖/删除改变当前文件树，不从 ConversationEvent 自动重演脚本来恢复文件。
- [ ] 持久化范围覆盖完整 Workspace 目录；进程内存、连接和容器系统盘不因持久卷自动保存。用户脚本和分析文件保存在工作目录，语言及分析依赖由固定预装镜像提供。
- [ ] 达到存储额度时停止新增写入，不自动删除旧文件；`write/edit/bash` 的所有写入路径都受存储层额度约束，不能只在工具 API 入口检查。
- [ ] 配额分别统计持久字节、计算实例/活跃秒数、命令与文件传输；读操作与明确释放空间不因满额被一并禁止。

### P5-S03：Sandbox Tool Proxy

- [ ] 定义与具体 OpenSandbox SDK 解耦的 `WorkspaceRuntime` 接口。
- [ ] 实现 `EnsureWorkspace/Attach/Exec/Read/Write/Edit/StartProcess/Send/Cancel/ReleaseCompute/DeleteWorkspace` 等职责分离接口；实例释放与目录删除不能共用一个模糊 Terminate 行为。
- [ ] Agent 执行调用带企业、会话、Run、ToolCall、Workspace ID、计算实例代次、Deadline 和预算；用户文件传输/工作区管理绑定当前主体、企业/会话/Workspace 和请求 ID，不要求预先创建 Run/ToolCall 或启动计算实例。
- [ ] 实现路径规范化、Root 限制、Symlink/Traversal 防护和文件大小限制。
- [ ] 实现 stdout/stderr 分离、输出截断、退出码、超时和取消。
- [ ] 实现进程树与计算实例回收，不依赖 Worker 本地 PID 作为唯一事实；持久 Workspace 删除使用独立的明确生命周期请求。
- [ ] OpenSandbox API 不支持所需能力时，在批准镜像中运行受控 Supervisor，并通过隔离内部通道通信。
- [ ] 持久卷名称、挂载路径和归属由服务端生成校验，不允许模型指定任意 PVC 或 Worker 宿主路径。
- [ ] 增加受控结果导入目录、业务文件上传与目录文件导出接口；大文件使用流式大小限制和授权检查，不把整个 Workspace 塞入单项 Tool Artifact，会话提供真实上传/产物下载入口。
- [ ] 离线 Runtime 的管理/文件通道不可被用户代码当作联网代理，MCP 凭据不能通过导入或持久目录暴露给四基础工具。

### P5-S04：四个基础工具

- [ ] `read`：只读 Workspace Root 下文件，支持行范围和大小上限。
- [ ] `write`：原子创建/覆盖文件，父目录策略明确，拒绝设备/特殊文件。
- [ ] `edit`：基于精确旧文本或补丁应用，匹配歧义时拒绝，不静默选择。
- [ ] `bash`：允许在离线 Sandbox 自由编写/执行程序，固定工作目录、超时、输出上限和取消；不是固定业务命令白名单，也不走 Preview/Commit。
- [ ] 四个工具全部通过 SandboxBuiltinExecutor，不接触 Worker 宿主 `os/exec` 或文件系统。
- [ ] Tool Result 使用原生 Tool Message；大输出进入 Artifact/Projection。
- [ ] Snapshot 非 `ready` 时四个 Schema 不进入模型请求。
- [ ] `read/write/edit/bash` 均无外部网络能力，不能通过命令、包管理器或共享 MCP 执行环境获得联网权限。

### P5-S05：Sandbox Supervisor

- [ ] Supervisor 使用版本化内部协议并仅监听 Sandbox 隔离网络/Channel。
- [ ] 离线 Supervisor 只管理四工具进程与文件，不承载客户 MCP；本期不实现联网 stdio Supervisor。
- [ ] 保存可重建的进程元数据；Worker 重启后可以对账或明确终止失联进程。
- [ ] stdout 采用背压和上限，stderr 作为独立受控诊断，不混入 MCP JSON-RPC。
- [ ] 计算实例结束时终止其进程并清理临时 Secret/非持久运行文件；保留 Workspace 工作目录。只有明确删除工作区时才删除持久文件。
- [ ] Supervisor 镜像与 Agent Workspace 镜像使用批准 Digest 和供应链清单。

### P5-S06：后续 stdio MCP（不在本期交付）

stdio 的连接表单、程序启动、联网 Sandbox、协议适配和故障测试全部延后，不作为本期完成门禁。未来另行设计独立 Sandbox 中的进程/凭据/网络隔离；不得回退到 Worker 宿主或给离线四工具开网。本期只需确保不支持的传输不会被保存/展示为可用连接。

### P5-S07：Remote MCP Adapter 边界

本期唯一客户 MCP 传输为 Remote Streamable HTTP；配置入口为企业管理页，会话选择器显式启用：

- [ ] 使用服务端 Streamable HTTP Client，支持 JSON 与可选 SSE 响应。
- [ ] 不提供旧双端点 HTTP+SSE 的配置或自动降级；Streamable HTTP 自身的 SSE 响应仍属于本期支持。
- [ ] 提供模型 Schema 前完成 `initialize/tools/list` 或取得有效版本快照，规范化别名、保存 Schema Hash 并对账变化；不能省略协议发现步骤。
- [ ] Endpoint 来自服务端保存的客户连接配置，按连接认证与平台网络隔离规则访问；不能由模型参数替换或取得平台内部服务身份。
- [ ] 支持 MCP Session ID、取消、超时、重连和结果大小限制。
- [ ] 会话调用/Template/模型参数不能替换连接 Endpoint 或认证 Header；企业管理员的连接配置表单经独立管理 API 保存地址和只写凭据。
- [ ] 以上限制只保护连接配置，不误禁客户工具自身 Schema 合法定义的 URL、命令等业务参数。
- [ ] 远程 Tool 按连接身份直接注册给模型，不映射自有 `category + name`；读写暂可直接执行，结果只有文字/结构化投影。
- [ ] 不适配 Remote MCP Presentation Extension，不渲染其 HTML/Dashboard，不创建 Template iframe。
- [ ] Remote MCP 的网络调用不依赖离线 Workspace 或 OpenSandbox Readiness。

### P5-S08：故障降级

- [ ] 启动时未配置：Capability=`not_configured`，无错误噪声，Agent 正常。
- [ ] Backend 不健康：依赖该 Backend 的离线四工具不可用；Remote MCP 和自有三个元工具不受影响。文件存储/下载独立报告状态，不用计算实例是否存在替代文件可用性。
- [ ] Profile 缺失：Capability=`profile_unavailable`，平台显示配置诊断。
- [ ] 计算配额不足：不创建新计算实例，不回退宿主；已有持久文件不删除。存储满额：停止新增写入，不自动清理旧文件。
- [ ] 运行中断连：已派发而无权威终态的命令记录 `SANDBOX_COMMAND_RESULT_UNKNOWN`，停止自动推进；后续显式 Run 刷新 Snapshot，不盲目重发。
- [ ] 实例回收后重挂载原目录；挂载失败明确返回存储错误，不创建空目录冒充原工作内容。
- [ ] 客户 MCP 调用超时/回执丢失不自动证明上游未执行；缺少上游幂等保证时不盲目重放写调用。不得以此重新增加逐次 Preview 门禁。

### P5-S09：企业 MCP 连接管理

- [ ] 在企业管理域提供连接创建、编辑、测试、启停与成员授权，复用统一 API Client、组件、权限、中英文与明暗主题。
- [ ] 本期连接表单只支持 Remote Streamable HTTP，不提供 stdio 的启动命令/包/镜像字段，不暴露旧 HTTP+SSE 配置。
- [ ] 连接及凭据归属当前企业；企业管理员可管理，普通成员仅使用获授权连接，不提供个人连接配置入口。
- [ ] 凭据使用只写接口和受保护存储，不回显到列表、测试错误、模型或普通用户；平台管理员维护底层环境，不通过平台 MCP 管理接口读取/代管客户业务凭据。
- [ ] 修改授权或停用连接立即使后续工具执行校验失败，不以旧 Run/缓存/会话选择为继续调用凭据。
- [ ] 连接的配置、测试和授权不自动将其加入任何已有会话。

### P5-S10：会话 MCP 选择与直接工具集合

- [ ] 工作台提供已授权企业连接的显式选择，保存会话所选连接 ID，刷新和后续 Run 恢复原选择。
- [ ] 开始 Run 时从会话选择、当前授权、启用和健康状态构造实际 ExternalMCPToolSet，将所选连接的可用工具直接提供模型。
- [ ] 用户调整选择影响后续 Run，不在正在流式执行的 ModelCall 中替换 Schema；授权撤销和连接停用独立实时生效。
- [ ] 未授权、已停用连接不可执行；UI 明确显示已选连接失效状态，不悄悄替换为其他连接。
- [ ] 运行前由服务端检查完整所选工具 Schema 与必要上下文/输出预留；超容量时明确提示减少连接或换模型，不派发本次 Agent 执行、不静默丢弃工具、不自动换模型。
- [ ] 容量失败保留当前消息、已上传业务文件和连接选择；用户调整后重新预检。工具定义本身超限不能靠压缩聊天历史伪装为可运行。

### P5-S11：离线预装环境

- [ ] 为批准镜像预装常用语言和分析库，发布版本化语言/库清单，并让 Agent 可以获取当前环境的准确能力信息。
- [ ] 缺依赖时返回明确诊断，不能自动联网下载或开放出站网络；本期不建设用户上传离线包的依赖安装流程。
- [ ] 预装环境与镜像版本绑定，运行中不偷偷升级；更新后的镜像需验证挂载既有 Workspace 的文件兼容性，不覆盖工作目录。
- [ ] 环境清单与实际镜像一致，预装语言/库可在无网络情况下执行代表性的分析任务。

### P5-S12：会话业务文件上传与产物下载

- [ ] 会话附件入口真实上传业务文件到当前持久 Workspace，展示上传中/完成/失败状态；只存文件名不算上传成功，未完成附件不能提交为模型已可读输入。
- [ ] 上传按企业/会话/Workspace 鉴权，限制大小和持久容量、规范化路径、处理重名并原子写入；中断或满额不损坏已有文件、不自动删旧文件腾空间。
- [ ] 用户消息保存已上传文件的受控引用、名称和安全工作路径，Agent 通过 `read/bash` 访问，不把完整文件内容自动塞入 Prompt。
- [ ] 模型生成文件时，服务端核验文件属于当前 Workspace，保存交付元数据/内容 Hash 和受控引用，在会话提供下载；模型输出任意 URL 或容器路径不视作交付。
- [ ] 下载 API 重新校验企业/会话权限，真实传输文件；已交付引用绑定确定内容，之后工作目录同名文件变化不静默替换历史交付内容。
- [ ] 刷新页面、跨 Run 和回收计算实例后，已上传工作文件与已交付下载仍可访问；明确删除 Workspace/永久删除会话时按其归属清理文件与交付引用。
- [ ] 文件存储/传输独立于活跃计算实例；文件能力不可用时明确提示，不阻断普通对话、自有查询或 Remote MCP。
- [ ] 业务文件上传不自动变成依赖安装，产物下载由宿主文件入口提供，不赋予模板 iframe 下载或网络能力。
- [ ] 复用统一组件、API Client、双语和明暗主题，交付最小附件/产物闭环，不扩展为完整文件编辑器。

## 安全要求

- [ ] 离线四工具无生产 Secret、Connector Credential、RemoteAccessTicket、Worker 宿主挂载和 Kubernetes API。
- [ ] 离线四工具网络为 `none`；客户 MCP 执行域允许联网，二者凭据、进程、网络与管理通道必须隔离。
- [ ] 禁止特权容器、Host PID/Network、Docker Socket 和任意 RuntimeClass 降级。
- [ ] 客户 MCP 的连接 Secret 仅由其服务端 Adapter/独立进程使用，不注入离线 Runtime，不写入 Tool Result、日志、Artifact 或持久 Workspace。
- [ ] 命令、文件路径、stdout/stderr 和 MCP Result 全部执行大小、分类和脱敏限制。
- [ ] Production 继续要求强化 Runtime；Evaluation 降级必须显式显示。

## 测试

- [ ] 无 OpenSandbox 配置下 Agent + Native Tool 完整 E2E。
- [ ] `read/write/edit/bash` 同 Workspace 多轮一致性与跨 Run 重附着。
- [ ] 写入完整目录 → 回收计算实例 → 新实例挂载同一目录 → 文件 Hash 一致；覆盖/删除后的当前状态也正确保存。
- [ ] 关闭页面、结束 Run、归档与长期闲置后目录仍存在；明确删除 Workspace/永久删除会话才清理，迟到写入不能复活目录。
- [ ] 存储满额后 `write/edit/bash` 均不能增加占用，旧文件仍可读取，不发生自动清理。
- [ ] 路径逃逸、Symlink、特殊文件、超大文件和命令超时。
- [ ] Worker 重启、Redis 清空、Sandbox API 短暂中断和计算实例过期不删除持久目录；存储故障不静默退化为空工作区。
- [ ] Worker 宿主无用户代码执行；本期不存在客户 stdio 启动路径，不支持的传输配置明确拒绝。
- [ ] Remote MCP JSON、SSE、Session、认证失败、SSRF 和超时测试。
- [ ] 客户 MCP 工具直接进入模型并成功读写，调用没有绕经三个元工具，不创建自有 Preview 或模板。
- [ ] 企业管理员可管理本企业连接/授权；普通成员无法读取凭据、配置连接或使用未获授权连接，平台管理域不能代管业务凭据。
- [ ] 会话选择跨刷新/Run 保留，新增连接不自动加入；撤权/停用后旧模型工具调用立即被拒绝。
- [ ] 预装语言/分析库在离线环境正常工作，缺依赖明确报告且不尝试联网安装。
- [ ] 客户 MCP 可联网，离线 `bash` 不能联网或访问 MCP 凭据/控制通道；外接工具同名不覆盖核心工具。
- [ ] 用户上传业务文件/授权查询结果 → Workspace 离线分析 → 会话下载生成文件，真实字节与 Hash 一致，导入内容不含自有模板源码或私有动作记录。
- [ ] 上传中断、重名、超限/满额、跨会话或跨企业拒绝；下载在刷新/实例回收后可用，明确删除后不可继续获取已清理内容。
- [ ] 外接 Schema 超容量运行前拒绝，无模型/业务 Tool 执行、无静默裁剪/模型回退；用户减少连接或显式换模型后可运行。
- [ ] Sandbox 配额耗尽不影响普通 Agent Worker Readiness。
- [ ] 临时 Kubernetes 测试结束时明确删除本次测试 Workspace/会话，再清理其 Sandbox、进程、PVC、Artifact Fixture 和 Lease；不清理其他会话持久目录。

## 完成标准

1. OpenSandbox 是可选能力，未配置时 Agent 正常运行。
2. 四基础工具全部且只在无网络 OpenSandbox 中自由执行。
3. 客户 MCP 首期仅 Remote Streamable HTTP，stdio 和旧 HTTP+SSE 不在配置入口、执行路径或本期门禁中。
4. Remote MCP 有独立服务端 Adapter，不经过三个元工具、浏览器或 Template。
5. 任一 Sandbox 故障都不会触发 Worker 宿主 fallback。
6. 核心/外接能力快照、模型工具可见性和实际执行状态一致；客户 MCP 直接执行和文字数据路径成立。
7. 每会话完整工作目录最新状态持久保存，实例回收不删目录；明确删除才清理，满额停止新增写且不自动删除旧文件。
8. 企业连接管理、成员授权和会话选择形成完整路径，平台不代管客户凭据；离线依赖由预装镜像提供。
9. 工具超容量运行前明确阻止，用户可减少连接或换模型后重试，无静默工具裁剪或自动模型切换。
10. 用户业务文件真实上传到会话 Workspace，生成文件作为会话产物下载；持久化、权限、大小和失败恢复均有端到端证据。
