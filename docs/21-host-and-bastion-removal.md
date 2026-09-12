# 主机与堡垒机卸载

本文定义普通主机和堡垒机的删除与卸载边界。已注册资源的软件卸载是独立于记录删除和 Kubernetes Connector 生命周期的持久操作；尚未注册的接入记录允许取消安装并直接删除，不要求先安装成功。

## 1. 用户语义

- 命令安装的机器默认生成同平台的一条系统级卸载命令。
- SSH 安装的机器重新执行 Connection Test，固定当前 Host Key，并通过原执行路径建立短期 SSH 会话。
- SSH 卸载弹窗默认复用当前 Connector 安装记录的账号和凭据引用，展示账号、凭据名称及更换入口。专用 connection-defaults 接口校验租户、目标授权、资源版本和当前安装身份，只返回元数据；不返回密码、私钥或安装计划。凭据轮换后使用当前有效版本重新测试并申请短期 Lease；原凭据不可用时才要求更换。普通主机（直连或经堡垒机）及堡垒机采用相同交互，历史 ConnectionTest 和 Lease 不复用。
- 安装来源按当前 Connector 身份定位。已注册但安装操作在等待上线时失败的机器仍有可验证的当前身份，可以复用该次 SSH 安装引用；较新的失败替换不得覆盖当前身份的来源。堡垒机通过其根 Host 的显式数据授权访问安装元数据，Scope ID 不替代 Host 授权。
- 尚未注册 Connector 的安装中、安装失败或命令延期记录不进入正式软件卸载。列表、详情和待接入堡垒机卡片直接加载冻结的删除预览：安装中显示“取消安装并删除”，失败或延期显示“删除记录”，用户确认后终止本次接入并删除资源记录。该路径不请求或补填 SSH 凭据，不创建 ConnectionTest，也不通过 SSH 清理目标机；界面明确提示目标机可能保留安装残留，且不会卸载属于其他身份的 Connector。Preview 和 Commit 都校验资源版本与当前注册身份，晚注册、身份替换或版本变化时拒绝删除。已有卸载操作仍优先展示操作进度。
- 当前身份已有卸载操作时，弹窗直接展示该操作进度或恢复入口，首屏不读取安装凭据、不创建新的 ConnectionTest。切换目标、版本或重新打开弹窗时隔离全部临时状态和迟到结果；历史其他身份的卸载任务不作为当前操作。
- 读取安装凭据元数据失败时展示对应错误与重新加载入口，不把权限、状态或服务故障降级为空白 SSH 表单。只有服务端明确表示原引用不可用，或用户主动选择更换连接信息时，才展示账号和凭据编辑。
- 卸载弹窗的提示、表单和操作组使用 design token 定义纵向间距，按钮保持内容宽度。滚动容器与内容排版分别处理；表单由统一的 Zod 校验展示字段错误，避免浏览器原生校验提前拦截而不显示应用错误提示。
- 普通卸载弹窗仅由共享 `Dialog` 的 body 承担滚动；内容层不复用 wizard 的 `argus-dialog__flow` 滚动类，避免重复滚动区域裁切名称确认输入框的焦点边框。浏览器回归检查输入框聚焦后四边的 outline 均位于祖先裁切区域内，覆盖中英文和深浅色。

2026-09-11 输入框焦点边框修复已部署到本地 `argus.dev`。新增几何检查先在修复前的中英文场景中复现横向、纵向裁切，修复后卸载 Playwright 13/13 通过，深浅色截图均显示完整边框。前端镜像内容摘要为 `sha256:eb7fca39ce112782a3c9095d749ba5fd8ac9c159abc124afd156a72ba09df063`，Web Deployment 已就绪，真实 HTTPS 卸载模块已确认去除重复滚动容器。
- 正常卸载成功后资源状态为 `uninstalled`，用户再次确认“删除记录”后才从活动清单消失。
- 永久失联时可使用 critical 风险的“仅从 Argus 移除”；它要求 Step-up 和资源名称确认，吊销服务端身份并记录 `local_cleanup=unknown`。
- 确认组件收到服务端 `STEP_UP_REQUIRED` 时打开统一 MFA 二次验证弹窗，验证成功后重试原 `action_ref` 的确认；取消或验证失败不继续执行，不重新创建预览，也不放宽服务端的 critical 风险或审批约束。
- 卸载弹窗顶部并列展示“卸载机器上的 Argus 软件”和“仅从 Argus 移除”，默认正常卸载。仅移除选项对离线或卸载异常资源开放，在线资源展示禁用原因；选择后不要求 SSH 账号或凭据，也不创建或轮询 Connection Test，提交按钮显示“生成移除预览”。SSH 超时、拒绝连接、不可达及认证失败明确提示尚未开始卸载和处理建议。切换方式或关闭弹窗会停止原预览的轮询并忽略迟到结果，不取消已经确认执行的持久卸载操作。
- 堡垒机存在成员、Edge Collector、Collector 操作、遥测路由/隧道、会话、命令、Credential Lease 或成员控制隧道时不得卸载或强制移除。阻塞项在确认页中链接到对应主机、任务或会话页面。

