# Node drain 的审批与执行边界

2026-10-19，T11 实施中。drain、PDB 管理、取消 API、门户与原生工具预览已经实现；下列真实组件证据不抵扣正式 Connector、真实模型或完整工作台验收。

drain 复用现有 PendingAction、KubernetesOperation 和执行通道。除 `kubernetes.node.manage` 外，必须显式具有 `kubernetes.node.drain` 和 Cluster 数据范围；Namespace 授权不足以维护整个节点。权限注册表升级为 17，写协议升级为 `argus.kubernetes_mutation/v5`，不维持旧版本长期兼容。

## 预览与冻结

- 读取节点上全部 Pod，冻结 Pod UID、Namespace UID、控制器身份、规格和存储绑定；新 Pod、同名替换、规格或存储变化要求重新预览。目标已消失只减少执行范围。
- 展示受影响 Pod、匹配 PDB 及实际 dry-run 驱逐结果。API 429 标为驱逐预算或限流阻断，不推断为一定由某个 PDB 造成。
- Node cordon 和每个候选 Pod 的 `policy/v1 Eviction` 都经过实际 API dry-run；审批不等于业务完成。
- DaemonSet、镜像静态 Pod、已结束 Pod 明确跳过。无已确认控制器的 Pod，以及 emptyDir/hostPath/临时卷和本地/节点约束 PVC 均阻断实际执行。本轮不暴露强制删除、绕过 PDB 或丢弃数据开关。
- 每次最多 200 个 Pod，超限或审批差异超过预算明确失败，不截断审批影响列表。

## 执行、继续与恢复

确认后先以 resourceVersion CAS cordon，并写入不可猜测操作标记。随后使用 UID/resourceVersion 前置条件调用 Eviction；每批最多 20 个候选，保留待驱逐、预算阻断、删除中、消失和跳过的实际状态。节点保持 cordon；恢复调度使用独立 uncordon 动作。

Worker 在每次继续前复核操作者权限、Cluster 授权、认证版本、接入版本与冻结计划；Gateway 在下发时再次复核。撤权后不再下发新批次，已下发批次可能完成；已经发生的驱逐不会自动回滚。PDB/限流阻断可在后续批次恢复；不使用 Pod DELETE 兜底。

协议增加独立 `advance_drain` 写阶段；它与 dry_run/observe_only 互斥。只读观察接口仍不写入。失联后先确认本操作的节点标记；未确认已开始时保持 unknown。已确认开始的批次只操作冻结 UID，不会驱逐同名替换 Pod。阶段超过 15 分钟以超时终止继续，并保留节点及已发生的实际状态。

## 取消及 PDB 管理

门户及 `POST /enterprise/kubernetes-clusters/{id}/operations/{operationId}` 可停止后续批次，要求当前版本和 drain 权限、Cluster 数据范围，记录独立审计；重复取消幂等，不撤销已下发请求或自动 uncordon。Execution 随取消收敛。Agent 当前可通过通用变更工具预览 drain 和 PDB 变更；独立取消尚未接入模型工具，不能据此声明 T15 完成。

标准对象目录新增 `pdb`，当前内置类型 32 个，沿用 Namespace UID 和 workload.manage 权限。PDB 写入等待原生控制器的 observedGeneration；不把 API 接受等同于预算已经生效。

## 验证与复现

- [原生 API 与实际数据库工作流](../../artifacts/plan8-drain-pdb-workflow-live.log)：在本轮专用 kind 双节点集群中，实际 dry-run 无节点副作用、PDB 拒绝、失联只继续冻结 UID、仅撤权限即停止、取消/重试/版本冲突、PDB 审批变更及原生 observedGeneration、原 Pod 退出、替代 Pod 在另一节点运行、Execution 收敛均通过。Namespace 按 UID/owner 删除，节点保持状态在测试后恢复。后端注入真实 API，尚非正式注册 Connector/门户部署链路。
- 原生单元验证覆盖本地/临时/未知 inline 存储阻断、PVC/PV 身份变化、无控制器、静态/DaemonSet 保留、Pod/Namespace UID 与规格变化、只读观察及伪造完成状态；Connector 拒绝 v4 和更早协议及混合读写阶段。
- 门户在 mock 模式验证双人审批、原申请人执行范围、逐 Pod 预览、取消和中英/明暗/桌面布局；最终执行日志另行同步。真实数据库/集群证据不由 mock 替代。

本轮收口证据：[三个具名组同次冻结执行](../../artifacts/plan8-drain-final-20261019-3588167696/manifest.json)均通过且 source_unchanged=true；[八个门户场景](../../artifacts/plan8-drain-browser-final.log)覆盖中英、明暗和 1280/1920 宽度，包含双人审批和取消，全部通过。API Client 的审批/原申请人范围/取消场景也有独立自动化用例。首次发现的集群对象 undefined 清单字段、危险操作审批绕过领域执行的 mock 问题已修复；测试保留了失败迭代，没有删除或弱化审批策略。

[`plan8-kubernetes-local.ps1`](../../scripts/plan8-kubernetes-local.ps1) 提供 Prepare/Test/Cleanup。它锁定 kind v0.33.0 及其 SHA256、Kubernetes v1.36.1，使用独立 kubeconfig 和两个自有节点，保存实际 Docker ID，拒绝自动接管无所有权记录的同名集群；Cleanup 复验 ID、Namespace 所有权及 default 无业务 Pod，只删除本轮集群并保留证据。平台仍要求可用 Docker Desktop、kubectl、缓存节点/Alpine 镜像和隔离 PostgreSQL。这些是 Linux 容器验收，不启动已暂缓的 Windows 主机/IIS 验收。

```powershell
# ARGUS_MONITORING_TEST_DATABASE_URL 已设置为本轮隔离数据库；不打印值。
powershell.exe -NoProfile -File scripts/plan8-kubernetes-local.ps1 -Mode Prepare -RunId plan8-drain-example
powershell.exe -NoProfile -File scripts/plan8-kubernetes-local.ps1 -Mode Test -RunId plan8-drain-example
powershell.exe -NoProfile -File scripts/plan8-kubernetes-local.ps1 -Mode Cleanup -RunId plan8-drain-example
```

`e2e-plan8 --suite drain` 运行原生不变量、Connector 协议及真实双节点工作流三个具名组；缺少专用 context、kubeconfig 或隔离数据库即失败，Skip 不计为通过。当次完整 full 注册扩展到 20 组，后续控制面验证又增加检查，当前清单以[验收入口](./35-acceptance-harness.md)为准；仍需正式阶段，不表示 Plan8 整体完成。

Helm、终端/实时日志/端口转发会话、CSI 成功扩容、完整正式 Connector 与模型/部署验收继续列为剩余项；P8-A69 未据上述组件测试直接改为 passed。

依据 [Kubernetes 安全维护节点](https://kubernetes.io/docs/tasks/administer-cluster/safely-drain-node/)及 [Eviction API](https://kubernetes.io/docs/concepts/scheduling-eviction/api-eviction/)。此实现保持原有领域/执行分离，只显式扩展多步骤维护的继续阶段。
