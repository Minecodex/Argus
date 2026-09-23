# PlanV5 实施状态

本文件记录实际实施证据，不能代替 Task 01～06 的完整验收。当前实现和验收仍在进行；Task 05 的删除门禁已按证据收口，其余任务按新增集群/真实模型门禁继续验收。

最新状态（2026-09-23）：R01～R17 已有修复与验收证据；R17 的 52 个正式工具元数据、中文检索和版本/缓存边界已通过工程、数据库与完整 P5 b，见 [修复记录](./fixes-r17-2026-09-23.md)。真实中文模型评测仍未运行，整体计划未收口。

2026-09-21 复核后的最新补齐内容与门禁见 [补齐记录](./fixes-2026-09-21.md)。下文按历史顺序保留当时状态，早期“尚未通过”不代表最新运行结果。

## 已落地的代码与定向证据

- 基线为 `909c40c`，保留原有计划文档修改；未提交 Git。
- Provider 原生消息、Chat/Responses 流分片与完整性校验、逻辑工具别名已实现，定向测试通过。
- 通用 ToolSet、自有三元网关、分类/版本/权限/Schema 验证已接入 server/worker；500/1000 工具目录测试通过。
- Agent 已使用会话作用域事件、实际投影消息、增量压缩和 Run 固定工具快照；工具调用批次先持久化，再派发并记录终态/未知结果。
- 企业 Remote MCP 的 JSON/SSE、Session、有界 GET 恢复、成员授权、连接修订、Schema 快照、加密凭据、实时撤权已实现。协议和真实 PostgreSQL 定向集成测试通过。
- 新 MCP、Workspace、Presentation 和 preflight 源契约已生成；删除 Card 命令和 Card 公开契约。AgentEvent 的 conversation_id 与永久删除 Conversation 接口已生成。
- Workspace 具备额度预留、每会话 PVC、数据库租约和 fence、IO RPC/mTLS、OpenSandbox 挂载、admission、原子上传、Hash、不可变对象交付、Range 下载、显式删除/空闲回收状态机。已接入正式服务；正在临时 Kubernetes Namespace 执行验收，尚未通过完整 Workspace 门禁。
- HTTP 二进制上传避开 OpenAPI 的隐式 Body 缓冲，保留路径/头参数校验和正式身份校验；对应测试通过。
- 干净 PostgreSQL `argus_p5_v3` 已通过当前完整迁移链（00001/00002）；Card 表、外键及 SQLC 查询已物理删除。
- 后端 Card 服务、命令、处理器、配置、权限注册已删除。通用 action_bindings 保留且解除 Card 依赖。
- 自有 Resource/Telemetry Tool 已随包嵌入不可变模板，Presentation 独立保存，读取时核验当前授权范围；不会把模板传给模型。
- 新 Template Host 在 @argus/ui，独立 template-runtime 应用；只支持 resize/open_resource/open_result，nonce/sequence/port 校验和无网络 CSP。原 Card Host/Runtime 跟踪源码已删除。
- 前端已有 MCP 管理页、会话显式选择、真实上传进度/附件、文件列表、删除 Workspace、下载和通用模板组件；容量预检失败保留输入附件。
- Enterprise、UI、Template Runtime 类型检查通过；API Client 83 个测试、Chat 8 个测试通过。Template Host 单元测试首次因 jsdom 导航替换 window 导致 spy 失效，已修正，10 个 Host 测试通过；Runtime 2 个测试通过。

## 仍需完成与重点复核

- 完整 P5 Kubernetes 流程尚未通过：持久卷硬额度写满、离线 DNS/IPv4/IPv6、计算/IO Pod 交接、撤销旧写入者和文件交付仍需真实运行证据。
- 六类模板及正式 Preview、宿主审批/执行、授权变化、模板失败降级和未知写结果需完成集成验收。
- 跨 Run、增量压缩、Worker/Redis 恢复与容量预检需在两种 Provider 及真实执行环境收口。
- P4、M7/M10 受影响回归、最终全量门禁和资源清理/服务恢复报告尚未完成。
- SQLC/源契约/生成代码已切换；主设计文档和开发/运维手册已更新，最终任务逐项证据仍待补齐。

## 本次临时资源

本次测试 PostgreSQL 容器 `argus-p5-db-07c42775`，标签 `argus.io/task=planv5`，当前本机端口 56456（重启前为 62937）。包含最初的 argus 数据库及新基线测试库 argus_p5_v2、argus_p5_v3；任务完成后只清理该容器。测试已部署专用存储和多轮临时完整栈；没有正式 Argus 部署被暂停，Agentx 未修改。

## 2026-09-13 首轮 Kubernetes 执行

- 运行 ID：`p5-20260913-a`，证据目录 `artifacts/p5-e2e/p5-20260913-a`。
- 完成镜像构建与数据服务安装；在安装产物发布阶段失败，原因是旧夹具仅为部分 suite 设置了与临时私钥配对的 Collector 公钥，P5 配置仍指向示例公钥。
- 已将所有需要 Artifact 的 suite 接入同一临时签名根，并使用独立本机 registry 端口。
- 本次临时 Namespace、测试 CRD、镜像加载 Pod 和全局 Lease 已清理；当前没有正式 Argus 部署被暂停，Agentx 保持运行。
- 这次运行没有进入业务验收，不能计为 P5 通过。

## 后续代码进展

- 原生 CatalogRevision 纳入安装时锁定的 backend 镜像标识；Search/Describe 缓存按主体、权限和版本隔离。
- 大工具结果改为私有对象存储；Template Source 按 Hash 去重。模型结果读取/导入增加当前授权范围校验。
- Run 持久化只读验证阶段及模型调用计数；ModelCall 记录实际 Provider JSON Hash、能力/工具快照和派发时间。
- 版本化 SkillContextSource 接口已加入，不包含编辑器或 Marketplace。
- 新 `argus-sandbox` chart 管理 server/template 配置，保留固定上游 Controller；RawFile 安装、证书、admission、IO RBAC 和文件桶已接线，待实机验证。
- 离线分析镜像和 egress 派生镜像已本地构建成功；基础镜像 Digest 与 18 个 Python 包版本/Hash 已锁定。
- SQLC 生成接入声明级拆分，生成源码保持单文件 2000 行内。
- 第一轮全量 Go 测试发现的旧 Card 契约/权限测试已迁移；新 Template Schema 校验通过。完整门禁仍在推进。

## 2026-09-13 后续验证与修复

