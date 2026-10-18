# 共享 OpenSandbox 控制器归属移交

日期：2026-10-16。适用环境：本机 `docker-desktop` 集群。

## 原因与边界

用户明确要求删除 Judex 的 Kubernetes 命名空间、镜像、容器及持久化数据。盘点发现 Argus 的 Kubernetes 沙箱服务复用了 `judex/opensandbox-controller-manager`；直接删除该命名空间会中断 Argus 后续沙箱调度。

将共享控制器独立放入 `argus-sandbox`，由 Helm Release `argus-shared-opensandbox` 管理。此移交属于本次明确授权的退役运维操作，自动安装器继续只读验证和复用外部依赖，不自行接管外部资源。

## 部署与配置

- Deployment：`argus-sandbox/opensandbox-controller-manager`。
- Helm Chart：现有缓存的 OpenSandbox `helm/opensandbox-0.2.0` 中 `kubernetes/charts/opensandbox-controller`，Chart 版本 `0.2.0`。
- 参数：[opensandbox-shared-local.values.yaml](../../deploy/profiles/opensandbox-shared-local.values.yaml)。
- 控制器实际运行镜像：`sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/controller@sha256:bf7f63852e772c910a7930cfba1b1d1d810d8613561d358830f7f5108b165c42`。
- 本机 `deploy/.cache/argus-install-{consolidated,local,desktop}.yaml` 的 `openSandbox.sharedController.namespace` 已从 `judex` 改为 `argus-sandbox`，显式摘要锁定保持原值。
- 原有固定 ClusterRole/ClusterRoleBinding `opensandbox-manager-role` 交由新 Release 管理，角色规则逐项对比未变；绑定指向新命名空间的控制器 ServiceAccount。
- 三份共享 CRD：`batchsandboxes.sandbox.opensandbox.io`、`pools.sandbox.opensandbox.io`、`sandboxsnapshots.sandbox.opensandbox.io`。仅将 Helm 所有者和 instance 标签改为新 Release，UID 与完整 Spec 对比一致。
- `crds.install=false`：本次部署复用已存在的 CRD，禁止用较旧 Chart 模板覆盖当前 Schema；后续 CRD 升级仍需单独执行兼容性验证。

## 迁移与验证

1. 保存原 CRD、全局 RBAC 和工作负载清单；渲染新 Release，完成 Kubernetes server dry-run。
2. 停止旧控制器，再安装新 Release，避免不同命名空间的选主租约导致双控制器同时调度。
3. 等待新控制器就绪，核对实际运行摘要与消费者锁定摘要一致。
4. 在 `argus-sandbox` 创建带任务归属标签的临时 BatchSandbox。控制器生成 Pod，BatchSandbox `ready=1`、`phase=Succeed`，Pod `1/1 Running`；通过 `kubectl exec` 执行命令并得到 `controller-migration-ok`。
5. 删除临时 BatchSandbox，确认其 Pod 随之删除；再次确认三个共享 CRD 的 UID 和 Spec 未变。
6. 完成以上验证后才删除 Judex 命名空间及其独占资源。共享 Controller、CRD 和镜像继续供 Argus 使用。

证据保存在本机临时目录中的 `judex-cleanup-before-shared-*.json`、`judex-cleanup-shared-controller-rendered-20261016.yaml` 和 `judex-removal-controller-check-20261016.json`；临时文件不包含业务 Secret。
