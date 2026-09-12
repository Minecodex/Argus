# 跨平台主机接入、堡垒机中继与快速安装

本文是 Host、Bastion、Connector、Collector 和人工远程会话的当前权威设计。旧的 `connection_mode`、Host 自助安装 Collector、WinRM/WinRS 和 Direct Executor 人工会话设计均已删除。

## 1. 稳定资源模型

`Host` 只保存稳定事实：

- `role`: `managed_host | bastion`
- `platform`: `linux | windows`
- `architecture`: `amd64 | arm64`
- `connector_id`: 该机器本机 Connector 身份
- `control_path`: `direct | bastion_relay | executor_tunnel`
- `bastion_scope_id`: 仅 `bastion_relay` 成员和 Bastion 根 Host 使用

安装方式和 SSH 执行路径冻结在 `host_onboarding_operations` 中。所有路径都由同一个阶段推进器写入当前阶段和有序事件；手工引导在领取脚本时记录本地预检与传输交接，SSH 路径在远端探测和产物传输完成后逐级推进：

- `install_method`: `manual | ssh`
- `ssh_path`: `none | direct_executor | bastion_connector`
- 阶段：`queued → probing → transferring → installing → enrolling → waiting_online → completed`

Operation 同时冻结目标平台、ConnectionTest、SSH Host Key、Credential 版本、控制路径、Bastion Scope、Connector 发行版、Trust Bundle epoch/hash 和计划哈希。切换任一安全相关字段都必须重新 Preview；同一已围栏 Connector 的正常重连可以产生更高 connection epoch，不使已完成的 Host Key 与凭据证据失效。

## 2. Connector 角色

`argus-connector` 是统一接入程序：

| role | 平台 | 职责 |
| --- | --- | --- |
| `host` | Linux amd64/arm64、Windows amd64 | 本机类型化命令、PTY/ConPTY、可选本机 OpenSSH、Collector 生命周期、本机 TCP 会话隧道 |
| `bastion` | Linux amd64/arm64 | 根 Host 能力、成员 SSH 探测和安装、固定 TLS 密文中继 |
| `kubernetes` | Linux 容器 | Kubernetes 查询和 Collector 生命周期 |

普通主机首次只安装 Connector。Collector 必须由用户随后在主机详情执行独立 Preview/Commit 才会安装。Connector 身份、Collector 遥测身份和 Gateway 身份互相独立。

## 3. 安装矩阵

企业门户的普通主机向导先展示五种网络场景，再进入主机信息。场景分别对应“命令直连、Direct Executor SSH 直连、Direct Executor SSH + 控制隧道、命令经 Bastion Relay、Bastion Connector SSH + Relay”；页面由场景唯一派生安装方式、SSH 执行者和控制路径，不要求用户理解或组合内部枚举。Linux/Windows 选择、SSH 地址和凭据放在第二步，第三步执行真实连接测试与 Preview/Commit。

| 目标 | 一行命令 | 平台 Direct SSH | Bastion Connector SSH | Executor 控制隧道 | Bastion TLS 中继 |
| --- | --- | --- | --- | --- | --- |
| Linux Host | 是 | 是 | 是 | 是 | 是 |
| Windows Host 2019+ x64 | PowerShell 5.1+ | OpenSSH Server | 经 Linux Bastion 的 OpenSSH | 是 | 是 |
| Linux Bastion | 是 | 是 | 不适用 | 是 | 作为中继端 |

SSH 安装先固定 Host Key，再探测 OS、发行版或 Windows Server 版本、架构、管理员权限、systemd/SCM、至少 512 MiB 可用磁盘和 Enrollment/Gateway 目标回连。上述证据写入 ConnectionTest，并连同凭据版本冻结到安装计划；任一事实变化都会使旧 Preview 失效。Windows 命令始终显式调用 `powershell.exe -NoProfile -NonInteractive -EncodedCommand`。二进制通过 SSH stdin 传输，不进入命令行。SSH 只承担安装；安装完成后控制和人工会话依赖 Connector 主动长连接。

集群内的 Artifact 预检、下载和 Connector Enrollment 隧道转发共享同一个内部 HTTPS 入口。`argusctl` 按 IngressClass 发现唯一的 ingress-nginx HTTPS Service，将内部拨号地址注入 Server、Worker 和 Direct Executor；多个入口或其他控制器必须在 `spec.exposure.httpsInternalAddress` 显式指定 `host:port`。Artifact 客户端只对公开 Artifact Origin 使用内部入口，保留 URL、HTTP Host、TLS SNI、CA 校验和不可变产物摘要/签名，拒绝跨 Origin 重定向；Enrollment 仍保留公开 URL、TLS SNI 和 CA 校验。内部路由不下发给外部 Host/Bastion，外部节点继续使用自己的直连或固定中继地址。集群内路径不回退公开 DNS，也不再通过另一个明文 MinIO 地址绕过实际下载路径。

