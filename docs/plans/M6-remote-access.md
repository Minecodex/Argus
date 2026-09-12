# M6 人工远程访问

当前链路：

```text
RemoteAccessGrant
  → RemoteAccessRule / ApprovalWorkflow / SessionProfile
  → AccessRequest / Approval / Lease
  → one-time Ticket
  → Connector Gateway
  → Host Connector
  → shell | ssh | rdp
  → encrypted recording / termination / revocation
```

## 已实现范围

- `shell`: Linux PTY 与 Windows PowerShell/ConPTY。
- `ssh`: Host Connector 只连接目标本机 `127.0.0.1` OpenSSH；Host Key、ManagedAccount 和短期 Credential Lease 均受校验。
- `rdp`: Host Connector 只连接本机 3389；Gateway 创建临时 loopback bridge，同 Pod guacd 生成浏览器 Guacamole 画面。
- Ticket 单次消费、AuthorizationVersion 撤权、跨 Gateway peer 路由、并发和时长限制。
- 命令行使用 `asciicast_v2`，RDP 使用 `guacamole_v1` 输出流；两者分别分片加密写入 Artifact Store 并保存哈希链。浏览器按格式选择终端播放器或 Guacamole 画面播放器。
- 剪贴板、文件、pipe/blob、磁盘映射、打印机和会话分享默认禁用。
- RDP 密码只进入 Gateway/guacd 短期租约，不进入浏览器或 Host Connector。

## Windows RDP 启用边界

Connector 心跳上报 OpenSSH、RDP 注册表、NLA、`TermService` 和内置防火墙规则状态。安装只检测。RDP 未就绪时服务端拒绝会话；管理员必须执行 `host.windows_rdp.enable.preview/commit`，Preview 展示以下变更：

- `fDenyTSConnections=0`
- `UserAuthentication=1`
- 启动 `TermService`
- 启用 `RemoteDesktop-UserMode-In-TCP/UDP`

## 已删除路径

- Direct Executor 人工远程会话。
- WinRM/WinRS 和 PowerShell 行模式。
- `direct_ssh`、`direct_winrm`、`connector_local`、`via_bastion` 会话路由枚举。

所有会话现在绑定 Host 本机 Connector 和 `control_path=direct|bastion_relay|executor_tunnel`。Direct Executor 只用于 SSH 安装和固定隧道。

## 验收

- Linux PTY、Windows ConPTY、OpenSSH PTY。
- Windows Server 2019/2022 RDP NLA、画面、键鼠、缩放、撤权、断线和录像。
- RDP 未启用时只检测；独立 Preview/Commit 后才允许会话。
- Ticket 重放、旧 AuthorizationVersion、错误 Connector epoch、Gateway 漂移和必需录像失败均 fail closed。
- 中英文、深浅色和桌面 Web 布局通过 Playwright；测试 Namespace、VM 会话、Credential Lease 和 Artifact 在结束后清理。
- `argus-dev check guacd` 使用真实 guacd 1.6 验证握手；`argus-dev e2e windows-host` 负责 Windows Server 2019/2022 OpenSSH、SCM、ACL、ConPTY 与 RDP 就绪证据。画面、键鼠、NLA 和断线恢复最终由实体 VM 会话验收。
