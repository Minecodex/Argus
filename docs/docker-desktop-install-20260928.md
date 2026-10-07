# Docker Desktop Kubernetes 正式命名空间安装（2026-09-28）

**最新结构已收敛为 `argus-system`、`argus-ingress`、`argus-sandbox` 三个命名空间；遥测与 Workspace CSI 已并入 system，旧两个命名空间已删除。见 [合并记录与验证](./namespace-consolidation-20260928.md)。初始化凭据和根证书未变。以下部署阶段记录保留历史事实。**

当前部署在 **`docker-desktop`**，使用 Docker Desktop 自带 Kubernetes。此前独立 `kind-argus-local` 集群已不存在；本文件替代其安装记录。安装配置仍为本地单节点 evaluation，使用真实 API 和正式构建镜像，不表示高可用 production Profile 已开放。

## 当前资源与访问

- 业务：`argus-system`、`argus-observability`、`argus-sandbox`。
- 支撑：`argus-ingress`、`argus-workspace-storage`；复用现有 cert-manager/trust-manager。
- 配置：`deploy/.cache/argus-install-desktop.yaml`；`deploy/.cache/argus-install-local.yaml` 已同步为相同目标。
- 企业：`https://argus.dev`；平台：`https://platform.argus.dev`；模板：`https://cards.argus.dev`；产物：`https://artifacts.argus.dev`；Connector：`grpcs://argus.dev:9443`。
- 使用普通 `kubectl get ns` 即可查看，无需独立 kubeconfig。未更改 Judex 的命名空间、Deployment、StatefulSet、PVC 或共享 OpenSandbox CRD。

新初始化入口保存在 `artifacts/docker-desktop-install-20260928/initialize-platform.url` 和 `setup-link.txt`，仅当前用户可读。旧 `artifacts/local-install-20260928/` 下两个快捷入口也已更新为新 Token。有效至北京时间 **2026-09-29 20:20:59**，初始化后失效。未创建测试账号、企业或模型配置。

本轮最后读取到的 hosts 仅包含 `templates.argus.dev`。域名直连需要保留其他项目配置并补全：

```text
127.0.0.1 argus.dev platform.argus.dev cards.argus.dev artifacts.argus.dev templates.argus.dev
```

HTTPS 服务证书已在安装自检中通过显式 CA 验证。新公开根证书为 `artifacts/docker-desktop-install-20260928/Argus-Docker-Desktop-CA.cer`，指纹 `A61C924E4190B40DCBEE6539515B4A1F8AE74334`。当前用户信任库导入仍在等待 Windows 确认；服务端检查通过不能替代浏览器信任库确认。

## 共享 OpenSandbox 的处理

Judex 已有全局控制器，其标签为浮动 `latest`，因此不能仅凭标签推定兼容。此次配置显式固定外部控制器 namespace `judex` 及实际运行的 manager 镜像摘要：

```text
sha256:bf7f63852e772c910a7930cfba1b1d1d810d8613561d358830f7f5108b165c42
```

安装器验证 CRD 契约、Helm 归属、Deployment 就绪、Pod/ReplicaSet/Deployment 归属及实际运行镜像摘要。默认路径仍要求锁定 v0.2.0；显式固定摘要不放开任意标签，不接管或改写外部资源。Argus 在自己的 `argus-sandbox` 安装 Server、凭据与工作负载，通过该全局控制器调度。

实际创建、执行和销毁 Sandbox 已通过。外部控制器重建或升级后需要重新检查兼容性，不能随意删除摘要配置绕过检查。

## 本轮证据

- 安装成功；`argusctl verify` **20/20 通过，退出码 0**：`artifacts/docker-desktop-install-20260928/verify/verify.json`。
- 只读共享控制器专项、安装器单元测试及契约测试通过，新增反例覆盖摘要不符、namespace/归属错误、无 manager、未就绪、无 Pod、滚动更新中和未配置的浮动标签。
- Judex 的 2 个 PVC、3 个 Deployment、2 个 StatefulSet 的 UID/spec 不变；3 个共享 CRD 的 UID/spec/归属注解不变：`protected-resources-check.json`。
- Collector 镜像已构建并加载到 Docker Desktop 节点；镜像加载 DaemonSet 已按 UID/resourceVersion 删除。没有 Argus 探测 Namespace/Sandbox 或独立集群容器残留。
- 之前明确要求删除的四个 Agentx Namespace、9 个 PVC/PV 仍无残留。Judex 与公共组件保留。
- NetworkPolicy 未证明执行 deny、无外部 Egress Gateway、共享容器沙箱的本地限制保留。安装自检不是 PlanV2 当前整版浏览器及真实模型联合回归。

本轮开始时旧独立集群尚能返回 `uninitialized`；随后再次检查时其容器已不存在，未把消失原因归于此次安装动作。新集群为空业务初始化，不声称从已消失集群完成了数据迁移。