失败或到期且尚未注册 Connector 的 SSH Host 可以通过 `hosts/{id}/actions/preview-retry` 使用新的 ConnectionTest 和当前凭据创建重试预览。Commit 再次验证 Host 版本、原失败任务和机器未注册状态，在同一事务内撤销原 Enrollment Token、原操作的 Executor 控制隧道与凭据租约，保留 Host ID 并创建带 `retry_of` 的新操作。清理按企业、Host 和原操作绑定的 Connector 定位，不影响其他隧道。Executor 每秒核对隧道状态、owner、fence 与 epoch，权威状态失效时关闭旧监听和 SSH；连续三次状态读取失败也关闭通道。已注册身份必须走替换生命周期。列表以安装投影展示失败状态、具体错误码和阶段；普通连接状态不覆盖安装失败。

SSH Host 安装在传输二进制前，经目标机实际回连路径执行严格 HTTPS 预检：使用冻结的 Enrollment URL、拨号覆盖和 Trust Bundle 访问 `/api/v1/setup/status`，要求平台已初始化，禁止 HTTP 重定向、代理绕行和关闭证书校验。仅能连接目标回环端口不代表隧道上游可用。该预检不携带 Enrollment Token；DNS、连接、TLS、HTTP 状态、响应内容和超时失败以 `HOST_ONBOARDING_CALLBACK_*` 安全错误码记录在 `probing` 阶段，页面提供中英文说明，后台记录不含凭据的预检错误。

创建、重试及替换安装的回连检查同时前移到 ConnectionTest，位于生成 Preview 之前。向导根据已选场景发送 `onboarding_control_path`；服务端冻结公开 Enrollment/Gateway 地址、Trust Bundle 内容与代数，以及 Bastion Relay 的地址、端口和代数。Direct 与 Bastion SSH 探测必须从目标机实际拨号，验证 Enrollment 严格 HTTPS 状态和 Gateway 服务端 CA/SNI/h2；Gateway 预检只验证服务端 TLS，不代表未注册机器已完成客户端 mTLS 认证。仅 TCP 可连接、普通 SSH 成功或旧 Connector 未返回回连证据都不能作为安装通过的依据。

Executor Tunnel 场景使用本次 SSH 会话的两个临时回环监听，将探测转发到执行器既有的内部 HTTPS/Gateway 目标；监听采用临时端口，在完成、失败和取消时关闭，不创建资源、注册令牌或持久控制隧道。Bastion Relay 场景验证冻结的成员可达中继地址。Preview 与 Commit 均要求检查目的、路径和成功证据匹配，并拒绝 CA、公开端点或 Relay 代数变化后的旧证据；快速重试只能复用仍有效且匹配的安装预检。正式安装仍在传输前再次验证实际链路。卸载等纯 SSH 操作不发送 `onboarding_control_path`，不依赖平台回连可用性。

连接测试失败时，页面同时展示已通过的 SSH/系统检查和具体回连错误，并将失败摘要滚入视野。直连回连失败提示目标机检查平台域名解析、端口及网络；不统一误导为集群内部 Service 故障。生产环境通过目标网络的 DNS 提供真实可达入口；临时测试主机可配置对应 hosts 映射，不能将浏览器侧的 loopback 解析当作外部主机可用的入口。

2026-09-12 本地 `12345`（`192.168.0.100:2222`）的失败根因为目标机将 `argus.dev` 解析到自身 `127.0.0.1`，另一个 Gateway 域名解析到 `198.18.0.37` 后 TLS 握手返回 EOF。保留原 `/etc/hosts` 备份后，仅为该测试主机将两个域名映射到已验证的当前平台入口 `192.168.0.100`；使用原冻结 CA/SNI 验证 Enrollment HTTP 200 和 Gateway h2 成功。旧本机 Connector 对应的 Host 已删除且证书已吊销。发布 `dev-callback-preflight-20260912-v1` 后，沿原创建者的当前权限、数据授权与正式 Preview/Confirm 工作流为原 Host 创建新连接测试和重试操作：新测试包含 `onboarding_callback=passed`，操作 `01a09650-c03e-7d71-996c-5153315da784` 为 `succeeded/completed`，新 Connector `01a09650-c03b-7044-a608-4fa940f4c394` 与原 Host 均为 `online`，Action 同样收敛为 `succeeded`，并保留 `retry_of`。

