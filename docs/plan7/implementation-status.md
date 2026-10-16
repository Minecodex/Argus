# 实施状态

更新日期：2026-10-15。当前仅完成需求收敛、只读源码调查、官方产品资料核对和开发计划整理。未编写 Plan7 生产代码，未运行 Plan7 自动化测试，未连接资源执行命令，未部署或修改集群。

最新确认：Q15～Q22 规则保留；Q23／Q24 明确不同聊天同资源不排队，并发执行事实仅注入 AI 上下文，不展示聊天消息或提示 UI。单计划／同 shell 仍串行。产品基线已明确，当前无待回答问题，生产任务尚未开始。

## 已有实现与新增工作

| 能力 | 当前源码事实 | Plan7 工作 |
| --- | --- | --- |
| Workspace | [四工具](../../internal/workspace/tools.go)；[离线网络配置](../../internal/sandbox/workspace.go) | 继续离线分析，补齐执行后分析与新策略验收 |
| 平台元工具 | [Gateway](../../internal/toolgateway/gateway.go)；[分类定义](../../internal/toolruntime/categories.go) | 保留分类搜索契约、现有业务权限与确认 |
| 待确认动作 | [PendingAction](../../internal/resource/pending_action.go)；[确认服务](../../internal/action/service.go)；[Executor](../../internal/action/executor.go) | 承载新的远程请求和计划批准，补逐步执行与恢复 |
| 会话恢复 | [动作恢复](../../internal/agent/action_recovery.go)；[执行门禁](../../internal/agent/loop_execution.go)；[历史提示](../../internal/agent/loop_history.go) | 替换一刀切 VerificationOnly 约束，支持远程确认后继续 Workspace 分析与新申请 |
| 当前确认 UI | [PendingActionCard](../../web/apps/enterprise/src/components/chat/pending-action-card.tsx)；[共享控件](../../web/packages/ui/src/preview-commit.tsx) | 新增排查／计划、命令预览、意见修订和逐步结果 |
| 模式持久化 | 当前 Conversation 没有这两个产品模式契约 | 新增 OpenAPI、数据库、客户端、真实与 Mock 行为 |
| 主机后台执行 | [Connector 固定分派](../../internal/app/connector/executor.go)；[受限 helper](../../internal/app/connector/privileged_helper_unix.go) | 新增类型化远程命令请求，不复用人工会话票据 |
| 主机身份 | [SSH 预检](../../internal/sshtarget/probe.go)；[安装与运行账号](../../internal/directexecutor/host_onboarding.go)；[接入计划](../../internal/connector/host_onboarding.go) | 沿接入身份绑定实际执行，处理 manual 等没有可继承账号的路径 |
| Windows PowerShell | [现有终端](../../internal/app/connector/local_terminal_windows.go)；[Windows 后台边界](../../internal/app/connector/privileged_helper_windows.go) | 现有 powershell.exe／ConPTY 不证明通用执行具备；补接入身份、版本、错误、进程树停止和完成回执 |
| K8s 查询 | [查询与凭据租约](../../internal/kubernetesreader/reader.go)；[资源授权](../../internal/resource/kubernetes_service.go) | 既有 List/GetLogs 不能当作通用 kubectl/exec；新增有界执行适配 |
| 遥测文件 | [查询封存](../../internal/dashboard/query_worker.go)；[文件分块](../../internal/dashboard/query_files.go)；[Workspace 导入](../../internal/workspace/import_query.go) | 复用来源／完整性链路，将远程结果按同类原则导入 |
| 共享解释器／长连接 | [Connector 流](../../api/proto/argus/connector/v1/connector.proto)；[人工会话](../../internal/app/connector/remote_session.go)；[断线路由](../../internal/connector/remote_hub.go) | 可复用传输和进程组件；没有 AI 逐批批准、上下文版本和重启恢复契约 |
| Workspace↔远端文件 | [文件交付](../../internal/workspace/files.go)；[来源授权](../../internal/workspace/source_authorization.go)；[安装协议](../../api/proto/argus/connector/v1/control.proto) | 已有封存交付及安装传输不等于通用远端文件能力；新增路径／批准／覆盖／完整性与完成状态 |
| 生命周期与资源并发 | [人工会话策略](../../internal/remoteaccess/policy.go)；[命令派发](../../internal/connector/gateway.go) | 新增空闲回收和跨聊天并发活动感知；不新增资源独占排队，保留连接容量和单会话串行 |
| 并发活动可见性 | [聊天归属](../../internal/storage/postgres/queries/runtime.sql)；[执行读取](../../internal/transport/httpapi/workflow_handler.go)；[动作 HTTP](../../internal/transport/httpapi/resource_action_handler.go)；[Native 动作读取](../../internal/toolgateway/resources.go) | 当前没有通用资源活动摘要；会话／执行／动作的读取范围不同，HTTP 与 Native 规则需核对统一，不能直接共享完整投影 |

