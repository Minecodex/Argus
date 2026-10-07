# 三命名空间部署收敛（2026-09-28）

本机访问复核（2026-09-28 23:25）：平台仍为 `uninitialized`，已保存的初始化链接与当前 Secret 匹配，到期时间仍为北京时间 2026-09-29 20:20:59。根证书已在当前用户信任库中。本机 `127.0.0.1:443` 有 WSL relay 监听且 HTTPS 探测超时；通过 `127.0.0.2:443` 访问同一 Docker 入口、保留域名与证书校验时，初始化 API 正常返回。因此当前手工访问可将 hosts 中 Argus 相关条目配置如下，并保留其他项目条目：

```text
127.0.0.2 argus.dev platform.argus.dev cards.argus.dev artifacts.argus.dev
```

已按用户确认的结构修改安装器、默认配置并重建 Docker Desktop 部署。Argus 自身仅保留：

| Namespace | 内容 |
| --- | --- |
| `argus-system` | 业务服务、遥测、PostgreSQL/Redis/MinIO、Kafka/ClickHouse、Workspace CSI 驱动 |
| `argus-sandbox` | OpenSandbox Server、沙箱 Pod 和 Workspace PVC |
| `argus-ingress` | 本地入口；复用已有 Ingress Controller 时可省略 |

`argus-observability` 和 `argus-workspace-storage` 已删除。其他项目与 Kubernetes 公共命名空间保留。当前上下文仍为 `docker-desktop`。

## 实现变化

- 默认 profiles、Helm values 将遥测及存储驱动指向 system。省略 observability/storageNamespace 时按 system 解析；沙箱不得与控制面、遥测或特权驱动共用命名空间。
- Namespace、LimitRange、基础 NetworkPolicy、PKI Role/RoleBinding 与 PKI 目标列表去重，保留 system 所有权标签；状态、网络发现、诊断和恢复检查按物理命名空间去重。
- 组件仍使用独立 Deployment/DaemonSet、ServiceAccount 和资源配置。Namespace 合并不承诺更新整包时遥测不中断；现有本地安装流程仍会滚动更新业务及遥测服务。
- 卸载先释放 sandbox，再等待 Workspace PV 回收，最后删除 CSI 驱动及控制面；存储组件单独卸载不会删除共享 system Namespace。共享外部 CSI 驱动不被接管或删除。已由本发布拥有的 CSI 驱动更换命名空间时必须先迁移/重建，不能静默复用旧位置。
- Helm 升级显式使用 server-side apply，避免从 Helm 3 创建的基础 Release 升级时，`ForceConflicts` 与自动选择的 client-side apply 冲突。
- E2E 配置将存储驱动命名空间绑定本次运行的 system，避免沿用默认 profile 的固定名称；既有临时信号隔离 fixture 仍可显式指定独立 observability Namespace。

## 重建与凭据保留

重建前实际查询：enterprises、dashboards、conversations 均为 0。已私密备份数据库、生成凭据和根证书，重建后核对初始化 Token/到期时间与根 CA 数据保持一致。原初始化快捷入口继续有效，不需要因为此次合并重新获取 Token 或导入新的根证书。此前浏览器端 hosts/证书信任的待确认事项不会被此次部署自检自动视为完成。

安装配置 `deploy/.cache/argus-install-local.yaml`、`argus-install-desktop.yaml` 已同步到合并布局；历史配置仅用于记录，不应再次直接安装到当前环境。

## 验证与证据

- `go test ./internal/app/argusctl ./internal/app/argusdev ./tests/contract` 通过。
- 回归测试覆盖合并渲染无重复资源、system 标签保留、沙箱隔离、CSI 回收顺序、回收失败保留控制面、外部驱动不变及恢复目标不与原 namespace 交叉重叠。
- 实际部署安装自检 **20/20 通过，退出码 0**：`artifacts/namespace-consolidation-20260928/verify/verify.json`。
- 独立临时 Namespace 中创建真实 128 MiB RawFile PVC：写入后删除原 Pod，新 Pod 重新挂载读取一致，随后 Namespace/PVC/PV 全部回收：`storage-probe.json`。
- default、argus-server、argus-telemetry-ingest 三个 ServiceAccount 均不能在 system 创建 Pod。该检查不替代对所有 Kubernetes 用户的完整 RBAC 审计。
- Judex 原有 2 个 PVC、3 个 Deployment、2 个 StatefulSet 的 UID/spec 不变。临时镜像加载器已通过 UID/resourceVersion 前置条件删除。
- 最终三个 Namespace、权限和外部资源核对：`artifacts/namespace-consolidation-20260928/result.json`；初始化与 CA 连续性：`credential-continuity.json`。

本轮是部署布局及存储回归，不替代 PlanV2 全套浏览器与真实模型联合验收，也不增加生产 HA/网络强隔离承诺。
