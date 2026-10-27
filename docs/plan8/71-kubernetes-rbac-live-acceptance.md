# Kubernetes RBAC 真实授权与变更验收

日期：2026-10-21。状态：专项真实 API/PG 验收通过；已纳入操作工作流检查，整体 Plan8 尚未完成或发布。

`TestLiveRBACApprovalScopeDelegationAndNativeAPIRejection` 使用自有临时 Namespace、准确 UID 和隔离 PostgreSQL，执行生产领域服务的 Preview、确认、Worker 和结果核验。

- Role、RoleBinding、ClusterRole、ClusterRoleBinding 分别新建、修改标签和删除；预览阶段实际对象不存在，批准后检查真实 API 对象及执行状态。
- 仅 Namespace 授权不能委派 Kubernetes API 权利；单独授予 `kubernetes.rbac.escalate` 也不能扩大数据范围。普通 Cluster 编辑权限仍不能隐式获得委派能力。
- Cluster 范围与显式委派同时存在时，才执行批准的变更。测试中的绑定只指向本轮 ServiceAccount，ClusterRole 只允许读取本轮 Namespace。
- 另创建权限受限的执行身份，通过真实 TokenRequest 取得短期 Token，并移除管理员认证材料。即使平台侧已授权委派，目标 API 仍拒绝该身份授予其不持有的 Secret 权限，以及绑定 `cluster-admin`；拒绝后检查没有遗留对象。
- 清理按本轮标签和 UID 删除 Cluster 级对象与 Namespace，并等待 Namespace 消失；隔离数据库由测试驱动删除。

实际证据：`artifacts/plan8-rbac-native-live-product.log`，根测试通过 33.04 秒，包通过 35.027 秒；没有跳过。首次通过使用只增加该测试入口的 overlay，随后将完全相同的测试纳入仓库及 `k8s-operation-workflow` 具名检查，组数仍为 73。

这是生产领域服务、真实数据库和真实 Kubernetes API 的组合验证。注册身份与 API 传输适配器为明确的受控夹具，不能将其描述为已部署 Connector 或真实门户整轮验收。完整矩阵仍须汇总各项实际证据。

实际目标 API 的负向断言另加强为必须返回 `ErrForbidden`，避免将网络或其他错误误算授权拒绝。正常源码的复验见 `artifacts/plan8-rbac-native-explicit-forbidden.log`；保留首次 overlay 证据，不改写其范围。