- b～e 轮发现并修复 RawFile v0.15.1 镜像标签、StorageClass 参数与 Helm hook 所有权、containerd 镜像 Digest 别名以及测试入口可达性。
- f 轮使用独立测试 Ingress，通过 M2 平台初始化与真实身份认证的 3 项 Playwright 用例；M3 旧夹具缺少 onboarding_control_path，正式 Preview 拒绝，随后补齐请求。
- g 轮发现共 Pod 的 M3 目标机继承了 P4 loopback 故障域名映射，已按实际夹具拓扑分开；P4 故障断言保留。
- h 轮正在执行；M3 正式连接预检 callback_verified=true，主机 online、接入操作 succeeded，资源浏览器回归正在运行。此处不表示 P5 通过。
- f/g 临时 Namespace、RawFile 所有资源和全局 Lease 已清理；公共 cert-manager/trust-manager 由此次测试安装，最终资源收口时须核查依赖后处理。
- Workspace 字面替换编辑改为一个有界输入缓冲和流式原子输出，IO 内存按文件上限预留；workspacefs/workspace 定向测试通过。
- 企业 MCP Basic 凭据不回显、会话选择刷新恢复、真实字节上传/刷新/读取/明确删除两项 mock Playwright 通过。原宿主确认与审批等失败用例已定向修复并通过。
- 删除旧 Card 管理用例和无用 i18n；当前运行源代码、契约、部署和构建没有旧 Card 领域引用。
- Context compactor 的并发 CAS 失败改为可重试，避免等待中的 Run 丢失唤醒；压缩模型调用记录实际请求 Hash/派发状态，并拒绝不完整摘要。
- `contracts check` 已通过，包含 OpenAPI、protobuf lint、契约测试与生成一致性。Workspace RPC 使用独立响应类型。前端完整类型检查与 lint 通过；最终全量 Go/前端测试正在重跑。

## 后续门禁记录（h/i）

- h 轮通过正式 Host 连接预检与接入，M3 的前三项 real Playwright 通过；英文元数据编辑按钮选择器过期，已修正。
- i 轮 M3 前五项 real Playwright 通过；第六项旧明文安装命令断言失败。当前命令使用 base64 封装引导脚本，测试改为解码校验角色与 Connector 内容，并避免打印一次性令牌。
- h/i 均已清理本次临时栈。Agentx 9 个 Deployment 仍为 1/1。
- 全量 Go 测试、前端单元测试（Enterprise 122 项）、i18n、lint、类型检查、mock/real 构建均通过。mock 桌面 Playwright 37 项通过，移动端 1 项按正式范围跳过。
- 文件 RPC 集成测试通过：跨 Workspace、过期 fence、上传截断、原文件保留、Range 和路径越界。
- Workspace admission 已复用 tlsmaterial 动态证书加载，支持 cert-manager 续期；定向测试通过。
- P5 增加旧后台写入者撤销的可执行脚本证明、按 Run 成功率/Token/首个有效结果延迟统计；安装器清理追踪 Tag/Digest 别名，并保护其他标签及活跃 Pod 引用。
- j 轮准备执行以上更新；完整 P5、P4、M7/M10 仍不得标为通过。

## Agent 与模板追加证据

- j 轮 M3 real Playwright 6 项全部通过。随后 M4 重复创建 M3 已存在的角色绑定而失败；套件改为复用已证明存在的绑定。
- 单独 M4 的 `p5-m4-20260913-a` 已通过 Chat/Responses 兼容探测、真实 Search/Describe/Invoke、两个 Run 和 Conversation 增量摘要。失败点是旧 one-time result v2 顶层 command 断言；已对齐现行 v3 instruction_sets，同时保留幂等重放/再次领取拒绝测试。
- 日志数组、GraphQL 根字段别名/Trace span 以及 Kubernetes summary 展示适配已修复。六类查询、Pod 日志和预览详情的 zh-CN/en-US × light/dark 32 项浏览器测试全部通过。
- 已清理 b～j 轮遗留的 51 个精确测试镜像引用，删除前确认没有活跃 Pod 使用；没有执行全局镜像或构建缓存清理。
- `api/contracts/cutovers.yaml` 登记 PlanV5 无兼容切换基线。M4 专项和完整 P5/P4/M7/M10 验收仍在进行。

## 并发恢复补齐与 M4 专项结果

- M4 c 轮后端流程与 2 项真实浏览器用例通过；安装器最后 3 项 HTTPS/CORS 探针误连本机 443，现已支持本轮明确的转发端口，保持原始域名/SNI/Origin 和 CA 校验。
- 增加 Agent/Compaction 按 Run 的活动 Task 唯一约束，事件事务与调用派发校验任务 owner/fence。最后一次尝试崩溃的任务可被重新领取用于终态收敛；中断模型调用和 Step 不再悬挂。
- Hard Compaction 的等待状态与任务创建改为一个事务，重新组装上下文判断先前 Soft Compaction 是否已解除容量压力。
- 新基线数据库 `argus_p5_v4` 的 00001/00002 迁移通过。真实 PostgreSQL 证明同 Run 排他、旧 Worker fence 失效、重试耗尽任务接管以及后续任务继续；Workspace/MCP 集成再次通过。
- 外接 MCP 的业务发送钩子测试证明持久派发失败时零业务 HTTP 请求；禁用成员不能通过旧连接授权执行。32 项 Tool 模板浏览器矩阵通过。
- 上述并发修改之后的完整 P5 Kubernetes 仍待执行；不视为整体完成。

## P5 k 轮与适配器契约修正

- k 轮 M3/M4 全部业务和浏览器阶段通过；P5 自有工具调用成功并保存 Presentation，但读取使用了错误的 tool-calls/.../presentation URL，返回 404。
- 前端 real 适配器与 P5 harness 已统一到权威契约 tool-presentations/{tool_call_id}。新增 Vitest 覆盖所有 PlanV5 适配器 URL/HTTP 方法与生成 OpenAPI 的一致性，已通过。
- 会话工具集合构建时重新发现所选 MCP 当前工具，新增工具不再等待管理员手工测试才可进入下一 Run。成员撤权/禁用、Schema 更改、旧 Run 拒绝和新目录刷新真实 PostgreSQL 测试通过。
- 原生目录规模证据：1/500/1000 项目录的模型工具数均为 3，核心 Schema 始终 1296 字节，日志见 build/p5-catalog-scale.log。
- l 轮执行上述更新；完整 P5 Kubernetes 及 P4/M7/M10 收口仍未完成。

## 主机重启后的继续执行

- Windows 于 2026-09-13 12:21 重启，l 轮进程与并行检查中断。Docker 后端启动因失效 IPC socket 报错，未重置集群。已将仅含两个 socket 的 run 目录重命名为 run-p5-recovery-20260913，重新创建运行目录后 Docker/Kubernetes 恢复，节点 Ready。
- l 轮残留通过原安装配置卸载；独立 Ingress Namespace/RBAC/IngressClass 和 Lease 按 owner、UID/resourceVersion 清理。仅删除创建时间确认为 l 轮、且全局没有实例的 10 个 Strimzi CRD，定义备份在 build/p5-interrupted-cleanup。
- Agentx 的当前资源状态与重启前不同；此次只启动现有 Docker 环境和清理明确属于 l 轮的资源，未操作 Agentx Namespace、Deployment 或 PVC，也未按旧快照恢复其他项目。
- 本次测试 PostgreSQL 容器已重新启动；恢复后相关 Go 包检查通过。m 轮重新执行完整 P5。

## P5 m 轮预检契约修正