## 2. 持久操作与围栏

### 2.1 未注册接入记录的取消与删除

判断依据是当前注册身份，而不是 SSH/命令安装方式或单独的 `install_failed` 标签。普通 Host 的 `connector_id` 为空时，安装等待、失败、过期及运行中的记录可使用既有 Host Delete Preview/Commit；Bastion 的 Scope 和根 Host 均未绑定 Connector 且没有成员时，采用相同规则。已注册身份，即使安装操作在等待上线阶段失败，也仍走软件卸载。

未注册记录的删除为 `write` 风险，沿用企业授权、数据权限和 Preview/Commit；默认流程无需 SSH 信息，一次确认完成，企业额外审批策略仍有效。正在运行的安装显示“取消安装并删除”，其余未完成接入显示“删除记录”。确认时重查资源版本和注册身份，Bastion 根 Host 与 Scope 必须在同一事务删除。

共享取消流程先锁定目标的 Enrollment Token，与注册消费使用同一行锁协调；已消费或已注册的事实会阻止未注册删除。事务内撤销注册令牌，按命令、安装操作的顺序终止未完成工作，删除操作临时 Secret，回收本次安装和控制隧道的临时 Lease，并将隧道标为 removed。共享 SSH Credential、Secret 和原失败原因保留。非终态安装操作进入 `cancelled`，历史失败操作不改写；Action/Execution 在目标记录删除后仍能收敛为对应终态。

Direct Executor 在安装期间单独检查操作状态、所有者、fence、attempt 和租约有效期，取消后关闭 SSH 上下文；堡垒机自身的 SSH 安装同样响应取消和租约丢失。经 Bastion 派发的安装命令被限定到原操作并过期，迟到的派发、确认、结果和凭据请求不会恢复命令，也不会关闭共享 Bastion 控制流。已经发出的远端工作仍受原截止时间约束，已撤销的注册令牌不能完成注册。

删除完成只表示接入记录和服务端权限已终止，不表示目标机软件全部清理。远端可能保留本次安装文件；清理这些文件不作为删除前提，也不触碰另一 Connector 身份。需要清理时必须以原安装身份和其临时目录为边界单独处理。

### 2.2 已注册资源的软件卸载

`host_removal_operations` 冻结目标 Host、Bastion Scope、Connector ID/version/connection epoch、资源版本、`removal_generation`、安装来源、SSH 证据、Trust Bundle epoch、组件清单和计划哈希。一个目标只能存在一个活动操作。

阶段为：

```text
queued → draining → terminating_sessions → awaiting_manual_execution（命令模式）
       → uninstalling_workloads → stopping_relay（堡垒机）
       → uninstalling_connector → verifying_cleanup → revoking_identities → completed
```

异常状态为 `failed` 和 `cleanup_unknown`。步骤表保存每一步后置条件和结果哈希；事件表保存有序时间线。Worker 使用租约和 `FOR UPDATE SKIP LOCKED` 认领，重启后从首个未验证步骤继续。防误删以 Connector ID、Host/Scope 绑定、目标平台及 `removal_generation` 为准；同一身份的正常重连和证书轮换允许连接代次、证书版本单调递增。撤权、重新安装或资源绑定变化返回 `TARGET_IDENTITY_CHANGED`，不得清理新安装。SSH Worker 丢失执行租约时停止本次连接，旧尝试不能覆盖新尝试的状态。

