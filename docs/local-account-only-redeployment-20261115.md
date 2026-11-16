# 本机仅保留账号的重部署与磁盘回收

2026-11-15，按用户明确授权清空 Argus 非账号业务数据、删除 `artifacts` 与 `D:\gocache`，重新部署服务，并离线压缩 Docker 虚拟磁盘。此维护完成时没有运行中的 Plan9 场景。随后用户另行指定进入 [B11c](./plan9/batches/B11c-review.md)，新场景与本次空基线的完成证据分别记录。

## 保留范围

- 保留 1 个平台账号、4 个企业账号（包括原有停用账号）、2 个服务账号，以及企业归属、4 个部门、12 个角色、角色绑定、密码凭据、MFA 和必要的平台设置。
- 保留用户 ID、既有密码哈希及停用状态；没有通过重新初始化生成替代账号。
- 保留正常启动所需的 TLS/PKI、服务密钥、Kubernetes 基础组件及不可变安装镜像。安装目录和 Go SDK 未删除。
- 账号备份放在 C 盘受限目录，只授权当前 Windows 用户与 SYSTEM；不进入仓库。

## 清理与重建

先导出账号数据与 schema，在独立空数据库恢复。17 张账号／安装元数据表逐项哈希一致，183 张其他业务表为空，之后才暂停服务、替换正式数据库。

- 停止并删除 B11b 两个按运行标签与容器 ID 核验过的容器、4 个状态／日志卷和专属网络。
- 对 Kafka、Keeper、Redis、MinIO、ClickHouse 五个数据卷逐一核对 PVC/PV UID，停写和卸载后清空。每卷清理后仅有 4KiB 目录开销；保留的是卷绑定，不是旧内容。
- 正式 PostgreSQL 切换为仅恢复账号的新库。验证原密码登录后删除原业务数据库，原库约 2.67GiB，新库初始约 20.6MiB。
- 清空旧会话、主机、集群、采集、告警、模型、文件、录像、Workspace、资源授权及遥测数据。当前 26 张关键业务表为 0，ClickHouse 遥测事实行数为 0，Workspace 文件和远程录像桶为空。
- 平台与企业审计链按正常初始化契约建立新的空链根，未导入旧审计事件。安装器重新生成必要的 PKI、目录、模板和软件发行元数据。
- 通过当前 `argusctl` 正常安装。七个产品镜像统一引用 `local-20261115-account-reset`；三份 Collector 安装包的摘要、大小与 Ed25519 签名均核对后重新发布，Connector 由当前源码构建签名发行版。

当前正式配置：`deploy/.cache/argus-local-formal.yaml`，维护配置副本为 `deploy/.cache/argus-account-reset-20261115.yaml`。沿用 evaluation 单机配置及既有网络能力边界。

验收客户端现在支持显式 `ARGUS_PLAN9_CA_BUNDLE`，指向稳定配置路径 `D:/workspace/go/Argus/deploy/.cache/argus-trust-bundle.pem`。清理历史 `artifacts` 后不再依赖已删除的 CA 路径；缺少文件仍拒绝连接，不关闭 TLS 校验。应用源码与接口未修改。

## 文件与缓存

Windows 数值统一按 1024 进制：

| 对象 | 结果 |
| --- | --- |
| `D:\workspace\go\Argus\artifacts` | 清空约 47.1GiB、323,009 个文件；2,161 个 reparse point 只移除链接，未跟随删除外部目标；确认无 Git 跟踪文件 |
| `D:\gocache` | 官方 `go clean -cache -modcache` 后清理残余临时文件，目录为空；Go SDK 保留 |
| Docker 构建缓存 | 清理 73.82GB（Docker 报告口径），最终为 0 |
| Docker 旧镜像 | 清理闲置镜像，Docker 报告回收 53.62GB |
| 节点镜像 | 删除 1,781 个旧引用，保留运行及安装所需镜像；补回并保留版本锁定的 Sandbox execd/egress 依赖 |
| Registry | 删除 435 个旧标签与 526 个旧清单，离线 GC；保留 9 个当前标签，17 个必需清单及 131 个层／配置对象全部可读 |

未执行全局 volume prune，Kubernetes 基础卷与当前服务必需镜像保留。

## 虚拟磁盘压缩

用户完成 Windows 管理员授权后，执行在线 sync/TRIM、正常停止 Docker Desktop、仅终止 Docker 的 WSL 发行版。管理员脚本核对精确目标、无 reparse point、独占文件访问和 `vhdxfile` 签名，再运行 DiskPart `compact vdisk`。

- 精确目标：`D:\docker_files\DockerDesktopWSL\disk\docker_data.vhdx`。
- 离线压缩完成时文件长度约 **437.2GiB → 30.7GiB**，原生日志确认“DiskPart 已成功压缩虚拟磁盘文件”；服务恢复并完成验证后约 **30.9GiB**。
- D 盘可用空间约 **25.3GiB → 519.6GiB**，净增加约 **494.3GiB**；最终占用约 26.2%。
- 随后启动 Docker Desktop，Kubernetes 节点、数据服务和全部工作负载恢复就绪。

动态 VHDX 的离线压缩条件见 [Microsoft DiskPart 文档](https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/compact-vdisk)；Registry 标签删除使用 [Distribution v3 原生 API](https://github.com/distribution/distribution/blob/v3.0.0/registry/handlers/manifests.go)。

## 验证与中间失败

- 账号恢复演练和正式替换后的 17 张表逐项一致；正常登录前检查包含所有密码哈希。重启后再次确认密码哈希、账号数量和原 ID。
- 压缩后使用正常浏览器、严格 TLS，平台 `admin` 与企业 `kakj` 均以原密码登录成功；企业 `test` 保留，资源、会话和录像列表为空。
- 最终在全部工作负载 Ready 后，标准健康检查 **20/20** 通过，测试生成的 S3 冒烟桶已清理。
- 安装初次遇到被维护暂停的共享 Sandbox 控制器未就绪，恢复其原副本数后继续；ClickHouse 原生 StatefulSet 恢复，Kafka PodSet 通过受支持的滚动更新注解重新协调。
- 清理缓存后首次 Sandbox 生命周期检查因 execd 冷启动超时，恢复并预加载固定摘要的运行依赖后通过。Docker 重启后的首次检查撞上工作负载初始化，待就绪后重新完整验证；未跳过或降低检查。

本次数据库写操作仅用于明确授权的维护恢复，不用于制造监控或批次业务数据。B11b 退役不等于用户人工通过，历史运行记录与本次状态分开保存。

完成报告及操作证据：`C:/Users/23574/.codex/artifacts/argus-account-reset-20261115/completion.json`。其中公开摘要不含密码、令牌或私钥；账号恢复包与原安装配置位于其受限 `private/` 子目录。