本次验证通过 Go 全量测试与 vet、契约再生成检查、真实 SSH/HTTPS/Gateway TLS 与临时隧道清理测试、独立 Kubernetes Namespace 内的 PostgreSQL 预检证据/目的/CA 与 Relay 漂移集成测试、65 项相关前端组件与 83 项 API Client 测试、41 项桌面 Mock Playwright，以及更新后两个门户同源登录请求测试。部署检查 20/20 通过，12 个原有 PVC 的 UID、绑定卷和 Bound 状态保持不变；测试 Namespace、恢复 Job 和端口转发均已清理。未执行 Windows VM 实机和完整多主机 Kubernetes 接入矩阵；本地既有网络隔离降级项仍不代表生产隔离验收。验证日志分别位于临时目录的 `argus-callback-recovery-success.log` 与 `argus-callback-deployed-verify/`。

Direct Executor 在安装期间续期任务租约；取消或失去租约时关闭 SSH。Bastion 派发前重查凭据版本，恢复执行时仅处理自身持有租约的操作；回连探测同时使用冻结的 Enrollment 和 Gateway 地址。

Linux 使用 `/usr/local/bin/argus-connector`、`/var/lib/argus-connector`、`/etc/argus-connector`、独立系统账号和 systemd。Windows 使用 `C:\Program Files\Argus\Connector`、`C:\ProgramData\Argus\Connector`、SCM `ArgusConnector` Service、Windows ACL 和服务恢复策略。

## 4. 堡垒机固定 TLS 中继

Linux Bastion Connector 内置两个 TLS 密文 Listener。创建和安装堡垒机时不要求用户填写绑定地址或端口：

- HTTPS 从 `8445` 起向上尝试最多 20 个端口：Enrollment、Bootstrap 和 Artifact HTTPS 密文
- Connector Gateway 从 `9445` 起向上尝试最多 20 个端口：Connector Gateway gRPC/TLS 密文

首次启动直接执行原子端口绑定，不使用“先检查再监听”的竞态流程。两个端口均成功后，Connector 将成员拨号地址、实际端口和 `generation=1` 原子保存到本机状态目录并通过结构化心跳上报。后续重启只绑定已保存端口；如果端口被占用，Relay 报告 `RELAY_PORT_IN_USE` 并保持 Connector 控制通道在线，不扫描新端口。首次候选范围耗尽报告 `RELAY_PORT_RANGE_EXHAUSTED`。运维人员释放冲突端口并重启 Connector 即可恢复；未来若提供主动迁移端口的能力，必须使用独立 Preview/Commit 并冻结受影响成员和新的端口代数。

中继只连接安装时冻结的 Argus 上游。连接必须来自配置的源 CIDR，TLS ClientHello 的 SNI 必须等于 Argus 端点，ALPN 仅允许 HTTPS 的 `h2/http/1.1` 或 Gateway 的 `h2`。中继限制每源速率、总并发、握手大小和超时。TLS 在目标 Host Connector 与 Argus 之间端到端终止；Bastion 无法读取 Enrollment Token、客户端私钥、控制命令或会话内容。成员到堡垒机入口的路由、防火墙、ACL、NAT 和 DNS 由客户管理；Argus 只展示最终端口并执行可达性检查。

Connector 只有在两个 Listener 均绑定成功、入口地址可用且本机端口状态已持久化后才通过心跳上报 `relay_status=ready`。控制面保存实际端口，后续成员安装操作冻结该地址、端口和代数；控制面不会在 enrollment 成功时提前宣称 Relay 可用。

## 5. TLS 引导

集群配置仍使用 `ArgusInstallConfig`：

```yaml
spec:
  pki:
    mode: managed # 或 existing-cluster-issuer
    bootstrapTLSMode: insecure-first-fetch # 或 strict
```

`managed` 默认 `insecure-first-fetch`，`existing-cluster-issuer` 默认 `strict`。

`insecure-first-fetch` 只允许下载首次动态 Bootstrap 脚本的请求跳过服务端证书校验。该请求携带一次性 Enrollment Token，因此 UI 和 `argusctl plan` 必须明确提示可能被伪造服务端截获。Bootstrap 脚本内嵌当前 Trust Bundle；安装器、Manifest、Artifact、Enrollment、Connector Gateway 和全部运行期连接始终严格校验 CA、SNI、有效期、Bundle epoch 和身份，不存在运行期开关。

