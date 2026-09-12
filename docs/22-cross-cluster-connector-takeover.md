# Connector 跨集群接管

本文定义同一台 Linux 或 Windows 机器依次接入两套独立 Argus 时的行为。机器只运行一套 Host/Bastion Connector；后执行的有效安装命令或 SSH 安装操作接管机器，前一套 Argus 中的 Connector 在连接断开和心跳超时后转为离线。

## 1. 用户语义

- A 集群安装成功后机器在 A 在线。
- 用户在 B 集群为同一台机器生成一条新的命令，或提交一次新的 SSH 安装操作；B 安装成功后机器在 B 在线，A 的连接被关闭并转为离线。
- 之后可在 A 的原资源上执行“替换 Connector”，生成新的命令或 SSH Operation，把机器重新接回 A。
- 每次接管都使用目标集群新生成的一次性命令或 Operation。已经成功消费的旧命令不能再次使用，避免允许旧 Token 携带不同公钥重新注册。
- 页面不增加平台或脚本 Tab。堡垒机卡片直接提供“替换 Connector”；命令模式返回一条新命令，SSH 模式重新执行 Host Key 和凭据连接测试。

## 2. 本机事务边界

Connector ID 不同时，`GenerateLocalIdentity` 不修改当前密钥、CSR、证书和 CA，而是在正式状态目录下的私有 `.enrollment/<new-connector-id>` 子目录生成或复用新密钥与 CSR。服务端 enrollment 成功后按以下顺序提交：

1. 写入新私钥、CSR、证书和运行期 Trust Bundle。
2. 删除旧 Connector 的命令结果和 Bastion Relay 端口状态。
3. 最后写 `identity.json` 作为本机身份提交标记。
4. 删除所有 enrollment 暂存私钥。

安装器使用已经验签的新 Connector 二进制执行暂存 enrollment。在 enrollment 成功前，不停止当前 Connector，也不替换运行二进制、CA、systemd/SCM 配置或 `.desired-connector-id`。因此下载、TLS、Token、CSR 或服务端校验失败时，当前集群连接继续运行；相同 Token 的安装重试复用暂存密钥，满足服务端幂等公钥约束。

## 3. 成功接管

enrollment 成功即授权本次接管。安装器随后：

- 停止旧 Connector 和 privileged helper。
- 停止并删除旧 Collector；普通 Host 首次进入新集群仍保持“只安装 Connector”。
- Bastion Connector 停止后释放 Relay 端口；新身份不复用旧 `bastion-relay.json`，重新从 8445/9445 起选择端口并上报。
- 原子提升暂存二进制、CA 和签名密钥，写入新的 desired Connector ID。
- 写 systemd 或 SCM 配置，启动新 Connector 并等待服务运行。
- SSH 安装成功后删除 `/var/lib/argus-connector-install/<id>` 或 `C:\ProgramData\Argus\Install\<id>`；失败时保留 root/SYSTEM-only 暂存目录供同一操作续跑。

B 集群无法直接写 A 集群数据库。A 的 Gateway 观察到 mTLS 流断开后，按正常在线状态机进入 `suspected_offline/offline`；A 中的历史资源、审计和遥测保留，由用户在 A 中继续替换、卸载记录或执行“仅从 Argus 移除”。

同一套 Argus 内重新接入时，enrollment 事务按 `(enterprise_id, instance_id)` 查找旧的有效 Connector。设备指纹相同则在同一事务中撤销旧证书、Session、命令、Tunnel 和遥测路由，把旧 Host/Scope 置为离线，再创建新 Connector；任一步失败都会回滚。设备指纹不同仍返回冲突，避免克隆镜像或重复 hostname 误覆盖另一台机器。

## 4. 安装路径

相同事务顺序覆盖：

- Linux POSIX 一行命令。
- Windows PowerShell 一行命令。
- Direct Executor SSH 安装 Linux/Windows Host。
- Bastion Connector SSH 安装 Linux/Windows 成员 Host。
- Direct Executor SSH 安装 Linux Bastion，包括固定控制隧道模式。

Windows SSH 始终显式运行 PowerShell EncodedCommand，暂存目录和正式状态目录应用 SYSTEM/Administrators ACL；停止 SCM Service 后等待 `Stopped`，并重试二进制替换。安装不会卸载或修改 OpenSSH。

## 5. 验收记录

2026-09-07 在本机 `argus-test-host`（SSH `127.0.0.1:2222`）完成真实接管：

1. 保留来自旧集群的 Bastion Connector 身份和正在运行的服务。
2. 使用 `dev-f80d925bdef08996` 命令安装验证本机事务，新 Connector 注册在线，旧进程停止，CSR/证书/marker/identity 收敛到同一 ID。
3. 使用 Direct SSH 安装再次接管，同一 SSH 暂存操作在错误 CA 失败后修复 CA 并续跑成功，目标 Host Connector 在线且暂存目录清理。
4. 再使用一条 Bastion 命令反向接管，SSH Connector 被停止并置为 revoked，稳定 Bastion Scope 恢复 `active/ready`。
5. Collector 保持 `inactive`，最终机器只运行一个 Connector 身份；测试 Host 已软删除，未消费的测试 Token 已吊销。

最终部署发行版为 `dev-d4c6552735e83b11`。在不预先撤销旧实例的条件下再次完成 Direct SSH → 命令反向接管：服务端自动将旧 Connector 置为 `revoked`，旧 Host/Bastion Scope 立即离线；新 Connector 上线后 Scope 收敛为 `active/ready`，Relay 上报 `8445/9445`。

Windows Server 2019/2022 的 SCM、锁文件重试和 A→B→A 接管继续按 [Windows Server 主机接入实机验收](./20-windows-host-e2e.md) 在实体 VM 执行。
