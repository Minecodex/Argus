# Connector、堡垒机、主机与 Kubernetes 资源管理

## 1. 领域边界

Argus 将资源事实、安装执行、控制连接、遥测和人工会话分开：

- Host/Kubernetes 资源服务保存用户可管理的稳定资源事实。
- Connector 服务负责 enrollment、mTLS 长连接、类型化命令和主机会话。
- Host Onboarding Operation 负责手动或 SSH 安装的持久状态机。
- Telemetry 服务负责 Collector、配置版本、路由、身份和 OTLP 数据面。
- Remote Access 服务负责 Grant、审批、Lease、Ticket、录像和撤权。

普通主机首次只安装 Connector，不隐式安装 Collector。SSH 只承担初次安装；Connector 上线后，平台不再依赖 SSH 的持续可达性。

## 2. Host 模型

Host 稳定字段：

```text
role            managed_host | bastion
platform        linux | windows
architecture    amd64 | arm64
connector_id    本机 Connector
control_path    direct | bastion_relay | executor_tunnel
bastion_scope_id 仅 bastion_relay 成员和 Bastion 根 Host 使用
```

安装方式 `manual|ssh`、SSH 执行者 `none|direct_executor|bastion_connector`、目标地址、账号、Credential 版本、Host Key、发行版本和 Trust Bundle 只存在于短期 ConnectionTest/PendingAction/Operation，不进入 Host 稳定模型。

Windows 第一版只支持 `managed_host`，基线为 Windows Server 2019+ x64。Linux 支持普通 Host 和 Bastion。Windows 所有自动安装均使用 OpenSSH，不存在 WinRM/WinRS 契约。

## 3. Host 接入状态机

普通主机向导第一步只选择网络结果，不直接暴露 `install_method`、`ssh_path` 和 `control_path` 下拉框。当前五种场景固定映射为：

1. 主机可访问 Argus、一行命令：`manual + none + direct`。
2. 平台可 SSH、主机可访问 Argus：`ssh + direct_executor + direct`。
3. 平台可 SSH、主机无出站：`ssh + direct_executor + executor_tunnel`。
4. 主机可访问堡垒机、一行命令：`manual + none + bastion_relay`。
5. 堡垒机可 SSH 成员主机：`ssh + bastion_connector + bastion_relay`。

第二步选择 Linux 或 Windows 并只填写该场景需要的信息；SSH 场景填写地址、端口、账号和密码/私钥，平台自动探测架构；Bastion 场景只允许选择 Relay 已就绪且已上报入口的 Scope。场景切换会保留名称、环境、平台和标签，清除不兼容的 SSH/Scope 字段和 ConnectionTest。提交前仍由服务端穷举组合校验和真实连接测试兜底。

SSH 创建与重试的连接测试包含选定场景对应的平台回连验证，不能仅凭 SSH 登录和系统检查成功进入预览。向导内部派生 `onboarding_control_path`；回调的 DNS、HTTPS 或 Gateway TLS 失败在本步展示具体原因。普通 SSH 卸载仍使用独立、不要求平台回连的连接测试。证据冻结、临时控制隧道和提交时复验边界见 [19-cross-platform-host-onboarding.md](19-cross-platform-host-onboarding.md)。

普通主机和堡垒机创建时，在名称输入阶段查询当前名称是否可用，并在发起 SSH ConnectionTest 或创建预览前重新检查；冲突定位到名称输入框，修改名称后重新检测。安装重试沿用原资源名称，不把自身当作创建冲突。名称检查与创建统一去除首尾空白，企业内按 PostgreSQL `lower(name)` 判断重名，已删除记录不占用名称；普通主机检查全部 Host（含堡垒机根 Host），堡垒机同时检查 Host 和 Bastion Scope。生成创建预览前由领域服务再次检查，最终提交继续由现有部分唯一索引处理并发冲突。查询可用不代表名称已预留。

`host_onboarding_operations` 的阶段固定为：

```text
queued → probing → transferring → installing → enrolling → waiting_online → completed
```

失败保存稳定错误码、失败阶段、重试次数和事件时间线。手动命令安装同样创建 Operation；命令领取不表示安装完成，只有 Connector 成功建立长连接后才完成。

一次性命令绑定 Host ID、平台、架构、Connector role、控制路径、Bastion Scope、发行版本和 Bundle epoch/hash。切换任何字段必须重新 Preview 并生成新命令。

## 4. SSH 安装

Direct Executor 和 Bastion Connector 使用同一安装计划和平台探测规则：

- 固定 SSH Host Key，拒绝漂移。
- 探测 OS、架构、管理员权限、服务管理器、磁盘和目标回连。
- Linux 执行 POSIX Shell；Windows 显式执行 PowerShell 5.1+ EncodedCommand。
- 二进制通过 SSH stdin 传输，不嵌入命令行。
- Artifact 同时校验 HTTPS CA/SNI、SHA-256、大小和 Ed25519 签名。
- Credential Lease 只发给实际执行安装的一方；经 Bastion 安装时平台不获取成员凭据明文。

Linux 使用独立系统账号和 systemd。Windows 使用 SCM `ArgusConnector`、Program Files/ProgramData、ACL、MachineGuid 和服务恢复策略。

