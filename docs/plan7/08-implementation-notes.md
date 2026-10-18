# Plan7 实施记录

2026-10-18 PR #3 跨平台编译修复：Windows 主机验收编排改名为 `e2e_plan7_windows_host.go`。原 `_windows.go` 后缀使 Go 在 Linux／macOS 自动排除该文件，而通用场景入口仍引用其中的方法，导致 Portable CI 编译失败。编排代码本身使用跨平台 API，实际 Windows 验收仍由 `runtime.GOOS` 明确检查；没有放宽实机前件或 CI 门禁。

开始日期：2026-10-16。实施中，未验收完成。

## 本轮环境与已验证部分

- 用户确认 Linux 使用本地 Docker（SSH/systemd），K8s 使用临时 namespace，Windows 使用本机 Windows 11；Server 兼容性另列待验证。
- 复用 docs/plan6/e2e/glm-model.json 与 GLM_API_KEY 环境变量。未运行 Plan7 真实模型调用，未将密钥写入文件。
- 独立 PostgreSQL 17 容器 argus-p7-db-ac4b73f461（ID 42f9b739fbea1e388e0b040bea9e52ffb5b2f9a2b96aa4d5ca1a6c0c56f0789d），仅 127.0.0.1:51082，临时 trust 认证、空测试数据；全部 15 项迁移通过。所有权标签 argus.plan=plan7 / argus.owner=codex-plan7。运行记录在 build/plan7-db-fixture.json。测试结束需核对所有权并清理这一容器及其测试卷。
- 设置 ARGUS_P7_TEST_DATABASE_URL 指向上述隔离数据库后，remoteexecution 领域测试通过：确认前不执行、固定计划串行、重复任务不重放、下一批次重新确认、失败／未知停止后续步骤、上下文及授权变化使批准失效、跨企业／聊天拒绝、manual 主机不默认继承 root／服务账号。
- 本机 runner 测试通过：真实 PowerShell 变量／目录跨批次延续、原生退出码、普通输出不能伪造完成回执、停止导致上下文失效、输入 Hash／半成品／路径越界。
- 受影响 Go 包初步回归通过；前端类型检查通过一次，新增样式 token 发现问题后已修正，仍需统一复跑。

这些是组件和受控集成证据，不能替代全链路或 57 项验收门禁。

## 当前实现位置

- migration 00015_remote_execution.sql 与 queries/remote_execution.sql：模式快照、身份、共享会话、请求、步骤、文件、内部活动投影和 Action 执行关联。
- internal/remoteexecution：预览／确认扩展、后台计划执行、传输、输入审核、反馈、上下文及生命周期。
- internal/remoteexecution/runner：Bash／PowerShell 共享命令进程与文件操作；PowerShell 使用独立 Runspace、私有控制 socket 和 Windows Job Object；Bash 输出由有界管道收集，完成回执使用独立 NUL 分帧通道。普通输出不是控制消息。
- Connector 新命令 remote_execution_batch/control/file；remote-runtime 子命令用于登录账号下的受控 SSH helper，人工 RemoteAccessSession 仍独立。
- server/worker 已注册 connect 与 workflow.remote_plan.preview；所有实际远程执行由 Action／后台 Worker 推进。
- Workspace 新增固定 ListFiles RPC；仅扫描 remote/input，封存真实字节。初版输出导入 remote/results；2026-10-18 改为 results 下的业务相对路径和单一 meta.json，来源继续校验，见[结果目录更新](./19-workspace-results-and-metadata.md)。
- Chatbox 增加两模式与共享 CommandPlan 视图，API 客户端有真实／Mock 明确实现，修改意见不调用确认。

## 继续实施和验收清单

1. 完成 Kubernetes 正式 Connector 链路与故障验收；适配器实机测试已通过。补齐 direct-only 集群的执行路由，当前无 Connector 的资源明确返回不可用，不能计为完整覆盖。
2. 完成本机 Windows 正式控制链路验收。显式账号／SID 已接入手动接入 Preview、注册核验、API 和门户字段；SSH 接入也记录预检 SID，尚未取得正式 Windows E2E 证据。
3. 核验 Linux SSH helper 端到端路由、接入账号和文件流；当前代码已加入 bridge，尚未实机验收。
4. 完善停止回执（不能把 unknown 伪报为已停止）、停止后已批准的部分输出导入、输出上限和进程树验证。
5. 完善回收任务的原子持久化、失败重试和目录所有权／链接竞态。保留尚未收集的唯一输出。
6. 补齐 UI／适配器测试、计划意见修订及 supersede 锁顺序、跨聊天真实并发、模型内部提示时效／权限及恢复新版本链路。
7. 重新生成全部契约并检查生成结果，补充错误契约、部署参数及镜像打包。每个新文件保持 2000 行以内。
8. 扩展 argus-dev 的 Plan7 套件，冻结源码／镜像，运行真实 GLM-5.3、三类资源及浏览器完整门禁。保护其他 Argus/Judex 部署；如暂停现有 Argus，先记录并最终恢复。
9. 将 57 项逐项映射到真实测试与证据；未运行或未通过不得标为完成。最终清理所有本次拥有的容器、namespace、进程和转发。

当前生产任务均未完成，不应提交为 Plan7 已交付。

## 继续实施记录（2026-10-16）

- Windows 与 Linux Docker 执行器组件测试新增并通过：函数／对象延续、输出截断、自动传输拒绝硬链接、回收所有权和幂等。Windows 发现停止后进程句柄仍未释放的竞态，补等待进程退出后回收。
- 自动交换目录改为长期持有 os.Root 句柄；传输只在未执行命令时进行，普通文件校验拒绝链接、特殊文件和跨目录逃逸。命令输出只保留有界内容，再经独立文件通道发送。
- 步骤增加 collecting 状态：先原子保存完成回执及上下文递增，再收集文件。传输重试使用独立传输标识，命令批次标识不变；重试耗尽保留命令成功事实和未收集输出，禁止将其改写为命令结果未知。
- PostgreSQL 受控测试新增并通过：文件回收重试不重跑命令、重试耗尽保护唯一输出、停止前不派发、停止回执未知不伪报成功、意见修订与旧批准失效、同 ToolCall ID 不接受替换命令。不同聊天同资源并发用例连续运行 5 次通过。
- 并发测试发现 PostgreSQL 序列化冲突曾被误报为上下文丢失，已改为明确会话行锁及仅数据库事务的可重试处理；网络发送不包含在事务重试内。并发内部投影只做一次有界、实时授权过滤查询，跨聊天只输出固定命令名称摘要，隐藏任意参数、脚本、聊天及输出。
- Kubernetes 已加入独立 API 代理和 Pod exec 适配：凭据留在 Connector，命令 Pod 通过私有 exec 流访问固定 API；不向脚本提供 kubeconfig 密钥、Bearer Token 或私有确认 Token。Pod exec 核对 UID、container ID 与重启次数，批准后暂存受信 helper，沿容器实际账号执行。相关代理／实例校验及私有通道停止组件测试通过，真实 Kubernetes 测试正在准备，尚未计为验收通过。
- kubectl 环境需要配置 ARGUS_REMOTE_NAMESPACE、ARGUS_REMOTE_KUBECTL_IMAGE；Pod helper 由 ARGUS_REMOTE_HELPER_DIRECTORY 指定，默认 /usr/local/lib/argus/remote-runtime。受控命令 Pod 无 ServiceAccount Token 挂载，非 root、只读根文件系统、独立临时卷和默认拒绝网络策略；首次执行前用独立探针验证策略实际阻断连接，缺少有效隔离时明确不可用。流式 kubectl exec／port-forward 和无界 watch 不经此 HTTP 代理，进入容器使用 pod_bash。
- 尚未完成：Linux 正式接入／Connector E2E、Windows 正式账号／SID 绑定、Kubernetes 实机门禁、全契约重新生成、完整 UI／真实模型验收及同一冻结版本的 57 项证据。当前测试日志仍属于迭代记录，不能代替最终冻结版本证据。

### 后续进展与首次链路迭代

- 已通过隔离 Kubernetes 适配器实机测试：`argus-p7-b355b652`（UID `ff32a27e-367f-4bbe-8b58-9eb868cc61d1`）验证 Pod exec 状态与文件；`argus-p7-17b9386c`（UID `7ffecadd-1687-4cce-89f4-7a502b1e2cf3`）验证 kubectl、命名空间 RBAC、平台凭据隔离及实际网络拒绝探针。两 namespace 已按 UID 请求删除并确认不存在。这不是完整门户／控制面链路的替代证据。
- 迁移新增独立解释器代次历史，步骤保存代次；恢复后保留旧代次与唯一输出。独立 PostgreSQL 中的 `plan7_generations_20261016` 已完成全量 15 项迁移；随后增量字段在该隔离数据库同步，最终冻结版本仍需再次从空数据库验证。
- 新增迟到回执对账、结果未知后的显式停止、网关真正派发前的再次校验，以及停止消息的预留通道容量。已收到回执时保留实际完成事实，禁止自动启动原计划剩余步骤或递增新解释器的上下文。
- Bash／PowerShell 的子进程环境只保留普通 OS 设置；Connector 环境中的注册 Token、模型 Key 等不自动进入命令解释器。Windows 实机组件测试验证了 Unicode 输出、环境隔离和后台子进程终止。
- 门户命令卡片 5 项测试及既有 MFA 确认 3 项测试通过；源代码 i18n、样式和前端类型检查通过。全量 Go 初次回归发现权限版本断言和 SID fixture 生成问题，已修正并通过相关包复测；仍需最终全量回归。
- 新增 `argus-dev e2e run --suite plan7`，`--plan7-phase flow` 仅作为链路迭代，报告明确 `complete:false`。默认 `all` 还要求 57 项完整矩阵、同一 Run／源码指纹及证据 Hash，不能凭 happy path 标记完成。
- `plan7-20261016-f01` 的 flow 迭代已启动，日志 `build/plan7-flow-f01.log`，产物 `artifacts/plan7-e2e/plan7-20261016-f01`。使用已有配置中固定的 Judex Sandbox Controller，只读核验和复用，不修改 Judex；Argus 原副本及健康由验收器快照并在结束后恢复。当前处于构建／准备阶段，未取得该轮完整通过结果。

