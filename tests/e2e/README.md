# End-to-end tests

正式部署路径保持 `argusctl + Helm`。仓库 E2E 由跨平台 Go CLI `argus-dev` 编排，在专用干净 Kubernetes Context 的临时 Namespace 中调用真实 `argusctl` 完成安装、验证和卸载；client-go 负责 Lease、Fixture、等待、日志、exec、port-forward、脱敏诊断和无条件清理。

M6/M8 的堡垒机远程访问场景额外占用本机 `127.0.0.1:4222`，将临时 Fixture Chart 的 SSH Target 转发给运行在开发主机上的真实 Connector；`doctor e2e` 会与其他本地端口一起检查该端口。

```text
go run ./cmd/argus-dev doctor e2e
go run ./cmd/argus-dev e2e run --suite m2
go run ./cmd/argus-dev e2e run --suite m7 --run-id local-m7 --artifacts artifacts/m7-e2e/local-m7
go run ./cmd/argus-dev e2e run --suite m10-query --unit-only
go run ./cmd/argus-dev e2e run --suite p4
go run ./cmd/argus-dev e2e run --suite tls
go run ./cmd/argus-dev check installer-containers
go run ./cmd/argus-dev check guacd
go run ./cmd/argus-dev e2e windows-host --config tests/e2e/windows-host.example.yaml
```

Suite 依赖：

```text
m2        = M2
m3        = M2 + M3
m4        = M2 + M4
m5        = M2 + M3 + M4 + M5
m6        = M2 + M3 + M6
m7        = M2 + M3 + M4 + M5 + M7
m10-query = M7 + M10 Query
m8        = M6 baseline + M7 baseline + Local Hardening/Backup/Restore
p4        = M2 + unified Host/Bastion/Collector onboarding
tls       = two isolated M2 + strict Host onboarding installs (managed and customer ClusterIssuer)
```

P4 包含普通 Host `executor_tunnel` 专项：Executor 内将企业入口和 Artifact 公开域名解析到 loopback，目标主机禁止直连平台；仅对测试 Host 的首次隧道注入错误上游，验证 HTTPS 预检在 `probing` 阶段失败且没有传输文件。随后通过正式重试 API 验证旧隧道与租约退休、新 Connector 上线及 Executor Pod 接管恢复。故障注入以预览冻结的 Host UUID 为边界，触发器在确认后立即删除，测试主机通过正式卸载流程清理。

已有安装可运行 opt-in 的单场景回归。它只创建一个临时 systemd Target Namespace，快照并恢复 `argus-direct-executor` 的副本数与 `hostAliases`，通过正式 API 创建并清理 Host、Secret 和 Credential；不会调用完整 E2E cleanup，也不会删除正式 Namespace。必须使用指定测试企业中已具备 Host、Secret/Credential 和 Pending Action 权限的专用账号；Runner 不创建 MFA、不修改角色绑定。`FIXTURE_IMAGE` 必须预先存在于每个节点，仓库 `e2e-systemd-host.Dockerfile` 已包含 iptables；Runner 使用目标 Pod 内的 iptables 仅阻断发往 Ingress IP:443 的新连接，保留 ESTABLISHED 和 loopback，不依赖 kindnet 的 NetworkPolicy enforcement。

```powershell
$env:ARGUS_INSTALLED_TUNNEL_E2E = '1'
$env:CONFIG_PATH = 'D:\path\to\installed-config.yaml'
$env:ENTERPRISE_USERNAME = 'dedicated-test-admin'
$env:PASSWORD = '...'
$env:FIXTURE_IMAGE = 'registry/argus-e2e-systemd:tag'
$env:TOTP_SECRET = '...' # 仅账号已有 MFA 时设置
$env:ARGUS_TUNNEL_E2E_ARTIFACTS = 'artifacts/installed-tunnel/run-1' # 可选，保留脱敏证据

go test ./internal/app/argusdev -run '^TestInstalledExecutorTunnelE2E$' -count=1 -v -timeout 40m
```

若未设置 `ARGUS_TUNNEL_E2E_ARTIFACTS`，证据写入 Go 临时测试目录并随测试清理。测试在任何阶段退出都会尝试通过 API 卸载并逻辑删除本次 Host、禁用本次 Credential/Secret、验证 enrollment token 与 tunnel 已撤销，再删除自己创建的 Target Namespace 并恢复 Executor；它不会清理测试企业或账号。

Fixture Chart 位于 `tests/e2e/helm/argus-e2e-fixtures`，只提供 Linux OpenSSH/systemd Target、Replay Model、Artifact Server 和测试镜像装载，不进入正式发布包。Windows Server 2019/2022 使用 `argus-dev e2e windows-host` 的独立 VM 测试入口，不以 Linux 服务模拟；配置只引用密码/一次性命令的环境变量和私钥文件，不保存敏感值，并固定 OpenSSH Host Key。Harness 自动验证系统版本、管理员权限、SCM、磁盘、ACL、MachineGuid、Service 恢复策略、Trust Bundle、ConPTY、自启动、重启、可选升级/卸载和 RDP 就绪状态，输出脱敏 JSON。需要场景身份、Ticket 或短期证书的 Remote Client 与 Telemetry Generator 由 client-go 按用例创建受控 Pod/Job，并随临时 Namespace 清理。Playwright 仍使用 `web/apps/enterprise/e2e/*.spec.ts`。

`check installer-containers` 在 Ubuntu、Debian 和 Alpine 的 amd64/arm64 容器中执行安装器 fail-closed 预检，并在两种架构下真正运行 Connector 的 SHA-256/Ed25519 校验；Alpine 只作为 POSIX 脚本门禁，正式 Linux Service 仍要求 systemd。`check guacd` 启动发布锁定的真实 guacd 1.6，验证 Argus 协议握手和失败边界。

2026-08-24 最终运行 `fv-20260824-m8-final13` 验证了上述依赖闭包、脱敏诊断和零残留清理。完整 E2E 失败时不得保留 Secret；当卸载先删除 Service、随后端口转发返回 NotFound 时，Harness 将其作为幂等停止结果处理。

完整 E2E 需要 Docker、kubectl、兼容 Kubernetes、StorageClass、受支持节点架构、空闲本地端口、至少 25 GiB 主机磁盘，以及没有其他 Helm release 持有 Strimzi/OpenSandbox 固定 ClusterRole 的专用 Context。`doctor e2e` 和 `e2e run` 都在镜像构建前检查该所有权；当前正式部署所在集群不应作为 E2E 目标。`--kube-context`、`ARGUS_E2E_KUBE_CONTEXT` 和自动发现值按优先级决定唯一的检查与运行目标。成功或失败都会先用独立超时预算收集脱敏诊断，再用新的清理预算删除 Fixture、临时 Namespace/PVC、Lease、Cluster RBAC、测试镜像和本地子进程。

每个 E2E 镜像都写入本次 run 专属 OCI label，确保其 manifest digest 不与正式 `dev` tag 或其他运行共享。Registry 清理只接受本次运行的精确 tag，先通过 Registry V2 API 解析 digest 并枚举同仓库 tag；若发现共享 digest 或 Registry 禁止 DELETE，清理立即失败并报告残留，不会扩大到 repository 或误删正式镜像。仓库不再提供或执行 Bash E2E 脚本；Make target 仅转发到相同 `argus-dev` 命令。