- m 轮 M3/M4 与自有模板读取/Worker-Redis 恢复阶段通过；进入 agent-lite MCP 时，预检被错误的 Idempotency-Key 必填契约拒绝。
- 预检为实时容量检查，已移除多余幂等键要求，保留身份和 CSRF 校验。扩展适配器测试覆盖预检，并逐个核验真实请求的 URL、HTTP 方法和必需请求头；测试与后端契约测试通过。
- 重启后的完整前端单元/i18n、PostgreSQL 集成、契约生成一致性均通过。n 轮继续完整 P5。

## P5 n 轮与授权恢复补齐

- n 轮 M3/M4、自有模板及 Redis/Worker 恢复、agent-lite JSON/SSE MCP、未知写结果和容量拒绝通过。Workspace 平台配置请求缺少幂等键而失败，已补齐独立请求键，并将配额读取改为契约规定的企业详情地址；o 轮继续全套验收。
- E2E HTTP 失败信息与证据文件统一脱敏；非 JSON 错误正文不写入日志，避免上游异常响应带出凭据。回归测试通过。
- Workspace admission 增加挂载来源、凭据环境引用及固定文件角色入口校验。正常非 root 文件角色与 9 类伪造 Pod 测试通过。
- 模型输出及摘要绑定授权 Scope；撤权后不恢复旧摘要、回复、参数和结果，仍保持原生工具配对。真实 PostgreSQL 的跨三次 Run 恢复、双 Provider 请求序列及 ContextRevision 条件更新测试通过，证据为 build/p5-context-scope-postgres.log。
- 上述补齐后的最终 Kubernetes 验收和 P4/M7/M10 仍未通过完整门禁，不标记整体完成。

## P5 o/p 轮与平台离线 Profile

- o 轮再次通过 M3/M4 和 agent-lite MCP，Workspace 创建 Profile 被旧 OpenAPI 任务枚举拒绝。新契约统一为 smoke/agent_workspace；后端拒绝 Workspace 联网 Profile，新增公开 Schema 回归证明可创建离线分析配置。
- 平台 Profile 表单和客户端数据模型移除未保存的磁盘、进程、GPU、密钥注入、白名单与多组超时字段；仅编辑实际持久化的名称、镜像、CPU、内存及实例有效期。新增配置使用 agent_workspace/none，启停保留服务端版本。类型、lint、单元、接口回归及四项中英文/明暗主题浏览器测试通过。
- p 轮仅在镜像构建时遇到 Docker Hub EOF，未进入业务阶段；重试拉取 docker/dockerfile:1.7 成功，q 轮继续完整 P5。镜像访问恢复没有重置 Docker 或清理其他项目。
- 旧 Card 应用和包的源文件已删除；残留缓存删除被自动审批拒绝后，采用可恢复移动，归档至 build/retired-card-caches-20260913。正式应用/包及部署路径不再包含旧 Card Runtime。

## P5 q 轮与固定 Execd 协议验证

- q 轮 M3/M4、自有工具及恢复、agent-lite/agent-sandbox 的 Remote MCP 全部通过。真实上传完成同卷保存并校验 Hash，计算 Pod 成功挂载；第一次命令返回 HTTP 200 后被错误判断为结果未知。
- 对照 Execd v1.0.22 的固定源码 4a9db411879601610843af9c8e03563694325b2a，并在无网络、非 root、只读根目录的任务专用容器中复现，确认其 text/event-stream 实际是空行分隔的 JSON 记录，不带 data: 前缀；失败命令的 error 本身就是终态。解析器已按该固定版本修正。
- 同一 Execd 二进制配合完整 Go 客户端验证 pandas 求和和非零退出码通过，证据为 build/p5-execd-client-real.log。协议单元同时覆盖丢失终态、无效错误和未知事件。探针容器已按归属清理，r 轮继续完整 Kubernetes 验收。

## 平台计算配额与文件保留语义

- 配额模型、适配器和页面仅保留真实的并发数、月度计算秒数和版本。删除未生效的 Profile 白名单、日 CPU、文件存储和自动保留天数控件，页面明确计算额度不删除 Workspace 文件。
- 未配置企业可从版本 0 明确保存首份额度；已存在额度不能被版本 0 或旧版本覆盖，更新不自动读取新版本掩盖冲突。SQLC 已重新生成，真实 PostgreSQL 的创建/并发版本约束测试通过。
- 前端类型、lint、单元及 7 项相关浏览器检查通过；真实秒数 125 的 API 回归证明无分钟取整损失。证据分别为 build/p5-quota-postgres.log、build/p5-quota-web-tests.log、build/p5-quota-browser.log。

## P5 r 轮网络证明定位

- r 轮已通过 M3/M4、自有调用恢复、两种模式 Remote MCP、真实 CSV 上传、pandas 分析、不可变交付、Range 下载和旧后台写入者撤销。网络证明脚本非零退出，硬容量和后续真实浏览器/明确删除尚未执行。
- 网络证明改为固定检查项对应的退出码，后续失败可直接定位公网 IPv4/IPv6、DNS、集群地址、root 或管理凭据暴露，错误证据不输出环境值或业务文件正文。
- P5 fixture 从重复 M3/M4 中解耦，保留 M2 初始化、自身角色/双 Provider 模型/额度配置和完整 P5 断言。M4/P4/M7/M10 仍是独立必需门禁；M4 真实浏览器增加月度额度 125 秒保存与刷新验证。

## P5 s 轮与真实 DNS 拒绝证明

- s 轮最小 fixture 缺少安装包签名根，安装器正确拒绝不匹配的签名。已为独立 P5 保留 Artifact 签名 fixture，仍不创建 SSH/Systemd 接入测试目标。
- 使用固定 Egress v1.1.6 和无额外能力的非 root 分析容器共享测试网络，复现了网络证明中的误判：目的端口 53 被重定向到本地 DNS 策略代理，因此 TCP 握手不是公网 DNS 连通证明。
- DNS 验收现发送真实 UDP/TCP DNS 查询，要求拒绝响应且无解析记录，同时独立验证系统域名解析、公网 IPv4/IPv6、元数据和集群地址失败。固定镜像实测通过，证据为 build/p5-egress-real-probe.log、build/p5-egress-policy-real.log；测试容器已按归属清理，t 轮继续 Kubernetes 验收。

## P5 t 轮 Responses fixture 修正

- t 轮独立 P5 的安装、身份、自有 Chat 工具和恢复通过。新增 Responses 实际调用断言发现回放模型只读取字符串 content，遗漏了 Responses 标准 input_text 内容数组，因此没有发出工具调用。
- 修正回放模型输入解析，并用生产 Provider 客户端分别通过 Chat/Responses HTTP 端点完成三个原生调用与终止响应；测试通过。该带 m4e2e build tag 的回放测试现加入使用 Replay 的套件前置门禁，避免仅运行 go test ./... 时漏检。
- 后续完整 P5、P4/M7/M10 门禁仍待通过。中文真实模型成功率评测还需用户指定已有模型配置；已异步询问配置位置，不索取聊天中的明文密钥。

## P5 u 轮后端链路通过

- u 轮双 Provider 原生调用/恢复、两种模式 MCP、真实上传、离线网络、pandas 分析、不可变交付/Range、后台进程撤销、直接多进程写满硬额度和旧文件保留全部通过。
- 真实浏览器首项仍断言存在非空主机表格，但独立 P5 fixture 没有主机。改为明确检查模板已执行并呈现正确标题和空态，而非接受空 iframe；新增空资源的中英文/明暗主题模板用例。
- 上传下载/容量拒绝的后续真实浏览器用例和明确 Workspace 删除因首项失败尚未执行，整体 P5 保持未完成。