排空、卸载中及等待恢复的堡垒机继续接受当前 Connector 的合法心跳和 Relay 状态，但不恢复 Scope 为 active、不接纳新成员或任务。连接代次只用于控制流 fencing，不用于使已确认的卸载命令失效。

## 3. 本机执行

Linux 卸载脚本运行在独立 root Shell 中，将 Journal 写到 `/var/lib/argus-uninstall/<operation-id>`。Windows 卸载脚本运行在独立提升权限的 PowerShell 中，将 Journal 写到 `C:\ProgramData\Argus\Uninstall\<operation-id>`。执行前将已经过发行签名校验的 Connector 二进制复制到安装目录外，并以 `uninstall-local` 子命令作为独立 Helper。Helper 先卸载 Collector/Edge 组件，再停止 Connector 服务并删除 Argus 文件，最后验证服务、进程相关文件、Linux 系统账号和堡垒机监听端口均已消失。Windows 会等待 SCM 中的 Service 完全消失，并对文件占用进行最长 30 秒的收敛重试。

Helper 将当前阶段写到 `state.json`，把每次尝试追加到 `events.jsonl`。未满足后置条件时只写 `evidence.failed.json`；重试会再次执行全部幂等动作。全部后置条件满足后写 `evidence.json`，其结构固定为 `argus.host_cleanup_evidence/v1`，包含操作、Connector、`removal_generation`、平台、Collector/Connector 服务与文件、账号、Relay 端口、RDP 恢复状态和观测时间。服务端验证身份、平台、时间、全部后置条件及规范 JSON 的 SHA-256；不再接受由操作 ID 推导的固定“成功哈希”。回执丢失后再次执行同一命令会读取同一操作的已验证 Evidence 并补交回执，不创建新操作。

命令模式使用独立 Removal Token。Bootstrap 请求使用机器已安装的 Argus CA、主机名和脚本 SHA-256，运行期间不存在 `--insecure`。脚本在 Connector 停止后以独立 Receipt Token 提交清理回执。网络中断时本机 Journal 保留，服务端进入 `cleanup_unknown`，后续命令仍绑定原操作。

卸载统一读取 Connector 运行期的 `connector-ca.pem`（Linux 为 `/var/lib/argus-connector/connector-ca.pem`，Windows 位于 `C:\ProgramData\Argus\Connector`），并在清理前复制到独立 Journal。Bootstrap 在限时 Token 内允许同操作重取相同脚本，以恢复响应丢失；等待手动执行和执行中的操作均可重新生成命令。等待执行的操作也会到期收敛，不会无限停在等待态。执行前核对本机 `identity.json`，防止通过同一 SSH 地址误删其他集群接管后的安装。

SSH 模式不依赖目标 Connector 控制连接。Direct Executor 或 Bastion Connector 持有独立 SSH 进程和短期 Credential Lease，接收 Helper 输出的 Evidence 并验证摘要后才关闭 SSH/Executor Tunnel；Connector 回传结果同样携带完整 Evidence，Server 在持有 Removal Operation 行锁时再次校验。

两条 SSH 路径共用 `hostremoval.ExecuteSSH`，包括流式脚本、上下文取消、身份核对、证据解析和重放恢复。经堡垒机的 Windows 卸载负载包含冻结的 RDP 变更 ID、before/applied 快照；派发前再次验证凭据版本。

Gateway 与 Connector 使用 `connectorprotocol` 的同一份命令结果契约，包含卸载命令的 running/failed/succeeded 消息。不能只在执行函数中实现卸载而遗漏客户端首次 running 帧，否则执行前就会关闭控制连接。卸载重试与成功完成时，按 Operation ID 关闭旧的 delivery_unknown/result_unknown 尝试；这些命令可能发给父 Bastion，不能仅按被卸载成员的 Connector ID 清理，否则已成功移除的成员仍会阻塞堡垒机卸载。

## 4. Windows RDP 恢复

`host.windows_rdp.enable` 在修改前记录注册表、内置 RDP 防火墙规则和 `TermService` 运行状态，同时记录 Argus 实际应用值。卸载时逐项比较：只有当前值仍等于 Argus 应用值才恢复原值；管理员后续修改过的项保持不变并返回非阻断的 `LOCAL_CONFIG_DRIFT`。OpenSSH、用户软件和用户自行维护的防火墙规则不属于卸载范围。

## 5. API 与验收