### f01 环境失败及修复

- f01 卡在镜像内 kubectl 下载，超过十分钟无进展，尚未进入业务验收。只终止了命令行明确关联 f01 的构建进程及子进程，触发现有验收器收尾；总进程退出 1，没有把此轮记为通过。
- 20 个原工作负载的 UID、副本数和就绪数已核对一致，见该轮 `restoration-verified.json`。临时 namespace／注册表已清理，Judex 保持原就绪副本。
- 去掉构建中的独立 wget 下载，改用上游 kubectl／cli-runtime v0.35.3（与已有 client-go 一致）编译固定连接客户端。连接覆盖、凭据剥离测试通过，新镜像构建通过。使用该镜像的两项 Kubernetes 实机适配测试再次通过；本轮 namespace 为 `argus-p7-e1ed0227` 与 `argus-p7-ed27c98d`，均按 UID 清理。
- 已加入 Direct Executor 持久队列、解释器实例归属、心跳失联处理和迟到回执；两条执行路径共用 `internal/remoteexecution/kubernetesruntime`。新的隔离数据库 `plan7_direct_20261016` 全量 15 项迁移通过，领域测试含实例归属和不重放检查通过。Direct Executor 的正式资源链路仍需独立实机验收。
- 当前尚无 Plan7 完整业务、真实模型或浏览器通过证据，57 项矩阵仍未完成。

### f02 启动门禁失败

- 第二轮 `plan7-20261016-f02` 已越过构建下载，完成镜像加载、隔离数据库迁移和证书签发。服务启动时暴露工具注册配对缺口：`workflow.remote_plan.preview` 没有注册宿主提交声明，server／worker 因此拒绝启动。
- 已按现有原生 Preview 的做法补齐隐藏提交声明；真正执行仍由已实现的不可变 Action 提交处理器负责。新增测试确认 Registry 配对通过、提交入口不进入模型目录、模型直接调用提交被拒绝。
- 本轮未运行任何业务场景。已只终止本轮安装子进程，使验收器进入清理与原部署恢复，不能记为功能通过。

### f03／f04 前置链路迭代

- f03 的正常服务已启动，但镜像来源检查仍按旧版六种镜像判断，新增 remote-runtime 后触发门禁；已改为核对明确的必需镜像集合，并保留缺失、重复和构建类型检查。
- f04 已通过平台初始化及 M4 Agent 前置场景；切换到正式后端时，复用的协议测试证书配置仍只允许 Plan6，尚未开始 Plan7 远程业务场景。已允许隔离 Plan7 套件复用，仍保留 namespace／证书所有权与主机名校验。
- f03／f04 均如实失败并完成自动清理。f04 后已确认临时 namespace 不存在、原 Argus Deployment／StatefulSet 恢复就绪；未将前置成功计为 Plan7 完成。

### f05 与恢复边界补齐

- f05 遇到 Go 模块下载 unexpected EOF，自动重试后构建成功。M4 前置场景通过；正式后端切换误添加了 Plan6 遥测生成容器，其镜像为空，被 Kubernetes 拒绝。已将该容器限定为 Plan6 自监控场景，仍未取得 Plan7 完整业务通过证据。f05 已自动清理，原 Argus Deployment／StatefulSet 恢复就绪。
- 该轮源码的全量 Go 回归、Linux Docker 执行器 9 项测试、前端类型／样式／表单 lint、契约 lint 通过。后续补丁的契约生成一致性检查也通过，见 `build/plan7-contract-check-f06.log`；这些是迭代证据，最终需要同一冻结版本重跑。
- 已实现 Kubernetes 文件专用重连，禁止该通道执行批次、重建解释器或写 input。Pod helper 复用前校验内容 Hash；受控 kubectl Pod 按会话／代次标签恢复，只做文件读取和所有权回收。新建失败时清理本次创建的 Pod／NetworkPolicy，Pod helper 正常回收只删除固定文件和空目录。
- 隔离实机测试先因旧 helper 未包含 recover_files 而失败；换成本轮 helper／镜像后通过。成功运行 namespace 为 `argus-p7-5355a1b6`（UID `b778ddf4-fbaa-4708-81f3-dd342ff5f3ea`）与 `argus-p7-1cc50e7e`（UID `7a16f3b8-5f83-4117-83f0-a4570fe9e68f`），均已清理。实际重建 Pod 的 UID 从 `2014fbc9-aba8-4b96-8585-18cd238744f8` 变为 `968eec73-70ba-4f11-b9f9-5015f8f64a22`，旧通道拒绝继续。记录为适配层实机证据，不替代完整审批／Connector E2E。
- Direct Executor 允许失联实例的文件恢复／回收任务转交另一实例，普通命令仍不可接管或重放；SQL 归属测试通过。私有 API 代理请求现在绑定当前批次 ID，批次结束取消在途请求，迟到请求不能借用下一次批准，新增回归通过。
- 输入预览内容使用总量预算，较大文件仍可查看 Hash 并下载冻结内容。远程读取复验 Workspace 来源权限，并对循环来源做单次遍历；通用 Native 动作读取限制在当前模型聊天，新增撤权／跨聊天／循环来源集成测试通过。

### f06 正式主机与数据链路

- f06 已完成正式后端切换、Docker Linux 的正式 Connector 接入与 Collector 安装，真实 CPU／内存查询及仪表盘执行通过。产物在 `artifacts/plan7-e2e/plan7-20261016-f06`；完整远程确认链路未通过，不能计为完成。
- 首个命令流程没有生成预览：它复用了 M4 负向测试已耗尽配额的受控模型，Run 明确返回 MODEL_QUOTA_EXCEEDED。这是本地测试配置问题，不是 GLM 限流。已新增独立的 Plan7 确定性模型配置和配额；真实 GLM 阶段仍单独配置并验证。等待预览时现在检查 Run 终态，直接报告真实失败原因。
- f06 的全量 Go 回归和启用 PostgreSQL 的 Agent／Workspace／Gateway 集成回归通过，分别见 `build/plan7-go-test-f06.log`、`build/plan7-agent-integration-f06.log`。f06 已清理测试容器、临时部署，原服务恢复就绪。
- 补强适配层测试时发现旧 Pod 替换断言只检查「报错」，可能先被空请求校验拦住。新版使用完整合法文件请求并要求 REMOTE_POD_CHANGED；同时验证 helper 和交换目录确实消失。回收验证探针曾因未消费 exec 输出通道而卡住，已修复并加超时。只停止已核实的本轮测试进程并清理其 namespace，未计通过；随后 `build/plan7-staged-kube-recovery-f06b.log` 两项严格实机测试通过。成功 namespace `argus-p7-ccf3deeb`、`argus-p7-68e14f53` 已删除，替换 UID `3fe166ce-8015-4d09-a0e0-07d2bc447412` → `18bdf689-f9e2-4d0c-a39d-72d673d07e1f`。
- 已新增 Direct Executor 正式 API／临时 namespace ServiceAccount 的 Kubernetes 验收阶段，等待下一轮实机执行；未通过前不将其标为已验证。Windows 前台测试使用 ARGUS_REMOTE_DIRECTORY 将交换文件放进本轮 WorkDir，由测试进程停止后的既有所有权清理处理；正常默认目录不变。
- 增加私有 remote_execution_context_observations，将已过滤的并发活动与实际 model_call／projection_hash／当前授权版本关联，不生成聊天事件或公开接口。新的 `plan7_context_f07` 独立数据库完成 15 项迁移，包含模型实际消息投影、私有记录及无聊天事件的集成回归通过。真实 GLM 并发场景仍待运行。

### f07 模型端点边界与派发复验

- f07 在正式后端创建集群内 Replay 模型时被模型公共端点策略拒绝，返回 MODEL_COMPATIBILITY_FAILED。确认正式构建禁止该私有地址，这是正确边界；没有放宽生产端点策略。验收顺序调整为先在原有 m4e2e 模型测试环境跑确定性工具选择、实际资源和浏览器，再切换正式后端跑真实 GLM。各证据记录 backend_build；这两类测试不能互相替代，最终仍使用同一冻结源码。
- f07 的全量 Go 回归（开启 P5/P7 PostgreSQL 集成、顺序执行包）通过，见 `build/plan7-go-test-f07.log`；测试使用全新 `plan7_full_f07` 数据库。临时部署已清理，原部署恢复。
- 新增确认／最终派发双阶段的 24 个状态变化用例，覆盖企业／用户／部门停用、资源授权与执行权限撤回、资源删除／版本更新、身份／凭据版本变化、凭据停用、解释器代次和上下文变化。全部通过。另有过期预览、模型伪造 approved／action_ref／身份／代次字段的负向测试通过。
- 新测试发现最终派发还缺少 Connector 路由及计划前序步骤校验，已补齐并通过完整领域回归。文件专用恢复的控制请求也增加独立重试 ID，仍不更改命令批次 ID。记录：`build/plan7-dispatch-matrix-initial.log` 保留失败事实，`build/plan7-dispatch-matrix-fixed.log` 记录修复后的通过结果。

