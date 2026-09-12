# Windows Server 主机接入实机验收

Windows Connector、Collector、PowerShell/ConPTY、OpenSSH 和 RDP 的正式支持状态只由本套件产生的实机证据推进。Linux 容器不伪装 Windows SCM、ACL、NLA 或桌面行为。

## 1. 机器准备

- Windows Server 2019 x64（build 17763+）和 Windows Server 2022 x64（build 20348+）各准备可恢复快照。
- 启用 OpenSSH Server，使用本地或域管理员账号；Harness 所有命令都显式运行 `powershell.exe -NoProfile -NonInteractive -EncodedCommand`。
- 在可信控制台读取 OpenSSH 主机密钥指纹，例如：

```powershell
ssh-keygen.exe -lf C:\ProgramData\ssh\ssh_host_ed25519_key.pub -E sha256
```

- 需要 RDP 验收的目标先在 Argus 执行独立的 `host.windows_rdp.enable.preview/commit`；普通安装只检测，不会自动开启 RDP。
- 从 [windows-host.example.yaml](../tests/e2e/windows-host.example.yaml) 复制本地配置。配置只能写环境变量名或私钥文件路径，禁止写密码、Token 或一次性命令明文。

## 2. 四种安装路径

`scenario` 固定为：

| 值                     | 操作                                                                                 |
| ---------------------- | ------------------------------------------------------------------------------------ |
| `manual_direct`        | 页面生成 Windows 直连一行命令，命令只放进 `installCommandEnv` 指定的当前进程环境变量 |
| `manual_bastion_relay` | 页面生成经 Linux Bastion Relay 回连的一行命令，目标不能直连 Argus                    |
| `direct_ssh`           | 在页面提交平台 Direct SSH 安装；Harness 可先启动并等待 Service 出现                  |
| `bastion_ssh`          | 在页面提交经 Linux Bastion Connector SSH 安装；Credential Lease 只进入对应 Bastion   |

一次性命令只保存在当前终端进程：

```powershell
$env:ARGUS_WINDOWS_2019_PASSWORD = (Get-Credential -UserName Administrator).GetNetworkCredential().Password
$env:ARGUS_WINDOWS_2019_MANUAL_DIRECT_COMMAND = Read-Host '粘贴一次性 PowerShell 命令'
go run ./cmd/argus-dev e2e windows-host --config .\tests\e2e\windows-host.local.yaml --run-id windows-physical-01
```

命令执行、失败输出和 SSH 凭据不会写入证据。对于 `direct_ssh`/`bastion_ssh`，省略 `installCommandEnv`，启动 Harness 后在等待期限内从页面提交对应 Preview/Commit。

## 3. 自动检查

每个目标均固定 Host Key 并验证：Windows Server build、amd64、管理员权限、SCM、至少 512 MiB 磁盘、MachineGuid、`ArgusConnector` 自动启动和恢复动作、程序与身份文件、SYSTEM/Administrators ACL、Trust Bundle epoch、证书到期时间、OpenSSH、Service 重启，以及 Connector `self-test` 对真实 ConPTY 的创建、缩放、输入和输出。

配置 `upgradeCommandEnv` 时要求升级后 Connector ID 不变；配置 `uninstallCommandEnv` 时要求 Service、程序、身份和私钥全部清除。`requireRDP=true` 还要求 RDP 注册表、NLA、`TermService` 和内置防火墙规则全部就绪。浏览器中的画面、键鼠、缩放、撤权、断线恢复和录像需同时按主机详情页的 RDP 会话用例执行。

脱敏结果位于 `artifacts/windows-host-e2e/<run-id>/result.json`。Windows Server 2019/2022、四种安装路径、升级/卸载、CA 轮换、PowerShell/ConPTY、OpenSSH、RDP 画面和录像全部通过后，才能把 Windows Collector Catalog 从 `validation_pending` 改为 `supported`。