## P5 v 轮与删除状态机收口

- v 轮所有后端链路及 3 项真实浏览器用例通过，包括详情空态、MCP 管理、真实附件/不可变下载和超容量保留输入。最后明确删除未完成，原因是 FinishWorkspaceDeletion 使用 deleted_at，但 Workspace 权威数据库基线缺少该列。
- 已补齐基线，并在新建 argus_p5_v5 空库运行完整迁移、重新生成 SQLC。新增 PostgreSQL 预编译门禁，791 条生成查询全部通过，避免仅凭 SQLC 生成成功判断 SQL 可执行。
- Workspace 创建/访问在会话锁内复核 Run 与原 Workspace 身份，用户文件请求仍不要求 Run。真实 PostgreSQL 证明：删除完成后的旧 Run/旧文件请求不能重建工作区，永久删除发生在初次授权与事务之间时也会被拒绝。
- 新基线上的 Agent、Workspace、Sandbox 配额、MCP 和查询检查通过。最终 P5 删除完成/安装器检查、P4/M7/M10 仍需执行；不标记整体完成。

## 完整 P5 w 轮通过

- p5-20260913-w 的 P5 全部业务断言、3 项真实浏览器用例和安装器最终检查通过，result.json 为 passed，进程退出码 0。
- Workspace 最终状态 deleted；工作卷和交付清理完成。按本轮归属扫描 Namespace、PV、StorageClass、Cluster RBAC、IngressClass、CRD 和 Lease，无残留；原副本记录为 null，没有暂停或修改正式服务。证据：artifacts/p5-e2e/p5-20260913-w/cleanup-verified.json。
- Replay 统计为 17 次 Run：15 次 succeeded，2 次故障注入按预期 result_unknown；输入 1280、输出 320 Token，首个有效结果平均 3145 ms。该计数属于确定性 Replay，不代表真实模型任务成功率或成本。
- 当前继续 P4 与 M10（包含 M7、M4 依赖）的必需回归，真实中文模型评测仍等待配置位置；整体任务尚未标为完成。

## P4/M7 验收入口切换

- P4 a 轮发现共享 Collector 目录检查仍请求已删除的 interactive-cards API。已删除该遗留调用，目录检查只负责签名分发包及配置。
- M7 在真实遥测信号就绪后，通过 metric.overview 的 Describe/Invoke 取得并读取当次 Tool Presentation，代替旧 Card Catalog 可用性断言。已有资源管理员绑定先读取复用，避免与 M3/M4 重复创建。
- 定向 Go 检查通过，正式代码/部署/API/测试路径不再包含该旧入口。P4 b 轮继续必需回归。

## P4 异步确认故障窗口

- P4 b 轮的执行器隧道预期故障没有发生。测试在确认 HTTP 返回时就撤销了注入触发器，而本次切换后确认会排队交给 Action Executor，实际创建隧道时故障已被撤销。
- 注入现在保持到观察到 probing 阶段的预期失败，再在显式重试前撤销；原有异常清理仍保留。Go 检查通过，b 轮所属测试主机和 Namespace 清理完毕后启动 c 轮。

## 当前环境阻塞与待确认事项

- P4 c 轮在本地 Registry 启动处遇到 Windows Docker API 代理卡住。Docker daemon 经 WSL Unix socket 的 ping/容器查询立即成功，Windows named pipe 的 ping 成功但容器查询超时；本轮 Registry 27d03e2b66af 保持 created，端口 62111 映射未完成。
- Kubernetes readyz 和节点仍健康，现有其他项目工作负载及 PVC 已记录至 build/p5-docker-restart-before.json。只取消了本轮卡住的 Registry 启动客户端和诊断查询，没有重启 Docker、没有重置集群、没有操作 Agentx 工作负载。
- 由于重启 Docker Desktop 会中断现有 Agentx 等非本轮容器，已异步请求用户确认。P4 c 的重试仍等待 Docker 恢复，其 Registry、临时 Namespace 和 E2E Lease 留待恢复后由本轮流程收口；不能将此轮或 P4/M7/M10 标为通过。
- 最终契约生成一致性、lint、Go 全量测试通过。Go 检查在生成器完成后重新执行，避免生成目录短暂重建造成的检查竞争；1135 个 Go/TypeScript/CSS 文件未超过 2000 行。
- 另一个待答问题是中文真实模型评测所用的已有配置位置（不要求在聊天中提供明文密钥）。确定性 Replay 统计不能替代该评测。

## 2026-09-19 恢复回归

- 重新核实 Docker 29.7.2 容器查询及 Kubernetes 节点均正常，不再需要此前提出的 Docker 重启。当前工作负载与六天前不同，以本次实时状态为准。
- P4 c 轮进程已不存在。按 release 标签、UID 和 resourceVersion 前置条件清理该轮 Ingress Namespace、ClusterRole/Binding、IngressClass 和全局 E2E Lease；Registry 身份及未运行状态核实后删除。没有操作其他项目容器。
- 使用当前源码重建 argus-dev，继续 P4，随后运行包含 M4、P5 native 和 M7 的 m10-query 回归。旧轮次故障记录保留，不将其改写为通过。
- 当前工作树的 Go 全量测试、前端类型/单元/i18n/lint 再次通过；证据为 `build/p5-20260919-go-all.log`、`build/p5-20260919-types.log`、`build/p5-20260919-web-tests.log` 和 `build/p5-20260919-lint.log`。正式路径扫描未发现旧 Card 入口，`git diff --check` 通过。真实数据库、浏览器和 Kubernetes 的历史通过记录与本轮结果分开报告。

## 2026-09-20 模板边界验收

- 日志、指标和概览模板补齐部分结果提示；空资源且 partial 的结果也保留提示。通用表格、详情和时间序列在展示数量达到上限时显示双语说明，要求新消息缩小查询范围，不增加分页 Bridge。
- 时间序列范围计算改为逐点遍历，避免合法大 Presentation 超过 JavaScript 函数参数数量上限。浏览器矩阵增加空日志/指标/调用链、五类 partial、1001 行列表、25 条序列和 13 万数据点。
- 80 项模板矩阵及 2 项 Runtime 隔离/非法握手测试全部通过，证据 `build/p5-template-boundaries-20260920.log`。本次改动的类型、lint 及 Gateway/Presentation/Telemetry Go 检查通过。
- 第一轮大数据断言把一条平直 polyline 的零高度包围框判为不可见；DOM 已正确渲染。测试数据改为有变化的数据点后仍验证相同数量级，不降低数据大小或跳过断言。
- 首帧观测从 iframe 创建开始，包含页面加载、握手和可见内容检查，预算沿用宿主 15 秒初始化上限。本机双 Chromium worker 的 80 个样本 P50 73.5 ms、P95 196.2 ms、最大 910.5 ms；13 万点的 4 个样本最大 249 ms，最大详情 780034 字节。逐项与汇总证据在 `artifacts/p5-e2e/template-boundaries-20260920/`，不是生产网络 SLA。
- 采集耗时前发现 Playwright 1.62.1 的浏览器缓存缺失，已安装该版本匹配的 Chromium/Headless Shell；随后 82 项重新通过，证据 `build/p5-template-boundaries-timing-20260920.log`。缓存缺失轮次没有执行页面断言，不能作为产品测试结果。