## 5. Bastion Scope 与 TLS Relay

Bastion 创建和安装不要求用户填写绑定地址或中继端口。Bastion Connector 首次启动时在所有接口上分别从 HTTPS `8445`、Connector Gateway `9445` 开始向上尝试最多 20 个端口，以实际绑定成功作为唯一判定；两个端口均成功后原子保存本机选择并通过心跳上报成员拨号地址、实际端口和端口代数。正常重启只复用已保存端口，冲突时保持控制通道在线并报告 `RELAY_PORT_IN_USE`，不得自动漂移。

Scope 保存 Connector 上报的成员入口。后续成员安装计划只读取 `relay_status=ready` 的实际地址和端口并冻结端口代数。成员到堡垒机的路由、防火墙、ACL、NAT 和 DNS 由客户网络负责；Argus 展示需要放行的 TCP 端口并检测可达性，但不修改客户网络。Bastion Connector 只连接 Argus 固定上游，检查源 CIDR、SNI、ALPN、连接数和速率，转发 TLS 密文。它不终止成员 TLS，也不能读取成员 Enrollment Token、私钥、命令或会话数据。

两个 Listener 绑定并持久化成功后 Connector 才在心跳中上报 `relay_status=ready`。首次 20 个候选端口全部不可用时报告 `RELAY_PORT_RANGE_EXHAUSTED`；已选端口在重启时冲突则报告 `RELAY_PORT_IN_USE`。Relay 重启、降级、离线和恢复均与 Connector 在线状态分别展示。

Bastion 创建方式：

- 一行 Linux 系统命令。
- 平台 Direct SSH 安装。
- 平台 Direct SSH 安装并保持 Executor 控制隧道。

## 6. Kubernetes Connector

Kubernetes Connector 保持独立 `role=kubernetes`，用于集群资源查询和 Kubernetes Collector 生命周期。它不作为 Host Connector，也不承载 Host RDP。Kubernetes 资源仍可通过 Direct 或 Bastion 路径访问，凭据和 Namespace 授权保持原有边界。

## 7. Collector 生命周期

主机详情中的“启用采集”创建独立 Preview/Commit：

- Host Connector 下载、校验、安装、配置、升级和卸载本机 Collector。
- Linux Collector 使用 systemd。
- Windows Collector 使用独立 `ArgusCollector` SCM Service。
- `direct_argus` 直接上报平台。
- `bastion_gateway` 先保证 Bastion Edge Gateway 存在，再安装 Leaf Collector。
- 无出站网络使用受控 Telemetry Tunnel；控制身份和遥测身份不复用。

首装 Host 完成时不得创建 CollectorInstance。只有用户确认 Collector Preview 后才创建并激活遥测身份和路由。

## 8. 类型化命令与人工会话

Host/Bastion 卸载使用独立 `host_removal_operations` 状态机。命令安装生成严格 TLS 的一次性卸载命令；SSH 安装重新建立独立 SSH 会话并在 Connector 停止后继续验证。正常卸载后资源保留为 `uninstalled`，只有再次确认才删除记录。堡垒机依赖、失联强制移除和本机 Journal 见 [21-host-and-bastion-removal.md](21-host-and-bastion-removal.md)。

同一台机器执行另一套 Argus 新生成的安装命令或 SSH Operation 时，后一套系统自动接管。新身份先在私有暂存目录完成 enrollment，成功后才停止旧 Connector/Collector、提升二进制和 CA 并启动新服务；前一套系统按连接超时转为离线。堡垒机卡片提供“替换 Connector”入口，不要求用户先登录机器手工清理。完整事务和一次性命令边界见 [22-cross-cluster-connector-takeover.md](22-cross-cluster-connector-takeover.md)。

ConnectorCommand 只接收版本化 protobuf 负载。Host Connector 支持本机 Collector 管理、运行状态探测、Windows RDP 启用和受控本机命令；任意 Shell 字符串不作为后台命令协议。

人工会话协议：

| protocol | 执行位置 | 条件 |
| --- | --- | --- |
| `shell` | Host Connector 本机 Linux PTY / Windows PowerShell ConPTY | Connector 在线 |
| `ssh` | Connector 到 `127.0.0.1` OpenSSH | 心跳确认 sshd 可用，Host Key 和 ManagedAccount 授权有效 |
| `rdp` | Connector 到 `127.0.0.1:3389`，Gateway/guacd 转浏览器画面 | Windows、RDP/NLA/防火墙/服务均已确认 |

RDP Credential Lease 只由 Gateway 领取并送入同 Pod guacd。浏览器和 Host Connector不接收密码。剪贴板、文件、磁盘和打印默认禁用；启用 RDP 必须使用独立 Preview/Commit，安装过程只检测不修改。

## 9. 删除与重建

本次切换不保留旧字段、API、Mock、页面和 WinRM 适配器。开发部署清空业务数据后运行新迁移；已有命令作废，所有 Host/Bastion 重新注册。删除 Host 会撤销 Connector 身份、进行中的命令、会话和 Collector 管理关系；Bastion 有成员时不能删除。

详细安装、TLS、RDP 和验收矩阵见 [跨平台主机接入设计](./19-cross-platform-host-onboarding.md)。