企业 API 提供统一 Removal Preview、Operation 查询、重试和命令重新生成接口；公开 Bootstrap/Receipt 端点只接受操作绑定的专用 Token。完整命令只进入一次性结果或专用重新生成响应，不进入 Host/Bastion DTO、日志或审计正文。

验收至少覆盖 Linux 命令、Direct SSH、经堡垒机 SSH、Executor Tunnel、Collector 顺序、堡垒机依赖阻塞、每阶段故障恢复、Token 重放、Host Key 漂移、强制移除，以及 Windows SCM、ACL、RDP 快照恢复和漂移。临时 Kubernetes Namespace、凭据和测试产物在 E2E 后删除。

2026-09-12 未注册主机与堡垒机的取消安装、直接删除已增量部署到本地 `argus-local`。先备份 PostgreSQL，再应用 `00002_cancel_unregistered_onboarding.sql` 并滚动更新 Server、Worker、Direct Executor、Connector Gateway 与 Web；数据库版本为 2，12 个已有 PVC 的 UID、绑定卷与 Bound 状态保持不变，用户的 `123` 主机记录仍保留。验证通过 Go 全量测试与 vet、三个运行包的 Linux race 检测、隔离 PostgreSQL 取消事务/回滚/凭据与命令隔离/迟到状态更新/已删除目标的 Action 收敛测试、Enterprise 94 项与 API Client 74 项单测、卸载 mock Playwright 25/25、部署后两个门户同源登录测试，以及部署健康检查 20/20。真实 Web 已包含取消删除入口且没有 `api.argus.invalid`。本次未执行完整 Kubernetes SSH 安装后删除 E2E 或 Windows VM 验收，也没有删除用户现有资源；本地 NetworkPolicy enforcement 未验证、无外部 Egress Gateway 和共享 Sandbox Runtime 的既有降级项仍保留。

2026-09-11 SSH 凭据复用与卸载入口状态修复已增量部署到本地 `argus-local`。验证包括 Go 全量测试与 vet、隔离 PostgreSQL 当前身份来源/历史卸载投影回归、真实 Linux SSH 卸载/堡垒机 SSH 卸载/严格 HTTPS 手动卸载及经堡垒机执行入口、Enterprise 91 项单测、API Client 69 项单测、卸载 mock Playwright 17/17，以及部署后两个门户的同源登录请求测试。部署健康检查 20/20 通过，12 个已有 PVC 的 UID、绑定卷与 Bound 状态均保持不变。P4 Kubernetes 卸载场景已改为从 connection-defaults 读取账号和凭据后创建新 ConnectionTest，覆盖 Direct、Executor Tunnel、经堡垒机成员和堡垒机；本次未执行完整 Kubernetes 卸载 E2E 或 Windows VM 实机卸载，也未卸载用户现有主机。

2026-09-10 两种处理选项的前端回归通过：Enterprise 类型检查、卸载弹窗与关键操作确认组件测试 14/14、`host-removal.spec.ts` Playwright 13/13。覆盖离线主机及堡垒机无 SSH 凭据生成仅移除预览、资源名称确认、在线禁用、SSH 超时等失败原因、切换/关闭后的迟到结果隔离，以及 MFA 成功后确认原预览、取消/无效证明不继续执行。中英文与深浅色截图已核对。Playwright 使用 mock 模式；本次未部署到 `argus.dev`，未执行真实集群 SSH 卸载、身份吊销或 Windows VM 验收。

2026-09-11 已重新构建并增量升级本地 `argus-local`：先启动已停止的 Docker Desktop，保存 PostgreSQL 备份及 PVC/镜像快照，再执行 `argusctl images build/load` 并滚动更新 9 个 Argus Deployment。部署配置与上次安装快照一致，重建后的 Backend/MinIO 内容摘要也保持一致，因此没有执行数据库迁移或完整 Helm 安装。前端新内容摘要为 `sha256:09423a75af2f4ad7bdcaf65cdfc2e716780c4003125ec052c460878b24676c30`，实际 HTTPS 提供的卸载模块已确认包含两种选项和 SSH 超时提示。部署检查 20/20、组件测试 14/14、mock Playwright 13/13 通过；两个真实门户的匿名登录页及主题/语言切换正常。6 个 PVC 的 UID、绑定卷保持一致且全部 Bound，13 条主机记录保留。证据位于 `artifacts/redeploy-20260911-removal-options/`。本次未提交真实主机移除或 MFA 证明。D 盘约剩 23 GiB，完整安装要求的 25 GiB 磁盘预检仍不满足；本地 NetworkPolicy enforcement 未验证、无外部 Egress Gateway、共享 Sandbox Runtime 的降级项仍保留。