### f08 首次远程确认与凭据审计修复

- f08 使用独立受控模型成功走到 Workspace 写入、connect 预览和宿主确认；首个远程请求在准备阶段返回 REMOTE_TRANSFER_FAILED，未完成命令执行。Linux 接入、Collector 和真实遥测再次通过。测试已按所有权清理，原服务恢复。
- 补充真实 Secret／Credential／租约的 PostgreSQL 回归，复现 audit_events_actor_type_check（SQLSTATE 23514）：新通道错误使用了未注册的 connector_gateway 审计主体。已改为绑定接收者的 connector 类型；测试确认审计类型、租约接收者及公开命令中无凭据明文。初次失败与修复通过均保留在日志中。
- 验收诊断现在导出远程请求、步骤、会话、Connector／Direct 队列、传输记录和私有模型活动的有界状态；不导出命令正文、凭据或私有令牌。准备错误日志记录受限 SQLSTATE／表／约束字段，不记录敏感 Detail。
- 输入改为小块流式传输，并记录每个 request/session 的上传完成事实。已完成上传不会因后续步骤或 Worker 重试重新覆盖；不完整回执和 Hash 不符不能记为完成。四个相关场景及完整领域回归通过，见 `build/plan7-f09-domain.log`。
- 新增未知终止活动回归，确认 paused_for_recovery 且 terminated=false 的任务仍进入有权模型的内部活动；terminated=true 则不再被报告为可能活动。之前漏项的失败记录在 `build/plan7-activity-unknown-initial.log`，修复后的领域回归通过。
- GLM 修复场景已改为本轮专用 systemd 服务：错误配置导致真实服务失败，验收需同时核对备份、配置、实际 MainPID、active/running 和新鲜心跳。独立 Docker fixture 实测通过，日志 `build/plan7-service-fixture-live.log`；容器 `argus-p7-service-caca82d9` 已删除。该结果只验证测试场景可用，尚不是 GLM 通过证据。

### f09 首个 Linux 批次通过与压缩恢复修复

- f09 的首个 connect 已成功：确认前未派发，确认后 SSH 控制链路、冻结输入、命令回执、输出文件和 Workspace 导入都有成功记录，见该轮 `plan7-linux-control-1.json` 及新增远程状态诊断。完整连续确认场景仍未通过。
- 后续 Agent 出现反复压缩／恢复：实际推理上下文包含远程事实和动作完成信息，压缩器的容量检查漏掉这些内容，错误判断「已能放下」并重新唤醒。已统一执行后消息组装，并给压缩器传入同一 RemoteFacts 来源；非容量错误不再转为压缩请求。
- 新增回归验证：遗漏远程事实时能容纳的上下文，在包含事实后必须按实际预算处理；没有可压缩边界时明确结束，不新增循环 Agent 任务；来源存储错误保留其真实原因。确定性模型现在采用所选 GLM 的真实配置预算（200000／16384），不再使用不匹配的 32768／1024；小预算失败门禁单独保留。
- 初次组合回归受到复用测试库中残留队列影响，出现任务排序失败。已为领域 fixture 增加按本次企业 ID 的任务收尾，并在全新 `plan7_compaction_f10` 库重跑 Agent、远程领域、Workspace、Worker 和验收器测试，全部通过，见 `build/plan7-context-resume-f10.log`。没有改动任务排序规则来放宽测试。
- f09 的全量 Go 回归通过；完整流程失败后临时资源已清理、原服务恢复。下一轮增加实际 Workspace 网络／凭据隔离检查，再继续三平台与真实模型流程；仍未满足 57 项完成门禁。

### f10 Workspace 隔离与受控模型续步

- Workspace 实机隔离检查通过，包括外部 IPv4／IPv6、DNS、元数据地址、集群 API、平台凭据环境变量、私钥路径和进程权限。Linux 接入、遥测及首个已确认批次再次通过，未再出现 f09 的压缩循环。
- 受控 Replay 把服务端追加的动作完成事实误当作新的用户请求，结束了预设步骤，Run 提前 succeeded。两种原生模型协议测试均复现此问题；仅修正测试提供者对该已知服务端消息的识别，协议回归通过。生产模型授权策略不变。
- 执行后提示进一步明确：动作完成不等于用户整个任务完成，执行记录 ID 不等于目标 Host／Cluster ID；应继续剩余 Workspace 分析，需要新远程操作时重新预览确认。Agent PostgreSQL 回归通过。
- 已准备并编译真实 GLM 的并发场景：两个聊天在同一 Host 上实际重叠执行，校验独立 shell、当前模型调用的私有活动记录、公开 DTO／聊天事件无内部活动投影，并显式停止长任务。该场景尚未取得实机通过结果。
- f10 已清理并恢复原服务；仍只有首个 Linux 批次的成功事实，不能标记整条计划或 Plan7 完成。

### f11 Linux／Windows 链路通过与 Kubernetes 环境修正

- Linux 连续两次确认、确认后 Workspace 分析、同一 Bash 状态延续和一次批准的两步串行计划已通过；Windows 11 正式前台 Connector 接入、实际账号 SID、PowerShell 对象延续、Unicode 和文件交换已通过。该轮原始证据位于 `artifacts/plan7-e2e/plan7-20261016-f11`；它们仍是确定性模型选择的控制链路证据。
- Kubernetes 在用户命令派发前被 REMOTE_NETWORK_ISOLATION_UNAVAILABLE 拦住。已查明平台 foundation 策略 argus-allow-platform-traffic 选中该 namespace 内所有 Pod；NetworkPolicy 的允许规则叠加，不能用另一个 deny 覆盖。未放宽隔离门禁，改为独立的本轮 target namespace，配置仅限该 namespace 的接入权限，并启用 restricted Pod 安全策略。
- 隔离探针增加独立的允许通信见证 Pod，先证明跨 Pod 路由和监听可用，再证明命令 Pod 的出站被拒绝；探针自己的入站规则不会代替被测出站限制。新增实机用例在正常策略下通过、在冲突 allow-all 下正确拒绝，日志 `build/plan7-isolation-positive-control.log`；namespace `argus-p7-c7168443`（UID `ef3aa74b-1ac3-4bea-aab5-c1fab240afa7`）已删除。
- target namespace 纳入既有诊断和清理，同时固定创建时 UID；替换对象、改变所有者或移除 restricted 策略会被拒绝。测试覆盖所有权变化和替换 namespace 不被删除。
- f11 清理显示 stop local process: exit status 1，经核对是主动停止 Windows 前台进程后的退出状态被当成崩溃。已区分「停止前已退出」和「本次请求终止」，缓存完成结果避免 Done 被读取后再次等待；主动停止、重复停止及真实预先崩溃回归通过，日志 `build/plan7-process-cleanup.log`。原服务已恢复，未停止或修改 Judex。

### f12 磁盘门禁

- f12 完成构建及镜像加载后，可用空间为 23.3 GiB，低于既有 25 GiB 门槛，验收器因此中止，未运行本轮业务场景。没有降低门槛，也没有把该轮算作通过。
- 已只读统计本次目录与可识别的 Plan7 编译缓存。随后重新测量 D 盘可用空间为 115.7 GiB，本轮没有执行缓存删除或迁移，不对空间释放来源作推断。保留 `build/plan7-owned-cache-inventory.json` 作为测量记录。
- 增加真实模型两阶段取证场景：第一批回传 manifest，在 Workspace 解析后再申请第二批，分别确认；命令接受范围限定为两次明确的 cp，验证文件中的指令式文本没有造成额外远端写入。代码已编译检查，真实模型结果仍待运行。

### f13 三平台通过与命令卡片修复

- f13 通过 Workspace 实际隔离、Linux 正式接入／Collector／遥测、连续确认和固定串行计划、Windows 11 SID 绑定／PowerShell 对象与文件交换，以及 Kubernetes Connector 的 kubectl／固定 Pod exec 和 Direct Executor 链路。原始证据在 `artifacts/plan7-e2e/plan7-20261016-f13`。该轮完整 Go 回归启用 P5/P7 PostgreSQL 集成，结果通过，见 `build/plan7-go-test-f13.log`。
- 浏览器发现真实页面崩溃：远程动作将通用 `diff` 持久化为对象，公共动作投影无法按数组解码；前端在进入远程卡片前先执行通用 Diff 展示，读取 null 数组导致错误边界。已修正源数据为数组，并在通用展示前分流远程卡片；数据库预览契约和父卡片集成回归通过。不能将此前单测的 Mock 数据视为真实卡片验收通过。
- 只停止已核对的本轮 Playwright 子进程，让父验收器完成原有清理流程。f13 临时 namespace／容器已删除，原 Argus Deployment／StatefulSet 已恢复就绪；未修改 Judex。
- f14 再次通过上述三平台控制链路，页面错误未复现。首个浏览器用例仍在等待通用工具结果展开按钮；实际页面已直接展示远程命令卡片，该场景不具有通用工具结果入口。正在修正用例入口，尚不能计为浏览器或真实模型通过。
- 额外测试通过：活跃任务、唯一未收集输出、未知终止不会被空闲回收；陈旧空闲候选不能回收新增活动；新建 Worker／数据库连接池继续文件收集时不重复命令；Windows 实际 PowerShell 超时、交互输入及非终止错误保持真实结果。新增测试暂存于隔离覆盖，待纳入下个冻结版本，不能提前合并为 f14 的通过证据。