`InstallInstructionSet` 每次只返回目标平台的一条系统级指令，包含 `platform`、`shell`、`privilege=system`、`command`、`bootstrap_tls_mode`、发行版本、Bundle epoch/hash、Bootstrap SHA、安装器 SHA 和有效期。一行命令在执行动态 Bootstrap 前先校验它的确定字节 SHA-256，因此 `insecure-first-fetch` 的放宽不能把任意响应直接交给 Shell/PowerShell。

Windows 首装不依赖 OpenSSL。下载的 Connector 先按 Manifest 校验大小和 SHA-256，再调用其受摘要约束的 `verify-artifact` 子命令校验 Ed25519 签名；Collector 后装复用已安装 Connector 的同一验证器。由此 PowerShell 5.1、`curl.exe` 和系统管理权限构成脚本侧最低依赖。

## 6. Collector 后装

主机详情的“启用采集”创建独立 Collector Preview/Commit：

- `direct_argus`: Collector 直接向 Argus 上报。
- `bastion_gateway`: Leaf Collector 向所属 Bastion Edge Gateway 上报；若 Edge Gateway 尚未存在，Preview 自动冻结 Gateway Collector 身份、发行版、Profile 和执行路径，并先列出启用 Gateway、再安装 Leaf 的顺序计划。Commit 同时创建两项管理命令，Leaf 命令只有在 Gateway 达到 `converged` 后才可派发。
- 无出站 Host 可选择 `executor_tunnel`；Bastion 网络可选择 `bastion_tunnel`。隧道默认使用 `14317/14318` 作为数据与身份回环端口，避免占用 Edge Gateway 或 OTLP Receiver 的标准 `4317` 监听。
- `bastion_tunnel` 的 Collector 生命周期命令仍发送给成员自己的 Host Connector；独立数据隧道由所属 Scope 当前在线的 Bastion Connector 发起，并复用该成员最近一次成功 SSH 接入操作中冻结且仍有效的地址、Host Key 与凭据版本。两种 Connector 身份不得混用。

Linux Collector 使用 systemd。Windows Collector 由 Host Connector 安装签名 ZIP，通过 Connector 的 Windows SCM 包装入口运行独立 `ArgusCollector` Service。配置、Trust Bundle、遥测身份和状态保存到 `C:\ProgramData\Argus\Collector`。

## 7. 人工会话

所有人工会话均通过 Host 本机 Connector：

- `shell`: Linux PTY；Windows PowerShell/ConPTY。
- `ssh`: 仅在心跳确认本机 OpenSSH 可用时开放，Connector 只连接 `127.0.0.1`。
- `rdp`: 仅 Windows；Host Connector 只连接 `127.0.0.1:3389` 并转发 RDP 密文。

RDP 密码的短期 Credential Lease 只由 Connector Gateway 领取并交给同 Pod 的 guacd，不发送给浏览器或 Host Connector。Gateway 为每次会话创建临时 loopback bridge，将 Guacamole 协议转发给浏览器。剪贴板、文件、pipe/blob、磁盘映射和打印默认禁用并在 Gateway 双向拦截；键鼠、分辨率、授权复检和会话时长受 SessionProfile 控制。输出流以 `guacamole_v1` 独立格式分片加密上传并保存哈希链，浏览器使用 Guacamole 画面播放器回放；Shell/SSH 录像继续使用 `asciicast_v2` 终端播放器。

Connector 心跳只检测 RDP 注册表、NLA、`TermService`、内置防火墙规则和 OpenSSH 状态。未就绪时服务端拒绝 RDP。启用必须使用 `host.windows_rdp.enable.preview/commit`，Preview 明确展示注册表、服务和防火墙变更。

## 8. 安全边界

- 一次性命令严格绑定 Host、平台、架构、Connector role、控制路径、Scope、发行版本和 Bundle。
- Token、密码、私钥和一次性命令只进入专用密文记录或一次性结果，不进入普通 DTO、日志、审计正文和浏览器持久化。
- Bastion SSH 安装的 Credential Lease 只发送给目标 Bastion Connector。
- Artifact 必须同时通过 HTTPS Trust Bundle、SHA-256、大小和 Ed25519 签名校验。
- 运行期 TLS 不允许 `--insecure`、`InsecureSkipVerify` 或错误 CA/SNI 回退。

## 9. 验收与切换