`go run ./cmd/argus-dev check installer-containers` 在 Ubuntu、Debian、Alpine 的 amd64/arm64 环境检查安装脚本，并在 Alpine 隔离文件系统中运行两个架构的卸载 Helper：先占用 Relay 端口注入失败，确认失败 Evidence 和 Journal，再释放端口连续重跑，确认 Collector/Connector 路径清理和成功 Evidence 幂等收敛。Windows Server 真实 SCM、锁文件重启恢复、OpenSSH 和 RDP 仍由专用 VM E2E 验收。

`ARGUS_REMOVAL_INTEGRATION=1 go test ./internal/hostremoval ./internal/app/connector -run 'TestRemovalIntegration|TestBastionSSHRemovalIntegration' -count=1 -v` 创建独立 PostgreSQL、systemd/OpenSSH 目标容器并自动清理。`ARGUS_REMOVAL_SYSTEMD_IMAGE` 指定预构建的 `deploy/docker/e2e-systemd-host.Dockerfile` 镜像，要求包含 curl，不在测试中依赖 apt 在线安装。覆盖实际严格 HTTPS 命令下载、独立 Helper 清理、回执提交前后断网重试、数据库终态、真实 SSH 传输、Bastion Connector 的 protobuf/SSH 执行入口，以及同机新身份拒绝删除。测试中的 Collector/Connector 服务使用固定测试进程；这些测试不替代 Windows VM 或完整 Kubernetes SSH 凭据派发验收。

2026-09-08 上述 Linux 容器集成测试通过；后端 `go test ./...`、Enterprise 类型检查及卸载对话框 Playwright 3/3 通过。此次修复区分连接重连与身份替换，修正卸载期间的堡垒机心跳处理，并统一 SSH 清理执行和恢复语义。

2026-09-09 本地滚动升级至 Connector 发行版 `dev-fc7c77d549790032`。在原 `argus-test-host` 上恢复已批准但返回 HTTP 409 的卸载操作，沿用原 Operation 和 Connector 身份，重新生成严格 TLS 命令后完成本机清理与回执提交。数据库 Operation 为 `succeeded/completed`，Host/Scope 为 `uninstalled`，本机服务、文件、账号及 Relay 监听消失，OpenSSH 保持 active，Connector 证书已撤销；记录没有删除。证据见 `artifacts/removal-user-{recovery,local-verification,server-verification}.log`。

滚动更新完成后的 `artifacts/verify-20260909-removal-fix-final/verify.json` 部署检查 20/20 通过。本地 evaluation 环境仍报告 NetworkPolicy enforcement 未验证、无外部 Egress Gateway 及共享 Sandbox Runtime 降级项；这些不等同于生产网络隔离验收通过。

2026-09-09 SSH 安装修复回归增加普通主机 SSH、堡垒机 SSH、堡垒机命令三个独立清理场景，均通过 PostgreSQL/真实 systemd 与 SSH 容器集成测试（`artifacts/ssh-fix-removal-matrix.log`）。Bastion Connector 执行成员 SSH 卸载、身份误配拒绝及重复执行也通过（`artifacts/ssh-fix-removal-integration.log`）。这些测试验证实际 Helper/SSH 与回执收敛，但未宣称通过完整 Kubernetes 凭据派发、Executor Tunnel 或 Windows VM 的全部卸载验收。

2026-09-07 已使用新 PostgreSQL 基线清空并重建 `argus-local`。正式库确认 `host_removal_operations`、步骤和 Token 表存在且 Host 数量为 0。补齐独立 Helper 和 Cleanup Evidence 后，三平台 Connector 发行版 `dev-d41a9c3dc43c39c3` 已同步到 Artifact Store；`artifacts/verify-20260907-host-removal-contract-final` 的 `argusctl verify` 20/20 通过。Windows Server 2019/2022 的真实 SCM、OpenSSH 和 RDP 恢复仍按 `docs/20-windows-host-e2e.md` 在实体 VM 执行。