### f14～f15d 浏览器入口、文件系统前件与补充验收

- f14 页面正常渲染了远程命令卡片；失败原因是用例错误等待 `.argus-tool-result__header`，而远程确认卡片直接展示。已改为等待实际 `.argus-command-plan`，未改变产品页面来迁就测试。该轮失败后，原 Argus 服务已恢复。
- 已将空闲回收／Worker 重建／Windows 错误与超时测试纳入源码。新增模式切换测试验证已批准批次不被取消、旧 Run 模式快照不变、新 Run 采用新模式。陈旧上下文在公开读取中投影为失效预览，不改写原批准事实；文件收集未完成与命令完成分开展示。
- 2026-10-16 22:35 左右发现 `artifacts` 及 `build/connector` 是指向 `C:/Users/23574/.codex/plan6-acceptance-archives/20261009-v` 的目录联接，该归档目标已不可访问；原因未确定。此前 `artifacts/plan7-e2e` 的迭代证据因此目前不能重新读取，不能以它们构成最终完整存证。`build/` 下本轮日志仍可读取。
- f15 在读取旧安装配置时终止，f15b／f15c 因失效构建目录联接在镜像阶段终止，均未计通过。核对目标确实不存在后，分别将旧联接保留为 `build/connector.unavailable-f15b`、`build/artifacts.unavailable-f15c`，在原生成路径建立普通目录。未删除外部归档或既有业务资源。具体路径记录在 `build/plan7-artifacts-unavailable.json`、两个 junction-recovery.json 中。
- 当前共享控制器的实际 Pod imageID 仍为原先信任的 `sha256:bf7f63852e772c910a7930cfba1b1d1d810d8613561d358830f7f5108b165c42`，部署 UID 为 `5def4f1e-66e1-4fb8-aed9-ee94cda0c3ba`。基于当前核对结果建立 `build/plan7-shared-controller.yaml` 供验收器读取；没有修改 Judex。新证据目录改为 `build/plan7-evidence`。
- f15d 当前冻结源码全量 Go 回归通过，使用新数据库 `plan7_full_f15d` 并开启 P5/P7 集成，日志 `build/plan7-go-test-f15d.log`；前端全量单测通过，Enterprise 49 个文件／227 项，日志 `build/plan7-web-test-f15d.log`。类型、i18n、样式、表单 lint 通过。完整浏览器／GLM 流程仍在执行，未完成 57 项门禁。
- 验收代码新增多目标／过期／部分失败／恢复卡片、真实模型 Windows 与 Kubernetes 共享状态及文件、无数据／陈旧／部分／修改副本的证据判断、实际 Worker 故障和 Redis 清空、多副本恢复、工程测试存证。未运行成功的新增场景仍是待验收，不能按代码存在计通过。

### f16 完整门禁与共享依赖变化

- f15d 通过全部平台控制路径及四组真实浏览器状态数据准备，但 `pnpm.cmd` 将 Playwright 筛选表达式的 `|` 当作 Windows shell 管道，浏览器未启动。已改为直接调用 Node 下的 Playwright CLI，并显式传入项目配置；实际 `--list` 正确列出 18 个主用例。修复的是验收启动路径，没有放宽产品校验。
- 57 项证据生成器已纳入：逐项要求实际通过的测试与阶段、同一 run/source、18＋1 浏览器用例无失败／跳过，以及 Kubernetes 实机测试和 Linux 执行器测试；记录文件 Hash。报告生成器自身的拒绝跳过／失败及完整编号回归通过。仍须完整实际运行才能标记完成。
- f16 前置检查发现原 Judex 控制器已不在集群，当前为 `argus-sandbox/opensandbox-controller-manager`，部署 UID `aa28b018-902f-47b9-bf9d-8ebadd547995`；实际就绪 Pod 仍运行同一已信任镜像摘要。此拓扑变化不是本任务执行的迁移。更新本轮复用配置，并让暂停器保留这个已验证的共享依赖，其余原服务仍按 UID／副本快照处理。回归通过，日志 `build/plan7-shared-dependency.log`。
- f16b 使用 `--plan7-phase all`，证据目录为 `build/plan7-evidence/plan7-20261016-f16b`，日志 `build/plan7-all-f16b.log`。截至当前记录已完成部署，完整业务／真实模型／浏览器／工程门禁尚在运行，未标记 Plan7 完成。

### f16b Worker 故障断言修正与 f17

- f16b 三平台控制路径通过；Worker 中断／Redis 清空之后，数据库保留前两步为成功且 terminated=true，第三步 command_id 为空、状态为 paused_for_recovery。请求为 paused_for_recovery／REMOTE_CONTEXT_LOST。测试只接受第三步 skipped，因此报出「dispatched after an unknown result」；该报错不等于实际发生第三步派发。
- 已区分「前一步结果未知，后续跳过」与「前一步已有成功回执，下一步在准备时发现上下文失效而暂停」。验收仍要求实际远端 effects 文件严格匹配、命令记录数量恰好正确，并新增第三步无 command_id 的检查。不会仅根据状态名字计为安全。五个正反例回归通过，日志 `build/plan7-worker-fault-oracle.log`。
- f16b 保留为失败轮次。临时资源清理后，原 20 个工作负载副本已恢复，覆盖 argus-system／argus-sandbox／argus-ingress；已接受的共享控制器持续运行。f17 重新执行同一完整门禁，证据 `build/plan7-evidence/plan7-20261016-f17`，日志 `build/plan7-all-f17.log`。尚未获得浏览器／GLM 完整通过结果。

### f17～f18 浏览器与 Workspace 存储前件

