# Docker 构建通道恢复记录

## 故障证据

2026-09-25，Windows Docker CLI 的 version/inspect/ps 长时间无响应，三个 Engine 命名管道无法建立连接。与此同时，Linux dockerd 和 guest-services Docker Unix 代理的 `/_ping` 均在毫秒内返回 200，Kubernetes `/readyz` 正常。故障定位到 Windows Desktop 控制/转发层，不能据此认定 Linux 引擎或业务数据已损坏；原始管道失去响应的底层触发原因没有进一步确定。

先结束五个经过 PID、命令和启动时间核对的旧 Argus 只读 Docker 客户端。随后尝试[官方 Docker Desktop restart](https://docs.docker.com/reference/cli/docker/desktop/restart/)，命令仍卡在控制接口、没有实际触发重启，因此取消本次悬挂命令，结束已确认的 Docker Desktop Windows services 进程，并重新启动 Desktop。

重启又暴露出临时 AF_UNIX 套接字问题：`sailor-ingest.sock` 和 `docker-secrets-engine/engine.sock` 无法访问或改名，阻止后端初始化。第一次 Secrets Engine 目录改名落入 Codex MSIX 缓存副本，真实目录中的旧套接字仍在。最终用同一用户身份的非 MSIX Windows PowerShell 进程完成检查、隔离和重启。

## 操作范围

- 每次目录移动前检查绝对父目录、目录自身不是重解析点、目录内仅有白名单中的零字节套接字；采用改名保留，没有删除目录内容。
- 生效的真实目录隔离位置为 `C:\Users\23574\AppData\Local\Docker\run.stale-planv2-native-20260925-171720` 和 `C:\Users\23574\AppData\Local\docker-secrets-engine.stale-planv2-native-20260925-171720`。
- 没有执行 factory reset、Docker prune、数据盘压缩或 PVC 删除，也没有修改代理/TUN、模型账户或 Docker 设置。
- 重启造成已有容器和 Kubernetes 服务短暂中断；恢复前后的资源身份与健康快照保存在本地 `artifacts/docker-recovery-20260925/`。快照只保留恢复所需信息，不作为公开发布产物。

## 恢复期间的依赖修复

现有 `agentx-e2e-deps-pvtest01/object-storage` 使用 `quay.io/minio/minio:latest` 和 Always。重启后该引用拉取失败，节点 registry mirror 返回 401，独立 manifest 检查也无法取得该引用，导致 MinIO 和依赖它的 Runtime Gateway 无法就绪。

检查 CRI 缓存及重启前 Pod 的 imageID 后，确认原来运行的镜像摘要仍完整存在：

```text
quay.io/minio/minio@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e
```

仅将这个 StatefulSet 固定为该摘要与 IfNotPresent，并校验 UID、resourceVersion、原镜像和原拉取策略。原 Pod 被旧的失败拉取状态阻塞，因此在核验 StatefulSet 所有权后重建该 Pod，复用原 PVC；没有更换镜像内容或数据版本。

这是现有集群的恢复补丁，未修改 Agentx 源码或 Helm values。后续重新部署该环境时应保留该镜像固定策略，避免旧 values 再次覆盖成失效的 latest 引用。

## 已验证结果

- Docker Engine 29.7.2 正常响应，Desktop 状态 running，BuildKit 可用，Argus `doctor e2e` 全部通过。
- 原有 4 个 Docker 容器保持原 ID 且运行；36 个 Docker 卷全部保留。
- 9 个 PVC 保持原 UID 与 volumeName，全部 Bound；原有 34 个运行 Pod 全部 Ready。
- MinIO 与 Runtime Gateway 就绪；通过现有 Ingress 转发访问 Agentx `/login` 返回 HTTP 200。没有把本来未绑定本机 80 端口的测试入口误判为业务故障。
- 使用正式 `deploy/docker/otelcol.Dockerfile` 成功构建并运行 `argus-otelcol:planv2-recovery-20260925`，amd64 镜像 ID 为 `sha256:441aec8e7bc79389b81a3de17de4b7f430e1f6bfaee46f3b7a99ef57861d5c25`。

## 恢复后的真实部署检查

- `p2-recovery-20260925-a` 已越过 Docker 阻塞，正式 Backend 镜像构建成功；Web 镜像因漏复制 Vite 引用的 `scripts/web-chunks.ts` 失败，未进入业务验收。本轮临时资源已清理，失败记录保留。
- Web Dockerfile 改为复制完整 scripts 构建工具目录，避免仅列举两个脚本而遗漏间接依赖。
- 独立 Ingress 原先在系统 Namespace 尚不存在时使用 `--watch-namespace`，导致启动退出。改为按本轮 release ID 的 [Namespace selector](https://kubernetes.github.io/ingress-nginx/user-guide/cli-arguments/) 监听，并保留独立 IngressClass；回归测试验证不会依赖未创建的固定 Namespace。`p2-recovery-20260925-b` 中控制器一次启动即 Ready，无重启。
- b 轮正式镜像构建与安装进一步暴露 Helm 只携带第一份 ClickHouse SQL、仍停在 v3 的问题。Chart 现按顺序打包并执行全部权威迁移（含来源身份 v4）；新增检查覆盖迁移文件集合、内容以及服务端要求版本。b 轮临时环境执行完整迁移后，Ingest/Writer/Query 全部 Ready，真实门户的 3 个身份流程浏览器检查通过。

## 最终空环境复验

`p2-recovery-20260925-c` 从新 Namespace、空数据库和修复后的正式 Chart 重新安装，全程没有手工补迁移或修改部署。ConfigMap 包含 00001/00002 两份 SQL，迁移 Job 执行完整有序文件集并成功结束；Ingest、Writer、Query 首次启动均 Ready、重启次数为 0，证据保存在 `schema-bootstrap.json`。

- M2 身份、M3 资源、M4 Agent/治理、P5 native、M7 遥测和 M10 查询阶段全部通过。
- 真实 Chromium 检查 23/23：身份 3、资源 6、Agent/治理 2、遥测双语/主题 12。
- 覆盖 Host/K8s Collector 安装、配置、修复、升级和卸载，三信号 Collector→Kafka→Writer→ClickHouse→Query、堡垒机网关、节点绑定、查询权限与故障恢复。
- 最终验证通过证书/HTTPS、Kafka 生产消费、MinIO 对象读写、PostgreSQL/Redis 读写、OpenSandbox 生命周期和工作负载就绪。
- b、c 两轮进程最终均以 0 退出并完成清理。本轮临时 Namespace、PVC、测试 RBAC/Lease 和自有镜像引用已清理；清理后集群恢复为原 9 个 Bound PVC，原有 34 个运行 Pod 全部 Ready，36 个 Docker 卷均保留。

最终结果为 `artifacts/m10-query-e2e/p2-recovery-20260925-c/result.json` 的 passed，以及完整进程退出/清理记录；不是仅根据结果文件提前认定清理完成。相关 Argusctl、Argusdev、Telemetry、契约测试、go vet、git diff --check 和源码行数限制检查通过。

这关闭了 Docker 环境和基础部署链路阻塞，不等于 PlanV2 全部通过。新版 Dashboard 的发布/冲突/归档/来源/APM 界面组合，以及 Dashboard 查询文件到 Chat/Workspace 的专项验收仍待完成；实际模型验收仍需要可用的模型配置，确定性 Replay 不代替模型任务正确性。