Host/Bastion 卸载不再复用 Connector 自停命令或 Host 直接删除。安装来源决定命令或 SSH 卸载路径；正常完成保留 `uninstalled` 记录，失联强制移除记录本机残留风险。完整状态机、围栏和 Windows RDP 恢复规则见 [21-host-and-bastion-removal.md](21-host-and-bastion-removal.md)。

本地门禁包含 Shell LF、真实 `dash`/PowerShell 解析、Linux/Windows 交叉编译、发行清单矩阵、状态机组合、TLS 例外扫描、Go/TypeScript 测试和 Helm 渲染。`check installer-containers` 已在 Ubuntu、Debian、Alpine 的 amd64/arm64 容器中执行 fail-closed 预检，并在 QEMU arm64 上真实执行 Connector 的 Ed25519 校验；`check guacd` 使用正式 guacd 1.6 验证协议握手。Kubernetes 临时 Namespace 验证 Linux 命令/SSH/经 Bastion/Executor Tunnel、Relay 重启与重放拒绝、缺失 Edge Gateway 时的自动顺序安装、Collector direct/Bastion 两条三信号链路。

TLS 专项运行 `20260906-tls-complete4` 已完成 `managed + strict` 与 `existing-cluster-issuer + strict` 两个独立临时集群：两者均验证未信任失败、预置 CA 后同命令注册成功，并分别通过 20/20 `argusctl verify` 后零残留清理。`managed + insecure-first-fetch` 由 P4 运行验证；错误 CA、SNI、过期证书、摘要篡改和旧 Bundle epoch 由 Linux HTTPS 执行测试与 Connector 状态测试覆盖。

最新 Linux/堡垒机回归为 `20260906-complete13`，包含 SSH Secret 轮换导致 Credential 版本推进、旧 ConnectionTest 过期和旧 Preview 拒绝，最终 real Chromium 与 20/20 校验通过并完成零残留清理。随后 `argus-local` 已从单一 PostgreSQL 基线全新部署，正式校验证据位于 `artifacts/verify-20260906-final-complete13/verify.json`。

2026-09-09 SSH 修复验证：`internal/artifacthttp` 覆盖共享 HEAD/GET 路由、固定 Origin、错误 SNI 和外部代理隔离；独立临时 Namespace 故意将 Artifact 域名映射到 loopback，使用内部 HTTPS 入口下载正式 Connector 并验证 SHA-256，证据为 `artifacts/ssh-fix-k8s-artifact-probe.log`，Namespace 已删除。安装失败与重试入口、卸载对话框的定向 Playwright 测试通过；后端全量测试通过。P4 新增错误 Artifact DNS 和 SSH 卸载矩阵，但完整 P4 运行被 dedicated-cluster 门禁阻止：当前集群已有 Argus/OpenSandbox/Strimzi 的全局资源，未覆盖或删除这些资源。随后通过服务端正式的领域 Preview/Commit 与已确认任务恢复流程，将用户主机 123 的新 SSH 安装操作推进至 online/succeeded，并保留 retry_of 关联。在独立 Docker 目标上，又完整验证了堡垒机 Direct SSH 安装、禁止直连 Argus 的成员经 Bastion SSH 安装并通过 TLS Relay 上线、成员经 Bastion SSH 卸载、堡垒机 Direct SSH 卸载，以及普通主机 Direct SSH 安装/卸载。三项临时资源均 local_cleanup=verified、记录 deleted、证书 revoked，容器已清理。证据为 artifacts/ssh-fix-user-recovery.log 与 artifacts/ssh-fix-live-matrix-verification.log；没有修改原主机 hosts 或清空业务库。这些结果不代表 Windows VM、Executor Tunnel 和全部 Collector 遥测矩阵已重新验收。

Windows Server 2019/2022 专用 VM 使用 `argus-dev e2e windows-host`：配置只引用环境变量和私钥文件，固定 OpenSSH Host Key，并自动验证 PowerShell 安装、SCM、服务恢复、ACL、MachineGuid、升级/卸载、CA epoch、ConPTY、OpenSSH 和 RDP 就绪证据。在该实体矩阵通过前，Windows Collector Catalog 保持 `validation_pending`，不得向用户宣称已完成正式支持验收。

本次切换不兼容旧数据和旧 API。PostgreSQL 只保留 `00001_argus_baseline.sql`，直接创建当前结构，不包含 `TRUNCATE/ALTER` 兼容迁移。部署前清空开发数据库，旧命令全部作废，所有 Host/Bastion 必须重新生成命令并注册。
