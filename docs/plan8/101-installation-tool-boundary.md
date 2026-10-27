# 安装工具与受限远程工具分离

2026-10-25。实际验证和发布状态见 `progress.json` 与 [交付记录](./57-completion-delivery.md)。

## 原目标暴露的问题

R80 的隔离业务、发布恢复与官方前 75 项门禁通过，但原目标升级在 Altinity CRD 的 pre-upgrade Hook 失败。Hook 使用 `argus-remote-runtime` 中的 `kubectl`；该客户端刻意删除集群凭据，并只接受受信的本地连接代理。它不具备安装 Hook 所需的 ServiceAccount 访问能力。

在同一实际镜像中，以普通 Pod ServiceAccount 只读访问 API 版本，复现 `REMOTE_CONNECTION_OVERRIDE_FORBIDDEN`。见 `artifacts/plan8-r80-actual-install-kubectl-boundary.json`。原应用数据库仍为 Schema 19，六个应用运行镜像未变，副本恢复已核验；失败任务和日志保留。

## 边界与实现

- `argus-remote-runtime` 保持受限连接、无集群凭据的现有行为；不增加回退到 kubeconfig 或 ServiceAccount 的通道。
- 新增独立 `argus-install-tools` 镜像，只有标准上游 kubectl 与 Hook 所需 shell。kubectl 使用已锁定的 Go 模块 v0.35.3，基础镜像按摘要固定，支持现有 Linux 构建平台。
- 安装器将该镜像单独构建、导入、解析为带版本和摘要的引用，并仅用于安装 Hook。它不进入远程运行时镜像、模型工具目录或默认 Workspace 分析环境。
- 镜像构建、加载和清理使用同一清单，避免安装依赖漏入发行物料。完整交付物料包含该安装镜像；历史运行镜像验证需明确区分旧运行程序与当前安装工具。
- 保留上游 Hook 的命令、ServiceAccount、RBAC、server-side apply 和执行顺序，不能以禁用 Hook 作为修复。

Pod 内标准客户端使用自身 ServiceAccount 和 CA 访问 API；这是安装身份，不是用户远程执行身份。[Kubernetes 官方说明](https://kubernetes.io/docs/tasks/run-application/access-api-from-pod/)

## 必需验证

1. 受限远程客户端仍拒绝连接覆盖和未绑定的集群访问。
2. 安装镜像可在真实、隔离 Namespace 中使用 ServiceAccount，执行原 Hook 命令创建及更新本轮拥有的 CRD；撤去权限时拒绝执行。
3. 验证镜像加载、固定摘要和清理覆盖新镜像；不能只验证模板渲染。
4. 新版本冻结后重做适用组件、完整部署、发布恢复和原目标验证，旧版本通过结果保留原范围，不直接改标为新版本完成。

## 已取得的针对性结果

`TestLiveInstallationToolsHookUsesServiceAccountAndRejectsRevocation` 在 Docker Desktop 的独立 Namespace 中执行了原上游 Hook 命令：创建 CRD、撤销与恢复 RBAC、更新 CRD，以及保留先前实例。15.23 秒通过，测试 Namespace、实例、CRD 和授权资源清理完成，见 `artifacts/plan8-installation-tools-live-r81.json`。这只是新镜像的真实针对性验证，整体验收与发布仍待当前冻结版本完成。

发行组新增 `install-tools`，完整组为八个镜像，包含七个原有运行镜像和一个安装依赖。旧版本演练仍使用原有七个运行镜像，安装依赖明确来自当前安装器；该依赖不伪装为旧版本程序，也不加入受限远程镜像。