- f17 的实际 Worker 故障恢复通过：三步均成功，只有三条命令记录，effects 文件中 first／second／third 各一次。浏览器首例因直接点击 React Aria 底层 radio 被可见标签拦住而失败；已改用仓库现有 setChoice 键盘交互，并断言真实 data-theme。未降低按钮或主题要求。
- f18 已走通模式切换和确认，远端命令成功，Connector 文件读取也成功，但 Workspace 文件服务 Pod 因卷挂载失败一直 Pending。诊断事件为 `mknod /dev/loop8 (lost) b 7 8 (lost)`；当前 Docker Desktop 节点只有 loop0～loop7 设备节点，固定 RawFile v0.15.1 的 [attach_loop 实现](https://github.com/openebs/rawfile-localpv/blob/v0.15.1/rawfile/utils/rawfile.py)未处理 util-linux 返回的 `(lost)` 后缀。此轮如实失败，没有把延长等待作为修复。
- 本地验收前件现会在已验证的 docker-desktop／desktop-control-plane、固定 RawFile 镜像摘要与 CSI 归属下检查并补齐标准 loop0～loop63 节点。已有路径必须是对应 major=7/minor 的块设备，否则拒绝；不会卸载设备、变更现有挂载、格式化或访问卷数据。设备别名作为 Docker Desktop 运行环境前件保留，这不是生产驱动版本升级或任意节点修复接口。
- 独立实机存储验证通过：临时 namespace `plan7-storage-b67d8f88`（UID `61bb0776-69d3-440a-93cf-28fec628e71e`）同时挂载 9 个 PVC，写入后换新 Pod 再读取所有标记；已按 UID 清理 namespace。证据 `build/plan7-storage-f19`，日志 `build/plan7-storage-live-f19.log`。完整门禁将再运行同一测试。
- 恢复上下文增加持久派发事实 sent／not_sent／unavailable，并且不会把未派发或缺少可信完成回执的默认数值 0 作为实际退出码。仅用于当前聊天的内部执行事实，未加入普通 DTO／SSE 的并发数据。已通过三组 PostgreSQL 回归，日志 `build/plan7-context-dispatch.log`，本补充需随新冻结版本重跑完整门禁。

### f19 主浏览器通过与正式镜像切换

- f19 三平台、Worker／Redis 故障恢复及主浏览器 18 项全部通过。浏览器覆盖完整命令、模式与刷新、主动停止、中英文×明暗×三种桌面宽度、多目标一次批准、过期预览、部分失败、关联恢复计划；已检查中文浅色 1280 与英文深色 1920 的实际截图。证据 `build/plan7-evidence/plan7-20261017-f19/playwright-plan7`，没有跳过用例。
- 正式构建切换的镜像核验发现旧 Workspace 文件服务 Pod 仍运行 m4e2e 镜像，因此在真实 GLM 阶段之前失败。没有将这些 Pod 排除出核验。修复 warm reuse：文件服务镜像必须与当前配置一致，旧独立 IO 或计算 Pod 的 IO sidecar 都不能继续复用；已有租约流程负责退出和重建，PVC 保留。
- 切换验收通过正式文件 API 访问每个本轮 Workspace，触发正常更新，并核对 PVC UID／PV 名称不变、既有文件 Hash 一致，再进行镜像核验。不通过删除 Workspace 数据或调整空闲阈值来规避问题。Workspace 与 argusdev 全量回归通过，日志 `build/plan7-f20-ready.log`。
- 补齐 connect／修复计划的双语工具名称；聊天页既有说明改为准确区分自有远程执行确认与客户 MCP 的授权配置。相关组件与 i18n 检查通过，日志 `build/plan7-f20-ui.log`。
- f19 已清理并恢复原服务，保留为未完成的验收轮次。f20 使用最新冻结源码重新执行 all 门禁，目录 `build/plan7-evidence/plan7-20261017-f20`，日志 `build/plan7-all-f20.log`；真实 GLM 与完整 57 项结论仍待实际通过。

### f20 真实模型反馈与上下文展示补齐

- f20 三平台、Worker 故障恢复与主浏览器 18 项再次通过。正式后端切换核验通过：11 个 Workspace 保留同一 PVC／PV，10 个非空 Workspace 抽查既有文件 Hash 一致；证据 `plan7-production-workspaces.json`。
- GLM-5.3 自主遥测查询和 Workspace 分析通过原始数据 Oracle，13 次模型调用，用量完整；现场诊断也得到真实输出并形成报告。诊断脚本末尾使用 `exit 0`，导致共享 Bash 结束，服务端按事实暂停恢复。修复计划阶段连续遗漏必填 title，原工具错误只返回泛化 TOOL_INPUT_INVALID，模型因此误判工具不可用；随后未关联恢复记录的 connect 被拒绝。该轮未执行修复，按失败保留。
- 现已提供受限 Schema 字段路径反馈，例如 `/arguments/title`，不回显拒绝的参数值或内部异常；解释器丢失返回本聊天可用的恢复记录 ID 与重新确认要求。工具说明明确 title／description／steps 必填，且平台自行采集退出状态，普通批次不要用 exit 关闭共享解释器。保持原有 Schema、权限和人工确认要求。
- 新回归复现恢复计划再次修订时版本号从 2 回到 2 的问题；现在按相关前序版本的最大值递增，保留 recovery_of 与 supersedes 两条关联。失败与修复日志分别为 `build/plan7-recovery-revision-initial.log`、`build/plan7-recovery-revision-fixed.log`。
- 按共享上下文设计补充冻结的 known_directory、interpreter_version、previous_request_id、previous_sequence，来源仅是已批准执行的持久事实。未知时明确显示未观察到；同一计划后续步骤注明位置可能被前序命令改变。查看前序批次使用当前权限下的正式读取 API，不授权或执行命令。OpenAPI、SQL 查询与生成客户端同步，新增上下文／参数伪造／幂等性回归通过。
- 真实模型报告明确区分提供商完成与领域结果 Oracle；修正已验证子场景仍携带默认 passed=false 的报告字段，旧 f20 原始文件没有重写。浏览器继续每例独立 context，仅复用通过真实 MFA 的会话 Cookie，并在每个 context 向服务端核对用户身份；18＋1 用例和全部断言保留，减少无关的重复 TOTP 等待。
- 契约生成首轮遇到 Node 的 UV_HANDLE_CLOSING 异常退出，保留 `build/plan7-f21-contracts-initial.log`；关闭生成器更新通知后重试完成，日志 `build/plan7-f21-contracts.log`。这不是契约门禁豁免。当前新版本全量 Go／前端回归进行中，完整真实模型修复和 57 项冻结门禁仍未完成。

### f21 真实修复与剩余证据质量门禁

- f21 三平台控制路径、Worker 故障恢复、主浏览器 18 项及正式镜像切换通过。GLM-5.3 实际完成遥测取数、现场诊断、依据意见修订计划、一次批准执行三步修复；独立 systemd／心跳／配置与备份检查通过。连续两次证据获取、Workspace 分析、输出中的恶意指令隔离、压缩后丢失会话恢复、Windows 与 Kubernetes 共享变量和文件也通过。
- 证据目录 `build/plan7-evidence/plan7-20261017-f21`，完整日志 `build/plan7-all-f21.log`。该轮在无数据场景的模型回答解析处失败：`last` 返回对象，验收要求数值。失败前没有保存最终回答，清理后无法可靠还原对象内容；不补造原始证据。本轮仍为失败，尚未运行后续并发模型与工程汇总。
- 修复验收存证时机：质量场景不论 Oracle 成功或失败都保存实际回答、查询清单、用量及错误。提示词明确 samples 为整数、min／max／last 为数值标量；不放宽解析器。空 matrix 仅在真实清单为 no_data 且文件确实为空时计 samples=0、其他统计 null，成功结果为空仍失败；新增正反例及拒绝对象／数组／字符串的回归。
- f21 临时资源按本轮归属清理，原服务副本已恢复。f22 将使用新冻结源码重新运行 all；最终 57 项门禁仍未完成。

### f22 提供商超时与完整性拒绝

- f22 的三平台、Worker 故障恢复、18 项浏览器、正式镜像切换、真实模型遥测／诊断／修订后修复和连续两次取证再次通过。主浏览器耗时 38.7 秒，无跳过、失败或 flaky；实际截图已检查。
- 恢复推理中一次提供商调用达到既有 120 秒 HTTP 上限，记录 `MODEL_COMPATIBILITY_FAILED`，后续重试 75.6 秒成功；新恢复计划执行成功、旧计划保留 result_unknown。但该失败调用缺少提供商用量，因此严格用量完整性门禁拒绝，本轮失败。没有把重试成功等同于该轮完整验收通过，也没有更改超时阈值或用量规则。
- 证据为 `build/plan7-evidence/plan7-20261017-f22`，调用观察保存在 `model-timeout-observed.json`，清理前状态另存 `state-model-calls.json` 和远程请求／步骤等文件；完整日志 `build/plan7-all-f22.log`。f23 按相同源码重新执行 all，上一轮证据不混入新轮次。

### f23 计划模式的工具选择

- f23 与 f22 源码指纹同为 `086b3fd2430f6df34a08e8176850280308d1a30749523e40df1b4738bd8ab6ac`。三平台、故障恢复、18 项浏览器、真实遥测和现场诊断通过；随后在明确要求展示修复计划时，模型创建普通 connect 预览，验收在批准前拒绝。该轮没有批准这个请求，不能称修复已完成。原始脚本未在这一分支存证，保留的状态只能证明入口类型、待确认状态及无 command_id，不据此推断具体命令。
- 当前模式此前只在服务端数据块中具体标明，系统指令则是两种模式的通用说明。现在按 Run 快照补充明确的系统指令及 workflow 检索／详情／预览路径；修复使用固定计划，必要的额外取证仍允许 connect，所有远程操作继续确认。没有新增命令安全分类或绕过确认的捷径。
- 新 PostgreSQL 回归在修改前复现「实际提供商系统消息没有当前模式」，修改后通过；还验证切换聊天模式不影响旧 Run，新 Run 使用新模式。Agent、toolruntime、argusdev 全模块测试通过，日志 `build/plan7-mode-guidance-initial.log`、`build/plan7-mode-guidance-fixed.log`。修复预览现于类型／版本校验前保存完整快照，M04 要求该回归和原版／修订版预览证据。
- f23 证据 `build/plan7-evidence/plan7-20261017-f23`，日志 `build/plan7-all-f23.log`。f24 将按更新后的冻结源码重跑完整 all，所有任务仍为 in_progress。

### f24 基础越权检查的压缩任务基线

- f24 尚未进入 Plan7 场景，在 M4 的外部成员读取／压缩／取消私有 Run 检查处失败。HTTP 均返回预期 404，但测试断言该 Run 的全部 compaction 任务为 0；实际已有一条正常自动压缩任务，创建于 Run 被拒绝结束之前。新增系统提示使旧固定上下文更容易达到正常自动压缩阈值，并不表示外部成员成功创建了任务。
- 改为记录越权请求前的排序任务 ID 集合，并要求请求后完全相同；新增、替换或删除均失败。报告保留已有 ID 和未变化结果，继续要求 foreign_compaction_tasks=0。未删除已有压缩任务、关闭自动压缩或调整上下文阈值。验收模块回归通过，日志 `build/plan7-m4-compaction-baseline.log`。
- 本轮失败证据 `build/plan7-evidence/plan7-20261017-f24`，日志 `build/plan7-all-f24.log`。后续 f25 仍从完整 all 开始，保持全部门禁。

### f25 计划、恢复通过与多序列分析口径

- f25 基础越权回归实机通过：确有一条原有自动压缩任务，外部成员请求前后的 ID 集合不变。三平台、Worker 故障恢复、18 项浏览器、正式构建切换、GLM 遥测／诊断／固定计划修订与修复、连续取证、压缩后恢复、Windows／Kubernetes 模型场景均通过。新增模式系统指令下，模型先生成 3 步 plan，再按反馈生成关联修订，独立服务检查通过；恢复实际 effects 各一行，旧回执仍未知。
- 无数据场景的原始回答已保存到 `build/plan7-evidence/plan7-20261017-f25/plan7-no-data-evidence.json`。实际 CPU 文件有两条主机序列，最新时间相同；模型明确说明 last 取第一条，而原 Oracle 使用遍历中最后一条，提示未明确并列规则。模型还把无数据／覆盖不足称为 observed_issues，虽没有宣称整体健康，但不符合 Oracle 的资源健康分类。此轮仍记失败，不据此宣称数据分析门禁完成。
- 验收提示现明确 last 按原始文件顺序取最新时间的最后一个点，overall 仅表示运行状态，单凭证据缺失填 unknown 并说明局限。没有放宽数值精度、允许任选统计值或跳过 coverage 校验；原统计计算保持不变。新增并列时间／乱序点回归通过。Oracle 保存实际 Hash 校验后的数据文件、独立预期统计，并在差异错误中输出 JSON 数值而非 Go 指针地址。验收模块回归日志 `build/plan7-quality-semantics.log`。
- f25 完整日志 `build/plan7-all-f25.log`，证据目录 `build/plan7-evidence/plan7-20261017-f25`。后续 f26 仍使用新的冻结源码执行完整 all；证据质量、并发模型与最终工程汇总仍待实际通过。

### f26 正式切换后的 API 就绪前件

- f26 基础检查、三平台、Worker 故障恢复和主浏览器 18 项通过；正式后端切换后的首个 Workspace 列表 GET 等待响应头 30 秒超时，Ingress／Web 记录客户端断开 499。此时还未进入真实模型场景。列表路径仅读取数据库，不启动 IO Pod；现有证据不足以认定具体网络或数据库根因，未将其误写成文件传输故障。
- 原切换器只等待 Deployment Ready。现复用已有 read-only API readiness 检查，经实际 Ingress／Web 到 `/api/v1/setup/status` 验证上游响应后，才进行文件保留和模型检查；沿用启动检查的有界等待，不自动重试业务修改，不放宽文件 Hash、PVC／PV 或运行镜像核验。既有回归会依次拒绝 502 与静态 ready 文本，只接受实际 API 状态；argusdev 全模块通过，日志 `build/plan7-rollout-readiness.log`。
- 失败证据 `build/plan7-evidence/plan7-20261017-f26`，包含 `diagnostics/` 的实际 Pod 和服务日志；完整日志 `build/plan7-all-f26.log`。清理恢复后由 f27 重跑完整 all。

### f27 后台压缩与用量结算竞态

- f27 基础、三平台、Worker 故障恢复、浏览器 18 项、正式切换的实际 API／文件／镜像校验和模型遥测／诊断通过。模型正确生成修复计划并按意见修订，修订版执行成功；在独立服务验证之前，用量门禁立即拒绝。
- 原始记录显示原因是仍在执行的后台 compaction，非提供商失败：压缩调用于 05:53:39.780 开始，主推理于 05:53:47.007 成功，Run 随后 succeeded，但压缩任务及调用仍 running、用量尚未返回。验收过早读取样本，将未结算误当最终缺失。证据保存在 f27 的 `state-model-calls.json`／`state-runtime-tasks.json`／`state-runs.json`，未把这次情况改写成模型超时。
- 基准采样现在有界等待同一 Run 的 reserved／running 模型调用、pending／leased／running 压缩任务以及成功调用的 active 额度预留结算，然后使用原严格用量 SQL。未放宽 provider 来源、成功状态、额度 settled 或任何数值规则。新的真实 PostgreSQL 回归覆盖后台调用尚未创建、调用执行中、额度尚未结算和其他 Run 隔离，并继续执行既有失败／缺失／估算用量拒绝测试；全部 argusdev 回归通过，日志 `build/plan7-model-accounting.log`。
- f27 失败证据 `build/plan7-evidence/plan7-20261017-f27`，日志 `build/plan7-all-f27.log`。f28 仍从完整 all 开始，最终门禁尚未完成。

### f28 状态轮询的瞬时代理故障

- f28 基础、三平台、故障恢复、18 项浏览器及正式切换通过。真实模型遥测分析仍在运行时，观察 Run 状态的 GET 收到 502。Web 日志明确记录 `recv() failed (104: Connection reset by peer) while reading response header from upstream`；Server、Web、PostgreSQL 均 Running／Ready、restartCount=0，没有记录崩溃。不能将一次代理连接重置写成模型推理失败。
- 新增专用 PollJSON，仅供 Run／远程执行状态观察使用，只能发 GET、无请求体、要求 200。仅对 502／503／504、网络超时和不完整读取做最多 3 次且总计 45 秒的有界重试；每次失败以唯一观察 ID 存证，敏感头／参数／响应体不进入重试记录。批准、创建和执行仍使用一次请求；403、429、500 和业务状态失败不重试。
- 新回归复现旧 JSON 助手对 200 非 JSON 内容返回空成功的问题，已改为明确报错，不回显响应内容。真实 HTTP 测试验证恢复两次 502、永久故障到限失败、不重放批准、不忽略权限／限流／服务错误及敏感数据脱敏；全部 argusdev 测试通过，日志 `build/plan7-status-read-retry-initial.log`、`build/plan7-status-read-retry.log`。
- f28 失败证据 `build/plan7-evidence/plan7-20261017-f28`，完整日志 `build/plan7-all-f28.log`；原服务已恢复，f29 继续完整 all。

### f29 无数据通过与 Workspace 能力前件

- f29 基础、三平台、故障恢复、18 项浏览器、正式切换、模型诊断／计划修订／修复／连续取证／压缩后恢复和跨平台流程通过。新的无数据口径也实测通过：9 次模型调用，用量完整，Oracle 无错误，实际原始文件与独立统计已保存。
- 陈旧／部分／修改文件场景失败。原始回答承认未读取文件和计算 Hash，理由是在 9 个平台类别中没找到 Workspace 工具，随后把全部文件标为未验证；不能以 current_health=unknown 代替实际分析通过。证据 `plan7-snapshot-integrity-evidence.json`。旧诊断未保存 Run 的工具快照，无法仅据该回答断定工具当时确实缺失还是模型找错入口。
- 源码核查发现测试配额为 4 个计算沙箱，前序场景可保留未过期的计算会话；BuildTools 在容量不足时返回 quota_unavailable 并省略四工具。新模型场景现在先等待正常生命周期腾出名额并保存容量记录，不提高配额、缩短闲置时限或删除文件。完整性场景开始后还核对实际 Run 快照含四个 Workspace 工具，诊断新增安全的工具名／可用状态／沙箱容量与会话记录。
- 模型系统指令明确四工具与 connect 是顶层入口，平台类别搜索不包含它们；上下文补充当前实际内置工具名和 Run 启动状态，不推断权限原因。新增实际提供商上下文回归保证不会把旧快照或客户 MCP 工具伪装为当前内置能力。
- 首次回归使用长时间复用的测试库，遇到既有残留任务导致队列排序测试失败；保留 `build/plan7-workspace-availability.log`。新建完全独立的 plan7_full_f30 并迁移到 15 后，Agent／toolruntime／argusdev 全模块通过，日志 `build/plan7-workspace-availability-fresh.log`，未修改排序门禁。
- f29 证据 `build/plan7-evidence/plan7-20261017-f29`，日志 `build/plan7-all-f29.log`。最终完整性、并发模型和工程门禁仍待 f30 完整 all 验收。

### f30 完整证据质量通过与 Worker 并发补齐

- f30 通过全部前置与三平台、故障恢复、18 项浏览器、正式构建切换、真实模型修复／连续取证／丢失上下文恢复／跨平台／证据质量场景。完整性 Run 的实际快照为 ready，含四个 Workspace 工具；额外只读核对确认执行 sha256sum 并读取三个文件，保存 `plan7-snapshot-tool-proof.json`。无数据、陈旧、部分与修改文件结论均通过实际 Oracle。
- 真实并发未通过：另一聊天的请求已批准为 ready，remote_execution Task 持续 pending，而长命令占用唯一 running 消费者。领域级并发测试直接调用多个 handler 没覆盖该调度瓶颈，正式 Worker 原来每队列只有一个消费者；不能以新增副本绕过默认部署问题。
- Runtime Processor 现支持有界消费者池，默认远程执行 8 个／独立停止控制 2 个，其他队列保持 1。各消费者有独立 lease_owner；同计划和同解释器的数据库围栏、串行执行、批准前件不变。迁移 00016 增加 remote_execution_control 队列，停止任务进入该队列，且该队列拒绝普通执行。Direct Executor 原有独立停止槽位保持不变。Helm／环境变量／设计文档同步。
- 实际 PostgreSQL 调度回归验证两个任务同时运行、第三个受容量限制等待、独立控制在执行槽位满时仍完成。首次新测试缺少必要企业字段被数据库约束拒绝，已补齐测试身份，未修改约束；日志 `build/plan7-worker-concurrency-initial.log`、`build/plan7-worker-concurrency-runtime.log`。远程执行／配置／Worker 回归通过，全量 Go 使用全新迁移至 16 的数据库通过，日志 `build/plan7-go-test-f31.log`。
- 审查还发现私有活动记录保留组装 Hash，而 ModelCall 在派发时更新为实际提供商请求 Hash。新增回归复现不一致，现于同一派发事务从 ModelCall 更新私有记录，内容及权限投影不变；Agent／远程执行／契约回归通过，日志 `build/plan7-activity-wire-hash-initial.log`、`build/plan7-activity-wire-hash-fixed.log`。生成契约最终核对通过 `build/plan7-contracts-f31-final.log`。并发变量检查改为 test 成功后才写标记，避免 printf 掩盖失败退出码。
- f30 失败证据 `build/plan7-evidence/plan7-20261017-f30`，完整日志 `build/plan7-all-f30.log`。原服务恢复就绪；f31 用上述冻结版本重跑完整 all，最终 57 项仍未标记完成。

### f31 流式模型响应的超时边界

- f31 新消费者池下的三平台、Worker 故障恢复（3 条命令、3 个效果各一次）、18 项浏览器、正式切换及真实遥测／诊断通过。生成修复计划的一次模型调用触及旧 HTTP Client 的 120 秒总上限并失败；重试尚在运行时，模型预览等待的 3 分钟内部定时器到期。本轮没有批准修复计划，仍为失败，证据 `build/plan7-evidence/plan7-20261017-f31`，日志 `build/plan7-all-f31.log`。
- 总 HTTP Client.Timeout 覆盖整个 SSE 响应体，不能区分连接等待与持续生成。可控 HTTP 流式回归复现了已持续返回数据仍被总超时截断的情况；现在生产连接预算 30 秒、响应头 2 分钟，Stream 使用独立、最多 10 分钟的上下文截止时间，并复制 Client 保留共享配置不变。父取消与更短截止时间仍立即生效，普通 HTTP Client 的 2 分钟限制保留。未改动任何获批命令的 timeout_seconds。
- 模型预览等待对齐其他真实模型阶段既有的 10 分钟预算；运行状态／效果／完整用量仍严格校验，失败调用不因重试而计通过。长流、截止、取消和 endpoint 策略回归通过，日志 `build/plan7-stream-timeout-initial.log`、`build/plan7-stream-timeout-fixed.log`。f32 将使用该冻结版本重新执行完整 all。

### f32 预填充与响应头预算

- f32 通过基础、三平台、故障恢复、18 项浏览器、正式切换、模型遥测／诊断，并完成修订版远程计划执行。执行后的模型分析仍有一次约 120078ms 的调用失败，随后重试成功；严格用量门禁因此拒绝。本轮仍未完成修复阶段的独立验证，证据 `build/plan7-evidence/plan7-20261017-f32`，日志 `build/plan7-all-f32.log`。
- 代码核对确认流式 Client 总时限已移除，但仍有 2 分钟响应头限制；预填充也可能延迟首个响应头。现流式响应头和正文共享同一个最多 10 分钟预算，连接仍限 30 秒，普通 Client 总时限仍为 2 分钟。新增延迟响应头的可控回归，同时确认更短流式截止仍在首字节之前生效。
- 增加安全失败阶段日志，只记录请求 Hash、response_headers／response_body／provider_status、状态码、超时标志和耗时，不记录 endpoint／凭据／响应体／原始错误，避免后续只能依据 120 秒耗时推断原因。modelprovider／Agent／argusdev 回归通过，日志 `build/plan7-prefill-deadline.log`。清理恢复后以 f33 重新执行完整 all。

### f33 完整通过与交付

- `plan7-20261017-f33` 的完整 all 运行成功退出 0。源码构建前／验收后指纹一致：`417b000a100b6c5742723d6e7b05e9d85b75e2c82466e75554ba68c92ad58fee`。全部 57 项 required 门禁通过，矩阵中的 42 个不同证据文件 Hash 已独立复核一致，没有混用前轮通过项。
- Linux／Windows 11／Kubernetes 正式路径、Worker／Redis 故障、完整修复、连续确认、压缩后恢复、自动文件交换和证据质量均通过。跨聊天同资源执行真实重叠，独立变量检查成功；活动进入实际 GLM 请求，私有记录与真实请求 Hash 相同，聊天事件和普通 DTO 没有并发数据。19 项真实浏览器全部通过，无失败／跳过／flaky，恢复和并发截图已人工检查。
- 该轮 75 次 Agent 模型调用（推理 73、压缩 2）均成功，输入 868339、输出 81686 tokens，提供商用量与额度结算完整；不含注册兼容性探测或前轮失败迭代。
- 全量 Go、协议、契约、前端类型／单测／样式／i18n／real 构建、9 卷存储实机、Kubernetes 实机适配器和 Linux runner／runtimewire 全部通过。最终部署自检通过。完整报告与原始证据链接见 [09-acceptance-report.md](./09-acceptance-report.md)。
- 原 20 个工作负载逐项核对 namespace UID、对象 UID、副本和就绪数并恢复，共享控制器保留原身份且健康。本轮 namespace、Docker 目标、Windows 临时目录／进程和端口转发已收尾；独立测试 PostgreSQL 容器经 ID 与所有权标签核对后删除。源码、测试证据和无关业务数据保留。
- P7-T01～T09 更新为 completed。验收 Windows 范围按用户确认采用本机 Windows 11；Windows Server 兼容性仍单独待验证。新镜像仅用于隔离验收，没有发布到既有业务部署。

### 2026-10-17 完成情况复查

- 当前源码与 F33 清单一致，57 项矩阵、42 份引用证据 Hash、88 个具名 Go 测试和 19 项浏览器结果重新核对通过。本次没有重跑完整套件或修改生产源码。
- 发现 P7-R01 远程输出统一标为 JSON，以及 P7-R02 暂停前原环境健康／PVC 基线未自动存证。T06、T09 改为 follow_up_required，整体不再声称全部收尾；历史 F33 结果保留。问题定位和验证要求见[完成情况复查](./10-completion-review.md)。

### 复查问题修复与 f34 再验收

- P7-R01：共享导入在 Hash 校验流中采样文件内容，为远程输出识别 MIME 类型；查询 JSON 契约保留。前端优先使用明确类型，二进制内容不会仅因 `.txt` 后缀进入文本预览。新增真实 RPC／数据库的文本、CSV、二进制和写入后恢复回归，真实浏览器用例增加 CSV 预览、二进制提示与下载字节核对。
- P7-R02：暂停前先采集并保存 `original-environment.json`，包含原工作负载健康和 namespace／PVC／PV 身份及绑定；任何采集／存证失败都不开始缩容。恢复前后重新检查原存储身份，保存 `original-environment-restored.json`；增加 StatefulSet 就绪等待及健康对比，替换或退化不能记为通过。
- 本地受影响 Workspace／argusdev Go 测试（独立已迁移 PostgreSQL）、文件卡片单测和 Enterprise 类型检查通过。新完整运行使用 `plan7-20261017-f34`，尚未取得该轮最终结果，整体仍为 follow_up_required。F33 不补写历史前置快照。

- f34 在构建期间主动中断：复核发现普通上传的未知 MIME 也需要使用相同内容识别，否则前端收紧二进制预览后会影响 `.log` 等文件。已复用到上传入口并补分类回归，Workspace 回归通过。
- f34 的实际暂停／恢复已通过新实现核验：原 20 个工作负载、3 个 namespace、9 个 PVC 的身份／绑定与健康一致，证据 `build/plan7-evidence/plan7-20261017-f34/original-environment-restored.json`。拥有的 ingress／RBAC、注册表与临时文件已清理；该轮中断，不计功能验收通过。
- 完整再验收改为 `plan7-20261017-f35`。源码再次冻结，保留此前证据，不混合累计通过项。

- f35 的三平台、共享会话／文件、Worker 故障及主浏览器 18/18 通过；新增 CSV 预览、二进制类型与下载字节断言通过。正式 GLM 遥测分析任务超过既有 10 分钟场景上限，该轮记为失败。该 Run 有 10 次成功推理，最后一次调用在诊断快照中仍为 running、用量未返回；Agent 租约仍续期，Worker 就绪且没有重启。没有取得具体提供方错误码，不将其推断为额度耗尽，也不调整超时阈值或更换模型。

- f35 自动恢复证据通过：20 个原工作负载及 9 个 PVC／PV 与暂停前基线一致；完整命令退出 1，失败原因保留在 `plan7-timeout-summary.json`。新前置快照和恢复验证在实际环境生效。
- 排查额外发现并复现 SSE 收尾缺陷：已收到 Chat Completions `[DONE]` 或 Responses 终止事件后仍等待 HTTP EOF，连接持续打开会误超时。`TestTerminalModelEventDoesNotWaitForTransportEOF` 修复前失败、修复后通过；结束时仍要求有效完成状态及一致用量，Chat 的独立 usage 帧不会被 finish_reason 提前截断。Model Provider／Agent 全模块回归通过，原超时上限不变。F35 没有保存原始 SSE 帧，不能据此断言该缺陷就是该轮超时原因。
- 完整再验收使用 `plan7-20261017-f36`，包含上述流收尾修复与新增具名门禁；此前通过项不混入新冻结版本。

- f36 的三平台与主浏览器 18/18 再次通过；真实遥测分析、现场诊断、修复／意见修订／服务验证及连续两次取证均通过。恢复场景初始化时，模型未逐字复制预置脚本，严格审查在批准前返回 `fixture script was changed`；该请求只有 1 个 pending 步骤、派发 0 次。该轮失败，不能算恢复场景通过。
- f36 的原 20 个工作负载和 9 个 PVC／PV 已按新快照恢复并核对，完整命令退出 1。本轮精简诊断未保存被拒绝的原始脚本，不猜测具体改写内容；后续在目标范围及未派发校验后，额外保存私有 rejected-preview 证据。
- 固定恢复夹具输入改为明确的 JSON 字符串／字段值，要求解码后逐字保留，不改变 exact-script 比较、审批范围、超时和副作用校验。argusdev 回归通过。下一完整冻结运行 `plan7-20261017-f37`。

- f37 三平台、Worker 故障、主浏览器 18/18、真实遥测和诊断通过。修复 Run 最终完成，但其中一次推理收到 HTTP 200 后在 response_body 阶段失败，`MODEL_COMPATIBILITY_FAILED`、用量 missing、延迟 50917ms；其余调用有提供方用量。严格用量门禁拒绝整轮通过，不能用后续重试成功抹掉失败。证据 `plan7-model-failure-summary.json`、`state-model-calls.json` 和 Worker 日志；未保存原始 SSE，具体解析失败种类不明。
- 为后续诊断增加有界 `failure_kind`：区分 JSON、未完成流、工具调用、用量、网络及终止帧错误，不输出原始错误、URL、API Key 或响应正文；隐私回归与 Provider 测试通过。验收器也会保存 passed=false 的模型用量样本，随后仍返回原失败，不放宽门禁。argusdev 回归通过。
- f37 原 20 个工作负载及 9 个 PVC／PV 恢复核验通过，完整命令退出 1。完整冻结再验收为 `plan7-20261017-f38`。

- f38 三平台、Worker 故障和主浏览器 18/18 通过。真实 CPU 分析的 records/samples/min/max/last 均与不可变文件一致，但 `error_logs` 被填为 0，指标图的该字段应为 null；严格 Oracle 拒绝通过，原数据与期望保留在 analysis-data／analysis-expected 证据中。原 20 个工作负载和 9 个 PVC／PV 恢复核验通过。
- 增加通用证据范围系统指令：未观察或不适用的诊断计数不能默认为 0；指标数据不能证明错误日志数量，零错误必须有实际日志证据。验收提示进一步明确字段适用性，指标的 error_logs 必须为 null；Oracle 比较逻辑未改动。实际 Provider system message 回归确认该规则在两种模式均生效，toolruntime／Agent／argusdev 回归通过。
- 下一完整冻结运行 `plan7-20261017-f39`。既有阈值、审批及字段比较均保持。

- f39 三平台、主浏览器 18/18、真实遥测／诊断／修复和连续取证通过；固定恢复初始化脚本逐字匹配。恢复 Run 中一次响应报 `incomplete_tool_call`（HTTP 200、10923ms、用量 missing），最终用量门禁拒绝整轮通过。该旧错误同时涵盖身份／名称缺失与参数 JSON 错误，未保存原始流，不能事后断言具体缺失字段。原 20 个工作负载及 9 个 PVC／PV 恢复核验通过。
- 独立 HTTP 控制测试复现架构问题：Provider 提前拒绝非法参数 JSON，未继续收取后续 usage；配对历史也拒绝这些参数，阻碍工具错误反馈。[官方协议](https://developers.openai.com/api/reference/resources/chat)将 arguments 定义为可能非法的字符串，调用方须执行前验证。现将 JSON 对象校验放在 Agent 派发前：原始参数持久化，非法／非对象参数记录 TOOL_INPUT_INVALID 且不派发；重启继续拒绝，已派发的模糊结果仍 unknown、不重放。Provider 继续拒绝身份／名称缺失及不完整流，完整传输的真实用量不因参数错误丢失。未修改用量门禁 SQL 或放行任何无效工具调用。
- 新回归覆盖两协议的原始参数／用量／配对历史，实际 PostgreSQL 中的零派发、崩溃恢复、后续纠正及未知结果不重放。Provider 与 Agent 目标测试通过，公共契约检查在既有 CI 环境变量下通过。首次临时契约检查未设置 CI，遇到 Redocly Windows 更新通知 libuv 退出错误；复跑使用 CI=1、NO_UPDATE_NOTIFIER=1。
- 全量 Go 初次扫描误包含归档的恢复助手 `.go` 文件，归档已改为 `.go.txt` 并保留内容；所有业务包测试通过，完整命令将清洁复跑。下一完整冻结运行为 `plan7-20261017-f40`，包含 99 个具名 Go 证据要求，矩阵仍为原 57 项。

### F40 完整通过与复查闭环

- `plan7-20261017-f40` 的 all 命令成功退出 0，源指纹 `616ca4c3cf27e03a9d198d274c2324276ac3161e6d9a8df0f8651211c7c6c439` 构建前后相同；2627 个清单文件与当前源码逐项核对一致。
- 57/57 矩阵、99 个具名 Go 检查、19/19 真实浏览器全部通过，43 个不同引用证据文件 Hash 复核一致。完整 Go、Replay、契约、前端类型／单测／样式／i18n、real 构建、9 卷存储、Kubernetes 适配器和 Linux runner／runtimewire 均通过；部署自检通过。
- 76 次 GLM-5.3 Agent 调用（74 推理、2 压缩）全部成功，输入 906749、输出 87262 tokens，用量及结算完整。仅统计 F40，不包含注册探测及此前失败轮次。恢复与私有并发截图已检查，普通文本的类型标注正确，原未知执行事实保留。
- 暂停前及恢复后原 20 个工作负载、9 个 PVC／PV 身份与健康核验通过；本轮 namespace、Docker 目标、Windows 进程／临时目录均清理。共享控制器 UID 与就绪状态保持，独立 PostgreSQL 夹具经 ID／标签核对后删除，保留所有证据和无关业务数据。
- P7-R01／R02 关闭，T01～T09 恢复 completed。最新结果见[再次验收报告](./11-follow-up-acceptance.md)。Windows 实机范围为 Windows 11，Server 兼容性仍未验证；未将新镜像发布到既有业务部署。

### APIMart R01 与 P7-R03

- 用户追加本地重建部署和 APIMart glm-5.3-flash 整体测试，使用独立进程环境密钥。R01 源指纹与 F40 一致，七镜像重建、三资源路径、Worker 故障与 18 个浏览器通过；模型完成取数／Workspace 分析、现场诊断、计划修订与远端执行。
- R01 最终失败：一次响应头前传输错误（约 5 秒，状态 0，无用量），及修复后当前长任务无法继续缩减上下文，最终 CONTEXT_COMPACTION_FAILED。摘要模型调用本身成功；29 次模型调用成功、1 次失败的实际记录及失败证据保留，不能混入通过轮次。原 20 工作负载／9 PVC 恢复通过。
- P7-R03 复现最新要求／反馈只剩摘要的问题；新增硬阈值完整工具组切点，并从事件 Ledger 重建本 Run 原始输入与附件状态。SQL 查询及生成代码同步，新增 PostgreSQL＋HTTP 压缩恢复回归；验收矩阵保留 57 项，并增加三项具名检查。完整再次验收与本地升级仍待完成。

- P7-R03 的三个新增回归及关联 Agent／Conversation／RemoteExecution／argusdev 回归通过，契约检查通过。多包并行回归首次出现独立数据库上的序列化冲突，按既定完整工程门禁的 -p 1 顺序复跑通过，未改事务隔离或跳过测试。
- R02 保持相同指定模型和 16384 输出配置，改用该域名另一条当前 DNS 可见、TLS 和正式 Provider 验证通过的公网地址 104.26.11.94。这是本地网络接入调整，不将其宣称为已证实的 R01 传输错误根因。新版本拟运行 plan7-apimart-20261017-r02 全量门禁，具名检查增加到 102；尚未记为通过。

### APIMart R02 与 R03 准备

- R02 全部 16 次模型调用成功，但在第一段遥测结论核验失败：CPU 数值正确，records 把 1 条序列误写为 13 个点。原环境 20 工作负载／9 PVC 恢复通过，失败轮未计为整体通过。
- 增补 P7-R04：实际系统指令与结构化说明明确 matrix 的 records=序列条数、samples=values 点数；增加独立计数回归，不修改数据核对逻辑。
- R02 中在主浏览器之前切换为正式 Web 配置，18 项主浏览器通过。R03 将此策略固化到 Plan7 构建入口，自动保存 web-build-config.json，并核对实际镜像摘要；旧测试套件的夹具配置不进入 Plan7 发布候选。新增负面回归拒绝 loopback 覆盖、Mock、来源错配和摘要替换。完整矩阵保持 57 项，具名检查增至 104。

### 本地发布完成与追加模型结果

- 独立发布回归 plan7-release-20261017-r04 使用此前已授权的 GLM-5.3，进程退出0；同一源码指纹1bfaa8486b974c54fc674920826677e2c855cebf1c25c294fef7c5ed3678a7e3，2633文件构建前后一致，57矩阵、104具名检查、19浏览器及完整工程门禁通过，45引用证据Hash一致。82次实际模型调用成功（80推理、2压缩），用量及内部结算完整。
- 三个本地Helm release保留values升级至data12／sandbox11／platform28，schema16，10运行镜像及6配置引用正确。已有Workspace镜像条件更新，原27工作负载、9PVC、39Secret及12表25个原业务ID保留，健康20/20、两个实际门户默认TLS登录渲染、清单及8发行制品实际HTTPS下载Hash／签名核验通过。
- APIMart三轮追加实测均未整体通过，最后一次包含HTTP200后流读取或处理错误及十分钟任务超时；底层原因未定位，不将GLM发布回归作为APIMart通过证明。具体已完成与未验证范围见13-local-release-result.md。
- P7-R03／R04代码与契约修复已验证关闭；额外APIMart接入验证保持failed。原始失败证据保留，未提高时限、放宽数据／用量门禁或归一化错误答案。
- 测试namespace、Docker目标、Windows进程／临时目录及独立PG夹具均清理，原Docker三容器及共享控制器保留。临时APIMart加密文件按精确路径及当前用户归属清理，460证据／日志无明文匹配；最初组合命令被策略拒绝后，经只读核验与更严格单文件删除完成，无遗留阻断。