以上是源码调查，不是这些路径在当前部署中的实机安全验证。

## 必须避免的误判

- 当前 24 小时审批请求有效期不是同会话同工具的免确认授权；该新授权需求已经退出 Plan7。
- Connector 的 24 小时命令结果缓存是结果重放／去重，不是授权缓存，也不保证任意远端副作用 exactly-once。
- 多个 PendingAction 属于同一 Run，不等于已有整份计划一次批准和逐步可靠执行。
- 现有人工 SSH/PTY 入口不属于模型 connect 能力；不得将其票据交给 AI、模板或 Workspace。
- 文档要求 Native 工具先 Describe，不代表当前所有分类都有硬门禁；[现有门禁](../../internal/toolgateway/description_history.go)主要针对 alert，不能把 Prompt 当授权凭据。
- 配置了 default-deny 网络策略不等于本轮已验证隔离。需要真实环境证明 Workspace 不能绕过 connect。
- 本次调研没有证明所有 K8s 都有上传的 kubeconfig，也没有证明所有 Host 都有可直接复用的登录账号。

## 门禁状态

| 项目 | 状态 |
| --- | --- |
| 当前产品主线与 Q1～Q14 记录 | 已记录 |
| Q15～Q24 | 已明确并同步任务／验收，未实现 |
| 待回答产品问题 | 无；接口／阈值／实现细节由开发任务固化 |
| 生产任务 P7-T01～T09 | 全部 todo |
| 领域／API／协议契约 | 未固化 |
| 权限／恢复／沙箱隔离测试 | 未运行 |
| 真实模型工具选择与长流程 | 未运行 |
| Linux／K8s／Windows PowerShell 远程执行闭环 | 未运行 |
| 真实 API 浏览器 E2E | 未运行 |
| 本地／正式部署 | 未进行 |

任务状态以 [progress.json](./progress.json) 为机读入口；只有完成任务列明门禁、记录不可变源码版本与实际证据后，才可将生产任务改为 completed。不能沿用 Plan6 或 PlanV5 的历史成功结论替代 Plan7 验收。

## 本轮文档校验

2026-10-15 首次落档：9 个新增文件通过严格 UTF-8 读取；60 处新增本地链接存在；9 个生产任务 ID、依赖顺序及当时的 Q15～Q18 引用一致，全部保持 todo。8 份既有文档相对编辑前快照均只有新增段落，保留原有工作区内容；git diff --check 通过。后续回答更新不改变这些历史检查的范围。

Q16～Q18 初次更新时（历史）：Plan7 的 9 个文件通过 UTF-8、格式和 JSON 校验；目录内 45 处本地链接存在，9 项生产任务仍为 todo，任务依赖有效，当时待决问题仅 Q15；文档总索引 diff 检查通过。

Q15 共享会话更新时（历史）：Plan7 的 10 个文件通过 UTF-8、格式和 JSON 校验；目录内 63 处本地链接存在，9 项任务仍为 todo，当时待决为 Q19～Q21；相关架构文档 diff 检查通过。

Q19 确认及 Q20 目录提案更新时（历史）：10 个文件、63 处本地链接、任务依赖与格式校验通过；9 项生产任务仍为 todo，当时待决为 Q20／Q21，目录交换尚未定案。

Q20／Q21 确认时（历史）：专用目录自动交换与暂停后分析恢复进入产品基线；10 个文件、65 处本地链接及 9 项任务依赖校验通过，生产任务全部 todo。当时待决为 Q22／Q23，没有产品代码或运行时验收。

Q22／Q23 确认时（历史）：空闲回收与跨聊天并发感知进入基线，资源排队建议已移除；10 个文件、68 处本地链接、任务依赖及格式校验通过。9 项生产任务全部 todo，当时 Q24 尚待澄清，没有产品代码或运行时验收。

Q24 澄清后：并发信息仅注入模型上下文，不进入聊天记录或并发提示 UI；产品基线收敛，待决问题为空。10 个文件、67 处本地链接、任务依赖及格式校验通过，已移除人类并发提示要求；9 项生产任务全部 todo，未修改业务代码。

这些结果仅证明文档结构和链接有效，不计入上表任何产品、真实模型或运行时安全门禁。