## 2026-09-20 P4 重试契约与证据输出

- 契约生成一致性、SQLC、生成后的 Go 全量测试及 mock/real 前端构建通过。证据分别为 `build/p5-contracts-check-20260920-retry.log`、`build/p5-sqlc-20260920.log`、`build/p5-go-final-20260920.log`、`build/p5-{mock,real}-build-20260920.log`。契约首次执行在校验成功后遇到 Windows Node 退出断言，完整重试通过，未忽略退出码。
- P4 `p5-p4-20260919-a` 已进入真实业务链路，执行器回调故障注入按预期生效；后续 retry Preview 因测试发送契约外的 `expected_version` 返回 400。重试接口使用 HostPreviewCreate 请求，服务端读取并冻结当前资源版本；测试已按公开契约修正，新增 OpenAPI 正反向校验通过，不增加兼容字段或放宽校验。
- E2E 调用安装器的 stdout/stderr 复用现有逐行脱敏规则，避免初始化链接令牌进入测试输出；覆盖任意写入分块、未换行结尾、超长行和输出失败。正式安装器仍正常显示初始化链接。
- SSH、systemd 和 Replay 夹具镜像复用后端既有 BuildKit Go 缓存，保持固定版本和 go.sum 校验。P4 准备环境时观察到的 npm/Ubuntu 下载故障由原有重试处理，没有修改版本锁或关闭校验。

## P4 b 轮与卸载计划规范化

- `p5-p4-20260920-b` 的执行器失败重试通过：旧隧道 removed、旧活动 Lease 为 0，新 Connector 和新隧道在线。后续接入、遥测隧道恢复及真实浏览器 1 项通过；在第二台主机的卸载确认处返回 PENDING_ACTION_INVALIDATED，整轮不计通过。
- 复现测试证明：PendingAction 持久化规范化 JSON 键顺序，而卸载 plansEqual 原先按 Marshal 后的原始字节比较，内嵌 ComponentInventory 非空时会误判同一权限计划；双方内嵌 JSON 非法时忽略 Marshal 错误还会错误返回相等。失败证据 `build/p5-removal-plan-reproduction-20260920.log`。
- 比较现复用同一 CanonicalJSON 规则，任何编码错误均拒绝，身份/版本/依赖内容仍严格比较；依赖查询按类型及 ID 固定顺序，避免查询行序影响冻结结果。继续运行 SQLC、全量 Go 和新一轮 P4，不放宽确认或以自动重试掩盖失效。
- 修复后的 SQLC 及全量 Go 测试通过（`build/p5-removal-go-all-20260920.log`）。独立全新 PostgreSQL 18.6 数据库完成迁移，791 条生成 SQL 全部预编译通过（`build/p5-clean-migration-20260920.log`、`build/p5-all-query-prepare-20260920.log`）；按容器 ID 和本轮标签确认后删除了该临时数据库。

## P4 c 轮与卸载操作 Hash

- c 轮 Preview/确认已越过此前失败点，真实浏览器通过；执行器读取卸载操作时又因 HOST_REMOVAL_PLAN_INVALID 停止。该操作另有一条直接 Marshal→Hash→JSONB→结构重编码的路径，内嵌 JSON 顺序仍会改变其 Hash。
- 卸载计划比较和写入统一使用 canonicalPlan；操作读取对完整的原始 JSON 规范化后校验 Hash，不能先解成结构而丢弃未知字段。直接执行器、堡垒机派发、人工卸载指令和 bootstrap 读取复用同一 DecodeOperationPlan，没有旧 Hash 回退。
- 包测试覆盖键顺序、非空清单、Windows 恢复快照、非法 JSON、身份变化和新增未签字段。真实 PostgreSQL JSONB 往返通过，证据 `build/p5-removal-jsonb-20260920.log`；该独立测试数据库已按 ID/标签清理。全量 Go 与新一轮 P4 继续执行。
- 统一序列化后的全量 Go 通过，证据 `build/p5-removal-hash-go-all-20260920.log`。现有真实 PostgreSQL/systemd/SSH 卸载集成测试的 managed_host_ssh、bastion_scope_manual、bastion_scope_ssh 三项全部通过，覆盖人工 bootstrap 和回执丢失重试；证据 `build/p5-removal-systemd-integration-20260920-current.log`。
- 首次集成运行使用本机旧默认镜像，缺少 curl，两个分支未执行；改为本轮仓库构建镜像后通过。测试容器新增归属标签，清理同时删除匿名卷。首轮遗留的一个数据库卷经 Docker 创建记录、时间及只读副本中的三组专用测试数据交叉核实后精确删除，未清理其他 dangling 卷；证据 `artifacts/p5-e2e/removal-integration-20260920/cleanup-verified.json`。

## P4 d 轮与租约状态收敛

- d 轮在瞬时查询中过期但 status 尚为 active 的凭据租约为 1，未进入最终卸载回归，整体仍失败。代码核实 Fulfill/Consume/Renew 均按 expires_at 立即拒绝；默认 Worker 的 LeaseReconciler 每 5 秒更新持久状态，诊断未见清理失败。
- 该项验收改为最多等待 20 秒收敛至零，超时仍失败；明文泄露检查继续立即执行。没有扩大租约有效期，也没有测试代码直接更新租约表。定向检查通过，证据 `build/p5-lease-reconcile-gate-20260920.log`。
- c/d 轮所属资源与全局 Lease、Registry 已清理并通过归属扫描，正式副本未修改；对应 cleanup-verified.json 已保存。e 轮继续完整 P4。

## 完整 P4 e 轮通过

- `p5-p4-20260920-e` 的完整 P4 业务、真实浏览器、所有 SSH 卸载路径和安装器最终检查通过，result.json 为 passed，进程退出码 0。此前的 retry 请求、内嵌 JSON 比较、操作 Hash 和租约收敛问题均经过本轮路径验证。
- 按 release 标签及 PV claim namespace 复核无残留，Registry 和全局 Lease 已删除，原副本记录为 null。证据 `artifacts/p5-e2e/p5-p4-20260920-e/cleanup-verified.json`；未修改正式服务副本。
- 已开始 `m10-query` 的 M3/M4/P5-native/M7/M10 串行回归。真实中文模型评测仍需已有模型配置，整体 PlanV5 未标记完成。

## M7/M10 Collector 架构选择

- M10 a 轮在创建环境前被历史 arm64-only 门禁拒绝。当前构建链已有 amd64 和 arm64 的锁定分发包，但 E2E Collector 镜像未传 DIST_PATH，实际总是使用 Dockerfile 的 arm64 默认目录。
- M7/M10 预检接受这两个已支持架构，打包明确选择目标架构目录，并读取真实 ELF Header 校验 64 位及 Machine 后才构建镜像；错误架构或未知平台直接失败。M8 仍保留其已确定的 arm64 local-hardening 验收边界。
- 两个架构的选择及错误二进制拒绝测试通过，证据 `build/p5-collector-platform-tests-20260920.log`。M10 b 轮继续在本机 amd64 Kubernetes 运行真实 DaemonSet 与业务回归。

## M10 b 轮与上游存储资源命名

- b 轮 RawFile 安装时，上游 Chart 在长 release 名后追加组件后缀，Service 名超过 63 字符，环境未进入业务阶段。安装器现复用已有 upstreamReleaseName 规则，安装与卸载一致使用稳定的短 release 名，保留完整 Argus 归属标签，不依赖缩短测试运行号规避。
- 固定 SHA 的 RawFile v0.15.1 Chart 在本机 Kubernetes 版本条件下实际渲染：本次失败的 ID 和 53 字符 ID 均生成长度合法的 Service/其他资源及标签。argusctl/argus-dev 包测试通过，证据 `build/p5-rawfile-names-20260920.log`。
- b 轮自动清理后，归属扫描及 RawFile CSI/RBAC 扫描无残留，Registry/Lease 已删除，正式副本未修改。c 轮继续 M10 完整回归。

## M10 c 轮与集群内测试地址

- c 轮存储安装和 M2 三项真实浏览器检查通过，M3 的 Kubernetes via_bastion 连接失败。实测宿主 kubeconfig 地址为 loopback 的 53161 端口，测试原样发给 Pod 内的堡垒机，导致请求指向堡垒机自身。
- 该 fixture 固定运行于被测集群内，现将 kubeconfig、连接测试和 Preview 的地址统一为 Kubernetes Service DNS，继续使用原 CA 和限定 ServiceAccount 授权。生产路由、TLS 验证和直连 loopback 拒绝规则未放宽。argus-dev 测试通过，证据 `build/p5-m3-cluster-route-20260920.log`。

## M10 d 轮与真实配额表单提交

- d 轮 M3 连接、资源流程及 6 项真实浏览器通过，M4 后端和 Agent 事实页面通过；平台配额保存未发出 PUT，浏览器轨迹显示带表单字段的页面 GET。QuotaEditor 与 FormDrawer 各自声明 form，造成嵌套表单。
- QuotaEditor 现在统一拥有数据加载、校验和 FormDrawer 提交，父 Tab 只管理打开的企业；加载失败时不允许保存。移除嵌套 form，保持精确秒数和显式版本校验。
- 回归新增保存 125 秒、关闭/刷新持久化、禁止页面跳转和无嵌套 form 断言。相关 7 项浏览器、类型及 lint 通过，证据 `build/p5-quota-submit-{browser,types,lint}-20260920.log`。d 轮归属清理已验证，e 轮继续真实后端回归。

## M10 e 轮与 Collector 请求契约

- e 轮 M3 全部通过；M4 两项真实浏览器通过，包括真实 PUT 保存 125 秒及刷新恢复；P5-native 工具/恢复也通过。M7 首次 Collector Preview 因缺少当前必填 transport 被拒绝。
- 安装、配置、修复、升级和卸载共用的请求生成器现明确 direct_argus + direct，镜像覆盖与 expected_version 继续按操作携带。10 组主机/镜像覆盖请求通过当前 OpenAPI Schema 校验；缺少 transport 的负例仍被拒绝。证据 `build/p5-m7-preview-contract-20260920.log`。
- 同时复核后续采样器：主机本机 OTLP 与 Kubernetes 下游 mTLS 接口分别沿用对应配置，未将正确的本机采样改成不匹配的 TLS 路径。继续完整 M7/M10 回归。

## M10 f 轮与 Kubernetes Collector 平台冻结

- f 轮主机 Collector 安装及后续操作已通过，失败请求实际属于 Kubernetes Collector。服务端 collectorTarget 和执行重验仍固定 linux_arm64，导致所选已支持 amd64 分发记录被误报为 validation pending。
- Kubernetes 的计划平台现从所选分发记录的唯一 Linux 产物冻结，支持已有的 amd64/arm64，并拒绝未知、Windows 或不明确的清单；执行阶段保持目录版本、产物 Hash、配置、资源和连接身份校验。实际镜像兼容性由目标集群 rollout 验证，不把控制面架构当成目标集群架构。
- 平台选择、错误清单拒绝及 Telemetry/Direct Executor/Connector/argus-dev 检查通过，证据 `build/p5-kubernetes-collector-platform-20260920.log`。继续 g 轮完整 M7/M10。

## Collector Namespace 准备与删除保护

- Collector 管理器要求已验证的 Kubernetes Connector 安装器预先创建带 Argus 标识的 Namespace。M3 的进程夹具不执行完整安装脚本；M7 现主动创建专用 Namespace，并在安装前登记清理，拒绝接管任何已存在 Namespace。
- E2E Namespace 删除复核 release 归属，并带 UID/resourceVersion 条件；单元证明不会删除其他任务 Namespace，创建失败也不会将既有 Namespace 加入清理列表。证据 `build/p5-owned-namespace-tests-20260920.log`。
- 正在运行的 g 轮使用启动时的旧工具，已按同一规则补建 Namespace，UID 记录在该轮 `collector-namespace-ownership.json`。当前源码后续运行会自动准备，无需手工步骤；本轮结束额外复核此 Namespace 的归属清理。

## M10 g 轮与 Connector 运行前置条件

- g 轮 Kubernetes Collector 已通过平台和 Preview 校验，但运行命令报 COLLECTOR_MANAGEMENT_FAILED。诊断显示测试 Connector 没有任何环境变量；统一 TLS 客户端要求 CA 路径，故在创建 Kubernetes 资源前失败。
- 进程夹具现使用其已挂载的 Connector CA；M7 Namespace 准备补齐正式安装器提供的 Collector 读取绑定，以及仅在该 Namespace 管理 identity Role/RoleBinding 的权限。没有关闭 TLS 或增加 cluster-admin。
- Namespace/权限准备的归属保护和 Collector 管理器测试通过，证据 `build/p5-collector-runtime-fixture-20260920.log`。g 轮预建 Namespace 按记录 UID 和 release 标签单独清理；后续由当前工具自动准备与清理。

## M10 h 轮与会话请求标识

- h 轮 Collector DaemonSet/Gateway 已就绪，基础采样 Job 完成，前面的运行配置和权限问题已越过。新 Tool Presentation 验证创建标题为 telemetry overview 的会话时，旧辅助函数将显示标题拼入 Idempotency-Key，被空格校验拒绝。
- 会话辅助函数现使用标题的稳定截断 SHA-256 作为 ASCII 请求标识，显示标题继续保留原文，避免空格/中文影响协议头或证据文件名。不是放宽请求头校验；当前工具继续完整回归。

## M10 i 轮通过与 2026-09-21 环境复核

- `p5-m10-20260920-i` 的 M3、M4、P5-native、M7、M10-query 阶段均通过，包含 12 项 M7 中英文/明暗真实浏览器；result.json 为 passed，安装器 verify 全部通过。日志最终记录 Argus release removed。
- 原执行进程句柄已失效，无法恢复退出码；今天集群节点和 Namespace 已重建，不能以今天的空扫描倒推昨天清理零残留。该证据限制及原始文件 Hash 记录在 `artifacts/p5-e2e/p5-m10-20260920-i/completion-assessment.json`，未伪造 cleanup-verified 记录。
- 今天先记录已有 Agentx/Ragflow 等项目的 PVC UID、服务副本及节点 UID；不暂停或清理这些资源。`p5-20260921-final-a` 使用独立 Ingress 和全局 E2E Lease，重新构建当前产品代码并运行完整 P5。

## E2E 磁盘边界及真实模型评测入口

- 发现历史 E2E 磁盘不足路径会执行全局 BuildKit prune，已删除该自动清理分支，改为不足 25 GiB 时明确失败。当前运行开始时磁盘余量足够，没有触发历史分支；回归覆盖足量、临界不足和探测失败。证据 `build/p5-disk-ownership-20260921.log`。
- 增加 `argus-dev e2e run --suite p5 --real-model-config <JSON>`。配置在创建环境前校验，凭据仅从指定环境变量读取，拒绝明文密钥字段与 Argus Replay；复用企业模型管理、配额、原生会话消息和现有 P5 环境，不引入平行测试框架。
- 六个中文自然语言任务通过持久化调用事实和实际 CSV 下载计算结果判定：主机/Kubernetes/Connector 空目录查询、业务 CSV 分组汇总、跨 Run 同 Workspace 文件加工和客户 MCP 只读查询。逐任务及汇总记录 Token、模型调用、推理轮次、有效结果延迟与成功率，单独保存真实模型统计。
- 两种 Provider 请求通过当前 OpenAPI 校验；不安全配置、错误/重复/缺失 CSV 数据、凭据脱敏等测试通过，证据 `build/p5-real-model-runner-20260921.log`。实际模型运行仍等待指定配置，尚无真实任务成功率结论；配置方法和统计边界已加入运维手册。

## P5 安装模式与无镜像缓存验收

- 复核发现此前 agent-lite 阶段仅不配置计算 Profile，但底层 OpenSandbox 仍被安装。这不足以证明可选依赖。P5 现从 `openSandbox.enabled=false` 开始，验证服务不存在及无计算后端的真实文件上传/读取/删除；再以同一 release、数据库、存储和签名身份重新安装启用 OpenSandbox。此处只切换能力开关，不增加部署 profile 枚举。模式转换和归属保护单元通过，证据 `build/p5-install-modes-20260921.log`。
- `p5-20260921-final-a` 在新集群的数据服务阶段遇到 `minio/mc:RELEASE.2025-08-13T08-35-41Z` 拉取拒绝；Docker 直接拉取同样失败，未进入业务验收。PostgreSQL 的镜像代理超时通过拉取相同固定镜像并导入本次受管 image-loader 恢复，没有修改集群镜像代理配置。
- 已明确终止本轮所属 argusctl 安装子进程，让父 E2E 进程走已登记的失败清理；该轮不记为通过。MinIO Client 改为从[官方固定版本](https://github.com/minio/mc/releases/tag/RELEASE.2025-08-13T08-35-41Z)及提交 `7394ce0dd2a80935aded936b09fa12cbb3cb8096` 编译进已有 MinIO 镜像，桶初始化和 smoke 复用该镜像。版本未升级，也未替换为第三方镜像。
- 当前契约完整重试、SQLC、Go 全量、前端类型/单元/lint、mock/real 构建均通过；源码 1139 个文件未超过 2000 行，旧 Card 运行/部署引用扫描为空。安装模式和 MinIO Client 变更后将重跑相应 Go/Helm 与完整 P5，不用先前通过记录覆盖新变更。

## RawFile 重装检查与 c 轮

- b 轮已真实通过无 OpenSandbox 安装时的上传/读取/明确删除、自有工具、跨 Run/Worker/Redis 恢复及 Remote MCP。启用计算的同 release 重装在 RawFile 检查处失败：固定上游 Chart 的 CSIDriver 不带版本标签，旧检查误判。
- 安装器现在根据 CSIDriver 的 Helm release/namespace 归属查找实际 Driver DaemonSet，核实固定版本镜像和 PROVISIONER_NAME；不通过补写标签掩盖实际版本。错误版本、伪造版本标签、错误 Digest、其他 release、错误 provisioner 和不明确的多工作负载均拒绝。定向回归通过，证据 `build/p5-rawfile-reinstall-20260921.log`。
- b 轮退出码 1，按归属清理并复核 Namespace、PVC/PV、RawFile Driver、Registry 和全局 Lease 均无本轮残留，原有 PVC UID 保留。c 轮继续完整安装切换和 Workspace 验收；所需固定镜像已导入本次测试节点，未更改镜像代理配置。
- PlanV2 正文中残留的 Card Runtime/确认 Card 依赖已替换为 Tool 模板及宿主确认；M1/M4/M5 明确标记历史设计边界，旧 `--suite m5` 命令不再被称为当前入口。

## 重装后的客户 MCP 测试边界

- c 轮已通过 lite 文件与工具流程，并成功完成同 release 启用 OpenSandbox 的重装。随后创建 sandbox 阶段 MCP 连接返回 `MCP_ENDPOINT_FORBIDDEN`：安装器重扫 Argus 三个基础设施 Namespace 时，将原先位于 Sandbox Namespace 的测试 MCP Service 正确纳入保护范围。
- P5 的 Replay/MCP 测试服务移至独立、明确归属本轮的 customer-mcp Namespace，TLS 名称、Service 和测试 NetworkPolicy 同步；M4/M7/M10 的 fixture 位置保持不变。没有豁免 MCP 的基础设施地址检查，也没有扩大产品端的访问策略。已有 Namespace 拒绝接管，创建后登记按 UID/归属清理；定向回归见 `build/p5-customer-fixture-20260921.log`。
- c 轮退出码 1，清理及原有 PVC 身份复核通过；`p5-20260921-final-d` 继续完整 P5。新增真实模型统计还检查 `usage_complete`，Provider 用量缺失时整体评测不能通过；最终统计查询在 PostgreSQL 18.6 空库通过，专用数据库及匿名卷已删除。

## 2026-09-21 完整 P5 d 轮通过

- `p5-20260921-final-d` 的完整新矩阵通过，实际进程退出码 0，20 项安装器检查全部通过。先完成禁用 OpenSandbox 的实际安装、无计算后端的文件 IO 和三元工具/Remote MCP，再以相同 release 启用计算并完成外接 JSON/SSE、离线分析、真实上传、不可变下载/Range、容量预检、硬写入上限、旧进程撤销及明确删除；M2/P5 共 6 项真实浏览器通过。
- `state-workspaces.json` 中两个 Workspace 均为 deleted；测试卷为 256 MiB，平台默认仍为 2 GiB。Replay 统计为 17 Run：15 succeeded、2 次注入的 result_unknown，输入/输出 Token 1280/320，首个有效结果平均 3054 ms；不能当作真实模型评测结果。
- 本轮 Namespace、PVC/PV、RawFile Driver、Registry 和全局 Lease 已回收，原有 9 个 PVC 的身份/绑定/状态、23 个工作负载的身份和副本数均与本日初始快照一致。共享 cert-manager/trust-manager 由安装器保留；专用 PostgreSQL 容器/匿名卷、独立 Client 验证镜像和临时 tar 均已清理。
- 关键运行、统计、镜像 Digest、清理及 Hash 索引见 `artifacts/p5-e2e/p5-20260921-final-d`；当前 Go 全量、前端各检查/构建、契约/SQLC、真实 PostgreSQL 与浏览器门禁有通过证据。1144 个受检源文件均未超过 2000 行。
- 真实中文模型评测入口已完成，但未获得模型配置，实际评测仍未执行。整体 PlanV5 保持未完成，未把 Replay 当作此项门禁的替代。

## 2026-09-21 用户要求逐项回顾后的更正

对照最终实施计划重新检查代码后，确认剩余工作不止真实模型配置。实际 Registry 仅六项基础 CRUD Preview；Sandbox 调用执行时重新选择运行环境而未校验冻结身份；SSE 响应头后取消未发通知；纯模型文件的 Workspace 页面删除按钮会禁用。另有逐调用回收与空闲 15 分钟复用的差异，以及未覆盖的故障/性能验收项。

两项 Go overlay 针对性测试已经失败复现 Preview 覆盖和 SSE 取消缺口，证据位于 `build/planv5-review-20260921`；原 P5 d 轮 14 个证据文件 Hash 核对通过，原通过记录没有撤销或扩写。具体影响、源码位置、静态/动态证据区别和待办顺序见 [逐项复核](./review-2026-09-21.md)。本轮仅进行复核和台账修正，这些实现缺口尚未修复。

## 2026-09-22 R06～R08 补齐

Preview 的等待/验证阶段改为持久事实恢复，容量预检与运行共用硬阈值，压缩失败与 Run 终态事件原子提交。全量 Go、数据库回归、前端与契约通过；P5 c 和 M4 a 均退出码 0，原有 9 个 PVC、27 个工作负载及就绪状态保留，本轮资源清理完成。逐项证据和历史失败见 [最新修复记录](./fixes-2026-09-22.md)。真实中文模型评测仍缺配置，整体计划未标为完成。

## 2026-09-22 后续复核 R09～R11

再次复核确认异常动作终态未推进 Run、手动压缩/Run 读取缺少会话所有者校验、Workspace 删除状态未进入模型投影。独立数据库四项审计断言复现失败，现有八包测试仍通过；未修改业务代码。详见 [最新复核](./review-2026-09-22-r09-r11.md)。真实模型评测仍未运行，整体计划未完成。

## 2026-09-22 R09～R11 修复完成

异常动作与继续任务在同一事务提交，Run 公开接口统一校验所有者及删除状态，Workspace 当前身份/状态和失效引用进入模型及摘要输入。数据库、全量 Go、契约/SQLC、前端与 mock/real 构建通过；P5 a/M4 a 均退出码 0，原有 9 个 PVC、27 个工作负载/就绪状态保留，临时资源清理完成。详见 [修复与验收记录](./fixes-r09-r11-2026-09-22.md)。真实模型评测仍未执行，整体计划不标为完成。

## 2026-09-22 R12～R13 修复完成

Workspace 来源先于字节持久化，统一约束整个目录和派生文件；用量按方向区分实报、估算和缺失，公开统计与真实评测不再把估算视为实报。数据库、双 Provider、Compactor 结算、公开 UI、完整 P5 c/d 均通过；最终 d 增加复制退出码、发布字节和交付撤权检查。失败的 a/b 轮、c 轮通过及最终 d 轮证据保留，清理和原资源保持已核验。详见 [修复记录](./fixes-r12-r13-2026-09-22.md)。真实模型评测仍未运行。

## 2026-09-22 后续复核 R14～R15

客户 MCP 返回认证值时，目录描述、结果 Artifact 和模型投影保留该值；独立数据库与本地 TLS MCP 复现失败。ModelCall 仍缺缓存 Token 和实际使用的 ContextSnapshot 关联，属于计划观测项未实现。现有八包测试和历史 P5/M4 证据有效，真实模型评测未运行。本轮未修改业务代码，详见 [复核记录](./review-2026-09-22-r14-r15.md)。

## 2026-09-23 R14～R15 修复完成

Remote MCP 解析入口在目录/结果持久化前拒绝已知认证值，已发送业务调用维持未知结果语义。双 Provider 缓存输入按实报/缺失记录；ContextSource 随实际组装消息传递，ModelCall 固定摘要 ID/Hash 及范围。工程、数据库和完整 P5 b 通过，资源归属清理与原资源保持已核对；a 轮预期失败状态的测试助手问题保留并已修正。详见 [修复记录](./fixes-r14-r15-2026-09-23.md)。真实中文模型评测仍未执行。

## 2026-09-23 后续复核 R16

异常多次用量更新可得到输入 50、缓存 80，仍标记为完整实报。正式流解析/聚合审计复现失败，现有定向检查通过。本轮未修改业务代码或集群，详见 [复核记录](./review-2026-09-23-r16.md)。

## 2026-09-23 R16 修复完成

最终流用量校验覆盖跨更新的缓存子集约束，异常响应不执行工具、不自动重试；推理、压缩、数据库、额度及统计统一使用 invalid 诊断来源。原审计断言、工程、独立数据库和完整 `p5-r16-20260923-a` 通过，实际退出码 0，10 项真实浏览器与 20 项安装器检查通过；17 项证据 Hash、归属清理和原有 9 个 PVC/27 个工作负载保持已核对。首次定向命令的标签错误保留记录，详见 [R16 修复记录](./fixes-r16-2026-09-23.md)。真实中文模型评测仍缺配置与执行证据。

## 2026-09-23 完成度复核 R17

重新核验最新 P5/M4 的 23 项 Hash、九包后端和 10 项 Template Host 测试，既有结果有效。新增正式 Registry 中英文检索对照确认：英文 create/list/delete 命中，中文创建/列表/删除均空；Describe 仅为分类和 ID 拼接，未完整实现计划中的业务元数据。详见 [复核报告](./review-2026-09-23-r17.md)。本轮只更新审计与文档，未改业务代码，未重跑集群或数据库。真实模型门禁继续保留为未完成。

## 2026-09-23 R17 修复完成

52 个正式工具提供版本化业务文档及严格运行时 Schema；Search/Describe 消费同包元数据，检索区分英文词边界、支持维护的中英文业务词，Search 摘要有界。原中英文对照、权限/缓存、内容 Hash、旧 Run 与游标失效及 500/1000 目录回归通过。工程、空库、数据库和完整 `p5-r17-20260923-b` 通过：10 项真实浏览器、20 项安装器、七分类九组持久化检索/描述检查，实际退出码 0。18 项 Hash、归属清理和原有 9 个 PVC/27 个工作负载保持已核验。a 轮镜像遗漏新嵌入包已修复，失败与清理证据保留。详见 [修复记录](./fixes-r17-2026-09-23.md)。真实中文模型评测仍缺配置和执行证据。
